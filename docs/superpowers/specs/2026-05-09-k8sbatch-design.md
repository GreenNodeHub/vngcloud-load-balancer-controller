# k8sbatch — deferred mutation batcher for Kubernetes objects

**Date:** 2026-05-09
**Status:** Design approved, pending implementation plan
**Owner:** annd2@vng.com.vn

## Problem

During a single reconcile, many small functions each want to update one or two
fields on the same Kubernetes object. Today's helper
(`k8sRepository.PatchMutateStatus*`) does a fresh `GET` → mutate → `PATCH` per
call. Five small functions touching one `LoadBalancerConfig` produce five GETs
and five PATCHes, even though one GET and one PATCH would suffice. Hot
reconcile loops amplify this into significant API server load.

## Goal

Provide a generic, reusable batcher that:

1. Accepts queued mutations against any `client.Object` from many call sites
   during a reconcile.
2. At an explicit `Flush` point, performs **one** GET and **at most two**
   PATCHes (Spec, Status) per distinct object.
3. Preserves the existing semantics: optimistic-locked patch, retry-on-conflict
   that re-runs the mutator against a freshly fetched object, "no diff → no
   patch."
4. Lets a single reconcile flush mid-way and continue (queue drains, lifecycle
   continues).

## Non-goals

- **Create-or-update.** The batcher operates on objects assumed to exist; if
  GET returns NotFound, it records an error and leaves the entry queued.
  Creation continues to be handled by existing one-shot use-case methods.
- **Migrating "owner controller" Spec writes** (Ingress/Service controllers
  writing to LBC/NSG Spec via `client.Update` / `client.Patch`). The package
  *supports* `MutateSpec` on day one, but call-site migration of those paths
  is deferred to a follow-up.
- **Goroutine safety.** Reconciles are single-goroutine per object key in
  controller-runtime; the batcher is not designed for concurrent use from
  multiple goroutines and will not add locking.
- **Type-specific diff-log filtering** (the `cmpopts.IgnoreFields(...)` rules
  in today's `patchMutateStatusObject`). The new diff log will show the full
  patched sub-resource diff at debug level.

## API

Package path: `pkg/k8sbatch`.

```go
package k8sbatch

type Batcher struct { /* unexported */ }

func New(c client.Client) *Batcher

// Queue a mutation for the Spec/metadata path. The mutator is invoked at
// Flush time against a freshly GET'd copy of the object. Return false to
// indicate "nothing to change for this entry"; if any mutator for this
// object returns true, a Spec patch is issued.
func MutateSpec[T client.Object](b *Batcher, obj T, mutate func(obj T) bool)

// Queue a mutation for the Status sub-resource. Same semantics as
// MutateSpec but flushed via client.Status().Patch.
func MutateStatus[T client.Object](b *Batcher, obj T, mutate func(obj T) bool)

// Drain the queue. Returns errors.Join of any per-object failures (nil if
// all succeed). Successful entries are removed; failed entries remain
// queued for a subsequent Flush. Idempotent and safe to call multiple
// times in one reconcile.
func (b *Batcher) Flush(ctx context.Context) error

// Pending reports the number of distinct objects currently queued.
// Intended for tests and diagnostics.
func (b *Batcher) Pending() int
```

`MutateSpec` and `MutateStatus` are package-level generic functions because Go
does not permit type parameters on methods. The `Batcher` type itself is not
generic; it stores entries keyed by `(GVK, namespace, name)`.

## Lifecycle

The batcher is caller-owned and typically per-reconcile:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    b := k8sbatch.New(r.Client)
    // ... use cases call k8sbatch.MutateStatus(b, obj, fn) etc. ...
    if err := b.Flush(ctx); err != nil {
        return ctrl.Result{}, err
    }
    return ctrl.Result{}, nil
}
```

`Flush` may be called more than once per reconcile (e.g., before a step that
needs a previously-queued field to be persisted before continuing). After a
mid-reconcile flush the queue is drained of successful entries; any further
`Mutate*` calls re-fill it for the next flush.

## Object identity

Two `Mutate*` calls coalesce into the same queue entry when they share GVK +
namespace + name. The GVK is resolved from the runtime scheme attached to
`b.client.Scheme()`. Different `*T` instances with the same identifying
fields are treated as the same logical object.

The first call to enqueue a given object's mutation captures the object
pointer used as the *type witness* for fresh GETs at flush time. Subsequent
mutators on the same key may pass any pointer of the same type; only the
mutator function is retained.

## Flush algorithm

For each `objKey` in queue iteration order:

1. **GET fresh.** `b.client.Get(ctx, key, freshObj)`. On NotFound or other
   GET error: record error, leave entry queued, continue to next key.
2. **Snapshot Spec base.** `oldSpec := freshObj.DeepCopyObject()`.
3. **Apply Spec mutators in queue order.** Track `specChanged = OR of
   mutator return values`. Even if a later mutator returns `false`, an
   earlier `true` triggers a Spec patch.
4. **If `specChanged`:** issue
   `client.Patch(ctx, freshObj, client.MergeFromWithOptions(oldSpec, MergeFromWithOptimisticLock{}))`,
   wrapped in `retry.RetryOnConflict(retry.DefaultBackoff, ...)`. On
   conflict the entire per-object pipeline restarts from step 1: re-GET,
   re-snapshot, re-apply *all* mutators against the new state. Matches
   today's `patchMutateStatusObject` behavior. On non-conflict error or
   retry exhaustion: record error, leave entry queued, **skip Status
   patch** for this object, continue to next key.
5. **Snapshot Status base.** `oldStatus := freshObj.DeepCopyObject()` (after
   the Spec patch has been applied to `freshObj`).
6. **Apply Status mutators in queue order.** Track `statusChanged`.
7. **If `statusChanged`:** issue
   `b.client.Status().Patch(...)` with the same retry-on-conflict wrapper
   and the same per-object-restart semantics on conflict.
8. **Per-object outcome.** Full success → delete entry from queue. Any
   error → keep entry; append error to local slice.
9. **Aggregate.** After all keys processed, return `errors.Join(errs...)`
   (nil if empty).

### Subtleties called out

- **Why re-apply mutators on conflict** rather than re-PATCH the same body:
  the user's mutator semantics are "compare-with-desired, change if
  different." Running it against the new fresh state is the *correct*
  recomputation; force-overwriting with the stale body would be a bug.
- **Spec-fail-skips-Status:** if a Spec patch ultimately fails for an
  object, that object's Status mutators are not applied this flush. The
  whole entry stays queued and both will be retried on the next `Flush`.
- **Diff logging:** preserve the existing `cmp.Diff` debug print pattern
  (gated on `logrus.IsLevelEnabled(logrus.DebugLevel)`). One diff line per
  patched sub-resource. The type-specific `IgnoreFields` rules from the
  old helper are not carried over; diffs may show a richer set of fields.
- **`MergeFromWithOptimisticLock`** is preserved; it is what makes the
  conflict-retry mechanism work correctly.

## Error semantics

- **Best-effort across objects.** A failure on one object never prevents
  attempts on other objects in the same flush.
- **Conservative within an object.** A Spec failure for object X skips X's
  Status patch; both stay queued.
- **NotFound on GET** is treated as a recoverable error: error recorded,
  entry stays queued. Caller may decide to create the object before the
  next flush.
- **Errors are returned as `errors.Join`.** Callers can `errors.Is`
  against `apierrors` sentinels to make routing decisions (e.g., requeue
  vs. give up) without losing the per-object detail.

## Integration & migration

**Phase 1 — Land the package.** New package + full test suite (envtest).
No production call sites changed. Existing `PatchMutateStatus*` helpers
untouched.

**Phase 2 — Re-route existing helpers** (`PatchMutateStatusLoadBalancerConfig`,
`PatchMutateStatusNodeSecurityGroup`, `PatchMutateStatusGlobalLoadBalancerConfig`,
`PatchMutateStatusVngcloudGlobalLoadBalancer`) through a one-shot batcher
under the hood. Tradeoff: tiny per-call overhead (one allocation, one map
op) in exchange for a single code path. Existing test suites guard
behavioral equivalence.

**Phase 3 — Migrate one reconciler at a time** to thread a single
`*Batcher` through reconcile, in this order:

1. `internal/usecase/glbc_uc/status.go` — smallest surface.
2. `internal/usecase/nsg_uc/status.go`.
3. `internal/usecase/lbc_uc/status.go` — largest, and the file CLAUDE.md
   flags as the prior reconcile-loop offender. Migrate last, with the
   pattern fully validated.

**Owner-controller Spec writes** (Ingress/Service writing to LBC/NSG Spec)
are explicitly deferred. The API supports them via `MutateSpec`; call-site
migration is a separate follow-up.

## Testing

Primary harness: **envtest** (`bin/k8s/` binaries already provisioned by
`setup-envtest` and used elsewhere in the repo). Reuse the existing
`LoadBalancerConfig` and `NodeSecurityGroup` CRDs from `config/crd/bases/`
to test against real types and exercise the generics path with two
distinct types in one batcher.

Test cases — one test per consequential branch:

1. Single mutator, single object, status changed → 1 GET, 1 Status patch,
   queue empty after Flush.
2. Single mutator returns `false` → 1 GET, 0 patches.
3. Multiple mutators on same object, queue order respected. Mutator A sets
   X=1; mutator B asserts X==1 then sets X=2; final patched state has
   X=2.
4. Spec + Status on same object → 1 GET, Spec patch first, Status base
   re-snapshotted, Status patch second.
5. Spec patch fails (e.g., admission rejection) → Status mutators not
   applied; both stay queued; returned error wraps the Spec failure.
6. Multiple distinct objects, one fails → best-effort: failed entry stays
   queued, successful entries cleared, returned error is `errors.Join` of
   failures.
7. NotFound on GET → error recorded, entry stays queued, no patch
   attempted.
8. Mid-reconcile flush → call `Flush`; queue more mutations; call `Flush`
   again; second flush sees a fresh GET (not stale state).
9. Conflict on patch triggers re-GET + re-apply. Synthesized via a
   parallel goroutine that patches the object directly between the
   batcher's GET and PATCH. The mutator must observe both GET states;
   final patched state must reflect both the out-of-band change and the
   batcher's mutation.
10. Coalescing by identity. Two `MutateStatus` calls with distinct `*T`
    pointers but matching GVK+namespace+name → 1 GET, 1 patch, both
    mutators applied in queue order.
11. Two distinct CR types in one batcher (LBC + NSG) → both flush
    correctly; type information is preserved.

**Parallelism gotcha.** Per CLAUDE.md, the new package will spin up its own
envtest API server. The repo convention `go test -p=1 ./...` already covers
this; no `Makefile` changes are required.

**Out of scope for tests:**

- Per-CR diff-log rendering — was type-specific in the old code and is
  intentionally generalized.
- Existing `PatchMutateStatus*` behavior — covered by the existing test
  suites in `lbc_uc/`, `nsg_uc/`, `glbc_uc/`. Phase 2 routing them through
  the batcher relies on those suites as the regression net.

## Open follow-ups (post-merge)

- Migrating owner-controller Spec writes to use `MutateSpec`.
- Optional linter/static-check rule: every `Reconcile` that constructs a
  `*Batcher` must call `Flush` before returning.
- Optional: a `Flush(ctx, opts)` overload that lets callers pick between
  `MergeFrom` and `StrategicMergePatch` if a future use case demands it.
  Not needed today.

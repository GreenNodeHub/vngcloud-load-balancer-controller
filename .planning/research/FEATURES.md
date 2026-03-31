# Feature Landscape

**Domain:** Kubernetes CRD operator managing cloud load balancer resources (Global LB)
**Project:** VNGCloud GlobalLoadBalancerConfig (GLBC) reconciler
**Researched:** 2026-03-15
**Mode:** Subsequent milestone — reconciler already exists, focus is verify and fix

---

## Context: What Already Exists

The reconciler code is in `internal/usecase/glbc_uc/`. The features below are assessed against
what is coded, what is broken/stubbed, and what industry-standard operators require.

---

## Table Stakes

Features users expect from any production-grade cloud LB controller operator. Absence means
the operator is unreliable and users cannot trust it.

| Feature | Why Expected | Complexity | Current State |
|---------|--------------|------------|---------------|
| Idempotent reconcile | Every cloud operator runs reconcile many times per spec generation; double-applying must be safe | Low | Present — equality checks before patch, WaitActive gates, name/port matching |
| Finalizer-based deletion | External cloud resources cannot use K8s owner refs; finalizer ensures cleanup before CRD removal | Low | Assumed present (controller-runtime wires this); delete flow exists in `delete_lb.go` |
| Status conditions (Ready type) | Users and automation poll `.status.conditions` to know if resource is ready | Low | Present — `GLBCConditionTypeReady` written on every reconcile in `glbc_uc.go` |
| ObservedGeneration in status | GitOps tooling (ArgoCD, Flux) requires this to confirm spec was applied | Low | Present — `status.observedGeneration` updated each reconcile |
| Async-safe WaitActive polling | VNGCloud API is async; mutations must wait for ACTIVE before issuing the next call | Medium | Present — `WaitGlobalLoadBalancerActive` called after every mutating operation |
| Create-or-adopt LB | Users may pre-create an LB and reference it by ID or name | Medium | Present — spec.loadBalancerId / spec.name lookup chain in `deployLoadBalancer` |
| Declarative pool management | Pools declared in spec are created; pools removed from spec are deleted | Medium | Present but broken — `deleteRedundantPools` works; `deleteRedundantListeners` broken (`canDeleteWholeListener` returns `ErrorNotImplemented`) |
| Declarative listener management | Listeners declared in spec are created; removed from spec are deleted | Medium | Broken — delete path blocked by `canDeleteWholeListener` returning `ErrorNotImplemented` |
| Pool member lifecycle management | Members added/removed from spec are reflected in the LB | Medium | Present — `deployPoolMembers` with create/update/delete bulk actions |
| Health monitor configuration | Users must be able to set protocol, thresholds, path, codes per pool | Medium | Present — full health monitor CRUD in `deploy_pool.go` |
| Error status propagation | When reconcile fails, the error message appears in status so users can debug | Low | Present — `LastReconcileMessage` set to `err.Error()` on failure |
| Orphan prevention on create | If controller restarts after creating LB but before saving status, must not create duplicate | High | Partial — LB is looked up by name before creating, but race window exists (see PITFALLS) |
| Safe package (resize) management | Changing LB size must not drop traffic; must wait for active after resize | Medium | Present — `deployPackageId` with WaitActive after resize |
| Status reflects actual VIPs/domains | Users need the LB endpoints to configure DNS/firewall | Low | Present — `statusAddLoadBalancerId` writes VIPs and domains from API response |

---

## Differentiators

Features that exceed baseline operator expectations. These are competitive advantages or
behaviors that make this operator distinctly reliable for VNGCloud's use case.

| Feature | Value Proposition | Complexity | Current State |
|---------|-------------------|------------|---------------|
| Per-LB mutex locking | Prevents concurrent modification when multiple GLBCs share a single LB — a rare and subtle correctness requirement | Medium | Present — `sync.Map` of `*sync.Mutex` keyed by LB ID in `glbc_uc.go`; requeue on new ID needed to re-acquire correct lock |
| 3-way merge for pool members | Preserves manually-added members (added via VNGCloud portal) while managing spec-declared ones, preventing portal overrides from being silently deleted | High | Present in `mergePoolMembers` — uses created/current/spec triple; **bug: `convertMember` omits SubnetID** |
| Ownership-gated deletion | LB is only fully deleted if this GLBC owns all listeners and pools; otherwise partial cleanup — prevents accidentally destroying a shared LB | High | Present in `canDeleteWholeLoadBalancer`; member-level check is TODO-commented out |
| Migrate LB on spec.loadBalancerId change | Allows users to move config to a different LB without recreating resources | Medium | Present — `migrateLoadBalancer` transfers to new ID |
| Self-healing on LB error status | If LB enters ERROR state after creation, the controller deletes and re-creates it automatically | High | Present in `createLoadBalancer` — detects `ErrorLoadBalancerStatusError`, deletes, requeues |
| Cross-GLBC validation | Detect port conflicts or invalid shared-LB state when multiple GLBCs point at the same LB | High | Stubbed empty — `validateCrossGLBCs` returns nil unconditionally |
| Self-validation before API calls | Spec-level consistency checks (e.g. listener references a non-existent pool name) run before any cloud API call, reducing wasted operations | Low | Stubbed empty — `validateSelf` returns nil unconditionally |
| Name-populated listener status | `CreatedGlobalListener.Name` in status allows human-readable tracking of which listeners are owned | Low | Broken — `deployListener` returns `CreatedGlobalListener{Id, Port}` only; Name is never set |

---

## Anti-Features

Features to deliberately NOT build in this milestone. These would add scope without fixing
the correctness problems that are the actual goal.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Tag management on LBs | VNGCloud SDK does not support tags at LB creation time; the code already has this commented out | Leave commented; add a future-phase TODO note when SDK supports it |
| CRD schema redesign | Changing the API shape is orthogonal to fixing reconciler correctness; schema changes require migration planning | Treat current schema as frozen for this milestone |
| VGLB (VngcloudGlobalLoadBalancer) reconciler | Separate CRD with its own build logic; already scoped to a later phase | Keep in `vglb_uc/`, do not mix into `glbc_uc/` |
| Gateway API integration | AWS LBC has added Gateway API support, but VNGCloud's GLB is a proprietary API — no Gateway API mapping exists | Not applicable; the CRD-based approach is correct |
| Multi-cluster member federation | The GLBC spec supports multi-region pool members, which is already the designed model | Do not abstract further; the current model handles multi-region natively via poolMember.Region |
| Webhook validation (admission controller) | Adds deployment complexity; the reconciler's `validateSelf` and `validateCrossGLBCs` cover the same ground at reconcile time for this stage | Implement validation in the reconcile loop; webhooks are a future hardening option |
| Controller sharding / leader election tuning | Default controller-runtime leader election is sufficient; sharding is premature | Use defaults |

---

## Feature Dependencies

```
validateSelf (stub)
  └── must run before deployLoadBalancer
        └── deployLoadBalancer
              ├── getLBLock (must re-acquire after new LB ID obtained → requeue)
              └── validateCrossGLBCs (stub)
                    └── deployPools
                          └── deployPoolMembers (depends on pool IDs)
                                └── deployListeners (depends on pool IDs for defaultPool ref)
                                      ├── deleteRedundantListeners (BROKEN: canDeleteWholeListener)
                                      │     └── deleteRedundantPools
                                      └── status update (CreatedListeners, CreatedPools)

canDeleteWholeListener
  └── currently returns ErrorNotImplemented
      └── blocks deleteRedundantListeners entirely
          └── makes listener cleanup impossible
              └── makes the full reconcile/delete/re-create cycle unreliable
```

**Critical path for the current milestone:**

1. `canDeleteWholeListener` must be implemented → unblocks `deleteRedundantListeners` → enables full reconcile cycle
2. `convertMember` must include `SubnetID` → fixes pool member 3-way merge (SubnetID is part of member identity)
3. `validateSelf` needs basic rules (pool name referenced by listener must exist in spec) → prevents silent misconfiguration
4. `validateCrossGLBCs` needs port-conflict detection → prevents two GLBCs clobbering each other on a shared LB
5. `CreatedGlobalListener.Name` must be populated → required for `canDeleteWholeListener` to match by name

---

## MVP Recommendation

For this milestone (verify and fix existing reconciler), the priority order is:

**P0 — Must fix to make delete flow work:**
1. Implement `canDeleteWholeListener` — check if listener's default pool is exclusively owned by this GLBC
2. Fix `convertMember` to include `SubnetID` — member identity is incomplete without it

**P1 — Must fix for correctness of shared-LB scenarios:**
3. Implement `validateSelf` — at minimum: listener must reference a pool that exists in spec
4. Implement `validateCrossGLBCs` — at minimum: detect port conflicts with other GLBCs on same LB
5. Populate `Name` in `CreatedGlobalListener` status — listener name is the human-readable handle

**P2 — Verify existing logic is correct:**
6. Verify pool member 3-way merge handles all edge cases (add, remove, update by name key)
7. Verify pool creation during LB create passes the first pool+listener through correctly
8. Verify full reconcile → delete → re-create cycle end-to-end

**Defer:**
- Tag management (SDK limitation)
- Webhook admission controller (premature)
- CRD schema changes (frozen scope)

---

## Sources

- Codebase: `internal/usecase/glbc_uc/` (direct code review)
- [Kubernetes Operators Best Practices — Red Hat](https://cloud.redhat.com/blog/kubernetes-operators-best-practices)
- [Kubernetes Finalizers — Official Docs](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/)
- [Kubernetes Cloud Controller Manager — Official Docs](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [AWS Load Balancer Controller — Multiple Controller Ownership Issue](https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues/2788)
- [Error Back-off with Controller Runtime — stuartleeks.com](https://stuartleeks.com/posts/error-back-off-with-controller-runtime/)
- [Subreconciler Pattern — Red Hat Engineering Blog](https://www.redhat.com/en/blog/subreconciler-patterns-in-action)
- [controller-runtime reconcile package — pkg.go.dev](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/reconcile)
- [Double Trouble: Multiple Controllers, Same LB — F5 DevCentral](https://community.f5.com/kb/technicalarticles/double-trouble-multiple-controllers-handling-the-same-kubernetes-loadbalancer-se/342539)

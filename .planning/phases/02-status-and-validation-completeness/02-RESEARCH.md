# Phase 2: Status and Validation Completeness - Research

**Researched:** 2026-03-15
**Domain:** Go Kubernetes controller reconciler — status tracking and listener drift detection
**Confidence:** HIGH

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| STAT-01 | Uncomment and activate pool member status tracking on pool creation in `deploy_pool.go` | Exact lines identified; `statusAddPoolMember` does not exist in glbc_uc — must use `statusUpdatePoolMember`; `CreatedGlobalPool.CreatedPoolMembers` must be populated from `ListGlobalPoolMembers` after create+wait |
| STAT-02 | Implement headers comparison in `buildListenerUpdateRequest` (replace TODO) | Type mismatch identified — spec is `[]string`, entity is `*string` (comma-joined); comparison logic and `WithHeaders` call pattern documented |
</phase_requirements>

---

## Summary

Phase 2 addresses two stubbed gaps in `internal/usecase/glbc_uc/`: commented-out pool member status recording on pool creation (STAT-01) and a missing header-comparison block in the listener update request builder (STAT-02).

**STAT-01 analysis.** In `deploy_pool.go:deployPool`, after `CreateGlobalPool` succeeds, two `// TODO: uncomment me` blocks are commented out. The first calls `t.statusAddPoolMember(ctx, _pool.ID, poolSpec.Name, poolSpec.Members)`, but `statusAddPoolMember` does not exist in `glbc_uc/status.go`. The analogous function is `statusUpdatePoolMember(ctx, poolId, name string, poolMembers []v1alpha1.CreatedGlobalPoolMember)`. Additionally, the `CreatedGlobalPool` return value has `CreatedPoolMembers` left as nil. The correct pattern — observed in `deployPoolMembers` — is to call `ListGlobalPoolMembers` after waiting for the LB to become active, then build `[]v1alpha1.CreatedGlobalPoolMember` from the API response and call `statusUpdatePoolMember`. The second commented block (in the update/exist-pool path) calls `statusAddPool`, which does exist in `glbc_uc/status.go` but should not be needed here because the pool already exists and `deployPoolMembers` calls `statusUpdatePoolMember` at line 75.

**STAT-02 analysis.** In `deploy_listener.go:buildListenerUpdateRequest`, the comment reads `// TODO: compare headers, somewhere is []string, somewhere is *string`. The spec field `GlobalListener.Headers` is `[]string`. The entity field `entityv2.GlobalListener.Headers` is `*string` (a comma-joined string, confirmed in both entity definition and SDK response struct). The `UpdateGlobalListenerRequest.Headers` is also `*string`, and `WithHeaders(...string)` joins with commas via `strings.Join`. The comparison must decode the entity's `*string` into `[]string` (split by comma), compare sets with the spec's `[]string`, and call `updateOptions.WithHeaders(listenerSpec.Headers...)` when they differ.

**Primary recommendation:** Activate STAT-01 by replacing the commented-out `statusAddPoolMember` call with a post-create `ListGlobalPoolMembers` call followed by `statusUpdatePoolMember`, and populate `CreatedPoolMembers` in the returned `CreatedGlobalPool`. Activate STAT-02 by adding a headers comparison block that normalizes both sides to `[]string` (splitting `*string` on comma) and calls `WithHeaders` when they differ.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/vngcloud/vngcloud-go-sdk/v2` | v2.17.4-0.20251225102644-877dacf16698 | VNG Cloud API calls | Project's SDK; all repo calls go through it |
| `github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1` | local | CRD types (spec/status) | Defines `CreatedGlobalPool`, `CreatedGlobalPoolMember`, `GlobalListener` |
| `github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository` | local | Repository interface | `ListGlobalPoolMembers`, `PatchMutateStatusGlobalLoadBalancerConfig` |
| `strings` (stdlib) | - | Header string splitting | `strings.Split(s, ",")` to decode `*string` header field |

### No new dependencies
Both STAT-01 and STAT-02 are pure logic additions. No new imports are required beyond `strings` (already imported in `deploy_listener.go`).

---

## Architecture Patterns

### Pattern 1: Status update after API create + wait (STAT-01)

**What:** After creating a pool, call `ListGlobalPoolMembers` to get the authoritative member list from the API, then record it in status via `statusUpdatePoolMember`.

**When to use:** Any time a new resource is created and its children (members) must be reflected in status immediately without requiring a second reconcile.

**The existing analogous pattern is in `deployPoolMembers` (lines 32-75):**
```go
// Source: internal/usecase/glbc_uc/deploy_pool_member.go:32-75
currentPoolMembers, err = t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, poolId)
if err != nil { ... }

createdPoolMembers := make([]v1alpha1.CreatedGlobalPoolMember, 0)
for _, poolMemberSpec := range poolMembersSpec {
    if currentPoolMember := searchPoolMemberByName(poolMemberSpec.Name); currentPoolMember != nil {
        createdMembers := make([]v1alpha1.GlobalMember, 0)
        for _, member := range currentPoolMember.Members.Items {
            createdMembers = append(createdMembers, v1alpha1.GlobalMember{
                Name: member.Name, Address: member.Address, Port: member.Port,
                BackupRole: member.BackupRole, Weight: &member.Weight,
                MonitorPort: &member.MonitorPort, SubnetID: member.SubnetID,
            })
        }
        createdPoolMembers = append(createdPoolMembers, v1alpha1.CreatedGlobalPoolMember{
            Id: currentPoolMember.ID, Name: currentPoolMember.Name,
            CreatedMembers: createdMembers,
        })
    }
}
return createdPoolMembers, t.statusUpdatePoolMember(ctx, poolId, poolName, createdPoolMembers)
```

**For the new-pool path in `deployPool`, the implementation must:**
1. After `WaitGlobalLoadBalancerActive` completes, call `ListGlobalPoolMembers(ctx, lbId, _pool.ID)`.
2. Build `[]v1alpha1.CreatedGlobalPoolMember` from the response (same loop as `deployPoolMembers`).
3. Call `t.statusUpdatePoolMember(ctx, _pool.ID, poolSpec.Name, createdPoolMembers)`.
4. Return `&v1alpha1.CreatedGlobalPool{Id: _pool.ID, Name: poolSpec.Name, CreatedPoolMembers: createdPoolMembers}`.

Note: `statusAddPoolMember` (referenced in the comment) does not exist in `glbc_uc/status.go`. The correct function is `statusUpdatePoolMember`.

### Pattern 2: Header type normalization for comparison (STAT-02)

**What:** The entity stores headers as a comma-joined `*string`; the spec stores them as `[]string`. Comparison requires normalizing both to `[]string` before set-equality check.

**When to use:** Any time a spec `[]string` field must be compared to an API entity `*string` field.

**Decode entity headers:**
```go
// Source: analysis of entityv2.GlobalListener.Headers (*string) and
//         UpdateGlobalListenerRequest.WithHeaders (strings.Join on ","  )
decodeHeaders := func(h *string) []string {
    if h == nil || *h == "" {
        return []string{}
    }
    return strings.Split(*h, ",")
}
currentHeaders := decodeHeaders(currentListener.Headers)
```

**Compare and conditionally update:**
```go
// Source: pattern from other field comparisons in buildListenerUpdateRequest
if !stringSlicesEqualUnordered(currentHeaders, listenerSpec.Headers) {
    message = append(message, fmt.Sprintf("headers (%v -> %v)", currentHeaders, listenerSpec.Headers))
    updateOptions.WithHeaders(listenerSpec.Headers...)
    isNeedUpdate = true
}
```

Note: `stringSlicesEqualUnordered` is already defined in `status.go` and is package-private, so it is available directly in `deploy_listener.go` (same package `glbc_uc`).

### Pattern 3: Existing status update pattern (reference)

The `statusAddPool` function in `glbc_uc/status.go` operates via `PatchMutateStatusGlobalLoadBalancerConfig` with an idempotency check. All status mutations follow this pattern — check if already equal on fresh copy, mutate only if different, return bool indicating change.

### Existing TODOs summary

| File | Line | TODO text | Action |
|------|------|-----------|--------|
| `deploy_pool.go` | 53-56 | `statusAddPoolMember(ctx, _pool.ID, poolSpec.Name, poolSpec.Members)` | Replace: use `ListGlobalPoolMembers` + `statusUpdatePoolMember` (function name in comment is wrong) |
| `deploy_pool.go` | 61-66 | `CreatedPoolMembers: poolSpec.Members` in return | Replace: use members from `ListGlobalPoolMembers` API response |
| `deploy_pool.go` | 78-81 | `statusAddPool(ctx, currentPool.ID, currentPool.Name)` | Remove (pool already exists; `deployPoolMembers` handles its own status update) |
| `deploy_listener.go` | 214 | `// TODO: compare headers, somewhere is []string, somewhere is *string` | Replace with headers comparison block |

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| String set equality (unordered) | Custom loop | `stringSlicesEqualUnordered` (already in `status.go`) | Already exists in the package, order-independent |
| Header encoding for update | Custom join | `updateOptions.WithHeaders(listenerSpec.Headers...)` | SDK method does `strings.Join` internally |
| Pool member status recording | Custom status patch | `t.statusUpdatePoolMember(ctx, poolId, name, members)` | Already fully implemented in `status.go` with idempotency |
| Pool member list after create | Manual loop over spec | `t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, poolId)` | Must use API response — only the API response has assigned member IDs |

**Key insight:** All required building blocks already exist. This phase is wiring, not building.

---

## Common Pitfalls

### Pitfall 1: Using spec members instead of API response for STAT-01
**What goes wrong:** Returning `CreatedPoolMembers: poolSpec.Members` (spec's `GlobalPoolMember` slice) instead of the API-assigned members.
**Why it happens:** The spec has the desired members, which seems correct to use. But `CreatedGlobalPoolMember.Id` must be the API-assigned pool member ID from the create response, not a spec field.
**How to avoid:** Always call `ListGlobalPoolMembers` after the create + wait cycle and build `CreatedGlobalPoolMember` from the API entity (which has `ID` field).
**Warning signs:** `createdPoolMembers[*].id` is empty in status after pool creation.

### Pitfall 2: Wrong function name in the TODO comment
**What goes wrong:** Implementing `statusAddPoolMember` as a new function when the TODO comment names it.
**Why it happens:** The `lbc_uc` package has `statusAddPoolMember`; someone copied the comment without checking `glbc_uc/status.go`.
**How to avoid:** The correct function in `glbc_uc/status.go` is `statusUpdatePoolMember(ctx, poolId, name, []CreatedGlobalPoolMember)`. Do not create a new function.
**Warning signs:** Compile error — `statusAddPoolMember undefined`.

### Pitfall 3: Nil-pointer dereference on `currentListener.Headers`
**What goes wrong:** Directly dereferencing `*currentListener.Headers` without a nil check causes a panic when no headers were set on the listener.
**Why it happens:** `entityv2.GlobalListener.Headers` is `*string` and is nil when no headers are configured.
**How to avoid:** Always nil-check before dereferencing. Use a `decodeHeaders` helper that handles `nil` and empty string.
**Warning signs:** Panic in `buildListenerUpdateRequest` on any listener that has no headers.

### Pitfall 4: Redundant `statusAddPool` call on existing-pool update path
**What goes wrong:** Uncommenting `statusAddPool` in the update path (lines 78-81 of `deploy_pool.go`) causes a spurious status patch on every reconcile of an existing pool, even when nothing changed.
**Why it happens:** The comment says "uncomment me" but the update path's `deployPoolMembers` at line 89 already calls `statusUpdatePoolMember`, which records the pool + members together. The `statusAddPool` call in the update path is not needed for correctness and adds noise.
**How to avoid:** Leave the `statusAddPool` block commented out in the update/exist-pool path. Only the new-pool path needs status activation.

### Pitfall 5: Header comparison with empty vs nil mismatch
**What goes wrong:** Treating spec `[]string{}` (empty slice, no headers) as different from entity `*string(nil)` (no headers), causing spurious update calls.
**Why it happens:** Empty spec headers = no headers configured. Nil entity headers = no headers set. These are semantically identical.
**How to avoid:** Normalize: `nil *string` → `[]string{}`, empty `[]string` → `[]string{}`, then compare. Use `stringSlicesEqualUnordered` which handles length equality first.

---

## Code Examples

### STAT-01: Post-create pool member status tracking

```go
// Source: based on deploy_pool_member.go:40-75 pattern
// In deployPool, replace the two TODO blocks for the new-pool path:

_pool, err := t.vngcloudRepo.CreateGlobalPool(ctx, lbId,
    t.buildCreatePoolRequest(ctx, lbId, poolSpec),
)
if err != nil {
    return nil, err
}

if _, err := t.vngcloudRepo.WaitGlobalLoadBalancerActive(ctx, lbId); err != nil {
    return nil, err
}

// Fetch members from API to get their assigned IDs
currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, _pool.ID)
if err != nil {
    return nil, err
}

searchPoolMemberByName := func(name string) *entityv2.GlobalPoolMember {
    for _, p := range currentPoolMembers.Items {
        if p.Name == name {
            return p
        }
    }
    return nil
}

createdPoolMembers := make([]v1alpha1.CreatedGlobalPoolMember, 0)
for _, poolMemberSpec := range poolSpec.PoolMembers {
    if currentPoolMember := searchPoolMemberByName(poolMemberSpec.Name); currentPoolMember != nil {
        createdMembers := make([]v1alpha1.GlobalMember, 0)
        for _, member := range currentPoolMember.Members.Items {
            createdMembers = append(createdMembers, v1alpha1.GlobalMember{
                Name: member.Name, Address: member.Address, Port: member.Port,
                BackupRole: member.BackupRole, Weight: &member.Weight,
                MonitorPort: &member.MonitorPort, SubnetID: member.SubnetID,
            })
        }
        createdPoolMembers = append(createdPoolMembers, v1alpha1.CreatedGlobalPoolMember{
            Id: currentPoolMember.ID, Name: currentPoolMember.Name,
            CreatedMembers: createdMembers,
        })
    }
}

if err := t.statusUpdatePoolMember(ctx, _pool.ID, poolSpec.Name, createdPoolMembers); err != nil {
    return nil, err
}

return &v1alpha1.CreatedGlobalPool{
    Id:                 _pool.ID,
    Name:               poolSpec.Name,
    CreatedPoolMembers: createdPoolMembers,
}, nil
```

### STAT-02: Headers comparison in buildListenerUpdateRequest

```go
// Source: analysis of entityv2.GlobalListener (Headers *string) and
//         UpdateGlobalListenerRequest.WithHeaders (joins with ",")
// Add after the defaultPoolId comparison block, before the isNeedUpdate check:

decodeHeaders := func(h *string) []string {
    if h == nil || *h == "" {
        return []string{}
    }
    return strings.Split(*h, ",")
}

currentHeaders := decodeHeaders(currentListener.Headers)
specHeaders := listenerSpec.Headers
if specHeaders == nil {
    specHeaders = []string{}
}
if !stringSlicesEqualUnordered(currentHeaders, specHeaders) {
    message = append(message, fmt.Sprintf("headers (%v -> %v)", currentHeaders, specHeaders))
    updateOptions.WithHeaders(specHeaders...)
    isNeedUpdate = true
}
```

---

## State of the Art

| Old Approach | Current Approach | Notes |
|--------------|------------------|-------|
| Pool member status empty on first create | Populate from `ListGlobalPoolMembers` after create+wait | STAT-01 fix |
| Listener headers never trigger update | Compare decoded current vs spec headers | STAT-02 fix |
| `statusAddPoolMember` comment (wrong name) | `statusUpdatePoolMember` (correct function in `glbc_uc/status.go`) | Function naming diverged from `lbc_uc` |

---

## Open Questions

1. **Does `ListGlobalPoolMembers` return members immediately after `WaitGlobalLoadBalancerActive`?**
   - What we know: The existing `deployPoolMembers` function calls `ListGlobalPoolMembers` after `WaitGlobalLoadBalancerActive` and works correctly.
   - What's unclear: Whether the create path's pool has member IDs available as soon as the LB is ACTIVE.
   - Recommendation: Assume yes, consistent with the existing pattern. If the returned member list is empty, `createdPoolMembers` will be an empty slice — not a regression, since today it is always nil.

2. **Should `currentListener.Headers` == `""` be treated as no-headers or as a literal empty header?**
   - What we know: `NewUpdateGlobalListenerRequest` sets `Headers: nil` by default; `WithHeaders` only sets a non-empty join. The API returns `*string(nil)` when no headers are configured.
   - Recommendation: Treat both `nil` and `""` as no-headers (empty `[]string{}`). This matches the encode/decode symmetry of `WithHeaders`.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing + testify/assert + testify/mock |
| Config file | none — `go test ./...` |
| Quick run command | `go test ./internal/usecase/glbc_uc/... -run TestSTAT -v` |
| Full suite command | `go test ./internal/usecase/glbc_uc/...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| STAT-01 | `CreatedGlobalPool.CreatedPoolMembers` populated from API after pool create | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeployPool_PopulatesCreatedPoolMembers -v` | Wave 0 |
| STAT-01 | `status.createdPools[*].createdMembers` written to status on first pool creation | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeployPool_StatusUpdatedOnCreate -v` | Wave 0 |
| STAT-02 | Changing `spec.Headers` triggers `UpdateGlobalListener` call | unit | `go test ./internal/usecase/glbc_uc/... -run TestBuildListenerUpdateRequest_Headers -v` | Wave 0 |
| STAT-02 | Unchanged headers do not trigger a listener update | unit | `go test ./internal/usecase/glbc_uc/... -run TestBuildListenerUpdateRequest_HeadersNoChange -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/usecase/glbc_uc/...`
- **Per wave merge:** `go test ./internal/usecase/glbc_uc/...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/usecase/glbc_uc/deploy_pool_test.go` — covers STAT-01 (pool creation populates members in status + return value)
- [ ] `internal/usecase/glbc_uc/deploy_listener_headers_test.go` — covers STAT-02 (headers change triggers update; headers unchanged does not)

*(Existing test infrastructure: `glbc_uc` package already has working `*_test.go` files using `repository.MockVngCloudRepository` and `testify/assert`. Pattern is established.)*

---

## Sources

### Primary (HIGH confidence)
- Direct source read: `internal/usecase/glbc_uc/deploy_pool.go` — exact TODO comments at lines 53-66, 78-81
- Direct source read: `internal/usecase/glbc_uc/deploy_listener.go` — TODO comment at line 214; `buildListenerUpdateRequest` full body
- Direct source read: `internal/usecase/glbc_uc/status.go` — all available status functions; `statusUpdatePoolMember` signature
- Direct source read: `internal/usecase/glbc_uc/deploy_pool_member.go` — `deployPoolMembers` as reference pattern for post-create member collection
- Direct source read: `api/v1alpha1/globalloadbalancerconfig_types.go` — `CreatedGlobalPool`, `CreatedGlobalPoolMember`, `GlobalListener.Headers []string`
- Direct source read: SDK entity `vngcloud-go-sdk/v2@v2.17.4.../entity/loadbalancer_global.go` — `GlobalListener.Headers *string`
- Direct source read: SDK request `glb/v1/glb_listener_request.go` — `UpdateGlobalListenerRequest.Headers *string`, `WithHeaders` joins with `","`, `CreateGlobalListenerRequest.Headers []string`
- Direct source read: SDK response `glb/v1/glb_pool_response.go` — `CreateGlobalPoolResponse.ToEntityPool()` does NOT include `GlobalPoolMembers` in entity conversion
- Direct source read: SDK `glb/v1/glb.go` — `CreateGlobalPool` returns `resp.ToEntityPool()` (no member data in entity)
- Direct source read: `internal/repository/contracts.go` — `CreateGlobalPool` returns `*entityv2.GlobalPool`

### Secondary (MEDIUM confidence)
- Cross-reference: `internal/usecase/lbc_uc/deploy_pool.go` — `lbc_uc` analogue showing `statusAddPoolMember` pattern (different type, confirms the intent)
- Cross-reference: `internal/usecase/lbc_uc/status.go` — `statusAddPoolMember` exists in `lbc_uc` but NOT in `glbc_uc`, confirming the comment's function name is wrong

---

## Metadata

**Confidence breakdown:**
- STAT-01 implementation: HIGH — all involved functions and types read directly from source; analogue pattern in `deployPoolMembers` is identical
- STAT-02 implementation: HIGH — type mismatch confirmed from SDK source; `stringSlicesEqualUnordered` and `WithHeaders` both verified from source
- SDK behavior (CreateGlobalPool response): HIGH — `CreateGlobalPoolResponse.ToEntityPool()` explicitly drops `GlobalPoolMembers` from entity, confirming `ListGlobalPoolMembers` is required
- Test patterns: HIGH — existing test files in same package establish the exact mock/assert pattern to follow

**Research date:** 2026-03-15
**Valid until:** 2026-04-15 (stable codebase, no active SDK churn expected in 30 days)

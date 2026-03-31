# Phase 1: P0 Bug Fixes - Research

**Researched:** 2026-03-15
**Domain:** Go Kubernetes controller / VNG Cloud GLB reconciler bug fixes
**Confidence:** HIGH (all findings from direct source-code inspection)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Delete Ownership Model (BUG-01: canDeleteWholeListener)**

The delete flow operates bottom-up across four levels: Listener → Pool → PoolMember → Member.

Cross-cluster constraint: GLBCs can run in different clusters sharing the same LB. Cannot query other GLBC specs — ownership decisions must rely ONLY on:
1. This GLBC's spec
2. This GLBC's status (created resources)
3. Current API state from VNG Cloud

Complete delete flow when a GLBC is deleted:

1. `canDeleteWholeLoadBalancer`: Check if ALL current listeners AND ALL current pools (with full member-level verification) are in this GLBC's status.
   - If YES → `DeleteGlobalLoadBalancer()` (delete entire GLB)
   - If NO → partial cleanup (step 2)

2. For each listener this GLBC created (in status.createdListeners):
   - `canDeleteWholeListener`: Bottom-up check — does this GLBC own ALL members in the listener's referenced pool?
     - If YES → delete listener + delete its pool (cascade delete)
     - If NO → listener and pool are kept. Go to member-level cleanup (step 3)

3. For each PoolMember group in the pool:
   - If ALL members in the group were created by this GLBC → delete the entire PoolMember group
   - If only SOME members are owned → patch: remove only this GLBC's members, keep others

4. After cleanup: if LB has no resources left → delete the empty GLB

Key rules:
- Pool deletion is tied to listener deletion — never independent.
- Empty PoolMember groups (zero members after cleanup) → delete the group
- Pool stays even if empty of members, as long as its listener stays

**Resource Matching Keys**

| Level | Primary Key | Fallback Key |
|-------|------------|--------------|
| Listener | ID match against status.createdListeners | Name |
| Pool | ID match against status.createdPools | Name |
| PoolMember | ID match against status.createdPoolMembers | Name |
| Member | Address + Port (no ID, no name fallback) | — |

**Delete API Call (BUG-03):** Call `DeleteGlobalLoadBalancer()` not `DeleteLoadBalancer()` when LB is empty.

**Listener Name in Status (BUG-04):** Populate `Name` field in `CreatedGlobalListener` from API response.

**SubnetID in convertMember (BUG-02):** Add `SubnetID` field to `convertMember` output.

### Claude's Discretion
- Member-level verification implementation details in `canDeleteWholeLoadBalancer`
- Exact patch request construction for partial member removal
- Error handling and logging within the new ownership checks
- Order of operations within the partial cleanup path

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| BUG-01 | Implement `canDeleteWholeListener` to check listener ownership before deletion | Full implementation design documented below; lbc_uc analog available as reference |
| BUG-02 | Fix `convertMember` to include `SubnetID` in the converted GlobalMember struct | Bug confirmed: `SubnetID` present on `GlobalPoolMemberDetail` entity, missing from `convertMember` return |
| BUG-03 | Fix shared-LB empty cleanup to call `DeleteGlobalLoadBalancer` instead of `DeleteLoadBalancer` | Bug confirmed at `delete_lb.go:66`; correct method exists in repository interface |
| BUG-04 | Populate `Name` field in `CreatedGlobalListener` returned by `deployListener` | Bug confirmed at `deploy_listener.go:97-100`; `Name` field present on `entityv2.GlobalListener` |
</phase_requirements>

---

## Summary

This phase fixes four isolated, code-level bugs in the `glbc_uc` package that block the reconciler's create/update/delete flows from completing correctly. Three bugs (BUG-02, BUG-03, BUG-04) are small targeted changes of 1–5 lines each. BUG-01 is the only substantive implementation — `canDeleteWholeListener` currently returns `domain.ErrorNotImplemented`, breaking all redundant listener cleanup.

All bugs are confined to `internal/usecase/glbc_uc/`. No API type changes, no CRD schema changes, and no cross-package refactoring is required. The `lbc_uc` package contains a complete, tested analog (`canDeleteWholeListener` for the non-global LB) that serves as the primary implementation reference. The test framework is already established (testify + mockery pattern) and an existing mock (`repository.MockVngCloudRepository`) covers all required API calls.

**Primary recommendation:** Fix BUG-02, BUG-03, BUG-04 first (trivial) then implement BUG-01 modelled on the lbc_uc analog, adapting for the GLB member hierarchy (PoolMember group → individual Members).

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/vngcloud/vngcloud-go-sdk/v2` | v2.17.4-0.20251225102644-877dacf16698 | VNG Cloud API entities and request types | Project's only cloud API client |
| `github.com/stretchr/testify` | (project-pinned) | Assertions + mock expectations | Already used in lbc_uc tests |
| `github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository` | local | Repository interface + MockVngCloudRepository | Mock already generated, used in lbc_uc tests |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/sirupsen/logrus` | (project-pinned) | Structured logging | All glbc_uc functions use `t.logger` |
| `github.com/pkg/errors` | (project-pinned) | Error wrapping | Existing error pattern in codebase |

---

## Architecture Patterns

### Existing File Layout (glbc_uc package)
```
internal/usecase/glbc_uc/
├── glbc_uc.go           # UseCase wiring, mutex lock, ensure/delete orchestrators
├── deploy_lb.go         # create/update LB
├── deploy_listener.go   # deployListener, buildCreateListenerRequest, buildListenerUpdateRequest
├── deploy_pool.go       # deployPool, buildCreatePoolRequest, buildPoolUpdateRequest
├── deploy_pool_member.go # deployPoolMembers, buildPatchGlobalPoolMemberRequest, convertMember
├── delete_lb.go         # deleteLoadBalancer, canDeleteWholeLoadBalancer, isLoadBalancerEmpty, canCover
├── delete_listener.go   # deleteRedundantListeners, canDeleteWholeListener (TODO)
├── delete_pool.go       # deleteRedundantPools, canDeleteWholePool
├── status.go            # statusAddListener, statusAddPool, statusUpdatePoolMember, helpers
└── validate.go          # validation stubs
```

### Pattern 1: Bottom-Up Ownership Check
**What:** Before deleting a resource, walk down the hierarchy verifying every child resource is owned by this GLBC (appears in status).

**When to use:** All delete decision points. The `canDeleteWholePool` function in `delete_pool.go` is the established example.

**Example (existing canDeleteWholePool, adapted for reference):**
```go
// Source: internal/usecase/glbc_uc/delete_pool.go:108-159
func (t *defaultModelDeployTask) canDeleteWholePool(...) (bool, global.IPatchGlobalPoolMembersRequest, error) {
    currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, poolId)
    // ...
    // Extract names this GLBC created
    createdPoolMemberNames := make(map[string]bool)
    for _, pm := range createdPool.CreatedPoolMembers {
        createdPoolMemberNames[pm.Name] = true
    }
    // Determine what to keep
    for _, pm := range currentPoolMembers.Items {
        if newCreatedPoolMemberNames[pm.Name] || !createdPoolMemberNames[pm.Name] {
            poolMembersToKeep = append(poolMembersToKeep, pm.Name)
        }
    }
    if len(poolMembersToKeep) == 0 {
        return true, nil, nil  // can delete whole pool
    }
    // ... build patch request for partial deletion
}
```

### Pattern 2: canCover Generic Helper
**What:** `canCover[T, U any]` in `delete_lb.go:179` checks that every element in `smallOne` is covered by `bigOne` via a predicate. Used by `canDeleteWholeLoadBalancer`.

**When to use:** Membership checks between status slice and live API results.

### Pattern 3: Status Update via PatchMutateStatus
**What:** All status writes go through `t.k8sRepo.PatchMutateStatusGlobalLoadBalancerConfig(ctx, obj, func(...) bool)`. The inner function receives a fresh copy; return `true` if changed.

**When to use:** Any status field population (BUG-04 uses this indirectly via `statusAddListener`).

### Pattern 4: Member Matching by Address+Port
**What:** Individual members inside a PoolMember group are identified by `{Address, Port}` tuple — not by Name or ID.

**Critical:** `checkIfPoolMemberExist` in `deploy_pool_member.go:280` already uses Address+Port+Weight+BackupRole+SubnetID+MonitorPort equality. The `canDeleteWholePool` function currently uses Name-based matching for PoolMember groups, which is correct (PoolMember group level). Member-level (individual IPs) must use Address+Port.

### Anti-Patterns to Avoid
- **Returning ErrorNotImplemented from a function called in a hot path:** `canDeleteWholeListener` is called during every delete reconcile; returning the error halts all listener cleanup.
- **Using `DeleteLoadBalancer` for global LBs:** The repository interface has two separate methods. `DeleteLoadBalancer` targets the non-global VLB API. `DeleteGlobalLoadBalancer` is required for GLB resources.
- **Using `DeletePool` for global pool deletion:** `delete_pool.go:80` calls `t.vngcloudRepo.DeletePool(ctx, lbId, candidateId)` — this is the non-global pool delete. Should be `DeleteGlobalPool`. (Discovered during research — not in CONTEXT but is a real bug in the same file. Flag for planner awareness.)

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Membership checking between slices | custom nested loop | `canCover[T, U any]` already in delete_lb.go | Generic helper already tested and in use |
| Repository mocking | hand-rolled fake | `repository.MockVngCloudRepository` (mocks.go) | Already covers all VngCloudRepository methods including all Global* methods |
| Status patch idempotency | manual equality check | Pattern in statusAddListener/statusAddPool: check fresh copy first, return false if unchanged | Already handles the read-modify-write race |
| Pool member equality | field-by-field compare | `checkIfPoolMemberExist` in deploy_pool_member.go | Handles all 6 fields including SubnetID (once BUG-02 is fixed) |

**Key insight:** The lbc_uc package implements a functionally identical pattern (canDeleteWholeListener for non-global LB) with full unit test coverage. This is the canonical reference — adapt, don't reinvent.

---

## Common Pitfalls

### Pitfall 1: PoolMember vs Member Hierarchy Confusion
**What goes wrong:** Conflating `GlobalPoolMember` (the group/container, has ID+Name+Region+Members[]) with `GlobalPoolMemberDetail` (an individual IP entry within the group).
**Why it happens:** The SDK entity `GlobalPoolMember.Members` is `*ListGlobalMembers` containing `[]*GlobalPoolMemberDetail`. The naming is similar but the levels are distinct.
**How to avoid:** When checking member ownership in `canDeleteWholeListener`, navigate: listener → pool (via `listener.GlobalPoolID`) → `ListGlobalPoolMembers` (groups) → each group's `Members.Items` (individual IPs).
**Warning signs:** Matching individual member addresses against PoolMember group Names — this will always fail.

### Pitfall 2: convertMember Missing SubnetID Causes Infinite Patches
**What goes wrong:** `convertMember` at `deploy_pool_member.go:305-314` converts `entityv2.GlobalPoolMemberDetail` to `v1alpha1.GlobalMember` but omits `SubnetID`. The `checkIfPoolMemberExist` comparison includes `SubnetID` as an equality field. Result: every reconcile sees the current member (with SubnetID from API) as different from the status member (empty SubnetID), triggering a spurious bulk-patch.
**How to avoid:** Add `SubnetID: member.SubnetID` to the `convertMember` return struct.
**Warning signs:** Patch calls logged on every reconcile even when spec is unchanged.

### Pitfall 3: Wrong Delete API Called
**What goes wrong:** `delete_lb.go:66` calls `t.vngcloudRepo.DeleteLoadBalancer(ctx, lbId)` in the "LB is now empty" branch. `DeleteLoadBalancer` targets the non-global VLB API endpoint. For GLB resources this either errors or deletes the wrong resource.
**How to avoid:** Use `DeleteGlobalLoadBalancer` — it exists on the interface at `contracts.go:100`.
**Warning signs:** Delete reconcile completes but the GLB still exists in VNG Cloud; or an API error about resource type mismatch.

### Pitfall 4: Listener Name Not Populated Breaks Ownership Matching
**What goes wrong:** `deploy_listener.go:97-100` returns `CreatedGlobalListener{Id: ..., Port: ...}` with `Name` left as empty string. The `CreatedGlobalListener.Equal` method checks `Name` equality — any comparison using the status entry will fail name-based lookups.
**How to avoid:** Populate `Name: currentListener.Name` in the returned struct.
**Warning signs:** `status.createdListeners[*].name` is empty string in the GLBC status subresource.

### Pitfall 5: deleteRedundantListeners Passes Empty newCreatedPools on Full Delete
**What goes wrong:** In `delete_lb.go:49-54`, the full-GLBC-delete path calls `deleteRedundantListeners(ctx, lbId, []v1alpha1.CreatedGlobalListener{}, []v1alpha1.CreatedGlobalPool{})`. The empty `newCreatedPools` slice means no pools are "in use by new spec" — so `canDeleteWholeListener` receives an empty list. This is intentional (deleting everything), but `canDeleteWholeListener` must not misinterpret this as "pool not found → cannot delete".
**How to avoid:** When `newCreatedPools` is empty and the pool is found in `status.CreatedPools`, proceed to member-level check — don't short-circuit to "cannot delete".

---

## Code Examples

### BUG-02: Fix convertMember — Add SubnetID

```go
// Source: internal/usecase/glbc_uc/deploy_pool_member.go:305-314
// CURRENT (broken):
func convertMember(member *entityv2.GlobalPoolMemberDetail) *v1alpha1.GlobalMember {
    return &v1alpha1.GlobalMember{
        Name:        member.Name,
        Address:     member.Address,
        Port:        member.Port,
        BackupRole:  member.BackupRole,
        Weight:      &member.Weight,
        MonitorPort: &member.MonitorPort,
        // SubnetID missing — causes spurious patches
    }
}

// FIXED:
func convertMember(member *entityv2.GlobalPoolMemberDetail) *v1alpha1.GlobalMember {
    return &v1alpha1.GlobalMember{
        Name:        member.Name,
        Address:     member.Address,
        Port:        member.Port,
        BackupRole:  member.BackupRole,
        Weight:      &member.Weight,
        MonitorPort: &member.MonitorPort,
        SubnetID:    member.SubnetID,
    }
}
```

### BUG-03: Fix Empty LB Delete API Call

```go
// Source: internal/usecase/glbc_uc/delete_lb.go:64-70
// CURRENT (broken):
err = t.vngcloudRepo.DeleteLoadBalancer(ctx, lbId)

// FIXED:
err = t.vngcloudRepo.DeleteGlobalLoadBalancer(ctx, lbId)
```

### BUG-04: Populate Listener Name in Status

```go
// Source: internal/usecase/glbc_uc/deploy_listener.go:97-101
// CURRENT (broken):
return &v1alpha1.CreatedGlobalListener{
    Id:   currentListener.ID,
    Port: currentListener.Port,
}, nil

// FIXED:
return &v1alpha1.CreatedGlobalListener{
    Id:   currentListener.ID,
    Port: currentListener.Port,
    Name: currentListener.Name,
}, nil
```

### BUG-01: canDeleteWholeListener — Reference Design

The `lbc_uc` analog (`internal/usecase/lbc_uc/delete_listener.go:88-162`) implements the same pattern for the non-global LB. The GLB version differs in:
- No "policy" concept — use pool member ownership instead
- Pool member hierarchy has two levels: PoolMember group → individual Member detail
- Pool lookup uses `listener.GlobalPoolID` (not `listener.DefaultPoolId`)

```go
// Source: internal/usecase/glbc_uc/delete_listener.go:75-78 (to replace)
func (t *defaultModelDeployTask) canDeleteWholeListener(
    ctx context.Context, lbId string,
    listener *entityv2.GlobalListener,
    newCreatedPools []v1alpha1.CreatedGlobalPool,
) (bool, error) {
    // If listener has no pool, can delete
    if listener.GlobalPoolID == "" {
        return true, nil
    }

    // Check if pool is in new spec (still in use) — cannot delete listener
    for _, p := range newCreatedPools {
        if p.Id == listener.GlobalPoolID {
            t.logger.Debugf("Cannot delete listener %s, pool %s still in new spec", listener.ID, listener.GlobalPoolID)
            return false, nil
        }
    }

    // Check if pool was created by this GLBC
    var createdPool *v1alpha1.CreatedGlobalPool
    for i := range t.lbConfig.Status.CreatedPools {
        if t.lbConfig.Status.CreatedPools[i].Id == listener.GlobalPoolID {
            createdPool = &t.lbConfig.Status.CreatedPools[i]
            break
        }
    }
    if createdPool == nil {
        t.logger.Debugf("Cannot delete listener %s, pool %s not created by this GLBC", listener.ID, listener.GlobalPoolID)
        return false, nil
    }

    // Member-level check: all pool member groups must be owned by this GLBC
    // and all individual members within those groups must be owned
    currentPoolMembers, err := t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, listener.GlobalPoolID)
    if err != nil {
        return false, err
    }

    // Build a map of PoolMember group IDs this GLBC created
    createdGroupNames := make(map[string]bool)
    for _, pm := range createdPool.CreatedPoolMembers {
        createdGroupNames[pm.Name] = true
    }

    for _, currentGroup := range currentPoolMembers.Items {
        if !createdGroupNames[currentGroup.Name] {
            // Group not created by us — cannot delete whole listener
            t.logger.Debugf("Cannot delete listener %s, pool member group %s not created by this GLBC", listener.ID, currentGroup.Name)
            return false, nil
        }
        // Check individual members within group
        if currentGroup.Members != nil {
            for _, member := range currentGroup.Members.Items {
                found := false
                for _, pm := range createdPool.CreatedPoolMembers {
                    if pm.Name != currentGroup.Name {
                        continue
                    }
                    for _, createdMember := range pm.CreatedMembers {
                        if createdMember.Address == member.Address && createdMember.Port == member.Port {
                            found = true
                            break
                        }
                    }
                    if found {
                        break
                    }
                }
                if !found {
                    t.logger.Debugf("Cannot delete listener %s, member %s:%d not created by this GLBC", listener.ID, member.Address, member.Port)
                    return false, nil
                }
            }
        }
    }

    t.logger.Debugf("Can delete whole listener %s", listener.ID)
    return true, nil
}
```

### canDeleteWholeLoadBalancer — Uncomment Member Verification

```go
// Source: internal/usecase/glbc_uc/delete_lb.go:129-145 (commented-out block)
// The TODO block needs uncommenting and adapting:
// - Replace checkIfPoolMemberExist with member address+port matching
// - convertMember must include SubnetID (fixed by BUG-02) for comparison to work
```

### Additional Bug Discovered: DeletePool vs DeleteGlobalPool

```go
// Source: internal/usecase/glbc_uc/delete_pool.go:80
// CURRENT (wrong method):
err := t.vngcloudRepo.DeletePool(ctx, lbId, candidateId)

// FIXED (correct global method):
err := t.vngcloudRepo.DeleteGlobalPool(ctx, lbId, candidateId)
```

This is a companion bug to BUG-03 — not listed in requirements but is in the same code path and uses the same wrong pattern.

---

## State of the Art

| Old Approach | Current Approach | Status |
|--------------|------------------|--------|
| `canDeleteWholeListener` returns `ErrorNotImplemented` | Must implement bottom-up member ownership check | BUG-01 |
| `convertMember` omits SubnetID | Must include all 6 equality fields | BUG-02 |
| `DeleteLoadBalancer` for GLB empty cleanup | Must use `DeleteGlobalLoadBalancer` | BUG-03 |
| `CreatedGlobalListener.Name` left empty | Must populate from API response | BUG-04 |
| `DeletePool` called for global pool in `deleteRedundantPools` | Must use `DeleteGlobalPool` | Extra fix in same path |

---

## Open Questions

1. **`canDeleteWholeLoadBalancer` — uncomment member check?**
   - What we know: The TODO block at `delete_lb.go:129-145` is already written but commented out. It calls `convertMember` which currently lacks SubnetID (BUG-02 fixes this).
   - What's unclear: CONTEXT.md says to implement member-level verification — does this mean uncomment and adapt, or rewrite?
   - Recommendation: Uncomment and adapt. The logic is sound once BUG-02 makes `convertMember` include SubnetID. The planner should decide whether this is a sub-task of BUG-01 or a standalone change. It is at minimum needed for the delete flow to correctly identify whole-LB ownership.

2. **`deleteRedundantPools` passes listener deletion responsibility to `deleteRedundantListeners`**
   - What we know: Per CONTEXT.md "Pool deletion is tied to listener deletion — never independent." Currently `deleteRedundantPools` CAN delete pools (line 80) even when called in the full-delete path (empty `newCreatedPools`). This seems intentional since listeners are deleted first.
   - What's unclear: Should the planner note the `DeletePool` → `DeleteGlobalPool` fix as part of BUG-01's plan wave or as a standalone fix?
   - Recommendation: Treat as part of the same wave as BUG-03 (same class of wrong-API-method bug). Very small change.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing + testify (github.com/stretchr/testify) + testify/mock |
| Config file | None (standard `go test`) |
| Quick run command | `go test ./internal/usecase/glbc_uc/... -v -run TestCan` |
| Full suite command | `go test ./internal/usecase/...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| BUG-01 | `canDeleteWholeListener` returns true when all pool members owned | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | Wave 0 |
| BUG-01 | `canDeleteWholeListener` returns false when pool not in status | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | Wave 0 |
| BUG-01 | `canDeleteWholeListener` returns false when member not owned | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | Wave 0 |
| BUG-01 | `canDeleteWholeListener` returns true when no pool | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | Wave 0 |
| BUG-02 | `convertMember` includes SubnetID | unit | `go test ./internal/usecase/glbc_uc/... -run TestConvertMember` | Wave 0 |
| BUG-02 | No spurious patch when SubnetID unchanged | unit | `go test ./internal/usecase/glbc_uc/... -run TestBuildPatchGlobalPoolMemberRequest` | Wave 0 |
| BUG-03 | DeleteGlobalLoadBalancer called when LB empty | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeleteLoadBalancer` | Wave 0 |
| BUG-04 | CreatedGlobalListener.Name populated after deployListener | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeployListener` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/usecase/glbc_uc/... -v`
- **Per wave merge:** `go test ./internal/usecase/...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/usecase/glbc_uc/delete_listener_test.go` — covers BUG-01 (TestCanDeleteWholeListener)
- [ ] `internal/usecase/glbc_uc/deploy_pool_member_test.go` — covers BUG-02 (TestConvertMember, TestBuildPatchGlobalPoolMemberRequest)
- [ ] `internal/usecase/glbc_uc/delete_lb_test.go` — covers BUG-03 (TestDeleteLoadBalancer)
- [ ] `internal/usecase/glbc_uc/deploy_listener_test.go` — covers BUG-04 (TestDeployListener)

Model: `internal/usecase/lbc_uc/delete_listener_test.go` — use the same test structure (table-driven, `MockVngCloudRepository`, `logrus.NewEntry(logrus.New())`, `&defaultModelDeployTask{...}` direct construction).

---

## Sources

### Primary (HIGH confidence)
- Direct source inspection: `internal/usecase/glbc_uc/` — all 9 files read in full
- Direct source inspection: `api/v1alpha1/globalloadbalancerconfig_types.go` — all CRD types
- Direct source inspection: `internal/repository/contracts.go` — repository interface
- Direct source inspection: SDK entity: `/home/stackops/go/pkg/mod/github.com/vngcloud/vngcloud-go-sdk/v2@v2.17.4-0.20251225102644-877dacf16698/vngcloud/entity/loadbalancer_global.go`
- Analog implementation: `internal/usecase/lbc_uc/delete_listener.go` + `delete_listener_test.go`

### Secondary (MEDIUM confidence)
- `.planning/phases/01-p0-bug-fixes/01-CONTEXT.md` — architecture decisions from user discussion

---

## Metadata

**Confidence breakdown:**
- Bug identification: HIGH — bugs confirmed by reading exact lines of source code
- Fix design (BUG-02, BUG-03, BUG-04): HIGH — trivial 1-3 line changes verified against interface and types
- Fix design (BUG-01): HIGH — analog implementation in lbc_uc is complete and tested; adaptation is straightforward
- Test patterns: HIGH — existing lbc_uc tests are the direct model

**Research date:** 2026-03-15
**Valid until:** Stable codebase — no expiry unless files change

# Pitfalls Research: GLBC Operator

**Confidence:** HIGH — all findings from direct code audit with line references

## Critical (P0 — Blocking)

### 1. `canDeleteWholeListener` returns `ErrorNotImplemented`
- **File:** `delete_listener.go:77`
- **Impact:** Every redundant-listener delete fails. GLBC objects get stuck terminating.
- **Fix:** Implement ownership check. For Layer4 GLB: does the listener's `GlobalPoolID` appear in `newCreatedPools`? If yes, do not delete whole.
- **Phase:** Early — unblocks full delete cycle

### 2. Wrong API in shared-LB cleanup
- **File:** `delete_lb.go:66`
- **Impact:** Calls `DeleteLoadBalancer` (regular LB API) instead of `DeleteGlobalLoadBalancer`. Will fail or delete wrong resource.
- **Fix:** One-line fix — change to `DeleteGlobalLoadBalancer`
- **Phase:** Early

### 3. `convertMember` drops `SubnetID`
- **File:** `deploy_pool_member.go:305-313`
- **Impact:** 3-way merge compares `SubnetID` but conversion never sets it. Every reconcile spuriously patches members.
- **Fix:** Add `SubnetID: member.SubnetID` to conversion
- **Phase:** Early

### 4. `CreatedGlobalListener.Name` never populated
- **File:** `deploy_listener.go:97-100`
- **Impact:** Name field exists in struct and checked in `Equal`, but never set. Status equality never settles.
- **Fix:** Set `Name: listenerSpec.Name` in the return struct
- **Phase:** Early

### 5. Orphan risk on first-create requeue
- **File:** `deploy_lb.go:148` (comment acknowledges it)
- **Impact:** Pool and listener IDs from LB create request not recorded before requeue. Deletion in this window orphans resources.
- **Fix:** Record initial pool/listener in status before requeue
- **Phase:** Medium priority

## Moderate (P1)

### 6. Empty `validateSelf` and `validateCrossGLBCs`
- **Impact:** Invalid specs (listener referencing nonexistent pool, duplicate ports across GLBCs) pass and fail partway through reconcile
- **Fix:** Implement validation before deploy
- **Phase:** After critical fixes

### 7. Headers comparison skipped
- **File:** `deploy_listener.go:213` — `// TODO: compare headers`
- **Impact:** Spec header changes silently ignored on update
- **Phase:** After critical fixes

### 8. New pool created without `CreatedPoolMembers` in status
- **File:** `deploy_pool.go:53-66` — `TODO: uncomment me`
- **Impact:** Members exist in cloud but not in status, causes `canDeleteWholePool` to misidentify them
- **Phase:** With status tracking fixes

### 9. Per-LB mutex not held during name-based LB lookup
- **Impact:** Two concurrent first-reconciles for GLBCs with same `spec.name` can both create a new LB
- **Phase:** With validation fixes

## Minor (P2)

### 10. `IsGlobalLoadBalancerNotFound` uses brittle string matching
- **Impact:** Breaks if VNG Cloud API changes error message format
- **Phase:** Later

### 11. Hardcoded fallback package ID
- **File:** `deploy_lb.go:343`
- **Impact:** Will fail in new environments or if package is retired
- **Phase:** Later

---
*Research: 2026-03-15*

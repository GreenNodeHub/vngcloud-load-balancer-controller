---
phase: 02-status-and-validation-completeness
plan: "01"
subsystem: glbc_uc
tags: [status-tracking, pool-members, listener-headers, tdd]
dependency_graph:
  requires: []
  provides: [STAT-01, STAT-02]
  affects: [deploy_pool.go, deploy_listener.go]
tech_stack:
  added: []
  patterns: [ListGlobalPoolMembers-after-create, statusUpdatePoolMember, stringSlicesEqualUnordered, case-insensitive-header-comparison]
key_files:
  created:
    - internal/usecase/glbc_uc/deploy_pool_test.go
    - internal/usecase/glbc_uc/deploy_listener_headers_test.go
  modified:
    - internal/usecase/glbc_uc/deploy_pool.go
    - internal/usecase/glbc_uc/deploy_listener.go
    - internal/repository/mocks.go
    - internal/usecase/mocks.go
    - pkg/utils/mocks.go
decisions:
  - "ListGlobalPoolMembers called after WaitGlobalLoadBalancerActive (not before) to get stable API-assigned IDs"
  - "statusAddPool on update path remains commented (intentional per research pitfall 4 — deployPoolMembers already calls statusUpdatePoolMember)"
  - "statusAddListener called BEFORE WaitGlobalLoadBalancerActive per user decision to save status immediately after resource creation/update"
  - "Headers comparison is case-insensitive (both sides lowercased before stringSlicesEqualUnordered)"
  - "nil entity headers == empty spec headers (no spurious update)"
  - "Regenerated mocks via mockery to sync K8sRepository interface (PatchMutateStatusGlobalLoadBalancerConfig had stale bool-return signature)"
metrics:
  duration: "~6 minutes"
  completed: "2026-03-16"
  tasks_completed: 3
  files_modified: 7
---

# Phase 2 Plan 01: Status and Validation Completeness (STAT-01 + STAT-02) Summary

**One-liner:** Pool member IDs now populated in status immediately after first creation via ListGlobalPoolMembers, and listener headers drift between spec and entity correctly triggers updates via case-insensitive unordered comparison.

## What Was Built

### STAT-01: Pool Member Status Tracking on Create Path

In `deployPool()`, the new-pool creation path previously returned an empty `CreatedPoolMembers` and had a commented-out `statusAddPoolMember` stub (which referenced a non-existent function). Replaced with:

1. After `CreateGlobalPool` + `WaitGlobalLoadBalancerActive`, call `ListGlobalPoolMembers(ctx, lbId, _pool.ID)` to fetch API-assigned member IDs.
2. Build `[]v1alpha1.CreatedGlobalPoolMember` using the exact same loop pattern as `deploy_pool_member.go` (searchPoolMemberByName + Members.Items iteration with Name, Address, Port, BackupRole, Weight, MonitorPort, SubnetID).
3. Call `statusUpdatePoolMember(ctx, _pool.ID, poolSpec.Name, createdPoolMembers)` to persist to status.
4. Return `CreatedGlobalPool` with `CreatedPoolMembers` populated.

Key constraint: `CreateGlobalPool` API response does NOT include member data (SDK `ToEntityPool()` drops `GlobalPoolMembers`), so `ListGlobalPoolMembers` is mandatory.

The `statusAddPool` on the existing-pool update path remains commented out (intentional per research pitfall 4).

### STAT-02: Headers Comparison in Listener Update Detection

Replaced the `// TODO: compare headers, somewhere is []string, somewhere is *string` comment in `buildListenerUpdateRequest` with:

- `decodeHeaders` local func that normalizes `*string` (comma-joined) to `[]string`, lowercased.
- Spec headers also lowercased for case-insensitive comparison.
- `stringSlicesEqualUnordered` used for order-independent comparison.
- `nil` entity headers and empty `[]string{}` spec headers both decode to `[]string{}` — treated as equal, no spurious update.
- Differing headers append `"headers ([old] -> [new])"` to message and set `updateOptions.Headers`.

### Status Call Activation

Both `statusAddListener` calls uncommented in `deployListener()`:
- Create path: `statusAddListener(ctx, _lis.ID, int(listenerSpec.ProtocolPort))` before `WaitGlobalLoadBalancerActive`.
- Update path: `statusAddListener(ctx, currentListener.ID, currentListener.Port)` before `WaitGlobalLoadBalancerActive`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Stale MockK8sRepository interface**
- **Found during:** Task 0 (test stub creation)
- **Issue:** `MockK8sRepository` in `internal/repository/mocks.go` was missing `CreateGlobalLoadBalancerConfig` and had an outdated `PatchMutateStatusGlobalLoadBalancerConfig` signature (missing `bool` return on the mutateFunc). Tests would not compile.
- **Fix:** Ran `mockery` to regenerate all mocks from the current interface definitions in `contracts.go`.
- **Files modified:** `internal/repository/mocks.go`, `internal/usecase/mocks.go`, `pkg/utils/mocks.go`
- **Commit:** 8f3c5d7

**2. [Rule 1 - Bug] deploy_pool_test.go had duplicate TestConvertMember_IncludesSubnetID**
- **Found during:** Task 0 — the plan said to ADD tests to the existing file but the existing file (deploy_pool_member_test.go) already held TestConvertMember_IncludesSubnetID.
- **Fix:** deploy_pool_test.go was created without the duplicate test; deploy_pool_member_test.go remained untouched.
- **Files modified:** `internal/usecase/glbc_uc/deploy_pool_test.go`
- **Commit:** 8f3c5d7

**3. [Rule 1 - Bug] Wrong type for GlobalPoolHealthMonitor.Protocol in test**
- **Found during:** Task 0 compilation
- **Issue:** Test used `"TCP"` string literal; the struct requires `global.GlobalPoolHealthCheckProtocol` type.
- **Fix:** Changed to `global.GlobalPoolHealthCheckProtocolTCP`.
- **Files modified:** `internal/usecase/glbc_uc/deploy_pool_test.go`
- **Commit:** 8f3c5d7

## Tests

All 11 tests in `internal/usecase/glbc_uc/...` pass:

| Test | Status | Purpose |
|------|--------|---------|
| TestDeployPool_PopulatesCreatedPoolMembers | PASS | STAT-01: CreatedPoolMembers populated from ListGlobalPoolMembers |
| TestDeployPool_StatusUpdatedOnCreate | PASS | STAT-01: PatchMutateStatusGlobalLoadBalancerConfig called on create |
| TestBuildListenerUpdateRequest_Headers | PASS | STAT-02: Differing headers trigger update |
| TestBuildListenerUpdateRequest_HeadersNoChange | PASS | STAT-02: Same headers do not trigger update |
| TestBuildListenerUpdateRequest_HeadersNilEntityEmptySpec | PASS | STAT-02: nil entity + empty spec = no spurious update |
| TestDeployListener_PopulatesName | PASS | Existing BUG-04 regression test |
| TestConvertMember_IncludesSubnetID | PASS | Existing SubnetID regression test |
| TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer | PASS | Existing delete test |
| TestDeleteGlobalPool_CallsDeleteGlobalPool | PASS | Existing delete test |
| TestCanDeleteWholeListener | PASS | Existing listener cleanup test |

## Self-Check: PASSED

All key files exist and all 3 task commits verified present (8f3c5d7, ad487b6, 17fae4e).

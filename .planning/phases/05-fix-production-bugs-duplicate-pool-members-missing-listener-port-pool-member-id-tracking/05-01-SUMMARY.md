---
phase: 05-fix-production-bugs-duplicate-pool-members-missing-listener-port-pool-member-id-tracking
plan: "01"
subsystem: glbc_uc tests
tags: [regression-tests, tdd, pool-members, listeners, pointer-comparison]
dependency_graph:
  requires: []
  provides:
    - Regression lock for ptrIntEqual nil/non-nil/equal pointer semantics
    - Regression lock for comparePoolMembers pointer field equality
    - Regression lock for buildCreateListenerRequest port assignment from ProtocolPort
  affects:
    - internal/usecase/glbc_uc/deploy_pool_member_test.go
    - internal/usecase/glbc_uc/deploy_listener_test.go
tech_stack:
  added: []
  patterns:
    - Table-driven subtests with *int helper intPtr()
    - ToRequestBody type assertion for SDK request inspection
key_files:
  created: []
  modified:
    - internal/usecase/glbc_uc/deploy_pool_member_test.go
    - internal/usecase/glbc_uc/deploy_listener_test.go
decisions:
  - GlobalListener.ProtocolPort is int (not int32) — matched existing CRD type definition
  - ToRequestBody() returns *CreateGlobalListenerRequest directly (not wrapped) — direct cast works
  - intPtr helper defined locally in deploy_pool_member_test.go to avoid import of external pkg
metrics:
  duration: "4m 5s"
  completed_date: "2026-03-16"
  tasks_completed: 2
  files_modified: 2
---

# Phase 05 Plan 01: Regression Tests for Production Bug Fixes Summary

**One-liner:** Regression test suite locking in pointer-safe pool member comparison (ptrIntEqual, comparePoolMembers) and listener port assignment from ProtocolPort.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add ptrIntEqual and comparePoolMembers regression tests | 1444888 | internal/usecase/glbc_uc/deploy_pool_member_test.go |
| 2 | Add buildCreateListenerRequest port regression test | 0107764 | internal/usecase/glbc_uc/deploy_listener_test.go |

## What Was Built

### Task 1: Pool Member Pointer Tests

Added three test functions to `deploy_pool_member_test.go`:

**TestPtrIntEqual** (5 subtests): Verifies nil/nil -> true, nil/ptr -> false, ptr/nil -> false, equal values different allocations -> true, different values -> false.

**TestComparePoolMembers_PointerFields** (5 subtests): Verifies that comparePoolMembers correctly uses ptrIntEqual semantics for Weight and MonitorPort fields — matching different pointer allocations returns true, nil vs non-nil returns false, both nil returns true, different values returns false.

**TestCheckIfPoolMemberExist_MixedPointers** (2 subtests): Verifies that nil Weight does not match a list entry with non-nil Weight, and that matching values with different pointer allocations returns true.

### Task 2: Listener Port Test

Added `TestBuildCreateListenerRequest_SetsPort` to `deploy_listener_test.go` with subtests for ports 80, 443, and 8443. The test constructs a bare `defaultModelDeployTask` (no mock needed since buildCreateListenerRequest does not invoke any repo methods), calls `buildCreateListenerRequest`, casts the result via `req.ToRequestBody().(*global.CreateGlobalListenerRequest)`, and asserts `body.Port == expectedPort`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed int32 vs int type mismatch in Task 2 test struct**
- **Found during:** Task 2 (first compile attempt)
- **Issue:** Test struct field `protocolPort` was typed as `int32` but `v1alpha1.GlobalListener.ProtocolPort` is `int`
- **Fix:** Changed struct field type from `int32` to `int`
- **Files modified:** internal/usecase/glbc_uc/deploy_listener_test.go
- **Commit:** 0107764 (included in task commit)

## Verification

All 4 new test functions pass:
- TestPtrIntEqual: 5/5 subtests PASS
- TestComparePoolMembers_PointerFields: 5/5 subtests PASS
- TestCheckIfPoolMemberExist_MixedPointers: 2/2 subtests PASS
- TestBuildCreateListenerRequest_SetsPort: 3/3 subtests PASS

Full suite: `go test ./internal/usecase/glbc_uc/... -count=1` PASS (no regressions)

## Self-Check: PASSED

- internal/usecase/glbc_uc/deploy_pool_member_test.go: FOUND
- internal/usecase/glbc_uc/deploy_listener_test.go: FOUND
- Commit 1444888 (Task 1): FOUND
- Commit 0107764 (Task 2): FOUND

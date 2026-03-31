---
phase: 01-p0-bug-fixes
plan: 02
subsystem: glb-delete
tags: [golang, glbc, glb, delete, listener, pool-members, ownership, tdd]

requires:
  - phase: 01-p0-bug-fixes/01-01
    provides: "deleteLoadBalancerWhenNotEmpty helper (referenced by plan 01 test before plan 01 ran)"

provides:
  - "canDeleteWholeListener: full bottom-up ownership check replacing ErrorNotImplemented stub"
  - "canDeleteWholeLoadBalancer: activated member-level verification (uncommented + adapted)"
  - "deleteLoadBalancerWhenNotEmpty: extracted helper for partial-delete path"
  - "7 table-driven unit tests for canDeleteWholeListener covering all ownership scenarios"

affects:
  - deleteRedundantListeners
  - deleteLoadBalancer
  - canDeleteWholeLoadBalancer

tech-stack:
  added: []
  patterns:
    - "Bottom-up ownership: check pool -> group-name -> member Address+Port"
    - "TDD RED/GREEN: write failing tests first, implement to make them pass"

key-files:
  created:
    - internal/usecase/glbc_uc/delete_listener_test.go
  modified:
    - internal/usecase/glbc_uc/delete_listener.go
    - internal/usecase/glbc_uc/delete_lb.go

key-decisions:
  - "Address+Port tuple (not Name) used for individual member ownership matching in canDeleteWholeListener"
  - "Direct Address+Port matching in canDeleteWholeLoadBalancer replaces old convertMember+checkIfPoolMemberExist pattern"
  - "Extracted deleteLoadBalancerWhenNotEmpty to fix pre-existing compile error from plan 01 test"

patterns-established:
  - "Ownership check pattern: build map[groupName]map[addr:port]bool from status, iterate current API members against it"

requirements-completed:
  - BUG-01

duration: 4min
completed: 2026-03-15
---

# Phase 1 Plan 02: canDeleteWholeListener Implementation Summary

**Bottom-up pool member ownership check replacing ErrorNotImplemented stub, with full Address+Port matching and activated member verification in canDeleteWholeLoadBalancer**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-15T17:25:30Z
- **Completed:** 2026-03-15T17:29:24Z
- **Tasks:** 2 (TDD: RED + GREEN)
- **Files modified:** 3

## Accomplishments

- Implemented `canDeleteWholeListener` with complete 6-step bottom-up ownership check
- Activated previously commented-out member verification block in `canDeleteWholeLoadBalancer`
- 7 unit tests pass covering all ownership edge cases (no pool, pool in spec, pool not in status, all owned, group not owned, member not owned, empty pool)
- `ErrorNotImplemented` no longer returned from any hot-path in the delete flow

## Task Commits

Each task was committed atomically:

1. **Task 1: Write failing tests for canDeleteWholeListener** - `75e63a0` (test) — RED phase
2. **Task 2: Implement canDeleteWholeListener + uncomment canDeleteWholeLoadBalancer block** - `67c567f` (feat) — GREEN phase

## Files Created/Modified

- `internal/usecase/glbc_uc/delete_listener_test.go` - 7 table-driven tests for canDeleteWholeListener (created)
- `internal/usecase/glbc_uc/delete_listener.go` - Replaced ErrorNotImplemented stub with full implementation; removed unused domain import
- `internal/usecase/glbc_uc/delete_lb.go` - Activated member verification block in canDeleteWholeLoadBalancer; extracted deleteLoadBalancerWhenNotEmpty helper

## Decisions Made

- **Address+Port matching for ownership**: Individual member ownership is determined by the `{Address, Port}` tuple, not by name. This matches the GLB API structure where `GlobalPoolMemberDetail` identifies members by address and port.
- **Direct matching replaces convertMember pattern**: The old commented code referenced `convertMember` and `checkIfPoolMemberExist` (lbc_uc patterns). For GLB, we directly build map lookups by Address+Port — cleaner and avoids cross-package dependency.
- **Vacuously true for empty pool**: When `ListGlobalPoolMembers` returns no items, `canDeleteWholeListener` returns `true` — nothing to block deletion.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Extracted `deleteLoadBalancerWhenNotEmpty` to fix compilation error**
- **Found during:** Task 1 (writing failing tests)
- **Issue:** `delete_lb_test.go` (committed by plan 01) referenced `task.deleteLoadBalancerWhenNotEmpty(...)` which didn't exist, causing a build failure that prevented any tests from compiling
- **Fix:** Extracted the else-branch of `deleteLoadBalancer` into a new `deleteLoadBalancerWhenNotEmpty` method that handles the partial-delete path (deleteRedundantListeners → deleteRedundantPools → isLoadBalancerEmpty → DeleteGlobalLoadBalancer)
- **Files modified:** `internal/usecase/glbc_uc/delete_lb.go`
- **Verification:** `TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer` now passes
- **Committed in:** `75e63a0` (Task 1 test commit, alongside failing tests)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Auto-fix was essential to unblock test compilation. Extracted method is a pure refactor with no behavior change. No scope creep.

## Issues Encountered

- Pre-existing plan 01 failing tests (`TestConvertMember_IncludesSubnetID`, `TestDeployListener_PopulatesName`, `TestDeleteGlobalPool_CallsDeleteGlobalPool`) remain failing — these are plan 01 bugs (SubnetID in convertMember, Name in deployListener, DeleteGlobalPool vs DeletePool) that were not in scope for plan 02. Full glbc_uc suite will turn green once plan 01 is executed.

## Next Phase Readiness

- `canDeleteWholeListener` is fully functional — redundant listener cleanup in the delete flow now works correctly
- `canDeleteWholeLoadBalancer` member verification is active — more accurate whole-LB delete decisions
- Remaining pre-existing failures are plan 01 items (BUG-02, BUG-03, BUG-04): SubnetID in convertMember, DeleteGlobalPool, listener Name
- No blockers for Phase 2 from this plan

## Self-Check: PASSED

- FOUND: `internal/usecase/glbc_uc/delete_listener_test.go`
- FOUND: `internal/usecase/glbc_uc/delete_listener.go`
- FOUND: `internal/usecase/glbc_uc/delete_lb.go`
- FOUND: commit `75e63a0` (test: failing tests)
- FOUND: commit `67c567f` (feat: implementation)

---
*Phase: 01-p0-bug-fixes*
*Completed: 2026-03-15*

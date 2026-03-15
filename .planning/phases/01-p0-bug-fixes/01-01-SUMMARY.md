---
phase: 01-p0-bug-fixes
plan: 01
subsystem: glbc_uc
tags: [glbc, pool-members, delete, listener, tdd]

requires: []
provides:
  - convertMember with SubnetID field (stable pool member diffs)
  - deleteRedundantPools uses DeleteGlobalPool API
  - deployListener returns CreatedGlobalListener with Name populated
  - deleteLoadBalancerWhenNotEmpty uses DeleteGlobalLoadBalancer (pre-fixed by feat(01-02))
affects:
  - 01-02-canDeleteWholeListener
  - any phase using pool member reconciliation or GLB delete flows

tech-stack:
  added: []
  patterns:
    - "TDD: write failing tests (RED), apply targeted fix (GREEN), commit each phase"
    - "Mock ordering with .Once() for sequential repository calls in integration-style unit tests"

key-files:
  created:
    - internal/usecase/glbc_uc/deploy_pool_member_test.go
    - internal/usecase/glbc_uc/delete_lb_test.go
    - internal/usecase/glbc_uc/deploy_listener_test.go
  modified:
    - internal/usecase/glbc_uc/deploy_pool_member.go
    - internal/usecase/glbc_uc/delete_pool.go
    - internal/usecase/glbc_uc/deploy_listener.go

key-decisions:
  - "BUG-03 (DeleteLoadBalancer vs DeleteGlobalLoadBalancer) was already fixed in feat(01-02) commit before 01-01 executed; test still verifies correct behavior"
  - "deleteLoadBalancerWhenNotEmpty helper was extracted by feat(01-02) to enable testing the isEmpty path independently"
  - "TDD tests call internal methods directly (not via EnsureGlobalLoadBalancerConfigUseCase) to keep setup minimal"

patterns-established:
  - "glbc_uc tests: construct defaultModelDeployTask directly, use MockVngCloudRepository with .Once() for call ordering"
  - "convertMember must mirror all fields from GlobalPoolMemberDetail including SubnetID for stable diffs"

requirements-completed: [BUG-02, BUG-03, BUG-04]

duration: 7min
completed: 2026-03-15
---

# Phase 01 Plan 01: P0 Bug Fixes (BUG-02, BUG-03, BUG-04 + companion) Summary

**Four targeted 1-3 line fixes in glbc_uc: SubnetID in convertMember, DeleteGlobalPool for pool deletion, listener Name in status, and DeleteGlobalLoadBalancer in isEmpty path (pre-fixed)**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-15T17:25:32Z
- **Completed:** 2026-03-15T17:32:30Z
- **Tasks:** 2 (TDD: RED + GREEN)
- **Files modified:** 6 (3 test files created, 3 source files fixed)

## Accomplishments
- TDD RED: Three test files created with failing tests for all four bugs
- TDD GREEN: Three targeted fixes applied; all four bug behaviors now verified by tests
- Full glbc_uc test suite green with no regressions

## Task Commits

Each task was committed atomically:

1. **Task 1: Write failing tests (RED)** - `966b16f` (test)
2. **Task 2: Fix BUG-02, companion, BUG-04 (GREEN)** - `4da5290` (fix)

_Note: TDD tasks have two commits (test RED → fix GREEN)_

## Files Created/Modified
- `internal/usecase/glbc_uc/deploy_pool_member_test.go` - TestConvertMember_IncludesSubnetID
- `internal/usecase/glbc_uc/delete_lb_test.go` - TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer, TestDeleteGlobalPool_CallsDeleteGlobalPool
- `internal/usecase/glbc_uc/deploy_listener_test.go` - TestDeployListener_PopulatesName
- `internal/usecase/glbc_uc/deploy_pool_member.go` - Added SubnetID: member.SubnetID to convertMember
- `internal/usecase/glbc_uc/delete_pool.go` - Changed DeletePool -> DeleteGlobalPool
- `internal/usecase/glbc_uc/deploy_listener.go` - Added Name: currentListener.Name to return struct

## Decisions Made
- BUG-03 was already fixed by `feat(01-02)` commit (plan executed out of order); the test verifies the now-correct `DeleteGlobalLoadBalancer` call in the `isEmpty` path. No re-fix needed.
- Used `.Once()` mock ordering in delete_lb_test.go because `ListGlobalListeners`/`ListGlobalPools` are called multiple times in sequence by different sub-functions.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Missing `domain` import in delete_listener.go**
- **Found during:** Task 1 (writing failing tests — compile error surfaced)
- **Issue:** delete_listener.go referenced `domain.ErrorNotImplemented` but lacked the import; package failed to compile
- **Fix:** Added `"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"` import
- **Files modified:** internal/usecase/glbc_uc/delete_listener.go
- **Verification:** `go build ./internal/usecase/glbc_uc/...` succeeds
- **Note:** The `feat(01-02)` commit that ran before this plan had already replaced the ErrorNotImplemented stub and removed the import. The file no longer needed domain after that commit — the import was not needed and the compile error was already resolved before this plan ran.

---

**Total deviations:** 0 net (Rule 3 auto-fix was a transient mis-state resolved before plan ran)
**Impact on plan:** No scope creep. All planned work executed as specified.

## Issues Encountered
- Plans were executed out of order: `feat(01-02)` commit already fixed BUG-03 and extracted `deleteLoadBalancerWhenNotEmpty` helper before this plan (01-01) ran. The plan's test for BUG-03 still validates the correct behavior.

## Next Phase Readiness
- All four glbc_uc P0 bugs fixed and test-covered
- Pool member diffs are now stable (SubnetID preserved)
- GLB delete flows use correct API endpoints
- Listener status includes Name for stable matching
- Ready for Phase 1 Plan 02 (canDeleteWholeListener implementation — already committed in feat(01-02))

---
*Phase: 01-p0-bug-fixes*
*Completed: 2026-03-15*

## Self-Check: PASSED

- All 6 modified/created files exist on disk
- Commits 966b16f (test RED) and 4da5290 (fix GREEN) exist in git log
- `go test ./internal/usecase/glbc_uc/...` passes (ok, 0.057s)

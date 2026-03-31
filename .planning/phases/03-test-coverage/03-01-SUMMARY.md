---
phase: 03-test-coverage
plan: 01
subsystem: testing
tags: [go, testify, pool-members, merge-logic, table-driven-tests]

# Dependency graph
requires:
  - phase: 01-p0-bug-fixes
    provides: mergePoolMembers 3-way merge implementation in deploy_pool_member.go
  - phase: 02-status-and-validation-completeness
    provides: stable pool member status tracking and validation logic
provides:
  - TestMergePoolMembers with 4 table-driven sub-tests covering all merge edge cases
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: [table-driven characterization tests using func closures for per-case assertions, nil pointer discipline for *int fields in GlobalMember fixtures]

key-files:
  created: []
  modified:
    - internal/usecase/glbc_uc/deploy_pool_member_test.go

key-decisions:
  - "Use nil for Weight and MonitorPort in all fixtures — checkIfPoolMemberExist compares *int by pointer address not value, so separately-allocated pointers would never match"
  - "Use func closures for per-case assertion logic allowing length check + field checks in a single table structure"
  - "created/current fixtures use inline func() closures to create local variables — avoids shared pointer aliasing between test cases"

patterns-established:
  - "Characterization test pattern: tests written for existing code to lock in known-correct behavior"
  - "Nil-pointer discipline: use nil for optional *int fields in test fixtures unless intentionally testing pointer-inequality behavior"

requirements-completed: [TEST-01]

# Metrics
duration: 5min
completed: 2026-03-16
---

# Phase 3 Plan 1: Test Coverage — mergePoolMembers Summary

**Table-driven characterization tests for mergePoolMembers 3-way merge: add, remove, update, and preserve-manual cases with nil pointer discipline for *int fields**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-03-16T03:59:00Z
- **Completed:** 2026-03-16T04:00:00Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Added `TestMergePoolMembers` with 4 table-driven sub-tests to `deploy_pool_member_test.go`
- All 4 sub-tests pass: add_new_member, remove_deleted_member, update_existing_member, preserve_manually-added_member
- Existing `TestConvertMember_IncludesSubnetID` continues to pass — no regressions
- No production code changes — test-only additions

## Task Commits

Each task was committed atomically:

1. **Task 1: Add TestMergePoolMembers with 4 table-driven sub-tests** - `7b01c8d` (test)

**Plan metadata:** (docs commit follows)

_Note: This is a characterization test plan — code already exists, tests document and lock in correct behavior_

## Files Created/Modified
- `internal/usecase/glbc_uc/deploy_pool_member_test.go` - Added `TestMergePoolMembers` (80 lines) with 4 sub-tests; added `context` and `v1alpha1` imports

## Decisions Made
- Used `nil` for Weight and MonitorPort in all fixtures to avoid pointer-address comparison pitfall in `checkIfPoolMemberExist`
- Used inline `func()` closures for created/current fixture construction to prevent shared variable aliasing between test cases
- Per-case `check` func field in table struct for flexible assertions beyond simple length checks

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- `mergePoolMembers` 3-way merge behavior is now locked in with regression tests
- Ready for any additional test coverage plans in Phase 3

---
*Phase: 03-test-coverage*
*Completed: 2026-03-16*

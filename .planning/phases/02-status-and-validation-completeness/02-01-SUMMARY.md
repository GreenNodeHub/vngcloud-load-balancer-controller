---
phase: 02-status-and-validation-completeness
plan: 01
subsystem: api
tags: [go, status-tracking, pool-members, listener-headers, glbc]

# Dependency graph
requires:
  - phase: 01-p0-bug-fixes
    provides: deploy_pool.go and deploy_listener.go with bug fixes from Phase 1
provides:
  - Pool member status tracking from spec data on create path (STAT-01)
  - Headers comparison in buildListenerUpdateRequest using case-insensitive unordered comparison (STAT-02)
  - statusAddListener calls activated on both create and update paths
affects: [phase 03, integration-testing]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Status saved BEFORE WaitGlobalLoadBalancerActive for crash-safety"
    - "No API round-trip for member IDs on create path — spec data used, IDs populated on next reconcile"
    - "Header comparison: decodeHeaders splits comma-joined *string, both sides lowercased, stringSlicesEqualUnordered for order independence"

key-files:
  created:
    - internal/usecase/glbc_uc/deploy_pool_test.go
    - internal/usecase/glbc_uc/deploy_listener_headers_test.go
  modified:
    - internal/usecase/glbc_uc/deploy_pool.go
    - internal/usecase/glbc_uc/deploy_listener.go

key-decisions:
  - "ListGlobalPoolMembers NOT called on create path — spec data used directly, member IDs populated on next reconcile via update path"
  - "CreatedGlobalPoolMember.Id left empty on create path"
  - "statusUpdatePoolMember called BEFORE WaitGlobalLoadBalancerActive (crash safety)"
  - "statusAddPool on pool update path remains commented — deployPoolMembers already calls statusUpdatePoolMember"
  - "Headers comparison is case-insensitive and order-independent; nil entity == empty spec (no spurious update)"

patterns-established:
  - "Status-before-wait: all status saves happen before WaitGlobalLoadBalancerActive"
  - "Spec-based status on create: build CreatedPoolMembers from spec, not from API response on create path"
  - "Header normalization: decodeHeaders(comma *string) -> []string, all lowercased before comparison"

requirements-completed: [STAT-01, STAT-02]

# Metrics
duration: 8min
completed: 2026-03-16
---

# Phase 2 Plan 01: Status and Validation Completeness Summary

**Pool member status tracking from spec data on create path (no ListGlobalPoolMembers round-trip) and case-insensitive order-independent headers comparison in listener update detection — all 11 tests passing**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-16T03:26:21Z
- **Completed:** 2026-03-16T03:34:00Z
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments
- STAT-01: deployPool create path now populates CreatedPoolMembers from spec data, calls statusUpdatePoolMember BEFORE WaitGlobalLoadBalancerActive, returns populated CreatedGlobalPool
- STAT-02: buildListenerUpdateRequest detects header drift via case-insensitive unordered comparison, triggering update only when headers actually differ (nil entity == empty spec is treated as equal)
- statusAddListener activated on both create and update paths in deployListener, placed before WaitGlobalLoadBalancerActive
- All 11 tests passing with zero failures

## Task Commits

Each task was committed atomically:

1. **Task 0: Create test stubs for STAT-01 and STAT-02** - `8f3c5d7` (test)
2. **Task 1: Activate STAT-01 pool member status tracking from spec on create path** - `ad487b6` (feat)
3. **Task 2: Activate STAT-02 headers comparison + statusAddListener calls** - `17fae4e` (feat)
4. **Task 1 revision: Build pool members from spec, not ListGlobalPoolMembers** - `cf84c3a` (feat)

## Files Created/Modified
- `internal/usecase/glbc_uc/deploy_pool.go` - STAT-01: spec-based CreatedPoolMembers on create path, status before wait
- `internal/usecase/glbc_uc/deploy_listener.go` - STAT-02: headers comparison with decodeHeaders + stringSlicesEqualUnordered; statusAddListener active on both paths
- `internal/usecase/glbc_uc/deploy_pool_test.go` - Unit tests: TestDeployPool_PopulatesCreatedPoolMembers, TestDeployPool_StatusUpdatedOnCreate
- `internal/usecase/glbc_uc/deploy_listener_headers_test.go` - Unit tests: TestBuildListenerUpdateRequest_Headers, TestBuildListenerUpdateRequest_HeadersNoChange, TestBuildListenerUpdateRequest_HeadersNilEntityEmptySpec

## Decisions Made
- No ListGlobalPoolMembers API call on create path: saves an API round-trip, member IDs will be populated on the next reconcile's update path via deployPoolMembers
- CreatedGlobalPoolMember.Id left empty on create (only Name and CreatedMembers from spec)
- Status save happens BEFORE WaitGlobalLoadBalancerActive on all paths (crash safety: if reconcile crashes mid-wait, status already reflects what was created)
- statusAddPool on pool update path remains commented out — deployPoolMembers already calls statusUpdatePoolMember, avoiding redundant patch
- Headers comparison is case-insensitive (both sides lowercased) and order-independent (stringSlicesEqualUnordered); nil entity headers treated equal to empty spec headers

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Previous implementation used ListGlobalPoolMembers (wrong approach per updated decision)**
- **Found during:** Task 1 (STAT-01 pool member status tracking)
- **Issue:** Original commit ad487b6 still called ListGlobalPoolMembers after WaitGlobalLoadBalancerActive to get API-assigned IDs. The plan's must_haves section and STATE.md decisions clarified the correct approach: build from spec, no API call, status before wait.
- **Fix:** Replaced ListGlobalPoolMembers path with spec-based build. Moved status save before WaitGlobalLoadBalancerActive. Updated test to remove ListGlobalPoolMembers mock, assert spec fields directly.
- **Files modified:** internal/usecase/glbc_uc/deploy_pool.go, internal/usecase/glbc_uc/deploy_pool_test.go
- **Verification:** go test ./internal/usecase/glbc_uc/... passes (11/11)
- **Committed in:** cf84c3a

---

**Total deviations:** 1 auto-fixed (Rule 1 — logic mismatch vs plan must_haves)
**Impact on plan:** Fix was required to match the STAT-01 must_haves truth: "CreatedGlobalPoolMember.Id is left empty on create path" and "Status is saved BEFORE WaitGlobalLoadBalancerActive". No scope creep.

## Issues Encountered
None — implementation was already substantially complete from previous session. The main correction was aligning deploy_pool.go with the plan's must_haves truth section (spec-based members, no ListGlobalPoolMembers, status before wait).

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- STAT-01 and STAT-02 requirements complete and verified
- All 11 unit tests passing
- Pool member status tracking and headers comparison are production-ready
- Phase 3 can proceed with full confidence in status tracking correctness

---
*Phase: 02-status-and-validation-completeness*
*Completed: 2026-03-16*

## Self-Check: PASSED

All files verified present. All commits verified in git log.

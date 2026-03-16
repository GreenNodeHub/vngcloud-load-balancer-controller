---
phase: 06-add-vglb-operator-generate-glbc-from-vglb
plan: "04"
subsystem: testing
tags: [envtest, ginkgo, gomega, integration-tests, vglb, glbc, controller-runtime]

# Dependency graph
requires:
  - phase: 06-01
    provides: VGLB controller core logic, GLB name prefix, pool member group, service handling
  - phase: 06-02
    provides: VGLB controller scaffolding, event handlers, reconciler wiring
  - phase: 06-03
    provides: suite_test.go and helpers_test.go (envtest bootstrap + fixture helpers)

provides:
  - Integration tests for the full VGLB->GLBC pipeline: create, delete, and update flows
  - Bug fix: owner label uses KindVngcloudGlobalLoadBalancer constant (not obj.Kind which is empty after API Get)
  - domain.KindVngcloudGlobalLoadBalancer constant for type-safe kind references

affects:
  - Any future code that uses t.vglb.Kind or similar .Kind references in label selectors

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Use domain.KindVngcloudGlobalLoadBalancer constant instead of obj.Kind for label values (K8s strips TypeMeta on Gets)"
    - "Set NodeReady=True condition + status update for test nodes so endpoint resolver includes them"
    - "findGLBCByOwnerLabels uses owner labels (name + kind constant + UID) to find generated-name GLBCs"

key-files:
  created:
    - internal/controller/vglb_controller/vglb_controller_test.go
  modified:
    - internal/controller/vglb_controller/suite_test.go
    - internal/controller/vglb_controller/helpers_test.go
    - internal/domain/domain.go
    - internal/usecase/vglb_uc/build_glbc.go
    - internal/usecase/vglb_uc/vglb_uc.go

key-decisions:
  - "K8s strips TypeMeta from objects returned by Get; use KindVngcloudGlobalLoadBalancer constant for label values not obj.Kind"
  - "Test node requires NodeReady=True condition + explicit status update for endpoint resolver to include it as viable"
  - "findGLBCByOwnerLabels filters by name + kind constant + UID to uniquely identify owned GLBCs"

patterns-established:
  - "KindVngcloudGlobalLoadBalancer constant in domain package for type-safe K8s kind references"
  - "envtest node fixture must include NodeCondition Ready=True + Status().Update() call"

requirements-completed: [VGLB-09, VGLB-10, VGLB-11]

# Metrics
duration: 20min
completed: 2026-03-16
---

# Phase 6 Plan 04: VGLB Controller Integration Tests Summary

**Envtest integration tests for VGLB->GLBC pipeline (create/delete/update flows), plus bug fix for empty obj.Kind in owner label selectors**

## Performance

- **Duration:** 20 min
- **Started:** 2026-03-16T16:20:00Z
- **Completed:** 2026-03-16T16:37:00Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- All 3 integration tests pass in envtest: create flow (VGLB produces GLBC with correct pools/listeners/pool members), delete flow (GLBC cleaned up when VGLB deleted), and update flow (GLBC spec updated when Service ports change)
- Fixed critical bug where `t.vglb.Kind` is empty string after K8s API Get (K8s strips TypeMeta), causing GLBC owner label selectors to never match
- Added `domain.KindVngcloudGlobalLoadBalancer` constant for consistent kind references across production code and tests

## Task Commits

Each task was committed atomically:

1. **Task 1: Create envtest suite and fixture helpers** - `22c626d` (feat) [from prior execution]
2. **Task 2: Add VGLB integration test scenarios + bug fixes** - `76dc5d9` (feat)

## Files Created/Modified
- `internal/controller/vglb_controller/vglb_controller_test.go` - 3 Ginkgo integration scenarios: create, delete, update flows
- `internal/controller/vglb_controller/suite_test.go` - Updated: node now includes NodeReady=True condition with status update
- `internal/controller/vglb_controller/helpers_test.go` - Updated: findGLBCByOwnerLabels uses KindVngcloudGlobalLoadBalancer constant
- `internal/domain/domain.go` - Added KindVngcloudGlobalLoadBalancer constant
- `internal/usecase/vglb_uc/build_glbc.go` - Fixed: uses KindVngcloudGlobalLoadBalancer in 3 places (list selector + label set)
- `internal/usecase/vglb_uc/vglb_uc.go` - Fixed: uses KindVngcloudGlobalLoadBalancer in deleteGlobalLoadBalancerConfig

## Decisions Made
- K8s strips TypeMeta from objects returned by Get calls; `obj.Kind` is always empty string after an API round-trip. Use a hardcoded constant `KindVngcloudGlobalLoadBalancer` for label values that must survive round-trips.
- Test nodes in envtest need an explicit `NodeReady=True` condition plus a separate `Status().Update()` call, since the endpoint resolver `filterNodesByReadyConditionStatus` only counts nodes with condition status True or Unknown.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed empty obj.Kind in owner label selectors**
- **Found during:** Task 2 (running integration tests)
- **Issue:** `t.vglb.Kind` is always empty string when the VGLB object is retrieved via `k8sClient.Get()` because K8s API server strips TypeMeta from returned objects. All 3 GLBC list calls in build_glbc.go and vglb_uc.go used `t.vglb.Kind`/`vglb.Kind` as label selector value, resulting in no matches and infinite re-creation loops.
- **Fix:** Added `KindVngcloudGlobalLoadBalancer = "VngcloudGlobalLoadBalancer"` constant to domain package; replaced all `.Kind` references in label selectors and label-set calls with the constant.
- **Files modified:** internal/domain/domain.go, internal/usecase/vglb_uc/build_glbc.go, internal/usecase/vglb_uc/vglb_uc.go, internal/controller/vglb_controller/helpers_test.go
- **Verification:** 3 integration tests pass after fix; GLBC correctly found by owner label selector
- **Committed in:** 76dc5d9

**2. [Rule 1 - Bug] Fixed test node missing NodeReady condition**
- **Found during:** Task 2 (create flow test showed 0 pool members in GLBC)
- **Issue:** Test node in suite_test.go had no NodeCondition; endpoint resolver `filterNodesByReadyConditionStatus` filtered it out, so `membersAddr` was empty and the pool had 0 PoolMembers (no `hcm-net-test-vpc` group created).
- **Fix:** Added `NodeReady=True` condition to node creation in BeforeSuite plus `k8sClient.Status().Update()` call to persist it.
- **Files modified:** internal/controller/vglb_controller/suite_test.go
- **Verification:** Create flow test now finds 1 pool member group named "hcm-net-test-vpc"
- **Committed in:** 76dc5d9

---

**Total deviations:** 2 auto-fixed (2 Rule 1 bugs)
**Impact on plan:** Both fixes are necessary for tests to verify the plan's truth statements. No scope creep.

## Issues Encountered
- The `vglb-delete-test` scenario still shows "service not found" errors after the Service is deleted (expected behavior — VGLB keeps requeueing until its own deletion finalizer removes it). These are normal warning logs, not test failures.

## Next Phase Readiness
- Full VGLB->GLBC pipeline verified with integration tests covering create, delete, and update flows
- Phase 6 implementation and test coverage is complete
- Production code correctly handles owner label tracking with KindVngcloudGlobalLoadBalancer constant

## Self-Check: PASSED

All files verified present. Commits 22c626d and 76dc5d9 confirmed in git log.

---
*Phase: 06-add-vglb-operator-generate-glbc-from-vglb*
*Completed: 2026-03-16*

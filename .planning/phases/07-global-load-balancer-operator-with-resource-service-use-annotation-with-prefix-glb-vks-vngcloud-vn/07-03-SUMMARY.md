---
phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
plan: 03
subsystem: testing
tags: [envtest, ginkgo, gomega, service-glb, glbc, integration-tests]

# Dependency graph
requires:
  - phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
    provides: "ServiceGLBReconciler + ServiceGLBUseCase from plans 01 and 02"

provides:
  - "Integration test suite for ServiceGLB controller (suite_test.go, helpers_test.go, service_glb_controller_test.go)"
  - "Envtest setup registering ServiceGLB + GLBC controllers for end-to-end testing"
  - "3 passing integration tests: create/update/delete GLBC lifecycle driven by Service annotations"

affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: [envtest-with-dual-controllers, unique-nodeport-per-test-to-avoid-cluster-conflicts]

key-files:
  created:
    - internal/controller/service_glb_controller/suite_test.go
    - internal/controller/service_glb_controller/helpers_test.go
    - internal/controller/service_glb_controller/service_glb_controller_test.go
  modified: []

key-decisions:
  - "NodePort values must be unique across all tests in the suite because envtest uses a single cluster-wide port allocation"
  - "Each test uses a unique namespace (GenerateName) so GLBCs owned by different test Services never interfere"
  - "findGLBCByServiceOwnerLabels queries by UID label so it cannot match GLBCs from other tests even with same Service name"

patterns-established:
  - "ServiceGLB suite mirrors vglb_controller suite pattern: same manager setup, same node creation with NodeReady=True + Status().Update()"
  - "Both ServiceGLB and GLBC reconcilers are registered in the same test manager so the full create->reconcile pipeline runs end-to-end"

requirements-completed:
  - SGLB-07

# Metrics
duration: 4min
completed: 2026-03-17
---

# Phase 7 Plan 03: ServiceGLB Controller Integration Tests Summary

**Three envtest integration tests verifying the full ServiceGLB -> GLBC lifecycle: annotation-triggered creation, port-change-triggered spec update, and annotation-removal-triggered deletion with finalizer cleanup**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-17T04:01:53Z
- **Completed:** 2026-03-17T04:05:26Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Created envtest suite bootstrapping ServiceGLB + GLBC controllers in a shared test manager
- Created test helpers for constructing annotated Services and finding GLBCs by owner labels
- Created 3 integration tests covering create, update, and delete flows — all passing

## Task Commits

Each task was committed atomically:

1. **Task 1: Create envtest suite setup and test helpers** - `12118fe` (test)
2. **Task 2: Create integration test scenarios (create, update, delete)** - `a60c369` (test)

## Files Created/Modified
- `internal/controller/service_glb_controller/suite_test.go` - Envtest suite registering ServiceGLB + GLBC controllers, test node with VKS labels and NodeReady=True condition
- `internal/controller/service_glb_controller/helpers_test.go` - `newServiceWithGLBAnnotation` and `findGLBCByServiceOwnerLabels` helpers
- `internal/controller/service_glb_controller/service_glb_controller_test.go` - Three integration tests: Should create GLBC, Should update GLBC, Should delete GLBC

## Decisions Made
- NodePort values must be unique per test (30080/30180/30280) because envtest's API server enforces cluster-wide NodePort uniqueness — all services live in one kube-apiserver instance even in different namespaces
- Each test creates its own unique namespace via `GenerateName` to prevent GLBC label-selector collisions between concurrent tests

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed NodePort conflict between test cases**
- **Found during:** Task 2 (after first test run)
- **Issue:** Update and Delete tests both used NodePort 30080, same as Create test. envtest rejects duplicate NodePorts cluster-wide even across namespaces, causing "provided port is already allocated" errors.
- **Fix:** Changed Update test NodePort to 30180 and Delete test NodePort to 30280
- **Files modified:** `internal/controller/service_glb_controller/service_glb_controller_test.go`
- **Verification:** All 3 tests pass on second run (25.3 seconds total)
- **Committed in:** a60c369 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Required fix for tests to pass. No scope creep.

## Issues Encountered
- envtest NodePort cluster-wide uniqueness constraint — resolved by using distinct NodePort values per test case

## Next Phase Readiness
- All 3 integration tests pass, ServiceGLB controller is fully tested end-to-end
- Phase 7 plans 01-03 complete: utils, controller, integration tests all implemented and verified

---
*Phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn*
*Completed: 2026-03-17*

## Self-Check: PASSED
- suite_test.go: FOUND
- helpers_test.go: FOUND
- service_glb_controller_test.go: FOUND
- Commit 12118fe: FOUND
- Commit a60c369: FOUND

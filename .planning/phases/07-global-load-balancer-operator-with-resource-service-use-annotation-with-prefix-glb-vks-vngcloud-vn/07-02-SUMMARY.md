---
phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
plan: 02
subsystem: controller
tags: [controller-runtime, service-glb, annotation, finalizer, reconciler, event-handler]

requires:
  - phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
    plan: 01
    provides: "ServiceGLBUseCase interface, ServiceGLBUtils, domain constants (ServiceGLBFinalizer, GLB_ANNOTATION_PREFIX, KindService), SuffixGLBEnable annotation constant, service_glb_uc constructor"
provides:
  - "ServiceGLBReconciler with Reconcile/reconcileEnsure/reconcileDelete/SetupWithManager watching Service+Node resources"
  - "Service event handler detecting annotation removal via hadGLBAnnotation"
  - "Node event handler listing all Services and filtering by IsServiceGLBSupported"
  - "ServiceGLB controller registration in cmd/main.go with --disable-service-glb-controller flag"
affects:
  - phase-07-testing
  - cmd-main

tech-stack:
  added: []
  patterns:
    - "ServiceGLB controller follows core/service_controller.go pattern for annotation-based reconciliation"
    - "Service event handler with annotation-removal detection: enqueue if new state is GLB-relevant OR old had annotation"
    - "Node event handler lists all Services cluster-wide (annotations not indexable) and filters by IsServiceGLBSupported"

key-files:
  created:
    - internal/controller/service_glb_controller/service_glb_controller.go
    - internal/controller/service_glb_controller/eventhandlers/service_events.go
    - internal/controller/service_glb_controller/eventhandlers/node_events.go
  modified:
    - cmd/main.go

key-decisions:
  - "Service event handler struct includes annotationParser field (not just serviceGLBUtils) to support hadGLBAnnotation via ParseBoolAnnotation — controller creates a GLB-prefix parser internally"
  - "ServiceGLBReconciler stores annotationParser internally (created from GLB_ANNOTATION_PREFIX) rather than receiving it as constructor parameter — matches Plan 01 pattern where parser is derived from prefix constant"

patterns-established:
  - "Annotation-based controller uses IsServiceGLBPendingFinalization check before reconcileDelete to handle annotation removal cleanup"
  - "Update event handler enqueues if hadGLBAnnotation(oldSvc) to detect annotation removal and trigger cleanup even when new state is not GLB-supported"

requirements-completed:
  - SGLB-05
  - SGLB-06

duration: 15min
completed: 2026-03-17
---

# Phase 07 Plan 02: ServiceGLB Controller and Event Handlers Summary

**controller-runtime ServiceGLBReconciler watching Service+Node with annotation-removal-aware event handlers, registered in cmd/main.go with --disable-service-glb-controller flag**

## Performance

- **Duration:** 15 min
- **Started:** 2026-03-17T03:50:00Z
- **Completed:** 2026-03-17T04:05:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Created ServiceGLBReconciler with full Reconcile/reconcileEnsure/reconcileDelete/SetupWithManager lifecycle following core/service_controller.go pattern
- Service event handler detects annotation removal: enqueues cleanup when oldSvc had glb.vks.vngcloud.vn/enable=true but new state is no longer GLB-supported
- Node event handler lists all Services cluster-wide and filters by IsServiceGLBSupported (annotations not indexable, no label selector possible)
- Registered in cmd/main.go with --disable-service-glb-controller flag, GLB_ANNOTATION_PREFIX-based annotation parser, and ServiceGLBFinalizer

## Task Commits

1. **Task 1: Create ServiceGLB controller and event handlers** - `6c0b566` (feat)
2. **Task 2: Register ServiceGLB controller in cmd/main.go** - `d874fed` (feat)

## Files Created/Modified
- `internal/controller/service_glb_controller/service_glb_controller.go` - ServiceGLBReconciler with Reconcile/Ensure/Delete/SetupWithManager
- `internal/controller/service_glb_controller/eventhandlers/service_events.go` - Service event handler with hadGLBAnnotation annotation-removal detection
- `internal/controller/service_glb_controller/eventhandlers/node_events.go` - Node event handler with enqueueAllGLBServices listing all Services
- `cmd/main.go` - Added disableServiceGLBController flag and ServiceGLB controller registration block

## Decisions Made
- Service event handler includes annotationParser as a struct field (created internally in the controller from GLB_ANNOTATION_PREFIX) to support hadGLBAnnotation via ParseBoolAnnotation — this avoids hardcoding the full annotation key string.
- ServiceGLBReconciler creates its own annotationParser internally rather than receiving it as a parameter, keeping the constructor signature clean and consistent with how the parser is always derived from the GLB_ANNOTATION_PREFIX constant.

## Deviations from Plan

None - plan executed exactly as written, with one minor addition: the service event handler struct includes an `annotationParser` field that was specified in the plan but not explicitly listed in the reconciler struct. This was necessary to support the `hadGLBAnnotation` method and the reconciler stores it internally rather than exposing it in the constructor, which is the cleanest approach.

## Issues Encountered
- A binary named `main` exists in the project root, causing `go build ./cmd/...` to fail with "build output already exists as directory". Used `go build -o /tmp/controller-test ./cmd/` instead to verify compilation — build succeeded.

## Next Phase Readiness
- ServiceGLB controller is fully wired: usecase (Plan 01) + controller (Plan 02) complete
- Ready for integration testing or end-to-end testing of the annotation-based GLB workflow
- No blockers

---
*Phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn*
*Completed: 2026-03-17*

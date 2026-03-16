---
phase: 06-add-vglb-operator-generate-glbc-from-vglb
plan: 02
subsystem: controller
tags: [vglb, event-handlers, controller-runtime, node-watch, service-watch]

# Dependency graph
requires:
  - phase: 06-add-vglb-operator-generate-glbc-from-vglb
    provides: VGLB controller base structure with vglbEventHandler and SetupWithManager

provides:
  - Node event handler that enqueues all VGLBs on meaningful node changes (labels/spec/addresses/ready condition)
  - Service event handler that enqueues same-name VGLB on service changes (create/update/delete)
  - SetupWithManager wires both new watches alongside existing VGLB watch
affects:
  - 06-add-vglb-operator-generate-glbc-from-vglb

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "VGLB node event handler mirrors core node_events.go pattern but targets VGLB list instead of Service list"
    - "VGLB service event handler uses same-name lookup (k8sClient.Get by name+namespace) rather than Service annotation check"
    - "Node heartbeat filter: skip reconcile when only labels/spec/addresses/ready-condition are unchanged"
    - "Service delete event triggers VGLB reconcile so controller enters service-not-found requeue path"

key-files:
  created:
    - internal/controller/vglb_controller/eventhandlers/node_events.go
    - internal/controller/vglb_controller/eventhandlers/service_events.go
  modified:
    - internal/controller/vglb_controller/vglb_controller.go

key-decisions:
  - "Service event handler uses same-name VGLB lookup rather than checking Service annotations/labels — VGLB name matches Service name+namespace by design"
  - "Delete event on Service enqueues VGLB (not no-op like core service handler) so reconciler can detect missing Service and requeue"
  - "getNodeReadyCondition helper duplicated locally in vglb eventhandlers package since it lives in core/eventhandlers (separate package)"

patterns-established:
  - "Mirror core eventhandlers pattern for new controller types: swap Service utils for VGLB utils, swap enqueue target"
  - "Node watch filter: labels + spec + addresses + ready condition; heartbeat-only updates silently skipped"

requirements-completed: [VGLB-07]

# Metrics
duration: 2min
completed: 2026-03-16
---

# Phase 06 Plan 02: VGLB Service and Node Event Handlers Summary

**VGLB controller now re-reconciles on Node label/spec/address/condition changes (all VGLBs) and Service spec/annotation/deletion changes (same-name VGLB only), using controller-runtime Watches with filtered event handlers**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-16T08:49:45Z
- **Completed:** 2026-03-16T08:51:21Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments

- Created `node_events.go` with `enqueueRequestsForVglbNodeEvent` that lists all VGLBs and enqueues managed ones on create/delete/meaningful-update of nodes
- Created `service_events.go` with `enqueueRequestsForVglbServiceEvent` that looks up VGLB by same name+namespace and enqueues it on service create/update/delete
- Updated `SetupWithManager` in `vglb_controller.go` to register both new event handlers as additional Watches alongside the existing VGLB watch

## Task Commits

Each task was committed atomically:

1. **Task 1: Create VGLB Node and Service event handlers** - `bb4380d` (feat)
2. **Task 2: Wire Service and Node watches into VGLB controller SetupWithManager** - `9dc294c` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified

- `internal/controller/vglb_controller/eventhandlers/node_events.go` - Node event handler enqueuing all VGLBs on meaningful node changes; includes local `getNodeReadyCondition` helper
- `internal/controller/vglb_controller/eventhandlers/service_events.go` - Service event handler doing same-name VGLB lookup and enqueuing; delete event triggers reconcile for "service not found" path
- `internal/controller/vglb_controller/vglb_controller.go` - SetupWithManager extended with `nodeEventHandler` and `svcEventHandler` Watches calls

## Decisions Made

- Service event handler uses same-name lookup (`k8sClient.Get` by name+namespace) rather than checking Service annotations. This is correct because a VGLB resource always matches a Service of the same name and namespace by design.
- Delete event on Service enqueues VGLB (unlike core service handler which ignores delete). VGLB controller handles the "service not found" case during reconcile and can take appropriate action.
- `getNodeReadyCondition` helper is duplicated locally since it lives in a different package (`core/eventhandlers`) and cannot be imported cross-package.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Self-Check: PASSED

All created files exist, all task commits verified.

## Next Phase Readiness

- VGLB controller now reacts to both Service and Node events
- Controller re-reconciles appropriate VGLB resources on any meaningful infrastructure change
- Ready for Phase 06 Plan 03 if it exists, or for integration testing

---
*Phase: 06-add-vglb-operator-generate-glbc-from-vglb*
*Completed: 2026-03-16*

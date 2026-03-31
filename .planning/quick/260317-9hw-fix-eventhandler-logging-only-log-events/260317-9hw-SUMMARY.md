---
phase: quick
plan: 260317-9hw
subsystem: eventhandlers
tags: [logging, event-handlers, noise-reduction, controllers]
key-files:
  modified:
    - internal/controller/service_glb_controller/eventhandlers/service_events.go
    - internal/controller/vglb_controller/eventhandlers/service_events.go
    - internal/controller/vglb_controller/eventhandlers/vglb_events.go
    - internal/controller/core/eventhandlers/service_events.go
    - internal/controller/core/eventhandlers/lbc_events.go
    - internal/controller/core/eventhandlers/endpoint_events.go
    - internal/controller/networking/eventhandlers/ingress_events.go
    - internal/controller/networking/eventhandlers/lbc_events.go
    - internal/controller/networking/eventhandlers/endpoints_events.go
    - internal/controller/networking/eventhandlers/secret_events.go
    - internal/controller/networking/eventhandlers/service_events.go
    - internal/controller/nsg_controller/eventhandlers/nsg_events.go
    - internal/controller/lbc_controller/eventhandlers/lbc_events.go
    - internal/controller/glbc_controller/eventhandlers/glbc_events.go
    - internal/controller/service_glb_controller/eventhandlers/node_events.go
    - internal/controller/vglb_controller/eventhandlers/node_events.go
    - internal/controller/core/eventhandlers/node_events.go
    - internal/controller/networking/eventhandlers/node_events.go
    - internal/controller/nsg_controller/eventhandlers/node_events.go
decisions:
  - "Log moved inside enqueue helper (after filter), not inside Create/Update/Delete directly"
  - "Node event handlers: log added inside per-item loop after filter check, not once at handler entry"
  - "Update handlers in service_glb already had inline filter before queue.Add — log placed inside if-block just before queue.Add"
metrics:
  duration: "8min"
  completed: 2026-03-17
  tasks_completed: 2
  files_modified: 19
---

# Quick Task 260317-9hw: Fix EventHandler Logging — Only Log Events That Get Enqueued

**One-liner:** Moved all event handler log lines from Create/Update/Delete entry points to inside enqueue helpers, after filter checks, eliminating log noise for dropped events across 19 files in 8 controllers.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Fix primary resource event handlers — move log after filter, standardize format | 758e60b | 14 files |
| 2 | Fix node event handlers — move log inside enqueue loop after per-item filter | 758e60b | 5 files |

## What Changed

### Pattern Applied (Primary Resource Handlers)

Before:
```go
func (h *...) Create(ctx context.Context, e event.CreateEvent, queue ...) {
    h.logger.V(1).Info("Create Service", ...)  // logged BEFORE filter
    h.enqueueManagedService(ctx, queue, e.Object.(*corev1.Service))
}

func (h *...) enqueueManagedService(_, queue ..., svc *corev1.Service) {
    if !h.serviceUtils.IsServiceSupported(svc) { return }  // silent drop
    queue.Add(...)
}
```

After:
```go
func (h *...) Create(ctx context.Context, e event.CreateEvent, queue ...) {
    h.enqueueManagedService(ctx, queue, e.Object.(*corev1.Service))
}

func (h *...) enqueueManagedService(_, queue ..., svc *corev1.Service) {
    if !h.serviceUtils.IsServiceSupported(svc) { return }  // silent drop — no log
    h.logger.V(1).Info("Enqueue Service", "namespace", svc.Namespace, "name", svc.Name)
    queue.Add(...)
}
```

### Pattern Applied (Node Event Handlers)

Before: single log at handler entry before calling enqueueAll* loop.
After: no log at entry; V(1).Info("Enqueue {Type}", ...) inside loop after per-item filter passes.

### Files Changed by Controller

| Controller | Files |
|-----------|-------|
| service_glb_controller | service_events.go, node_events.go |
| vglb_controller | service_events.go, vglb_events.go, node_events.go |
| core | service_events.go, lbc_events.go, endpoint_events.go, node_events.go |
| networking | ingress_events.go, lbc_events.go, endpoints_events.go, secret_events.go, service_events.go, node_events.go |
| nsg_controller | nsg_events.go, node_events.go |
| lbc_controller | lbc_events.go |
| glbc_controller | glbc_events.go |

## Deviations from Plan

None — plan executed exactly as written.

## Verification Results

- `go build ./...`: PASSED
- `go vet ./internal/controller/...`: PASSED
- Pre-filter log lines in Create/Update/Delete: 0 matches
- All log lines use V(1).Info("Enqueue ...") format: 20 matches across 19 files
- No bare `.logger.Info(` calls remaining in event handler files

## Self-Check: PASSED

Files exist:
- internal/controller/service_glb_controller/eventhandlers/service_events.go: FOUND
- internal/controller/core/eventhandlers/node_events.go: FOUND
- internal/controller/networking/eventhandlers/node_events.go: FOUND

Commit 758e60b: FOUND

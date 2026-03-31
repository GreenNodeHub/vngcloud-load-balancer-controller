---
phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
plan: 01
subsystem: api
tags: [glb, service, usecase, annotations, globalloadbalancerconfig]

# Dependency graph
requires:
  - phase: 06-add-vglb-operator-generate-glbc-from-vglb
    provides: "GLBC CRD, vglb_uc patterns for Init/Ensure/Delete and GLBC builder"
provides:
  - "ServiceGLBFinalizer, GLB_ANNOTATION_PREFIX, KindService constants in domain.go"
  - "SuffixGLBEnable annotation constant (glb.vks.vngcloud.vn/enable)"
  - "ServiceGLBUtils interface and implementation (pkg/service_glb)"
  - "ServiceGLBUseCase interface in contracts.go"
  - "serviceGLBUseCase with Init/Ensure/Delete in service_glb_uc package"
  - "GLBC builder using KindService owner labels and Service annotations"
  - "Pool builder with ClusterIP->TargetTypeIP forced, deterministic sort"
  - "Listener builder with genListenerName and TCP-only protocol"
affects:
  - "07-02 — controller that wires ServiceGLBUseCase to reconciler"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Annotation-driven GLB enablement via glb.vks.vngcloud.vn/enable=true"
    - "Owner labels on GLBC use KindService constant (not svc.Kind) — K8s strips TypeMeta"
    - "Network info (region, networkId, subnetId) read from node labels at Init time"
    - "ClusterIP services force TargetTypeIP in getTargetType"
    - "All annotation reads use t.service.Annotations (glb.vks.vngcloud.vn prefix)"

key-files:
  created:
    - internal/usecase/service_glb_uc/service_glb_uc.go
    - internal/usecase/service_glb_uc/build_glbc.go
    - internal/usecase/service_glb_uc/build_pool.go
    - internal/usecase/service_glb_uc/build_listener.go
    - pkg/service_glb/service_glb_utils.go
  modified:
    - internal/domain/domain.go
    - pkg/annotations/constants.go
    - internal/usecase/contracts.go

key-decisions:
  - "ServiceGLBUtils does NOT check ServiceType — any service type can be GLB-enabled via annotation"
  - "GLBC owner label uses domain.KindService constant (not svc.Kind) for label survival through API server round-trips"
  - "Pool member group name is {region}-{networkId}, matching vglb_uc pattern"
  - "Service GLB annotation prefix is glb.vks.vngcloud.vn (distinct from vks.vngcloud.vn used by vglb/service)"

patterns-established:
  - "pkg/service_glb pattern: mirrors pkg/service with GLB-specific enable annotation check"
  - "service_glb_uc pattern: mirrors vglb_uc with service as root object instead of VGLB CRD"

requirements-completed: [SGLB-01, SGLB-02, SGLB-03, SGLB-04]

# Metrics
duration: 5min
completed: 2026-03-17
---

# Phase 7 Plan 01: Service GLB Foundation Summary

**Service-annotation-based GLB usecase layer: ServiceGLBUtils, ServiceGLBUseCase interface, and full Init/Ensure/Delete implementation with GLBC/pool/listener builders using glb.vks.vngcloud.vn annotation prefix**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-17T03:43:22Z
- **Completed:** 2026-03-17T03:48:01Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Added `ServiceGLBFinalizer`, `GLB_ANNOTATION_PREFIX`, and `KindService` constants to domain.go
- Created `pkg/service_glb` package with `ServiceGLBUtils` interface that enables any service type (not just LoadBalancer) via the `glb.vks.vngcloud.vn/enable=true` annotation
- Created complete `service_glb_uc` package (4 files) implementing `ServiceGLBUseCase` with the same Init/Ensure/Delete pattern as vglb_uc, adapted to use the Service as root object and read annotations from `t.service.Annotations`

## Task Commits

Each task was committed atomically:

1. **Task 1: Add domain constants, annotation constants, and ServiceGLBUtils package** - `7bcccb2` (feat)
2. **Task 2: Create ServiceGLBUseCase interface and full usecase implementation** - `b825de6` (feat)

**Plan metadata:** (pending)

## Files Created/Modified
- `internal/domain/domain.go` - Added ServiceGLBFinalizer, GLB_ANNOTATION_PREFIX, KindService constants
- `pkg/annotations/constants.go` - Added SuffixGLBEnable = "enable"
- `pkg/service_glb/service_glb_utils.go` - ServiceGLBUtils interface and implementation
- `internal/usecase/contracts.go` - Added ServiceGLBUseCase interface
- `internal/usecase/service_glb_uc/service_glb_uc.go` - Main usecase struct with Init/Ensure/Delete
- `internal/usecase/service_glb_uc/build_glbc.go` - GLBC builder with KindService owner labels
- `internal/usecase/service_glb_uc/build_pool.go` - Pool builder with ClusterIP->IP forced, deterministic sort
- `internal/usecase/service_glb_uc/build_listener.go` - Listener builder (genListenerName, TCP-only)

## Decisions Made
- `ServiceGLBUtils.IsServiceGLBSupported` does NOT filter on ServiceType — any service type can be GLB-enabled via annotation (unlike `ServiceUtils` which checks for LoadBalancer/NodePort/ClusterIP with enable-lb)
- GLBC owner label uses `domain.KindService` constant, not `svc.Kind`, because K8s strips TypeMeta from objects returned by Get — using the constant ensures label values survive round-trips
- `getTargetType` in pool builder forces `TargetTypeIP` when `t.service.Spec.Type == corev1.ServiceTypeClusterIP`
- All annotation parsing reads from `t.service.Annotations` using the parser configured with `glb.vks.vngcloud.vn` prefix (distinct from `vks.vngcloud.vn` used by service/vglb controllers)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 8 files exist and compile (`go build ./...` and `go vet` pass)
- ServiceGLBUseCase interface ready for the controller (Plan 02) to wire up
- No blockers

---
*Phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn*
*Completed: 2026-03-17*

## Self-Check: PASSED

All 8 files exist. Commits 7bcccb2 and b825de6 verified. `go build ./...` exits 0.

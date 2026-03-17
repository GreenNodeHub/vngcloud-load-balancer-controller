---
phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
verified: 2026-03-17T04:30:00Z
status: passed
score: 11/11 must-haves verified
re_verification: null
gaps: []
human_verification: []
---

# Phase 7: Service GLB Operator Verification Report

**Phase Goal:** Create a new Service-annotation-based GLB controller that watches Services with glb.vks.vngcloud.vn/enable=true annotation and creates/manages GlobalLoadBalancerConfig (GLBC) resources, following the core/service_controller pattern. Independent of VGLB. Includes integration tests.
**Verified:** 2026-03-17T04:30:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| #  | Truth | Status | Evidence |
|----|-------|--------|----------|
| 1  | GLB annotation prefix constant and ServiceGLB finalizer constant exist in domain.go | VERIFIED | `domain.go:20` `ServiceGLBFinalizer = "glb.vks.vngcloud.vn/resources"`, `domain.go:28` `GLB_ANNOTATION_PREFIX = "glb.vks.vngcloud.vn"`, `domain.go:35` `KindService = "Service"` |
| 2  | ServiceGLBUtils correctly identifies Services with glb.vks.vngcloud.vn/enable=true annotation | VERIFIED | `service_glb_utils.go` implements `IsServiceGLBSupported` checking `ParseBoolAnnotation(annotations.SuffixGLBEnable, ...)`, DeletionTimestamp guard present |
| 3  | ServiceGLBUseCase interface is defined in contracts.go | VERIFIED | `contracts.go:47-51` defines `InitServiceGLBUseCase`, `EnsureServiceGLBUseCase`, `DeleteServiceGLBUseCase` |
| 4  | Usecase Init reads node labels for region/VPC/subnet info | VERIFIED | `service_glb_uc.go:70-99` reads `labelMgmtZone`, `labelNetworkId`, `labelSubnetId` from first node, calls `stripZoneSuffix` |
| 5  | Usecase Ensure creates GLBC with correct owner labels (Kind=Service), pools, and listeners from Service ports | VERIFIED | `build_glbc.go:106` `LabelOwnerResourceKind: domain.KindService`, `build_glbc.go:93` `GenerateName: t.service.Name + "-"`, pools/listeners built from `t.service.Spec.Ports` |
| 6  | Usecase Delete finds and deletes GLBCs owned by the Service | VERIFIED | `service_glb_uc.go:154-189` lists by `LabelOwnerResourceKind: domain.KindService` + UID, deletes each, returns still-exist list with `RequeueNeededAfter` |
| 7  | ClusterIP services force target-type=ip in getTargetType | VERIFIED | `build_pool.go:185-189` `if t.service.Spec.Type == corev1.ServiceTypeClusterIP { return domain.TargetTypeIP }` |
| 8  | ServiceGLBReconciler reconciles Services with glb.vks.vngcloud.vn/enable=true | VERIFIED | `service_glb_controller.go:83-100` `ServiceGLBReconciler` struct; `reconcile()` at line 117 checks `IsServiceGLBSupported`, delegates to `reconcileEnsure`/`reconcileDelete` |
| 9  | Removing the enable annotation triggers GLBC deletion via finalizer cleanup | VERIFIED | `service_events.go:95-99` `hadGLBAnnotation` detects annotation presence on old object; Update handler enqueues if `hadGLBAnnotation(oldSvc)` even when new state is not GLB-supported |
| 10 | Node changes trigger reconciliation of all GLB-annotated Services | VERIFIED | `node_events.go:72-92` `enqueueAllGLBServices` lists all Services cluster-wide and filters by `IsServiceGLBSupported`/`IsServiceGLBPendingFinalization` |
| 11 | Integration tests pass: GLBC create/update/delete lifecycle driven by Service annotations | VERIFIED | `go test ./internal/controller/service_glb_controller/... -timeout 120s` exits 0; 3/3 specs pass in 27.06s |

**Score:** 11/11 truths verified

---

## Required Artifacts

| Artifact | Provides | Status | Evidence |
|----------|----------|--------|----------|
| `internal/domain/domain.go` | `GLB_ANNOTATION_PREFIX`, `ServiceGLBFinalizer`, `KindService` constants | VERIFIED | Lines 20, 28, 35 |
| `pkg/annotations/constants.go` | `SuffixGLBEnable` constant | VERIFIED | Line 70: `SuffixGLBEnable = "enable"` |
| `pkg/service_glb/service_glb_utils.go` | `ServiceGLBUtils` interface and implementation | VERIFIED | Full interface + `defaultServiceGLBUtils` impl, `NewServiceGLBUtils` constructor |
| `internal/usecase/contracts.go` | `ServiceGLBUseCase` interface | VERIFIED | Lines 47-51 |
| `internal/usecase/service_glb_uc/service_glb_uc.go` | `serviceGLBUseCase` struct with Init/Ensure/Delete | VERIFIED | Full 190-line implementation; `NewServiceGLBUseCase` constructor |
| `internal/usecase/service_glb_uc/build_glbc.go` | GLBC spec builder with owner labels and annotation parsing | VERIFIED | `buildGlobalLoadBalancerConfig`, `glbcSpecEqual`, `UpdateServiceStatusAddress` call |
| `internal/usecase/service_glb_uc/build_pool.go` | Pool builder with member resolution and sorting | VERIFIED | `buildPool`, `sort.Slice` on pools/listeners/members, `ServiceTypeClusterIP` check, `t.service.Annotations` reads |
| `internal/usecase/service_glb_uc/build_listener.go` | Listener builder with naming and protocol | VERIFIED | `genListenerName`, `getListenerProtocol` (TCP only) |
| `internal/controller/service_glb_controller/service_glb_controller.go` | `ServiceGLBReconciler` with Reconcile/Ensure/Delete/SetupWithManager | VERIFIED | Named("service-glb"), Watches Service+Node, `InitServiceGLBUseCase` in runnable |
| `internal/controller/service_glb_controller/eventhandlers/service_events.go` | Service event handler with annotation-removal detection | VERIFIED | `hadGLBAnnotation`, `IsServiceGLBPendingFinalization`, `IsServiceGLBSupported` |
| `internal/controller/service_glb_controller/eventhandlers/node_events.go` | Node event handler enqueuing GLB-annotated Services | VERIFIED | `enqueueAllGLBServices`, `corev1.ServiceList` |
| `cmd/main.go` | ServiceGLB controller registration block with disable flag | VERIFIED | Lines 109, 136, 419-425: `disableServiceGLBController`, `"disable-service-glb-controller"`, `service_glb_controller.NewServiceGLBReconciler` |
| `internal/controller/service_glb_controller/suite_test.go` | Envtest setup with ServiceGLB + GLBC controllers | VERIFIED | `NewServiceGLBReconciler`, `glbc_controller.NewGlobalLoadBalancerConfigReconciler`, test node with `Status().Update` |
| `internal/controller/service_glb_controller/helpers_test.go` | `newServiceWithGLBAnnotation`, `findGLBCByServiceOwnerLabels` | VERIFIED | Both helpers present with `glb.vks.vngcloud.vn/enable`, `domain.KindService` |
| `internal/controller/service_glb_controller/service_glb_controller_test.go` | Integration tests: create/update/delete | VERIFIED | "Should create GLBC", "Should update GLBC", "Should delete GLBC" — all pass |

---

## Key Link Verification

| From | To | Via | Status | Evidence |
|------|----|-----|--------|----------|
| `pkg/service_glb/service_glb_utils.go` | `pkg/annotations/constants.go` | `SuffixGLBEnable` constant | WIRED | Line 48: `ParseBoolAnnotation(annotations.SuffixGLBEnable, ...)` |
| `internal/usecase/service_glb_uc/build_glbc.go` | `internal/domain/domain.go` | `domain.LabelOwnerResource*` constants | WIRED | Lines 69-71, 105-107: `domain.LabelOwnerResourceName/Kind/Uid` |
| `internal/usecase/service_glb_uc/build_pool.go` | `pkg/utils` | `endpointResolver.ResolveNodePortEndpoints` / `ResolvePodEndpoints` | WIRED | Lines 79-92: actual resolve calls with result assigned to `membersAddr` |
| `internal/controller/service_glb_controller/service_glb_controller.go` | `internal/usecase/service_glb_uc` | `serviceGLBUseCase.EnsureServiceGLBUseCase` / `DeleteServiceGLBUseCase` | WIRED | Lines 169, 187: direct method calls on `r.serviceGLBUseCase` |
| `internal/controller/service_glb_controller/eventhandlers/service_events.go` | `pkg/service_glb` | `serviceGLBUtils.IsServiceGLBSupported` / `IsServiceGLBPendingFinalization` | WIRED | Lines 65-66, 82 |
| `cmd/main.go` | `internal/controller/service_glb_controller` | `service_glb_controller.NewServiceGLBReconciler` | WIRED | Line 425: actual constructor call with all dependencies |

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SGLB-01 | 07-01 | Domain constants (GLB_ANNOTATION_PREFIX, ServiceGLBFinalizer, KindService) and annotation constant (SuffixGLBEnable) added | SATISFIED | `domain.go` lines 20/28/35; `annotations/constants.go` line 70 |
| SGLB-02 | 07-01 | ServiceGLBUtils package with IsServiceGLBSupported and IsServiceGLBPendingFinalization | SATISFIED | `pkg/service_glb/service_glb_utils.go` — full interface + implementation |
| SGLB-03 | 07-01 | ServiceGLBUseCase interface defined in contracts.go with Init/Ensure/Delete methods | SATISFIED | `contracts.go:47-51` |
| SGLB-04 | 07-01 | Usecase implementation: Init reads node labels, Ensure creates/patches GLBC with owner labels (Kind=Service), Delete removes owned GLBCs. Pool builder forces TargetTypeIP for ClusterIP. Deterministic sort. | SATISFIED | `service_glb_uc.go`, `build_glbc.go`, `build_pool.go` — all behaviors verified |
| SGLB-05 | 07-02 | ServiceGLBReconciler controller with Service+Node watches, annotation removal detection, enqueueAllGLBServices | SATISFIED | `service_glb_controller.go`, `eventhandlers/service_events.go`, `eventhandlers/node_events.go` |
| SGLB-06 | 07-02 | Controller registered in cmd/main.go with --disable-service-glb-controller flag | SATISFIED | `cmd/main.go` lines 109, 136, 419-435 |
| SGLB-07 | 07-03 | Integration tests: Service annotation creates GLBC, port change updates GLBC spec, annotation removal deletes GLBC | SATISFIED | 3/3 tests pass in `go test ./internal/controller/service_glb_controller/... -timeout 120s` (27.06s) |

All 7 requirements satisfied. No orphaned requirements.

---

## Anti-Patterns Found

None detected. No TODO/FIXME/placeholder comments, no empty handlers, no stub return values in any of the 11 files created in this phase.

---

## Human Verification Required

None. All behaviors are covered programmatically by the integration test suite:
- GLBC creation from annotated Service: verified by test "Should create GLBC"
- GLBC spec update on port change: verified by test "Should update GLBC spec when Service ports change"
- GLBC deletion on annotation removal + finalizer cleanup: verified by test "Should delete GLBC when enable annotation is removed"
- Node IP appearing in pool members: verified by `ContainElement("10.0.0.1")` assertion in create test

---

## Summary

Phase 7 goal is fully achieved. The Service-annotation-based GLB controller is implemented end-to-end across three plans:

**Plan 01 (foundation):** 8 files — domain constants, annotation constant, `ServiceGLBUtils`, `ServiceGLBUseCase` interface, and complete usecase implementation with GLBC/pool/listener builders. Annotation prefix (`glb.vks.vngcloud.vn`) is correctly distinct from the VGLB prefix (`vks.vngcloud.vn`). Owner labels use the `KindService` constant (not `svc.Kind`) for round-trip safety. ClusterIP services correctly force `TargetTypeIP`.

**Plan 02 (controller):** `ServiceGLBReconciler` watches Service+Node resources, follows `core/service_controller.go` pattern. Service event handler correctly detects annotation removal via `hadGLBAnnotation(oldSvc)` — this is the critical wiring that triggers cleanup when the user removes the enable annotation. Controller is registered in `cmd/main.go` with `--disable-service-glb-controller` flag.

**Plan 03 (tests):** 3 envtest integration tests using a shared manager with both ServiceGLB and GLBC controllers registered. All 3 tests pass in 27 seconds. Unique NodePort values per test (30080/30180/30280) avoid cluster-wide port conflicts.

`go build ./...` and `go vet` both exit 0. All 6 commits (7bcccb2, b825de6, 6c0b566, d874fed, 12118fe, a60c369) verified in git history.

---

_Verified: 2026-03-17T04:30:00Z_
_Verifier: Claude (gsd-verifier)_

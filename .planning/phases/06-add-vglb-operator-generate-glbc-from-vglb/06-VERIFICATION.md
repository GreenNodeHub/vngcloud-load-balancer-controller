---
phase: 06-add-vglb-operator-generate-glbc-from-vglb
verified: 2026-03-16T16:45:00Z
status: passed
score: 14/14 must-haves verified
re_verification: false
---

# Phase 6: Add VGLB Operator - Generate GLBC From VGLB Verification Report

**Phase Goal:** VGLB operator correctly generates GLBC resources from matching Services using node-label-derived network info, with Service and Node watches for reactivity, and integration tests verifying the full pipeline
**Verified:** 2026-03-16T16:45:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Init reads network info from node labels, not VNG Cloud API | VERIFIED | `InitVngcloudGlobalLoadBalancerUseCase` reads `labelMgmtZone`, `labelNetworkId`, `labelSubnetId` from `nodes.Items[0].Labels`; no `GetServerNetworkInfo` call exists in `vglb_uc.go` |
| 2 | Pool member group name is `{region}-{vpcId}`, not `"default"` | VERIFIED | `build_global_pool.go:128` uses `fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId)` |
| 3 | Pool member group region is derived from node label zone, not hardcoded `"hcm"` | VERIFIED | `stripZoneSuffix("hcm03b") -> "hcm"` via `zoneRe.ReplaceAllString`; integration test confirms `pm.Region == "hcm"` from actual label `"hcm03b"` |
| 4 | GLB default display name uses `"vks_"` prefix, not `"glb_"` | VERIFIED | `build_glbc.go:182` uses `"vks_" + t.vglb.Namespace + "_" + t.vglb.Name`; `build_glbc_test.go` expects `"vks_default_my_vglb"` |
| 5 | Service not found causes requeue, not empty pool generation | VERIFIED | `build_glbc.go:74-75` returns `errs.NewRequeueNeededAfter("service ... not found, waiting", 5*time.Second)` |
| 6 | ClusterIP service type causes requeue, not silent fallback to pod IPs | VERIFIED | `build_glbc.go:81-83` checks `svc.Spec.Type == corev1.ServiceTypeClusterIP` and returns `errs.NewRequeueNeededAfter(...)` |
| 7 | VGLB status address comes from GLBC domains only, not VIPs | VERIFIED | `getGLBCAddress` returns `glbcList.Items[0].Status.Domains[0]` only; no `.Vips` reference exists in `build_glbc.go` |
| 8 | VGLB controller watches Service resources and re-reconciles VGLB with same name+namespace | VERIFIED | `service_events.go` implements `enqueueRequestsForVglbServiceEvent` with `enqueueSameNameVglb`; wired in `SetupWithManager` as `Watches(&corev1.Service{}, svcEventHandler)` |
| 9 | VGLB controller watches Node resources and re-reconciles all VGLBs on meaningful node changes | VERIFIED | `node_events.go` implements `enqueueRequestsForVglbNodeEvent` with `enqueueAllVglbs`; wired in `SetupWithManager` as `Watches(&corev1.Node{}, nodeEventHandler)` |
| 10 | Node heartbeat-only updates (no label/spec/address/condition change) do not trigger reconciliation | VERIFIED | `node_events.go:49-54` skips reconcile when `equality.Semantic.DeepEqual` holds for Labels, Spec, Addresses, and ready condition |
| 11 | `stripZoneSuffix` correctly converts zone formats to region strings | VERIFIED | `TestStripZoneSuffix` passes all 5 variants: `hcm03b->hcm`, `sgn01a->sgn`, `han01->han`, `hcm->hcm`, `""->""`  |
| 12 | Integration test: VGLB create with NodePort Service produces correct GLBC | VERIFIED | Ginkgo test "should create GLBC from VGLB with matching NodePort Service" PASSES; asserts `vks_` prefix, 1 pool, `pm.Name == "hcm-net-test-vpc"`, 1 listener at port 80 |
| 13 | Integration test: VGLB delete causes owned GLBC to be deleted | VERIFIED | Ginkgo test "should delete GLBC when VGLB is deleted" PASSES; asserts GLBC gone after VGLB deletion |
| 14 | Integration test: Service port change triggers GLBC spec update | VERIFIED | Ginkgo test "should update GLBC when Service ports change" PASSES; asserts 2 pools and 2 listeners after adding port 443 |

**Score:** 14/14 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/usecase/vglb_uc/vglb_uc.go` | Node-label-based init, `stripZoneSuffix`, `defaultRegion` field | VERIFIED | Contains `labelMgmtZone`, `labelNetworkId`, `labelSubnetId` constants; `stripZoneSuffix` function; `defaultRegion string` field on `vglbUseCase` struct |
| `internal/usecase/vglb_uc/build_glbc.go` | `vks_` prefix, service-not-found requeue, ClusterIP rejection, domains-only address | VERIFIED | `"vks_"` at line 182; `NewRequeueNeededAfter` for both not-found and ClusterIP; `getGLBCAddress` returns domains only |
| `internal/usecase/vglb_uc/build_global_pool.go` | Dynamic pool member group naming using `defaultRegion` and VPC ID | VERIFIED | `fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId)` at line 128; no hardcoded `"default"` or `"hcm"` |
| `internal/usecase/vglb_uc/build_glbc_test.go` | Updated tests for `vks_` prefix | VERIFIED | All test expectations use `"vks_default_my_vglb"`, `"vks_production_test_vglb"`, etc. |
| `internal/controller/vglb_controller/eventhandlers/node_events.go` | Node event handler that enqueues all VGLBs on meaningful node changes | VERIFIED | `enqueueRequestsForVglbNodeEvent` with `enqueueAllVglbs`; Update filter uses `equality.Semantic.DeepEqual`; `getNodeReadyCondition` helper present |
| `internal/controller/vglb_controller/eventhandlers/service_events.go` | Service event handler that enqueues same-name VGLB | VERIFIED | `enqueueRequestsForVglbServiceEvent` with `enqueueSameNameVglb`; Delete event triggers reconcile |
| `internal/controller/vglb_controller/vglb_controller.go` | `SetupWithManager` wires Service and Node watches | VERIFIED | Lines 229-231: three `Watches(...)` calls for VGLB, Service, and Node; `NewEnqueueRequestForVglbNodeEvent` and `NewEnqueueRequestForVglbServiceEvent` instantiated |
| `internal/usecase/vglb_uc/vglb_uc_test.go` | Unit tests for `stripZoneSuffix` | VERIFIED | `TestStripZoneSuffix` with 5 table-driven cases; all PASS |
| `internal/usecase/vglb_uc/build_global_pool_test.go` | Unit tests for pool member group naming | VERIFIED | `TestBuildPool_PoolMemberGroupNaming` and `TestBuildPoolsAndListeners_1to1Mapping`; both PASS |
| `internal/controller/vglb_controller/suite_test.go` | envtest bootstrap with both VGLB and GLBC reconcilers | VERIFIED | `testEnv`, `NewVngcloudGlobalLoadBalancerReconciler`, and `NewGlobalLoadBalancerConfigReconciler` present; node with `NodeReady=True` created before Init |
| `internal/controller/vglb_controller/helpers_test.go` | Test fixture builders | VERIFIED | `newVGLBResource`, `newNodePortService`, `findGLBCByOwnerLabels` (uses `domain.KindVngcloudGlobalLoadBalancer`) |
| `internal/controller/vglb_controller/vglb_controller_test.go` | Integration test scenarios: create, delete, update | VERIFIED | Three Ginkgo `It(...)` scenarios: "should create GLBC", "should delete GLBC", "should update GLBC"; all 3 PASS |
| `internal/domain/domain.go` | `KindVngcloudGlobalLoadBalancer` constant | VERIFIED | `KindVngcloudGlobalLoadBalancer = "VngcloudGlobalLoadBalancer"` at line 32 |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `vglb_uc.go` | `build_glbc.go` | `defaultRegion` field on both structs | WIRED | `vglbUseCase.ensure()` passes `defaultRegion: uc.defaultRegion` to `defaultModelBuildTask` |
| `vglb_uc.go` | `build_global_pool.go` | `t.defaultRegion` used in pool member group naming | WIRED | `t.defaultRegion` referenced at `build_global_pool.go:129` in `buildPool` |
| `vglb_controller.go` | `node_events.go` | `Watches(&corev1.Node{}, nodeEventHandler)` | WIRED | `nodeEventHandler` instantiated via `NewEnqueueRequestForVglbNodeEvent` at line 217-221; registered at line 231 |
| `vglb_controller.go` | `service_events.go` | `Watches(&corev1.Service{}, svcEventHandler)` | WIRED | `svcEventHandler` instantiated via `NewEnqueueRequestForVglbServiceEvent` at line 222-226; registered at line 230 |
| `suite_test.go` | `vglb_controller.go` | `NewVngcloudGlobalLoadBalancerReconciler` registered with envtest manager | WIRED | Line 176-188: reconciler created and `SetupWithManager` called |
| `suite_test.go` | `glbc_controller.go` | `NewGlobalLoadBalancerConfigReconciler` registered with envtest manager | WIRED | Line 193-204: GLBC reconciler created and `SetupWithManager` called |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| VGLB-01 | 06-01 | Init reads network info from node labels instead of VNG Cloud API | SATISFIED | `InitVngcloudGlobalLoadBalancerUseCase` reads `labelMgmtZone`, `labelNetworkId`, `labelSubnetId`; `GetServerNetworkInfo` absent |
| VGLB-02 | 06-01 | Pool member group name uses `{region}-{vpcId}` format | SATISFIED | `fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId)` in `build_global_pool.go` |
| VGLB-03 | 06-01 | Region derived from zone label by stripping digit+letter suffix | SATISFIED | `stripZoneSuffix` via `zoneRe = regexp.MustCompile(\`\d+[a-z]*$\`)` |
| VGLB-04 | 06-01 | GLB default display name uses `vks_` prefix | SATISFIED | `"vks_" + t.vglb.Namespace + "_" + t.vglb.Name` in `build_glbc.go:182` |
| VGLB-05 | 06-01 | Service not found causes requeue | SATISFIED | `errs.NewRequeueNeededAfter("service ... not found, waiting", 5*time.Second)` in `build_glbc.go` |
| VGLB-06 | 06-01 | ClusterIP service type causes requeue; VGLB status address from domains only | SATISFIED | ClusterIP check with `NewRequeueNeededAfter`; `getGLBCAddress` returns `Status.Domains[0]` only |
| VGLB-07 | 06-02 | VGLB controller watches Service and Node resources via event handlers | SATISFIED | `SetupWithManager` registers both watches; event handlers fully implemented |
| VGLB-08 | 06-03 | Unit tests for `stripZoneSuffix` and pool member group naming | SATISFIED | `TestStripZoneSuffix` (5 variants) and `TestBuildPool_PoolMemberGroupNaming` pass |
| VGLB-09 | 06-04 | Integration test: VGLB create with NodePort Service produces correct GLBC | SATISFIED | "should create GLBC from VGLB with matching NodePort Service" PASSES in envtest |
| VGLB-10 | 06-04 | Integration test: VGLB delete causes owned GLBC deletion | SATISFIED | "should delete GLBC when VGLB is deleted" PASSES in envtest |
| VGLB-11 | 06-04 | Integration test: Service port change triggers GLBC spec update | SATISFIED | "should update GLBC when Service ports change" PASSES in envtest |

**All 11 requirements satisfied. No orphaned requirements.**

---

### Anti-Patterns Found

No blocking anti-patterns found.

Notable observations (informational only):

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `build_global_pool.go:198-204` | `getTargetType` has a `// TODO: store somewhere to avoid parsing again and again` comment | Info | Minor optimization opportunity; does not affect correctness |
| `vglb_controller_test.go:166` | Delete flow checks GLBC list by raw label string `"vks.vngcloud.vn/owner-resource-name"` rather than `domain.LabelOwnerResourceName` constant | Info | Works correctly but inconsistent with constant usage elsewhere in the file |

---

### Human Verification Required

None — all truths are fully verifiable programmatically.

For completeness, the following would benefit from manual inspection in a real cluster environment:

**1. End-to-end VNG Cloud API integration**

**Test:** Deploy VGLB operator against a real VKS cluster with live VNG Cloud credentials; create a NodePort Service + VGLB resource
**Expected:** GLB object created in VNG Cloud API with the correct name prefix, pool member groups showing actual node IPs, and VGLB status.address populated with the GLB domain
**Why human:** Mock provider is used in envtest; real API validation requires live credentials and network access

---

### Build and Test Results

```
go build ./internal/usecase/vglb_uc/...          -> PASS (no errors)
go build ./internal/controller/vglb_controller/... -> PASS (no errors)
go test ./internal/usecase/vglb_uc/... -run "TestStripZoneSuffix|TestBuildPool|TestBuildLoadBalancerName"
  -> PASS: TestBuildLoadBalancerName (4 subtests)
  -> PASS: TestBuildPool_PoolMemberGroupNaming
  -> PASS: TestBuildPoolsAndListeners_1to1Mapping
  -> PASS: TestStripZoneSuffix (5 subtests)
go test ./internal/controller/vglb_controller/... -timeout 300s
  -> Ran 3 of 3 Specs in 30.309 seconds
  -> SUCCESS! -- 3 Passed | 0 Failed | 0 Pending | 0 Skipped
```

---

## Summary

Phase 6 goal is fully achieved. The VGLB operator:

1. **Correctly generates GLBC resources** from matching Services using node-label-derived network info (`vks.vngcloud.vn/mgmt-zone`, `network-id`, `subnet-id`), with the `vks_` name prefix and dynamic `{region}-{vpcId}` pool member group naming.

2. **Handles edge cases correctly** — ClusterIP and missing Service both trigger requeue rather than silent failures; VGLB status address comes from GLBC domains only.

3. **Is reactive** — Service and Node watches are wired into `SetupWithManager` with filtering to avoid unnecessary reconciles on node heartbeats.

4. **Has comprehensive test coverage** — unit tests for `stripZoneSuffix` and pool member group naming, plus 3 envtest integration tests covering the full create/delete/update pipeline. All tests pass.

A production bug fixed during implementation (Plan 04): `obj.Kind` is always empty after K8s API round-trips; the `domain.KindVngcloudGlobalLoadBalancer` constant is now used in all owner label selectors, preventing infinite GLBC re-creation loops.

---

_Verified: 2026-03-16T16:45:00Z_
_Verifier: Claude (gsd-verifier)_

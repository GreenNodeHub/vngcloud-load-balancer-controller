---
phase: 04-add-test-from-controller-use-vngcloud-mock-repository
plan: "02"
subsystem: glbc-controller-tests
tags: [testing, integration-test, envtest, ginkgo, glbc]
dependency_graph:
  requires: ["04-01"]
  provides: ["CTRL-TEST-03", "CTRL-TEST-04", "CTRL-TEST-05"]
  affects: ["internal/controller/glbc_controller", "internal/usecase/glbc_uc"]
tech_stack:
  added: []
  patterns:
    - "envtest + Ginkgo v2 integration test suite for GLBC controller"
    - "MockProvider backend state verification in controller tests"
key_files:
  created:
    - internal/controller/glbc_controller/suite_test.go
    - internal/controller/glbc_controller/helpers_test.go
    - internal/controller/glbc_controller/glbc_controller_test.go
  modified:
    - internal/usecase/glbc_uc/deploy_pool_member.go
    - internal/usecase/glbc_uc/status.go
    - internal/usecase/glbc_uc/deploy_listener.go
decisions:
  - "statusAddListener signature extended to include name parameter — CRD requires name field in createdListeners list-map"
  - "NewPatchGlobalPoolUpdateBulkActionRequest receives pool member group ID (currentPoolMember.ID), not pool ID"
metrics:
  duration: "26 minutes"
  completed: "2026-03-16"
  tasks_completed: 2
  files_created: 3
  files_modified: 3
---

# Phase 04 Plan 02: GLBC Controller Integration Tests Summary

**One-liner:** envtest + Ginkgo integration test suite for GLBC controller covering create, full-delete, and partial shared-LB delete flows via MockProvider backend.

## What Was Built

Created a complete Ginkgo v2 integration test suite for the `GlobalLoadBalancerConfig` controller in `internal/controller/glbc_controller/`:

1. **suite_test.go** — envtest bootstrap with GLBC controller wiring. Uses `GlobalLoadBalancerOpts` with `glb-small` package name (matching MockProvider's `ListGlobalPackages` return value). Wires `NewGlobalLoadBalancerConfigReconciler` without `maxConcurrentReconciles` param (unlike NSG).

2. **helpers_test.go** — Clean-state helpers and resource factories:
   - `expectNoGLBs()` — polls MockProvider until all GLBs are gone
   - `expectNoGLBCObjects()` — polls K8s until all GLBC CRs are gone
   - `newGLBCResource()` — creates GLBC using `MockGLBCMinimalSpec()`
   - `newGLBCSharedResource()` — creates GLBC using `MockGLBCSharedSpec()` with shared LB ID

3. **glbc_controller_test.go** — Three integration test scenarios:
   - **Create flow**: Creates GLBC, verifies finalizer added, status populated (LB ID, 1 pool "test-pool", 1 listener "test-listener"), Ready condition True, mock backend has the LB
   - **Delete flow (full)**: Creates GLBC, waits for status, deletes it, verifies K8s object gone and mock backend LB deleted
   - **Delete flow (partial)**: Creates GLBC-A (owns LB), creates GLBC-B (shares LB via `LoadBalancerId`), deletes GLBC-A, verifies LB still exists, GLBC-B's pool/listener preserved, GLBC-A's resources deleted

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed pool member update uses wrong ID in PatchGlobalPoolUpdateBulkActionRequest**
- **Found during:** Task 2 — test "Create flow" failed with "heheh not found" error in PatchGlobalPoolMembers
- **Issue:** `buildPatchGlobalPoolMemberRequest` passed `poolId` (pool ID, e.g. `gpool-xxx`) as the first argument to `NewPatchGlobalPoolUpdateBulkActionRequest`, but the mock reads `rawAction.ID` to find the pool **member group** (which has ID like `gpoolmem-xxx`). This caused every update attempt to fail with "pool member not found".
- **Fix:** Changed `global.NewPatchGlobalPoolUpdateBulkActionRequest(poolId, updateRequest)` to `global.NewPatchGlobalPoolUpdateBulkActionRequest(currentPoolMember.ID, updateRequest)`
- **Files modified:** `internal/usecase/glbc_uc/deploy_pool_member.go`
- **Commit:** fed67c1

**2. [Rule 1 - Bug] Fixed statusAddListener missing name field causing CRD validation failure**
- **Found during:** Task 2 — test "Delete flow (partial)" failed with validation error `status.createdListeners[0].name: Required value`
- **Issue:** `statusAddListener` only stored `Id` and `Port` but the CRD schema marks `name` as required (`+listType=map +listMapKey=id` with `required: [id, name, port]`). When GLBC-B's status was patched with a listener missing the `name` field, the API server rejected the patch.
- **Fix:** Extended `statusAddListener(ctx, listenerId, port)` to `statusAddListener(ctx, listenerId, port, name)`. Updated both call sites in `deploy_listener.go` to pass `listenerSpec.Name` (on create) and `currentListener.Name` (on update).
- **Files modified:** `internal/usecase/glbc_uc/status.go`, `internal/usecase/glbc_uc/deploy_listener.go`
- **Commit:** fed67c1

## Test Results

```
Ran 3 of 3 Specs in 20.179 seconds
SUCCESS! -- 3 Passed | 0 Failed | 0 Pending | 0 Skipped
--- PASS: TestGLBCController (20.18s)
```

NSG controller tests unaffected (2/2 pass).

Note: Running `./internal/controller/...` concurrently fails due to multiple envtest instances contending for the same binary assets — this is a pre-existing environment constraint, not caused by this plan's changes. Each suite passes in isolation as specified by the plan verification commands.

## Self-Check: PASSED

Files verified:
- FOUND: internal/controller/glbc_controller/suite_test.go
- FOUND: internal/controller/glbc_controller/helpers_test.go
- FOUND: internal/controller/glbc_controller/glbc_controller_test.go

Commits verified:
- 3d200f7: feat(04-02): add GLBC controller suite_test.go and helpers_test.go
- fed67c1: feat(04-02): add GLBC controller test scenarios (create, delete-full, delete-partial)

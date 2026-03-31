---
phase: 04-add-test-from-controller-use-vngcloud-mock-repository
plan: "01"
subsystem: vngcloud_mocks
tags: [mock, glb, testing, fixtures]
dependency_graph:
  requires: []
  provides:
    - UpdateGlobalPool implementation in MockProvider
    - UpdateGlobalListener implementation in MockProvider
    - MockGLBCMinimalSpec fixture function
    - MockGLBCSharedSpec fixture function
  affects:
    - internal/controller/glbc_controller (controller integration tests)
tech_stack:
  added: []
  patterns:
    - Lock/unlock/updatingGlobalStatus pattern for deadlock-safe mock mutations
    - Fixture functions returning typed specs for reuse across tests
key_files:
  modified:
    - internal/repository/vngcloud_repo/vngcloud_mocks/vngcloud_mock_glb.go
  created:
    - internal/repository/vngcloud_repo/vngcloud_mocks/mock_glbc_fixtures.go
decisions:
  - UpdateGlobalPool and UpdateGlobalListener follow the same lock/unlock/updatingGlobalStatus/readyGlobalAfterTime pattern as PatchGlobalPoolMembers to prevent deadlocks
  - MockGLBCSharedSpec uses port 8080 for listener to prevent port conflicts with MockGLBCMinimalSpec (port 80)
  - Return domain.ErrorNotFound (not error) when pool/listener not found, matching other Delete methods
metrics:
  duration: ~5 minutes
  completed_date: "2026-03-16"
  tasks_completed: 2
  files_changed: 2
---

# Phase 4 Plan 01: MockProvider UpdateGlobal* + GLBC Fixtures Summary

MockProvider extended with UpdateGlobalPool/UpdateGlobalListener in-memory implementations and two reusable GLBC spec fixture functions (MockGLBCMinimalSpec, MockGLBCSharedSpec).

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Implement UpdateGlobalPool and UpdateGlobalListener in MockProvider | be98d55 | vngcloud_mock_glb.go |
| 2 | Create GLBC test fixture specs in mock_glbc_fixtures.go | 291d596 | mock_glbc_fixtures.go (new) |

## What Was Built

### Task 1: UpdateGlobalPool and UpdateGlobalListener

Replaced two `ErrorNotImplemented` stubs in `vngcloud_mock_glb.go` with real in-memory implementations.

**UpdateGlobalPool:**
- Type-asserts `opt` to `*global.UpdateGlobalPoolRequest`
- Acquires `m.mu`, finds pool by `lbID + poolID`, updates `Algorithm` and all health monitor fields
- Releases lock, then calls `updatingGlobalStatus(glbID)` and `go readyGlobalAfterTime(glbID)`
- Returns `domain.ErrorNotFound` if pool not found

**UpdateGlobalListener:**
- Type-asserts `opt` to `*global.UpdateGlobalListenerRequest`
- Acquires `m.mu`, finds listener by `lbID + listenerID`, updates `AllowedCidrs`, timeouts, `GlobalPoolID`, `Headers`
- Releases lock, then calls `updatingGlobalStatus(glbID)` and `go readyGlobalAfterTime(glbID)`
- Returns `domain.ErrorNotFound` if listener not found

Both methods call `updatingGlobalStatus` AFTER releasing the lock — critical for deadlock prevention since `updatingGlobalStatus` re-acquires `m.mu` when `WaitAfterTime > 0`.

### Task 2: GLBC Test Fixture Specs

Created `mock_glbc_fixtures.go` with two exported factory functions:

**MockGLBCMinimalSpec():** Returns a spec with:
- Pool: `test-pool` (TCP), member group `test-pool-member-group`, member `test-member` at `10.0.0.1:8080`
- Listener: `test-listener` on port 80, DefaultPoolName `test-pool`

**MockGLBCSharedSpec():** Returns a spec with:
- Pool: `test-pool-shared` (TCP), member group `shared-pool-member-group`, member `shared-member` at `10.0.0.2:9090`
- Listener: `test-listener-shared` on port 8080, DefaultPoolName `test-pool-shared`

Both use `MockNetID` and `MockSubnetID` constants from `mocks.go`.

## Verification

- `go build ./internal/repository/vngcloud_repo/vngcloud_mocks/...` passes
- No `ErrorNotImplemented` remains in `vngcloud_mock_glb.go`
- `go test ./internal/controller/nsg_controller/... -count=1` passes (2 of 2 specs, no regressions)

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check: PASSED

- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/repository/vngcloud_repo/vngcloud_mocks/vngcloud_mock_glb.go` — FOUND (modified)
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/repository/vngcloud_repo/vngcloud_mocks/mock_glbc_fixtures.go` — FOUND (created)
- Commit be98d55 — FOUND (Task 1)
- Commit 291d596 — FOUND (Task 2)

---
phase: 4
slug: add-test-from-controller-use-vngcloud-mock-repository
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-16
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + Ginkgo/Gomega + envtest |
| **Config file** | none — existing infrastructure |
| **Quick run command** | `go test ./internal/controller/glbc_controller/... -v -count=1 -timeout 120s` |
| **Full suite command** | `go test ./internal/controller/glbc_controller/... -v -count=1 -timeout 120s` |
| **Estimated runtime** | ~30 seconds (envtest startup + reconcile cycles) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/controller/glbc_controller/... -v -count=1 -timeout 120s`
- **After every plan wave:** Run full suite
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 1 | mock-setup | unit | `go build ./internal/repository/vngcloud_repo/vngcloud_mocks/...` | ✅ vngcloud_mock_glb.go | ⬜ pending |
| 04-01-02 | 01 | 1 | suite-setup | integration | `go test ./internal/controller/glbc_controller/... -v -count=1 -timeout 120s` | ❌ W0 | ⬜ pending |
| 04-01-03 | 01 | 1 | create-test | integration | `go test ./internal/controller/glbc_controller/... -run "create" -v -count=1 -timeout 120s` | ❌ W0 | ⬜ pending |
| 04-01-04 | 01 | 1 | delete-test | integration | `go test ./internal/controller/glbc_controller/... -run "delete" -v -count=1 -timeout 120s` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/controller/glbc_controller/suite_test.go` — envtest + Ginkgo suite setup
- [ ] `internal/controller/glbc_controller/helpers_test.go` — GLBC test helpers
- [ ] `internal/controller/glbc_controller/glbc_controller_test.go` — test cases

Existing infrastructure (Ginkgo, envtest, CRDs) covers framework requirements.

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

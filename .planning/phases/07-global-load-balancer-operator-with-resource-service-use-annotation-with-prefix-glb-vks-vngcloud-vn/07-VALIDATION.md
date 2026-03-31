---
phase: 07
slug: global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-17
---

# Phase 07 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go testing |
| **Quick run command** | `go test ./internal/usecase/service_glb_uc/...` |
| **Full suite command** | `go test ./internal/usecase/service_glb_uc/... ./internal/controller/service_glb_controller/... ./pkg/service_glb/...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/usecase/service_glb_uc/...`
- **After every plan wave:** Run `go test ./internal/usecase/service_glb_uc/... ./internal/controller/service_glb_controller/... ./pkg/service_glb/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | TBD | unit/integration | TBD | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/usecase/service_glb_uc/build_glbc_test.go` — unit tests for GLBC building
- [ ] `internal/controller/service_glb_controller/service_glb_controller_test.go` — integration tests
- [ ] `pkg/service_glb/service_glb_utils_test.go` — utils tests

*Existing go test infrastructure covers framework needs.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

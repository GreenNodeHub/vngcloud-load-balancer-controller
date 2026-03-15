---
phase: 1
slug: p0-bug-fixes
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-15
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + testify (github.com/stretchr/testify) + testify/mock |
| **Config file** | None (standard `go test`) |
| **Quick run command** | `go test ./internal/usecase/glbc_uc/... -v -run TestCan` |
| **Full suite command** | `go test ./internal/usecase/...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/usecase/glbc_uc/... -v`
- **After every plan wave:** Run `go test ./internal/usecase/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | BUG-01 | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | BUG-01 | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | ❌ W0 | ⬜ pending |
| 01-01-03 | 01 | 1 | BUG-01 | unit | `go test ./internal/usecase/glbc_uc/... -run TestCanDeleteWholeListener` | ❌ W0 | ⬜ pending |
| 01-02-01 | 02 | 1 | BUG-02 | unit | `go test ./internal/usecase/glbc_uc/... -run TestConvertMember` | ❌ W0 | ⬜ pending |
| 01-02-02 | 02 | 1 | BUG-02 | unit | `go test ./internal/usecase/glbc_uc/... -run TestBuildPatchGlobalPoolMemberRequest` | ❌ W0 | ⬜ pending |
| 01-03-01 | 03 | 1 | BUG-03 | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeleteLoadBalancer` | ❌ W0 | ⬜ pending |
| 01-04-01 | 04 | 1 | BUG-04 | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeployListener` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/usecase/glbc_uc/delete_listener_test.go` — stubs for BUG-01 (TestCanDeleteWholeListener)
- [ ] `internal/usecase/glbc_uc/deploy_pool_member_test.go` — stubs for BUG-02 (TestConvertMember, TestBuildPatchGlobalPoolMemberRequest)
- [ ] `internal/usecase/glbc_uc/delete_lb_test.go` — stubs for BUG-03 (TestDeleteLoadBalancer)
- [ ] `internal/usecase/glbc_uc/deploy_listener_test.go` — stubs for BUG-04 (TestDeployListener)

Model: `internal/usecase/lbc_uc/delete_listener_test.go` — use the same test structure (table-driven, `MockVngCloudRepository`, `logrus.NewEntry(logrus.New())`, `&defaultModelDeployTask{...}` direct construction).

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Full reconcile-delete-recreate cycle | BUG-01..04 | Requires live VNG Cloud API | Deploy GLBC, delete, verify via API logs |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

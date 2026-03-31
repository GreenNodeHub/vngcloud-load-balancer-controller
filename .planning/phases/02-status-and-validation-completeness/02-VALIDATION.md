---
phase: 2
slug: status-and-validation-completeness
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-15
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + testify/assert + testify/mock |
| **Config file** | none — `go test ./...` |
| **Quick run command** | `go test ./internal/usecase/glbc_uc/... -run TestSTAT -v` |
| **Full suite command** | `go test ./internal/usecase/glbc_uc/...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/usecase/glbc_uc/...`
- **After every plan wave:** Run `go test ./internal/usecase/glbc_uc/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | STAT-01 | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeployPool_PopulatesCreatedPoolMembers -v` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | STAT-01 | unit | `go test ./internal/usecase/glbc_uc/... -run TestDeployPool_StatusUpdatedOnCreate -v` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 1 | STAT-02 | unit | `go test ./internal/usecase/glbc_uc/... -run TestBuildListenerUpdateRequest_Headers -v` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 1 | STAT-02 | unit | `go test ./internal/usecase/glbc_uc/... -run TestBuildListenerUpdateRequest_HeadersNoChange -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/usecase/glbc_uc/deploy_pool_test.go` — stubs for STAT-01 (pool creation populates members in status)
- [ ] `internal/usecase/glbc_uc/deploy_listener_headers_test.go` — stubs for STAT-02 (headers change triggers update; unchanged does not)

Existing test infrastructure: `glbc_uc` package already has working `*_test.go` files using `repository.MockVngCloudRepository` and `testify/assert`. Pattern is established.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Pool member IDs appear in status after live pool creation | STAT-01 | Requires live VNG Cloud API | Create GLBC, check status.createdPools[*].createdMembers |
| Header change triggers live API update call | STAT-02 | Requires live VNG Cloud API | Change listener headers in spec, observe API call in logs |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

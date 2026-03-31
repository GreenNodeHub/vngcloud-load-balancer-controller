---
phase: 01-p0-bug-fixes
verified: 2026-03-15T17:45:00Z
status: passed
score: 9/9 must-haves verified
re_verification: false
---

# Phase 1: P0 Bug Fixes Verification Report

**Phase Goal:** The reconciler's create, update, and delete flows complete without code-level failures; status stabilizes after each reconcile
**Verified:** 2026-03-15T17:45:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                                               | Status     | Evidence                                                                                                           |
|----|---------------------------------------------------------------------------------------------------------------------|------------|--------------------------------------------------------------------------------------------------------------------|
| 1  | convertMember includes SubnetID so pool member diffs are stable when SubnetID is unchanged                          | VERIFIED   | `deploy_pool_member.go:313` — `SubnetID: member.SubnetID` present in return struct                                |
| 2  | Empty GLB deletion calls DeleteGlobalLoadBalancer, not DeleteLoadBalancer                                           | VERIFIED   | `delete_lb.go:43,76` — both canDelete and isEmpty paths call `DeleteGlobalLoadBalancer`; no `DeleteLoadBalancer` calls in any production file |
| 3  | Empty pool deletion calls DeleteGlobalPool, not DeletePool                                                          | VERIFIED   | `delete_pool.go:80` — `t.vngcloudRepo.DeleteGlobalPool(ctx, lbId, candidateId)`; no `DeletePool` calls in any production file |
| 4  | CreatedGlobalListener returned by deployListener has Name populated from API response                               | VERIFIED   | `deploy_listener.go:100` — `Name: currentListener.Name` present in return struct                                  |
| 5  | canDeleteWholeListener returns true when all pool members are owned and returns false when any are not              | VERIFIED   | `delete_listener.go:82-154` — full 6-step bottom-up ownership implementation, no ErrorNotImplemented stub         |
| 6  | canDeleteWholeListener returns true when listener has no pool (GlobalPoolID empty)                                  | VERIFIED   | `delete_listener.go:84-87` — step 1: `if listener.GlobalPoolID == "" { return true, nil }`                        |
| 7  | canDeleteWholeListener returns false when pool is not in status.createdPools                                        | VERIFIED   | `delete_listener.go:98-108` — step 3: linear scan for pool in status; if nil, return false                        |
| 8  | canDeleteWholeLoadBalancer member verification is active (not commented out)                                        | VERIFIED   | `delete_lb.go:137-172` — Address+Port matching block is active code, not comments; `ListGlobalPoolMembers` called  |
| 9  | Full glbc_uc test suite is green with no regressions                                                               | VERIFIED   | `go test ./internal/usecase/glbc_uc/... -v` — all tests PASS; `go vet` — zero issues                             |

**Score:** 9/9 truths verified

---

### Required Artifacts

| Artifact                                                        | Expected                                            | Status     | Details                                                                                   |
|-----------------------------------------------------------------|-----------------------------------------------------|------------|-------------------------------------------------------------------------------------------|
| `internal/usecase/glbc_uc/deploy_pool_member.go`               | convertMember with SubnetID field                   | VERIFIED   | Line 313: `SubnetID: member.SubnetID` present; file is 315 lines with full implementation |
| `internal/usecase/glbc_uc/delete_lb.go`                        | Correct GLB delete API call; member verification active | VERIFIED | Lines 43, 76: both paths call `DeleteGlobalLoadBalancer`; lines 137-172: active member check |
| `internal/usecase/glbc_uc/delete_pool.go`                      | Correct global pool delete API call                 | VERIFIED   | Line 80: `DeleteGlobalPool`; no `DeletePool` call anywhere in file                        |
| `internal/usecase/glbc_uc/deploy_listener.go`                  | Listener name in status                             | VERIFIED   | Line 100: `Name: currentListener.Name` in return struct                                   |
| `internal/usecase/glbc_uc/delete_listener.go`                  | canDeleteWholeListener implementation, min 40 lines | VERIFIED   | File is 154 lines; full 6-step ownership check at lines 82-154; no ErrorNotImplemented    |
| `internal/usecase/glbc_uc/deploy_pool_member_test.go`          | Unit test for convertMember SubnetID                | VERIFIED   | TestConvertMember_IncludesSubnetID — 3 table-driven cases; all PASS                       |
| `internal/usecase/glbc_uc/delete_lb_test.go`                   | Unit test for DeleteGlobalLoadBalancer call          | VERIFIED   | TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer + TestDeleteGlobalPool_CallsDeleteGlobalPool; both PASS |
| `internal/usecase/glbc_uc/deploy_listener_test.go`             | Unit test for listener Name population              | VERIFIED   | TestDeployListener_PopulatesName — 2 table-driven cases; all PASS                         |
| `internal/usecase/glbc_uc/delete_listener_test.go`             | Table-driven tests for canDeleteWholeListener, min 80 lines | VERIFIED | File is 257 lines; 7 test cases all PASS                                            |

---

### Key Link Verification

| From                             | To                                         | Via                                         | Status   | Details                                                                                      |
|----------------------------------|--------------------------------------------|---------------------------------------------|----------|----------------------------------------------------------------------------------------------|
| `deploy_pool_member.go`          | `v1alpha1.GlobalMember`                    | convertMember return struct                 | WIRED    | `SubnetID: member.SubnetID` at line 313; `checkIfPoolMemberExist` also checks SubnetID at line 286 |
| `delete_lb.go`                   | `repository.VngCloudRepository`            | DeleteGlobalLoadBalancer method call        | WIRED    | Called at lines 43 (canDelete=true path) and 76 (isEmpty=true path)                         |
| `delete_listener.go`             | `repository.VngCloudRepository.ListGlobalPoolMembers` | API call to fetch current pool members | WIRED  | `t.vngcloudRepo.ListGlobalPoolMembers(ctx, lbId, listener.GlobalPoolID)` at line 111        |
| `delete_listener.go`             | `v1alpha1.CreatedGlobalPool.CreatedPoolMembers` | Status lookup for owned pool member groups | WIRED  | `statusPool.CreatedPoolMembers` iterated at lines 124-129 to build `ownedGroups` map        |
| `delete_listener.go`             | `v1alpha1.GlobalMember (Address+Port)`     | Individual member ownership matching        | WIRED    | `memberKey{address: m.Address, port: m.Port}` checked against `ownedMembers` map at line 143-147 |

---

### Requirements Coverage

| Requirement | Source Plan | Description                                                                                    | Status    | Evidence                                                                                  |
|-------------|-------------|------------------------------------------------------------------------------------------------|-----------|-------------------------------------------------------------------------------------------|
| BUG-01      | 01-02-PLAN  | Implement canDeleteWholeListener to check listener ownership before deletion                   | SATISFIED | Full 6-step implementation in `delete_listener.go:82-154`; 7 tests pass                  |
| BUG-02      | 01-01-PLAN  | Fix convertMember to include SubnetID in the converted GlobalMember struct                     | SATISFIED | `deploy_pool_member.go:313` — `SubnetID: member.SubnetID`; TestConvertMember_IncludesSubnetID passes |
| BUG-03      | 01-01-PLAN  | Fix shared-LB empty cleanup to call DeleteGlobalLoadBalancer instead of DeleteLoadBalancer     | SATISFIED | `delete_lb.go:43,76` — both paths verified; TestDeleteLoadBalancer_CallsDeleteGlobalLoadBalancer passes |
| BUG-04      | 01-01-PLAN  | Populate Name field in CreatedGlobalListener returned by deployListener                        | SATISFIED | `deploy_listener.go:100` — `Name: currentListener.Name`; TestDeployListener_PopulatesName passes |

All four requirements declared in PLAN frontmatter are satisfied. REQUIREMENTS.md marks all four as complete (`[x]`). No orphaned requirements for Phase 1.

---

### Anti-Patterns Found

| File                      | Line | Pattern                                      | Severity | Impact                                                                                          |
|---------------------------|------|----------------------------------------------|----------|-------------------------------------------------------------------------------------------------|
| `deploy_listener.go`      | 54   | `// TODO: uncomment me` — statusAddListener  | WARNING  | Status listener tracking is commented out in create path. Relates to STAT-01 (Phase 2 scope).  |
| `deploy_listener.go`      | 86   | `// TODO: uncomment me` — statusAddListener  | WARNING  | Status listener tracking is commented out in update path. Relates to STAT-01 (Phase 2 scope).  |
| `deploy_listener.go`      | 214  | `// TODO: compare headers`                   | WARNING  | Header comparison stub. Relates to STAT-02 (Phase 2 scope).                                     |

All three TODOs are explicitly out-of-scope for Phase 1 — they correspond to Phase 2 requirements STAT-01 and STAT-02 and were pre-existing before this phase. None block Phase 1's goal.

---

### Human Verification Required

None. All Phase 1 fixes are logic-level code changes verified by unit tests. No UI, real-time behavior, or external service integration is involved.

---

## Summary

Phase 1 goal is **fully achieved**. All four P0 bugs (BUG-01 through BUG-04) are fixed with verified unit test coverage:

- **BUG-02** — `convertMember` now maps `SubnetID` from the SDK entity; pool member diffs are stable across reconciles.
- **BUG-03** — Both delete paths in `delete_lb.go` (whole-LB delete and isEmpty post-cleanup) correctly call `DeleteGlobalLoadBalancer`. The companion bug in `delete_pool.go` was also fixed: `DeleteGlobalPool` replaces `DeletePool`.
- **BUG-04** — `deployListener` returns `CreatedGlobalListener` with `Name` populated from the API response, enabling stable listener matching in status.
- **BUG-01** — `canDeleteWholeListener` is fully implemented with a 6-step bottom-up ownership check using Address+Port tuple matching. The `canDeleteWholeLoadBalancer` member verification block is active (uncommented and adapted). `ErrorNotImplemented` is gone from all hot paths.

All four commits (`966b16f`, `4da5290`, `75e63a0`, `67c567f`) are present in git history. The complete glbc_uc test suite (13 tests across 4 test files) passes. `go vet` reports no issues.

---

_Verified: 2026-03-15T17:45:00Z_
_Verifier: Claude (gsd-verifier)_

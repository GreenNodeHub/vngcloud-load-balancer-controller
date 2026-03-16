---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: planning
stopped_at: Completed 05-fix-production-bugs-duplicate-pool-members-missing-listener-port-pool-member-id-tracking/05-01-PLAN.md
last_updated: "2026-03-16T07:36:45.024Z"
last_activity: 2026-03-15 — Roadmap created, ready to begin Phase 1 planning
progress:
  total_phases: 5
  completed_phases: 5
  total_plans: 7
  completed_plans: 7
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-15)

**Core value:** Reliably sync GlobalLoadBalancerConfig spec to VNG Cloud GLB resources with accurate status tracking
**Current focus:** Phase 1 — P0 Bug Fixes

## Current Position

Phase: 1 of 3 (P0 Bug Fixes)
Plan: 0 of TBD in current phase
Status: Ready to plan
Last activity: 2026-03-15 — Roadmap created, ready to begin Phase 1 planning

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity:**
- Total plans completed: 0
- Average duration: -
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

**Recent Trend:**
- Last 5 plans: none yet
- Trend: -

*Updated after each plan completion*
| Phase 01-p0-bug-fixes P02 | 4 | 2 tasks | 3 files |
| Phase 01-p0-bug-fixes P01 | 7 | 2 tasks | 6 files |
| Phase 02-status-and-validation-completeness P01 | 6 | 3 tasks | 7 files |
| Phase 02-status-and-validation-completeness P01 | 8 | 3 tasks | 4 files |
| Phase 03-test-coverage P01 | 5 | 1 tasks | 1 files |
| Phase 04-add-test-from-controller-use-vngcloud-mock-repository P01 | 5 | 2 tasks | 2 files |
| Phase 04-add-test-from-controller-use-vngcloud-mock-repository P02 | 26 | 2 tasks | 6 files |
| Phase 05-fix-production-bugs-duplicate-pool-members-missing-listener-port-pool-member-id-tracking P01 | 5 | 2 tasks | 2 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Per-LB mutex locking — serialized access for shared LBs (pending verification)
- 3-way merge for pool members — preserve manually-added members (pending verification)
- Name-based matching for pools, port-based for listeners (pending verification)
- [Phase 01-p0-bug-fixes]: Address+Port tuple used for individual member ownership matching in canDeleteWholeListener (not Name)
- [Phase 01-p0-bug-fixes]: Extracted deleteLoadBalancerWhenNotEmpty to fix pre-existing compile error from plan 01 test
- [Phase 01-p0-bug-fixes]: convertMember must copy SubnetID from GlobalPoolMemberDetail for stable pool member diffs
- [Phase 01-p0-bug-fixes]: deleteRedundantPools uses DeleteGlobalPool (not DeletePool) for correct global LB API
- [Phase 01-p0-bug-fixes]: deployListener returns CreatedGlobalListener.Name from API entity for accurate status tracking
- [Phase 02-status-and-validation-completeness]: ListGlobalPoolMembers called after WaitGlobalLoadBalancerActive to get stable API-assigned pool member IDs on create path
- [Phase 02-status-and-validation-completeness]: statusAddPool on pool update path remains commented — deployPoolMembers already calls statusUpdatePoolMember avoiding redundant status patch
- [Phase 02-status-and-validation-completeness]: Headers comparison is case-insensitive and order-independent using stringSlicesEqualUnordered; nil entity == empty spec (no spurious update)
- [Phase 02-status-and-validation-completeness]: ListGlobalPoolMembers NOT called on create path — spec data used directly, member IDs populated on next reconcile via update path
- [Phase 02-status-and-validation-completeness]: statusUpdatePoolMember called BEFORE WaitGlobalLoadBalancerActive on create path (crash safety)
- [Phase 03-test-coverage]: Use nil for Weight/MonitorPort in test fixtures — checkIfPoolMemberExist compares *int by pointer address not value
- [Phase 04-add-test-from-controller-use-vngcloud-mock-repository]: UpdateGlobalPool/UpdateGlobalListener unlock before updatingGlobalStatus to prevent deadlock (updatingGlobalStatus re-acquires m.mu when WaitAfterTime > 0)
- [Phase 04-add-test-from-controller-use-vngcloud-mock-repository]: MockGLBCSharedSpec uses port 8080 for listener to avoid port conflicts with MockGLBCMinimalSpec (port 80) in shared-LB tests
- [Phase 04-add-test-from-controller-use-vngcloud-mock-repository]: statusAddListener signature extended to include name parameter — CRD requires name field in createdListeners list-map
- [Phase 04-add-test-from-controller-use-vngcloud-mock-repository]: NewPatchGlobalPoolUpdateBulkActionRequest receives pool member group ID (currentPoolMember.ID), not pool ID
- [Phase 05-fix-production-bugs-duplicate-pool-members-missing-listener-port-pool-member-id-tracking]: GlobalListener.ProtocolPort is int (not int32) — matched CRD type for test struct field

### Roadmap Evolution

- Phase 4 added: Add test from controller use vngcloud mock repository
- Phase 5 added: Fix production bugs: duplicate pool members, missing listener port, pool member ID tracking

### Pending Todos

None yet.

### Blockers/Concerns

- `canDeleteWholeListener` returns `ErrorNotImplemented` — blocks all redundant listener cleanup (Phase 1 target)
- `convertMember` drops `SubnetID` — causes infinite spurious member patches (Phase 1 target)
- Wrong API call in delete_lb.go (`DeleteLoadBalancer` should be `DeleteGlobalLoadBalancer`) — Phase 1 target
- `validateCrossGLBCs` query pattern against informer cache needs confirmation during Phase 2 implementation

## Session Continuity

Last session: 2026-03-16T07:33:34.836Z
Stopped at: Completed 05-fix-production-bugs-duplicate-pool-members-missing-listener-port-pool-member-id-tracking/05-01-PLAN.md
Resume file: None

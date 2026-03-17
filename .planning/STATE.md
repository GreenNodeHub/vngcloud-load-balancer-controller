---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: planning
stopped_at: Completed 07-03-PLAN.md
last_updated: "2026-03-17T04:12:44.699Z"
last_activity: 2026-03-15 — Roadmap created, ready to begin Phase 1 planning
progress:
  total_phases: 7
  completed_phases: 7
  total_plans: 14
  completed_plans: 14
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
| Phase 06-add-vglb-operator-generate-glbc-from-vglb P02 | 2 | 2 tasks | 3 files |
| Phase 06-add-vglb-operator-generate-glbc-from-vglb P01 | 15 | 2 tasks | 4 files |
| Phase 06-add-vglb-operator-generate-glbc-from-vglb P03 | 2min | 1 tasks | 2 files |
| Phase 06-add-vglb-operator-generate-glbc-from-vglb P04 | 20min | 2 tasks | 6 files |
| Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn P01 | 5min | 2 tasks | 8 files |
| Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn P02 | 15min | 2 tasks | 4 files |
| Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn P03 | 4min | 2 tasks | 3 files |

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
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: Service event handler uses same-name VGLB lookup (not annotation check) — VGLB name matches Service name+namespace by design
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: Delete event on Service enqueues VGLB (not no-op) so reconciler detects missing Service and requeues
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: Init reads network info from node labels (labelMgmtZone/labelNetworkId/labelSubnetId), not VNG Cloud API
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: Pool member group name is {region}-{vpcId}, not 'default'; region derived from node label zone via stripZoneSuffix, not hardcoded 'hcm'
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: GLB default display name uses 'vks_' prefix; service not found causes requeue (5s); ClusterIP causes requeue (30s); VGLB status address from GLBC domains only
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: Use mock.On with mock.Anything (not EXPECT()) for variadic opts args — testify EXPECT().Method() passes opts slice as positional arg making exact matching tricky; mock.On with mock.Anything is cleaner
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: K8s strips TypeMeta from objects returned by Get; use KindVngcloudGlobalLoadBalancer constant for label values not obj.Kind
- [Phase 06-add-vglb-operator-generate-glbc-from-vglb]: Test node in envtest requires NodeReady=True condition plus Status().Update() call for endpoint resolver to include it
- [Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn]: ServiceGLBUtils does NOT check ServiceType — any service type can be GLB-enabled via glb.vks.vngcloud.vn/enable annotation
- [Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn]: GLBC owner label uses domain.KindService constant (not svc.Kind) for label survival through API server round-trips
- [Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn]: Service GLB annotation prefix is glb.vks.vngcloud.vn (distinct from vks.vngcloud.vn); ClusterIP forces TargetTypeIP
- [Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn]: Service event handler includes annotationParser field for hadGLBAnnotation; controller creates GLB-prefix parser internally
- [Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn]: ServiceGLBReconciler stores annotationParser internally from GLB_ANNOTATION_PREFIX to keep constructor signature clean
- [Phase 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn]: NodePort values must be unique per test (30080/30180/30280) — envtest uses a single cluster-wide kube-apiserver that rejects duplicate NodePorts across all namespaces

### Roadmap Evolution

- Phase 4 added: Add test from controller use vngcloud mock repository
- Phase 5 added: Fix production bugs: duplicate pool members, missing listener port, pool member ID tracking
- Phase 6 added: add vglb operator, generate glbc from vglb
- Phase 7 added: global load balancer operator with resource Service, use annotation with prefix glb.vks.vngcloud.vn

### Pending Todos

None yet.

### Blockers/Concerns

- `canDeleteWholeListener` returns `ErrorNotImplemented` — blocks all redundant listener cleanup (Phase 1 target)
- `convertMember` drops `SubnetID` — causes infinite spurious member patches (Phase 1 target)

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260317-61v | Rename listener and pool to use port number instead of port name | 2026-03-17 | b4d9556 | [260317-61v-rename-listener-and-pool-to-use-port-num](./quick/260317-61v-rename-listener-and-pool-to-use-port-num/) |
| 260317-8sp | Create dedicated GLB annotation suffix constants for service_glb_uc | 2026-03-17 | fd3e52b | [260317-8sp-create-dedicated-glb-annotation-suffix-c](./quick/260317-8sp-create-dedicated-glb-annotation-suffix-c/) |
| 260317-9hw | Fix event handler logging — only log events that get enqueued (19 files, 8 controllers) | 2026-03-17 | 758e60b | [260317-9hw-fix-eventhandler-logging-only-log-events](./quick/260317-9hw-fix-eventhandler-logging-only-log-events/) |
| 260317-a50 | Skip Service status address update in ServiceGLB for type=LoadBalancer | 2026-03-17 | 1061b3d | [260317-a50-skip-service-status-address-update-in-se](./quick/260317-a50-skip-service-status-address-update-in-se/) |
- Wrong API call in delete_lb.go (`DeleteLoadBalancer` should be `DeleteGlobalLoadBalancer`) — Phase 1 target
- `validateCrossGLBCs` query pattern against informer cache needs confirmation during Phase 2 implementation

## Session Continuity

Last session: 2026-03-17T08:00:00Z
Stopped at: Completed quick task 260317-a50
Resume file: None

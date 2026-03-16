# Requirements: GLBC Operator

**Defined:** 2026-03-15
**Core Value:** Reliably sync GlobalLoadBalancerConfig spec to VNG Cloud GLB resources with accurate status tracking

## v1 Requirements

Requirements for this milestone. Each maps to roadmap phases.

### Bug Fixes

- [x] **BUG-01**: Implement `canDeleteWholeListener` to check listener ownership before deletion
- [x] **BUG-02**: Fix `convertMember` to include `SubnetID` in the converted GlobalMember struct
- [x] **BUG-03**: Fix shared-LB empty cleanup to call `DeleteGlobalLoadBalancer` instead of `DeleteLoadBalancer`
- [x] **BUG-04**: Populate `Name` field in `CreatedGlobalListener` returned by `deployListener`

### Status Tracking

- [x] **STAT-01**: Uncomment and activate pool member status tracking on pool creation in `deploy_pool.go`
- [x] **STAT-02**: Implement headers comparison in `buildListenerUpdateRequest` (replace TODO)

### Testing

- [x] **TEST-01**: Unit tests for pool member 3-way merge edge cases (add, remove, update, preserve manual members)

### Controller Integration Tests

- [x] **CTRL-TEST-01**: Implement `UpdateGlobalPool` in MockProvider (replace ErrorNotImplemented stub)
- [x] **CTRL-TEST-02**: Implement `UpdateGlobalListener` in MockProvider and create GLBC test fixtures
- [x] **CTRL-TEST-03**: Controller integration test for create flow (GLBC CR -> LB/pool/listener created, status populated)
- [x] **CTRL-TEST-04**: Controller integration test for full delete flow (sole-owner LB -> DeleteGlobalLoadBalancer, backend empty)
- [x] **CTRL-TEST-05**: Controller integration test for partial delete flow (shared LB -> only owned resources removed)

### Production Bug Regression Tests

- [x] **PBUG-01**: Regression tests for duplicate pool member addresses — verify ptrIntEqual and comparePoolMembers handle nil vs non-nil pointer fields correctly
- [x] **PBUG-02**: Regression test for listener port assignment — verify buildCreateListenerRequest sets Port from ProtocolPort
- [x] **PBUG-03**: Verify pool member ID tracking on create path (already tested by TestDeployPool_PopulatesCreatedPoolMembers)

### VGLB Operator — GLBC Generation

- [ ] **VGLB-01**: Init reads network info from node labels (`vks.vngcloud.vn/mgmt-zone`, `network-id`, `subnet-id`) instead of VNG Cloud API
- [ ] **VGLB-02**: Pool member group name uses `{region}-{vpcId}` format (not hardcoded `"default"`)
- [ ] **VGLB-03**: Region derived from zone label by stripping digit+letter suffix (`hcm03b` -> `hcm`)
- [ ] **VGLB-04**: GLB default display name uses `vks_` prefix (not `glb_`)
- [ ] **VGLB-05**: Service not found causes requeue (not empty pool generation)
- [ ] **VGLB-06**: ClusterIP service type causes requeue (not silent fallback to pod IPs); VGLB status address from domains only (not VIPs)
- [ ] **VGLB-07**: VGLB controller watches Service and Node resources via event handlers in SetupWithManager
- [ ] **VGLB-08**: Unit tests for stripZoneSuffix and pool member group naming
- [ ] **VGLB-09**: Integration test: VGLB create with NodePort Service produces correct GLBC (pools, listeners, member groups)
- [ ] **VGLB-10**: Integration test: VGLB delete causes owned GLBC deletion
- [ ] **VGLB-11**: Integration test: Service port change triggers GLBC spec update

## v2 Requirements

Deferred to future milestone. Tracked but not in current roadmap.

### Validation

- **VALID-01**: Implement `validateSelf` — listener must reference existing pool name in spec
- **VALID-02**: Implement `validateCrossGLBCs` — detect port conflicts on shared LBs

### Verification

- **VERIF-01**: End-to-end reconcile → delete → re-create cycle test

### Hardening

- **HARD-01**: Fix orphan risk on first-create requeue (status not saved before requeue)
- **HARD-02**: Fix per-LB mutex gap during concurrent name-based LB lookup

## Out of Scope

| Feature | Reason |
|---------|--------|
| Tag management | VNG Cloud SDK doesn't support tags at creation |
| CRD schema changes | Focus on reconciler logic, not API shape |
| Webhook admission controller | Premature; reconcile-time validation sufficient |
| Gateway API integration | Not applicable to VNG Cloud proprietary API |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| BUG-01 | Phase 1 | Complete |
| BUG-02 | Phase 1 | Complete |
| BUG-03 | Phase 1 | Complete |
| BUG-04 | Phase 1 | Complete |
| STAT-01 | Phase 2 | Complete |
| STAT-02 | Phase 2 | Complete |
| TEST-01 | Phase 3 | Complete |
| CTRL-TEST-01 | Phase 4 | Planned |
| CTRL-TEST-02 | Phase 4 | Planned |
| CTRL-TEST-03 | Phase 4 | Planned |
| CTRL-TEST-04 | Phase 4 | Planned |
| CTRL-TEST-05 | Phase 4 | Planned |
| PBUG-01 | Phase 5 | Planned |
| PBUG-02 | Phase 5 | Planned |
| PBUG-03 | Phase 5 | Planned |
| VGLB-01 | Phase 6 | Planned |
| VGLB-02 | Phase 6 | Planned |
| VGLB-03 | Phase 6 | Planned |
| VGLB-04 | Phase 6 | Planned |
| VGLB-05 | Phase 6 | Planned |
| VGLB-06 | Phase 6 | Planned |
| VGLB-07 | Phase 6 | Planned |
| VGLB-08 | Phase 6 | Planned |
| VGLB-09 | Phase 6 | Planned |
| VGLB-10 | Phase 6 | Planned |
| VGLB-11 | Phase 6 | Planned |

**Coverage:**
- v1 requirements: 26 total
- Mapped to phases: 26
- Unmapped: 0

---
*Requirements defined: 2026-03-15*
*Last updated: 2026-03-16 after Phase 6 planning*

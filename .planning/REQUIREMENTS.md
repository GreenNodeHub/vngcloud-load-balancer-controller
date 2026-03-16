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
| VGLB build logic | Separate operator, later phase |
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

**Coverage:**
- v1 requirements: 7 total
- Mapped to phases: 7
- Unmapped: 0

---
*Requirements defined: 2026-03-15*
*Last updated: 2026-03-15 after roadmap creation*

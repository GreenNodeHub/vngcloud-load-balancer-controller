---
phase: 06-add-vglb-operator-generate-glbc-from-vglb
plan: 01
subsystem: vglb-operator
tags: [golang, k8s-operator, glb, vglb, node-labels, pool-members]

# Dependency graph
requires:
  - phase: 05-fix-production-bugs-duplicate-pool-members-missing-listener-port-pool-member-id-tracking
    provides: stable pool member diffing and listener tracking in GLBC controller
provides:
  - "Node-label-based init replacing VNG Cloud API call in vglbUseCase"
  - "stripZoneSuffix function converting zone to region (hcm03b->hcm)"
  - "defaultRegion field on vglbUseCase and defaultModelBuildTask"
  - "vks_ prefix for default GLB display names"
  - "Service-not-found and ClusterIP rejection with requeue semantics"
  - "Pool member group Name/Region derived from defaultRegion+defaultNetworkId"
  - "VGLB status address from GLBC domains only (no VIP fallback)"
affects:
  - "06-add-vglb-operator-generate-glbc-from-vglb/06-02"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Node label reads for cluster network info instead of VNG Cloud API (labelMgmtZone, labelNetworkId, labelSubnetId)"
    - "Zone-to-region stripping via regex: zoneRe.ReplaceAllString strips trailing digits+letter suffix"
    - "Requeue-not-error pattern for transient states (service not found, ClusterIP type)"

key-files:
  created: []
  modified:
    - internal/usecase/vglb_uc/vglb_uc.go
    - internal/usecase/vglb_uc/build_glbc.go
    - internal/usecase/vglb_uc/build_global_pool.go
    - internal/usecase/vglb_uc/build_glbc_test.go

key-decisions:
  - "Init reads network info from node labels (labelMgmtZone/labelNetworkId/labelSubnetId), not VNG Cloud API"
  - "Pool member group name is {region}-{vpcId}, not 'default'"
  - "Pool member group region is derived from node label zone via stripZoneSuffix, not hardcoded 'hcm'"
  - "GLB default display name uses 'vks_' prefix, not 'glb_'"
  - "Service not found causes requeue after 5s, not empty pool generation"
  - "ClusterIP service type causes requeue after 30s, not silent fallback to pod IPs"
  - "VGLB status address comes from GLBC domains only, not VIPs"

patterns-established:
  - "stripZoneSuffix: zone string -> region string via regexp trimming trailing digits+letter (\\d+[a-z]*$)"
  - "Node label constants defined at package level: labelMgmtZone, labelNetworkId, labelSubnetId"

requirements-completed: [VGLB-01, VGLB-02, VGLB-03, VGLB-04, VGLB-05, VGLB-06]

# Metrics
duration: 15min
completed: 2026-03-16
---

# Phase 6 Plan 01: VGLB Core Logic Fixes Summary

**Six concrete correctness fixes to VGLB operator: node-label init, dynamic pool member group naming, vks_ GLB prefix, service-not-found requeue, ClusterIP rejection, and domains-only VGLB status address**

## Performance

- **Duration:** 15 min
- **Started:** 2026-03-16T08:35:00Z
- **Completed:** 2026-03-16T08:50:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Replaced VNG Cloud API call in init with node label reads — removes API dependency at startup
- Added stripZoneSuffix function and defaultRegion field for zone-to-region derivation
- Fixed 5 behavioral bugs in build_glbc.go and build_global_pool.go: name prefix, service handling, pool group naming, address source
- Updated all existing tests to expect vks_ prefix — all 18 tests pass

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix init to read node labels + add defaultRegion field + stripZoneSuffix** - `34582b2` (feat)
2. **Task 2: Fix GLB name prefix, pool member group naming, service handling, address logic, and update tests** - `d7174f5` (feat)

**Plan metadata:** (docs commit follows)

## Files Created/Modified
- `internal/usecase/vglb_uc/vglb_uc.go` - Node-label-based init, stripZoneSuffix, labelMgmtZone/labelNetworkId/labelSubnetId constants, defaultRegion field
- `internal/usecase/vglb_uc/build_glbc.go` - defaultRegion field on struct, vks_ prefix, service-not-found requeue, ClusterIP rejection, remove VIP fallback from getGLBCAddress
- `internal/usecase/vglb_uc/build_global_pool.go` - Dynamic pool member group naming using defaultRegion and defaultNetworkId
- `internal/usecase/vglb_uc/build_glbc_test.go` - Updated 3 test expectations from glb_ to vks_ prefix

## Decisions Made
- Node label reads chosen over VNG Cloud API: labels are already populated by VKS node provisioner, removes external call at init and avoids GetServerNetworkInfo dependency
- defaultRegion replaces defaultZone (common.Zone) entirely — the GLBC spec requires a plain string region, not a typed Zone value
- stripZoneSuffix uses regexp `\d+[a-z]*$` to handle zone formats like hcm03b, sgn01a, han01
- Pool member group name changed from hardcoded "default"/"hcm" to `{region}-{networkId}` format matching VNG Cloud GLB API conventions

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Task 1 build temporarily failed because premature `time` and `errs` imports were added to build_glbc.go before Task 2 usage — immediately removed; no impact on final state

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- VGLB operator core logic is now correct per all locked decisions
- Ready for phase 06-02: add event handlers and integration tests
- All existing unit tests pass; build is clean

---
*Phase: 06-add-vglb-operator-generate-glbc-from-vglb*
*Completed: 2026-03-16*

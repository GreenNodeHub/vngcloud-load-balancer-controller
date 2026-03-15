# VNGCloud Global Load Balancer Config (GLBC) Operator

## What This Is

A Kubernetes operator that reconciles `GlobalLoadBalancerConfig` CRDs against the VNGCloud Global Load Balancer API. When a GLBC resource is created or updated, the controller ensures the corresponding global load balancer, pools, listeners, and pool members exist and match the desired state in VNG Cloud. It is part of the broader vngcloud-load-balancer-controller, which also handles Service LBs, Ingress, and Network Security Groups.

## Core Value

The GLBC reconciler must reliably sync the desired state declared in `GlobalLoadBalancerConfig` spec to VNG Cloud's Global Load Balancer resources, and accurately track what it created in status so it can safely clean up on deletion.

## Requirements

### Validated

<!-- Inferred from existing code in internal/usecase/glbc_uc/ -->

- ✓ Create or adopt a Global Load Balancer by name or ID — existing
- ✓ Wait for LB to become ACTIVE before proceeding — existing
- ✓ Migrate LB when spec.loadBalancerId changes — existing
- ✓ Resize LB package when spec.packageId differs from current — existing
- ✓ Deploy pools with health monitor configuration — existing
- ✓ Update pool health monitor and algorithm when spec changes — existing
- ✓ Deploy pool members with 3-way merge (created vs current vs spec) — existing
- ✓ Patch pool members via bulk create/update actions — existing
- ✓ Deploy listeners with timeout, CIDR, headers, default pool — existing
- ✓ Update listener properties when spec changes — existing
- ✓ Delete redundant pools no longer in spec — existing
- ✓ Track created resources (LB ID, pools, listeners, pool members) in status — existing
- ✓ Per-LB mutex locking to prevent concurrent modifications — existing
- ✓ Update Ready condition and reconciliation tracking on each reconcile — existing
- ✓ Extract VIPs and Domains from LB entity into status — existing
- ✓ Delete whole LB or partial cleanup based on ownership — existing

### Active

- [ ] Verify and fix `canDeleteWholeListener` — currently returns `ErrorNotImplemented`
- [ ] Verify delete redundant listeners flow works end-to-end
- [ ] Verify pool member merge logic handles all edge cases correctly
- [ ] Verify cross-GLBC validation when multiple GLBCs share a LB (`validateCrossGLBCs` is empty)
- [ ] Verify self-validation logic (`validateSelf` is empty)
- [ ] Verify `convertMember` includes SubnetID (currently missing in conversion)
- [ ] Verify listener name is populated in `CreatedGlobalListener` status
- [ ] Verify headers comparison in listener update (marked TODO)
- [ ] Verify pool creation during LB create includes first pool+listener correctly
- [ ] Verify the full reconcile, delete, re-create cycle works correctly

### Out of Scope

- VGLB (VngcloudGlobalLoadBalancer) CRD build logic — separate operator, later phase
- CRD schema design changes — focus on reconciler logic, not API shape
- Service LB, Ingress, NSG controllers — separate domains in the same repo
- Tag management on GLB — SDK doesn't support tags at creation yet

## Context

- The GLBC controller is part of `vngcloud-load-balancer-controller`, a multi-CRD Kubernetes operator
- It uses `vngcloud-go-sdk/v2` to interact with VNG Cloud's Global Load Balancer API (`glb/v1`)
- The reconcile flow is: validate -> deploy LB -> validate cross-GLBCs -> deploy pools -> deploy listeners -> cleanup redundant -> update status
- Multiple GLBCs can share the same LB (identified by `loadBalancerId`), hence the per-LB mutex and cross-GLBC validation stubs
- Pool members have a complex ownership model: the controller tracks what it created and preserves members it didn't create (manual portal additions)
- The `canDeleteWholeListener` returns `ErrorNotImplemented`, meaning the delete flow for listeners is currently broken

## Constraints

- **VNG Cloud SDK**: Must use `vngcloud-go-sdk/v2` — API calls are async (need WaitActive after mutations)
- **Kubernetes**: Standard controller-runtime patterns, status subresource, finalizer-based deletion
- **Shared LB**: Multiple GLBCs can reference the same LB, requiring careful ownership tracking
- **API limitations**: SDK doesn't support tags at LB creation, pool member patches are bulk-only

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Per-LB mutex locking | Multiple GLBCs can share a LB, need serialized access | — Pending verification |
| 3-way merge for pool members | Preserve manually-added members while managing spec-declared ones | — Pending verification |
| Name-based matching for pools, port-based for listeners | Pools matched by name, listeners matched by port | — Pending verification |
| Status tracks created resource IDs | Enables safe partial deletion when LB is shared | — Pending verification |

---
*Last updated: 2026-03-15 after initialization*

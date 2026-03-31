# Architecture Research: GLBC Operator

**Confidence:** HIGH — all findings from direct codebase reading

## Three-Layer Architecture

```
Controller (K8s lifecycle) → UseCase (business logic) → Repository (API abstractions)
```

New code must never skip a layer.

## Reconciliation Pipeline

Linear task sequence in `defaultModelDeployTask.deploy`:

```
validateSelf → deployLoadBalancer → validateCrossGLBCs → deployPools → deployListeners → deleteRedundantListeners → deleteRedundantPools → updateStatus
```

Every VNG Cloud API mutation is followed by `WaitGlobalLoadBalancerActive` (hard constraint).

## Status as Ownership Source of Truth

- `status.CreatedPools` / `status.CreatedListeners` determine what is safe to delete
- Pool members use 3-way merge: status (what we created) vs live API (current) vs spec (desired)
- Preserves manually-added members (portal additions)

## VGLB → GLBC Decoupling

VGLB controller generates/patches `GlobalLoadBalancerConfig` resources. GLBC controller reconciles them against the cloud. Communication is only through K8s API (GLBC resources + label selectors).

## Current Blockers

1. `canDeleteWholeListener` returns `ErrorNotImplemented` — blocks full cycle
2. `convertMember` missing `SubnetID` — breaks 3-way merge
3. `DeleteLoadBalancer` called instead of `DeleteGlobalLoadBalancer` in shared-LB cleanup

## Suggested Build Order

| Step | Topic | Dependency |
|------|-------|------------|
| 1 | Fix `canDeleteWholeListener` | None |
| 2 | Fix `convertMember` SubnetID | None |
| 3 | Fix `DeleteLoadBalancer` → `DeleteGlobalLoadBalancer` | None |
| 4 | Fix status tracking gaps (listener name, pool members on create) | None |
| 5 | Implement `validateSelf` + `validateCrossGLBCs` | None |
| 6 | Headers comparison in listener update | None |
| 7 | End-to-end cycle verification | Steps 1-6 |

---
*Research: 2026-03-15*

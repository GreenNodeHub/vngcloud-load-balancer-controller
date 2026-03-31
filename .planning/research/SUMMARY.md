# Project Research Summary

**Project:** VNGCloud GlobalLoadBalancerConfig (GLBC) Operator — Verify and Fix Milestone
**Domain:** Kubernetes CRD operator managing VNGCloud Global Load Balancer resources
**Researched:** 2026-03-15
**Confidence:** HIGH

## Executive Summary

This is a subsequent milestone on an existing Kubernetes operator that manages VNGCloud Global Load Balancer resources via a `GlobalLoadBalancerConfig` (GLBC) CRD. The reconciler is substantially complete — it handles create/update/delete flows, 3-way pool member merging, per-LB mutex locking, and async WaitActive gating. The task is not to build from scratch but to identify and fix correctness bugs that prevent the full reconcile and delete cycle from working reliably.

Research identified four P0 bugs that collectively block the delete flow and cause status fields to never stabilize. The most impactful is `canDeleteWholeListener` returning `ErrorNotImplemented` unconditionally, which halts every redundant-listener cleanup and leaves GLBC objects stuck terminating. A wrong API call (`DeleteLoadBalancer` instead of `DeleteGlobalLoadBalancer`) in the shared-LB cleanup path would silently target the wrong resource or fail outright. Two structural correctness bugs — missing `SubnetID` in `convertMember` and missing `Name` in `CreatedGlobalListener` status — cause the 3-way merge and status equality checks to never converge.

The recommended approach is a fix-first, verify-second workflow: resolve all P0 blockers as isolated, independently deployable changes, then implement the two stubbed validation functions (`validateSelf`, `validateCrossGLBCs`), and finally run end-to-end cycle verification. No new dependencies are needed. The existing three-layer architecture (Controller → UseCase → Repository) is sound and should not be restructured. The scope is intentionally frozen: no schema changes, no webhook admission controllers, no VGLB reconciler work in this milestone.

## Key Findings

### Recommended Stack

The stack is fixed for this milestone. No new libraries are required. All tooling is already in `go.mod`. The only relevant constraint is that new test mocks should use the mockery/testify style — `golang/mock` is deprecated and archived; `go.uber.org/mock` v0.4.0 is already present as the replacement. Direct reading of the codebase confirms all versions are current and appropriate.

**Core technologies:**
- **Go 1.22.5**: primary implementation language — no change needed
- **controller-runtime v0.19.3**: K8s reconciler lifecycle, leader election — already wired correctly
- **vngcloud-go-sdk/v2 v2.17.4**: VNGCloud API calls — already used throughout Repository layer
- **testify v1.10.0 / ginkgo v2.22.0 / gomega v1.36.0**: test stack — use for all new tests
- **go.uber.org/mock v0.4.0**: mock generation for new repository mocks — replaces archived golang/mock

### Expected Features

The reconciler already implements most table-stakes features for a production-grade cloud LB operator. The following assessment is based on direct code review.

**Working (table stakes):**
- Idempotent reconcile — equality checks prevent spurious API calls
- Status conditions (`GLBCConditionTypeReady`) and `observedGeneration` — GitOps-compatible
- Async WaitActive polling after every mutation — hard API constraint is met
- Create-or-adopt LB by ID or name — present in `deployLoadBalancer`
- Pool/pool-member lifecycle management — create, update, delete bulk actions working
- Health monitor CRUD — full implementation in `deploy_pool.go`
- Error status propagation and VIP/domain status reporting — present

**Broken (must fix in this milestone):**
- Declarative listener deletion — blocked by `canDeleteWholeListener` returning `ErrorNotImplemented`
- Pool member 3-way merge — broken by `convertMember` dropping `SubnetID`
- Listener status Name field — never populated; breaks equality checks

**Stubbed (implement in this milestone):**
- `validateSelf` — currently returns nil unconditionally; needs listener→pool name validation
- `validateCrossGLBCs` — currently returns nil unconditionally; needs port-conflict detection

**Defer (out of scope for this milestone):**
- Tag management on LBs — SDK does not support tags at creation time
- Webhook admission controller — reconcile-time validation is sufficient for now
- CRD schema redesign — scope is frozen
- VGLB reconciler — separate CRD, separate phase

### Architecture Approach

The codebase follows a strict three-layer architecture: Controller handles Kubernetes lifecycle (finalizers, requeue, status patching), UseCase contains all business logic (the reconcile pipeline), and Repository abstracts all VNGCloud API calls. New code must not skip layers. The reconcile pipeline is a linear task sequence: `validateSelf → deployLoadBalancer → validateCrossGLBCs → deployPools → deployListeners → deleteRedundantListeners → deleteRedundantPools → updateStatus`. Status is the source of truth for ownership — `status.CreatedPools` and `status.CreatedListeners` determine what is safe to delete. The VGLB controller communicates with the GLBC controller only through the Kubernetes API (resource patches + label selectors), maintaining clean decoupling.

**Major components:**
1. **Controller (`glbc_uc.go`)** — acquires per-LB mutex, drives reconcile pipeline, writes status conditions, handles requeue
2. **Deploy tasks (`deploy_lb.go`, `deploy_pool.go`, `deploy_listener.go`, `deploy_pool_member.go`)** — each task creates/updates/deletes one resource class; all call WaitActive after mutations
3. **Delete tasks (`delete_lb.go`, `delete_listener.go`)** — ownership-gated cleanup; currently partially broken
4. **Validate tasks (`validate.go`)** — pre-deploy consistency checks; currently stubbed
5. **Repository (`vngcloud_repo/vngcloud_global.go`)** — wraps vngcloud-go-sdk; the only layer allowed to call the VNGCloud API

### Critical Pitfalls

1. **`canDeleteWholeListener` returns `ErrorNotImplemented`** — implement ownership check: does the listener's `GlobalPoolID` appear in `newCreatedPools`? If not owned exclusively, do not delete whole listener. Fix before any other delete-path work.
2. **`DeleteLoadBalancer` instead of `DeleteGlobalLoadBalancer`** — one-line fix in `delete_lb.go:66`; must be done before any delete-flow testing or the wrong resource type will be targeted.
3. **`convertMember` drops `SubnetID`** — add `SubnetID: member.SubnetID` in `deploy_pool_member.go:305-313`; without this, every reconcile spuriously patches all members because identity comparison always differs.
4. **`CreatedGlobalListener.Name` never set** — set `Name: listenerSpec.Name` in the return struct in `deploy_listener.go:97-100`; without this, status equality never settles and `canDeleteWholeListener` cannot match by name.
5. **Orphan risk on first-create requeue** — after LB create, pool/listener IDs returned in the create response are not recorded before the requeue; a crash in this window orphans cloud resources. Mitigate by recording initial IDs in status before requeuing.

## Implications for Roadmap

Based on combined research, the optimal structure is three tightly-scoped phases ordered by dependency depth.

### Phase 1: Critical Bug Fixes (P0 Blockers)

**Rationale:** All four P0 bugs are independent of each other (no interdependencies) and each blocks a different part of the reconcile or delete cycle. They must be fixed before any validation or end-to-end testing is meaningful. These are small, surgical changes — each is 1-10 lines.

**Delivers:** A reconciler whose create, update, and delete flows no longer fail due to code bugs. Status stabilizes after reconcile. Listener cleanup unblocked.

**Addresses:** `canDeleteWholeListener`, `convertMember` SubnetID, `DeleteGlobalLoadBalancer` API call, `CreatedGlobalListener.Name` status population.

**Avoids:** Wrong-resource deletion (Pitfall 2), infinite reconcile churn (Pitfall 3, 4), stuck terminating objects (Pitfall 1).

### Phase 2: Validation Implementation (P1 Correctness)

**Rationale:** With the delete and status flows unblocked, invalid specs and shared-LB port conflicts are now the next source of silent failures. Implementing `validateSelf` and `validateCrossGLBCs` before end-to-end testing means integration tests can use deliberately invalid specs and confirm errors surface correctly. Also address the `TODO: compare headers` in listener update and the missing `CreatedPoolMembers` in status on pool creation.

**Delivers:** A reconciler that rejects invalid specs before touching the cloud API, surfaces port conflicts across GLBCs, and correctly tracks members in status on first create.

**Addresses:** `validateSelf`, `validateCrossGLBCs`, headers comparison in listener update, pool member status on creation.

**Avoids:** Partial-apply failures from invalid specs (Pitfall 6), silent header drift (Pitfall 7), `canDeleteWholePool` misidentification from missing member status (Pitfall 8).

### Phase 3: End-to-End Verification and Hardening

**Rationale:** After Phases 1 and 2, the reconciler should be functionally complete. Phase 3 is integration testing of the full cycle (create → update → delete → re-create) plus addressing the medium-priority orphan risk and the per-LB mutex gap on concurrent first-reconciles. Minor issues (brittle string matching in error detection, hardcoded fallback package ID) can be addressed here if time allows.

**Delivers:** Verified end-to-end correctness, reduced orphan risk, improved resilience to concurrent creation.

**Addresses:** Orphan prevention on first-create requeue (Pitfall 5), per-LB mutex on name-based lookup (Pitfall 9), full cycle test coverage.

**Avoids:** Orphaned cloud resources on controller restart (Pitfall 5), duplicate LB creation under race (Pitfall 9).

### Phase Ordering Rationale

- Phases 1 and 2 are ordered by severity: P0 bugs block everything; P1 stubs are only meaningful once the core cycle works.
- Phase 3 depends on Phases 1 and 2 being complete because end-to-end testing of a broken delete cycle would produce misleading results.
- All phases stay within the frozen scope: no schema changes, no new CRDs, no VGLB work.
- The three-layer architecture is preserved in every phase — fixes go into the UseCase or Repository layer, never crossing boundaries.

### Research Flags

Phases with standard patterns (no additional research needed):
- **Phase 1:** All fixes are small, well-understood code corrections identified by direct audit. No research needed.
- **Phase 2:** `validateSelf` and `validateCrossGLBCs` have clear requirements from the codebase context. Implementation is straightforward predicate logic over existing data structures.
- **Phase 3:** End-to-end test patterns for controller-runtime operators are well-documented. Orphan-prevention pattern (record IDs before requeue) is standard.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All findings from direct go.mod and import reading; no inference |
| Features | HIGH | Every feature assessed by reading actual code; broken/stubbed states confirmed line-by-line |
| Architecture | HIGH | All layer boundaries and pipeline steps confirmed from code; no speculation |
| Pitfalls | HIGH | All pitfalls have file + line number references from direct audit |

**Overall confidence:** HIGH

### Gaps to Address

- **`canDeleteWholeListener` implementation logic:** The fix direction is clear (ownership check via `GlobalPoolID` in `newCreatedPools`), but the exact matching logic for Layer7 GLBs (if any) needs verification during implementation.
- **`validateCrossGLBCs` scope:** Port-conflict detection is confirmed as needed, but the query pattern for listing sibling GLBCs on the same LB ID needs to be confirmed against the controller's informer cache setup during Phase 2 implementation.
- **Orphan-prevention exact patch target:** The status fields to write before requeue (pool/listener IDs from LB create response) need to be confirmed against the API response structure from `vngcloud-go-sdk` during Phase 3.

## Sources

### Primary (HIGH confidence)
- Codebase direct read: `internal/usecase/glbc_uc/` — all bug findings, feature states, architecture
- Codebase direct read: `go.mod` — stack versions and mock library status

### Secondary (MEDIUM confidence)
- [Kubernetes Operators Best Practices — Red Hat](https://cloud.redhat.com/blog/kubernetes-operators-best-practices) — finalizer and status condition patterns
- [Kubernetes Finalizers — Official Docs](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/) — deletion lifecycle
- [controller-runtime reconcile package — pkg.go.dev](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/reconcile) — requeue and error back-off behavior
- [Subreconciler Pattern — Red Hat Engineering Blog](https://www.redhat.com/en/blog/subreconciler-patterns-in-action) — pipeline task structure
- [AWS Load Balancer Controller — Multiple Controller Ownership](https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues/2788) — shared-LB ownership patterns
- [Error Back-off with Controller Runtime — stuartleeks.com](https://stuartleeks.com/posts/error-back-off-with-controller-runtime/) — requeue behavior

---
*Research completed: 2026-03-15*
*Ready for roadmap: yes*

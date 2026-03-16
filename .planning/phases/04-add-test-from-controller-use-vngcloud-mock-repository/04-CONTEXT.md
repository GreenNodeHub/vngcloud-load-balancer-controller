# Phase 4: Controller Tests with VngCloud Mock Repository - Context

**Gathered:** 2026-03-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Add controller-level integration tests for the GLBC reconciler using the existing vngcloud_mocks.MockProvider. Tests exercise the full reconcile loop (envtest + manager) with a mocked VNG Cloud backend. No production code changes — tests and mock extensions only.

</domain>

<decisions>
## Implementation Decisions

### Test Framework and Setup
- Use envtest + Ginkgo/Gomega + vngcloud_mocks.MockProvider — matches the existing NSG controller test pattern
- Full controller manager setup (SetupWithManager + start manager in goroutine) — tests create GlobalLoadBalancerConfig objects, manager triggers reconcile automatically
- Extend the existing MockProvider in vngcloud_mock_glb.go — do NOT create a separate mock

### Test Scenarios
- **Create flow**: Create a GLBC object with minimal spec (1 pool, 1 pool member group with 1 member, 1 listener referencing that pool) → reconcile creates LB/pools/listeners in mock backend → verify status is populated
- **Delete flow (full)**: Controller owns the LB exclusively → delete GLBC object → reconcile calls DeleteGlobalLoadBalancer → verify mock backend is empty
- **Delete flow (partial)**: Shared LB scenario — some resources belong to another GLBC → delete one GLBC → only its resources are cleaned up, shared resources preserved
- Happy paths only — no error case tests (API failures better tested at usecase level)

### Mock Data Strategy
- MockProvider already simulates a real backend (in-memory state, random IDs, async status transitions) — extend it, don't replace it
- Implement `UpdateGlobalPool` and `UpdateGlobalListener` in MockProvider (currently return ErrorNotImplemented) — needed for reconcile flows
- Mock fixture data (sample GLBC CR specs, expected states) goes in the vngcloud_mocks package — reusable, not inline in test files

### Claude's Discretion
- Exact GLBC spec field values for test fixtures
- Helper functions for creating/verifying GLBC objects in envtest
- Assertion granularity (check specific status fields vs overall status)
- Suite/test file organization within glbc_controller directory

</decisions>

<canonical_refs>
## Canonical References

No external specs — requirements are fully captured in decisions above.

### Existing test patterns to follow
- `internal/controller/nsg_controller/suite_test.go` — envtest + Ginkgo setup pattern
- `internal/controller/nsg_controller/nsg_controller_test.go` — controller test structure
- `internal/controller/nsg_controller/helpers_test.go` — test helpers pattern
- `internal/repository/vngcloud_repo/vngcloud_mocks/vngcloud_mock_glb.go` — GLB mock methods to extend

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `vngcloud_mocks.MockProvider`: Already implements all VngCloudRepository methods. GLB methods in `vngcloud_mock_glb.go` simulate create/delete/list with in-memory state. `UpdateGlobalPool` and `UpdateGlobalListener` need implementation.
- `vngcloud_mocks.NewMockProvider()` + `Init()`: Constructor pattern from NSG tests
- `vngcloud_mocks.MockNode1-4`: Pre-built node fixtures — can create similar patterns for GLBC
- NSG `suite_test.go`: Complete envtest setup with controller manager, CRD loading, mock repo wiring

### Established Patterns
- Ginkgo `Describe/It` blocks for test organization
- `envtest.Environment` with CRD paths for K8s API testing
- `k8s_repo.NewK8sRepository(mgr.GetClient())` for K8s repository
- Controller reconciler constructor with all dependencies injected
- `Eventually` for async reconcile assertions

### Integration Points
- `internal/controller/glbc_controller/glbc_controller.go` — `NewGlobalLoadBalancerConfigReconciler()` constructor
- `internal/usecase/glbc_uc/glbc_uc.go` — `NewGlobalLoadBalancerConfigUseCase()` wires repos to usecase
- `api/v1alpha1/globalloadbalancerconfig_types.go` — CRD type definitions for test fixtures
- `config/crd/bases/` — CRD YAML for envtest

</code_context>

<specifics>
## Specific Ideas

- "I want to make the mocks return like a real backend API" — the MockProvider already does this with in-memory state and random IDs. Extend the pattern for missing GLB update methods.
- Follow the NSG controller test suite exactly for boilerplate (suite_test.go, helpers_test.go structure)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 04-add-test-from-controller-use-vngcloud-mock-repository*
*Context gathered: 2026-03-16*

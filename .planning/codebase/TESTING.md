# Testing Patterns

**Analysis Date:** 2026-03-31

## Test Framework

**Runner:**
- Go standard `testing` package (unit/integration tests)
- Ginkgo v2 v2.22.0 (BDD-style controller integration tests)
- Config: `internal/controller/core/suite_test.go`, `internal/controller/glbc_controller/suite_test.go`, etc.

**Assertion Library:**
- `github.com/stretchr/testify v1.11.1` — used in unit and integration tests via `assert` and `require`
- `github.com/onsi/gomega v1.36.0` — used in Ginkgo suites via `Expect`, `Eventually`, `HaveLen`

**Run Commands:**
```bash
make test                  # Run all non-e2e tests with envtest assets + coverage
go test ./...              # Run all tests (may need KUBEBUILDER_ASSETS set)
go test ./test/e2e/ -v -ginkgo.v   # Run e2e tests against a live cluster
```

## Test File Organization

**Location:**
- Unit tests are co-located with source files (same package, `_test.go` suffix)
- Controller integration suite tests live inside the controller package directory
- E2E tests live in `test/e2e/`
- Integration tests using `envtest` live co-located with their use-case packages (e.g., `internal/usecase/nsg_uc/nsg_uc_integration_test.go`)

**Naming:**
- `<subject>_test.go` — matches the file under test where possible
- `suite_test.go` — Ginkgo bootstrap files for controller packages
- `helpers_test.go` — helper functions for Ginkgo controller suites

**Structure:**
```
internal/
  controller/
    core/
      suite_test.go              # Ginkgo bootstrap + envtest setup
      service_controller_test.go # Ginkgo Describe/It specs
      helpers_test.go            # Test helper functions
    glbc_controller/
      suite_test.go
      glbc_controller_test.go
      helpers_test.go
  usecase/
    lbc_uc/
      validate_test.go           # Table-driven unit tests
      deploy_listener_test.go    # Commented-out testify unit tests
    service_uc/
      build_lbc_test.go          # Table-driven unit tests
      build_pool_test.go
    nsg_uc/
      nsg_uc_integration_test.go # envtest integration test (non-Ginkgo)
  repository/
    vngcloud_repo/
      vngcloud_mocks/
        vngcloud_mock.go          # Stateful in-memory mock VNG Cloud provider
        mocks.go                  # Shared mock fixtures (nodes, IDs, constants)
test/
  e2e/
    e2e_suite_test.go            # Ginkgo E2E bootstrap
    e2e_test.go                  # E2E specs
```

## Test Structure

**Unit Test Suite Organization (testify):**
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name        string
        // inputs
        annotations map[string]string
        // expected outputs
        expectedName string
        expectError  bool
        // mock setup
        setupMocks   func(vngcloud *repository.MockVngCloudRepository, k8s *repository.MockK8sRepository)
    }{
        {
            name: "descriptive_snake_case_name",
            // ...
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // arrange
            // act
            // assert
            assert.Equal(t, tt.expectedName, result, tt.description)
        })
    }
}
```

**Ginkgo Controller Suite Organization:**
```go
var _ = Describe("Service Controller", func() {
    AfterEach(func() {
        // Clean up all k8s resources
        cleanupAllServices()
        cleanupAllLBCs()
    })

    Context("When creating a LoadBalancer service", func() {
        It("should create LBC, LoadBalancer and SecurityGroup", func() {
            // arrange: create k8s resources via k8sClient
            Expect(k8sClient.Create(ctx, service)).Should(Succeed())

            // assert: use Eventually for async reconciliation
            Eventually(func(g Gomega) {
                lbcList, err := getLBCListForService(serviceName, namespace)
                g.Expect(err).ShouldNot(HaveOccurred())
                g.Expect(lbcList.Items).Should(HaveLen(1))
            }, timeout, interval).Should(Succeed())
        })
    })
})
```

**Patterns:**
- Setup: `BeforeSuite` bootstraps envtest environment, registers CRDs, creates reconcilers
- Teardown: `AfterEach` cleans all k8s resources; `AfterSuite` stops envtest
- Assertions: `assert.Equal`, `assert.NoError`, `assert.Error`, `assert.Contains` for unit tests; `Expect`/`Eventually` for controller tests

## Mocking

**Framework:** Two complementary approaches are used:

1. **`mockery`-generated interface mocks** (`internal/repository/mocks.go`, `internal/usecase/mocks.go`) — generated via `github.com/vektra/mockery`, implements `testify/mock`

2. **Stateful in-memory `MockProvider`** (`internal/repository/vngcloud_repo/vngcloud_mocks/vngcloud_mock.go`) — hand-written mock that fully implements `repository.VngCloudRepository`, simulates VNG Cloud API state (load balancers, listeners, pools, security groups, subnets) with in-memory maps, used in controller integration and envtest tests

**Mockery-generated mock pattern:**
```go
mockVngcloudRepo := repository.NewMockVngCloudRepository(t)

mockVngcloudRepo.EXPECT().
    GetLoadBalancerByID(mock.Anything, "lb-12345").
    Return(loadBalancer, nil)

mockVngcloudRepo.EXPECT().
    ListPool(mock.Anything, "lb-12345").
    Return(&entity.ListPools{Items: []*entity.Pool{}}, nil)
```

**Stateful MockProvider pattern (controller integration tests):**
```go
vngcloudRepo = vngcloud_mocks.NewMockProvider()
err = vngcloudRepo.Init(nil)
// MockProvider satisfies repository.VngCloudRepository and tracks state
// in memory — no EXPECT() calls needed
```

**What to Mock:**
- VNG Cloud API calls (all external HTTP calls via `repository.VngCloudRepository`)
- k8s repository calls when unit-testing use-case logic in isolation (`repository.MockK8sRepository`)
- CNI detector (`utils.MockCniDetector`)

**What NOT to Mock:**
- Real k8s client interactions in envtest/Ginkgo tests — use the live `k8sClient` against the envtest API server

## Fixtures and Factories

**Test Data:**
```go
// Shared mock constants and node fixtures in vngcloud_mocks/mocks.go
const (
    MockProjectID    = "projectID"
    MockSubnetID     = "subnetID-hcm-1a"
    MockL4PackageName = "NLB_Small"
    MockLBNameError  = "error-lb" // triggers error path in MockProvider
)

var MockNode1 = &corev1.Node{ /* ... */ }
```

**Location:**
- Shared mock fixtures (nodes, IDs, constants): `internal/repository/vngcloud_repo/vngcloud_mocks/mocks.go`
- Stateful mock provider: `internal/repository/vngcloud_repo/vngcloud_mocks/vngcloud_mock.go`
- GLB-specific mock fixtures: `internal/repository/vngcloud_repo/vngcloud_mocks/mock_glbc_fixtures.go`

## Coverage

**Requirements:** No enforced threshold configured; `cover.out` is generated but not gated

**View Coverage:**
```bash
make test          # generates cover.out
go tool cover -html=cover.out   # view HTML coverage report
```

## Test Types

**Unit Tests (testify, table-driven):**
- Scope: individual functions/methods within a package, same package declaration
- Files: `build_lbc_test.go`, `validate_test.go`, `build_pool_test.go`, `build_sec_group_test.go`, `build_lbc_subnet_zone_test.go`, `reconcile_test.go`, `errors_requeue_test.go`, etc.
- Approach: construct structs directly (no controller wiring), inject mock repositories via `repository.NewMockVngCloudRepository(t)`

**Integration Tests (envtest, testify or Ginkgo):**
- Scope: controller reconcile loop against a real Kubernetes API server (envtest), with `MockProvider` for VNG Cloud
- Files: `internal/controller/core/service_controller_test.go`, `internal/controller/glbc_controller/glbc_controller_test.go`, `internal/usecase/nsg_uc/nsg_uc_integration_test.go`
- Approach: spin up envtest in `BeforeSuite`/`TestXxx`, create CRDs from `config/crd/bases`, register and start reconcilers, interact via `k8sClient`, assert async behavior via `Eventually`

**E2E Tests:**
- Framework: Ginkgo v2
- Location: `test/e2e/`
- Approach: run against a live cluster (not envtest); requires external setup

## Common Patterns

**Async Reconciliation Testing:**
```go
const (
    timeout  = time.Second * 5
    interval = time.Millisecond * 250
)

Eventually(func(g Gomega) {
    lbc := &v1alpha1.LoadBalancerConfig{}
    err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, lbc)
    g.Expect(err).ShouldNot(HaveOccurred())
    g.Expect(lbc.Status.LoadBalancerId).ShouldNot(BeNil())
}, timeout, interval).Should(Succeed())
```

**Error Path Testing:**
```go
// Unit test: assert error returned
assert.Error(t, err)
assert.Contains(t, err.Error(), "load balancer entity is nil")

// Unit test: assert no error on not-found (controller should ignore)
assert.NoError(t, err)
```

**Envtest Binary Discovery:**
```go
// suite_test.go pattern — locates envtest binaries under bin/k8s/
// Run 'make setup-envtest' before running controller integration tests
func getFirstFoundEnvTestBinaryDir() string {
    basePath := filepath.Join("..", "..", "..", "bin", "k8s")
    // ...
}
```

**Running `nsg_uc` Integration Tests Directly (without `make test`):**

`nsg_uc_integration_test.go` uses envtest and requires the kubebuilder binaries. The test
locates them via `KUBEBUILDER_ASSETS` — it must be an **absolute path**, not a relative one.

```bash
# Step 1: download envtest binaries (one-time, idempotent)
./bin/setup-envtest use 1.31.0 --bin-dir ./bin/k8s

# Step 2: run the test with the absolute asset path
KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/k8s/1.31.0-linux-amd64" \
  go test ./internal/usecase/nsg_uc/... -v
```

The binaries land at `bin/k8s/k8s/1.31.0-linux-amd64/{etcd,kube-apiserver,kubectl}`.
Using a relative path (e.g. the output of `setup-envtest ... -p path` without `$(pwd)`) causes
envtest to fail with `fork/exec bin/k8s/.../etcd: no such file or directory` because the test
binary changes its working directory at startup.

**Commented-Out Tests:**
Many test files in `internal/usecase/lbc_uc/` contain fully-written tests that are commented out (e.g., `lbc_uc_test.go`, `deploy_listener_test.go`, `deploy_pool_test.go`). These are valid testify tests that were disabled, likely due to API changes. When re-enabling them, uncomment and verify import paths still match current interfaces.

---

*Testing analysis: 2026-03-31*

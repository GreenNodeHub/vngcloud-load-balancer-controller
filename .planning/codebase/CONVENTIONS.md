# Coding Conventions

**Analysis Date:** 2026-03-15

## Naming Patterns

**Files:**
- Lowercase with underscores: `reconcile.go`, `deploy_lb.go`, `build_glbc.go`
- Test files follow package naming: `reconcile_test.go`, `deploy_lb_test.go`
- Mock files follow pattern: `mocks.go` for generated mocks, `*_mock_*.go` for repository mocks
- Package names are lowercase, often abbreviated (e.g., `lbc_uc`, `vglb_uc`, `nsg_uc`)

**Functions:**
- PascalCase for exported functions: `NewLoadBalancerConfigUseCase()`, `HandleReconcileError()`
- camelCase for unexported functions: `deployLoadBalancer()`, `validateCrossLBCs()`, `buildLoadBalancerName()`
- Test functions follow pattern: `TestFunctionName()` (standard Go testing)
- Receiver methods often abbreviated: `(t *defaultModelDeployTask)`, `(uc *vglbUseCase)`

**Variables:**
- camelCase for local variables: `lbId`, `errorNewLbIdObtained`, `createdCerts`, `newCreatedListeners`
- Package-level variables use camelCase: `setupLog`, `conf`, `scheme`
- Constants use UPPER_SNAKE_CASE for domain constants: `SERVICE_ANNOTATION_PREFIX`, `INGRESS_ANNOTATION_PREFIX`, `VGLB_ANNOTATION_PREFIX`

**Types:**
- PascalCase for struct and interface names: `LoadBalancerConfig`, `VngCloudRepository`, `defaultModelDeployTask`, `vglbUseCase`
- Interfaces use descriptive names: `K8sRepository`, `VngCloudRepository`, `Parser`, `EndpointResolver`
- Private struct names follow lowercase pattern with descriptive suffixes: `defaultModelDeployTask`

## Code Style

**Formatting:**
- Standard Go fmt style enforced
- Line length limit: 120 characters (checked by lll linter, exceptions in api/ and internal/)
- No trailing whitespace

**Linting:**
- Tool: golangci-lint v1.59.1
- Config: `.golangci.yml`
- Key enabled linters:
  - `gofmt` - code formatting
  - `goimports` - import organization
  - `govet` - code correctness
  - `revive` - style checks (includes comment-spacing)
  - `errcheck` - error handling
  - `staticcheck` - static code analysis
  - `ginkgolinter` - Ginkgo test framework linting
  - `goconst` - duplicated constants
  - `gocyclo` - cyclomatic complexity

**Exclusions:**
- `api/*` - excluded from `lll` (line length)
- `internal/*` - excluded from `dupl` (duplication) and `lll`

**Run Formatting:**
```bash
make fmt      # Run go fmt
make lint     # Run golangci-lint
make lint-fix # Run golangci-lint with fixes
```

## Import Organization

**Order:**
1. Standard library imports (e.g., `context`, `fmt`, `testing`)
2. Third-party imports (e.g., `github.com/pkg/errors`, `github.com/sirupsen/logrus`)
3. Kubernetes/controller-runtime imports (e.g., `k8s.io/...`, `sigs.k8s.io/...`)
4. VngCloud SDK imports (e.g., `github.com/vngcloud/vngcloud-go-sdk/v2/...`)
5. Local project imports (e.g., `github.com/vngcloud/vngcloud-load-balancer-controller/...`)

**Example from `internal/usecase/lbc_uc/deploy_lb.go`:**
```go
import (
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/inter"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/config"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
)
```

**Path Aliases:**
- Used for disambiguation when multiple packages have similar names: `loadbalancerv2` (as opposed to `inter`)
- Kubernetes types aliased: `corev1 "k8s.io/api/core/v1"`, `networkingv1 "k8s.io/api/networking/v1"`
- Global namespace imports only for Ginkgo test framework: `. "github.com/onsi/ginkgo/v2"` (with `//nolint:golint,revive` comment)

## Error Handling

**Patterns:**
- Error wrapping with `github.com/pkg/errors` for context: `return errors.New("no nodes found in cluster")`
- Error type assertions for custom error types: `errors.As(err, &requeueNeeded)`
- Early returns for error cases: check and return immediately
- Custom error types for domain-specific errors: `RequeueNeeded`, `RequeueNeededAfter` in `pkg/errs/`

**Example from `pkg/errs/reconcile.go`:**
```go
func HandleReconcileError(err error, log *logrus.Entry) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeueNeeded *RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		log.Info("requeue immediately reason: ", requeueNeeded.Reason())
		return ctrl.Result{Requeue: true}, nil
	}

	log.Infof("requeue after 5 seconds + exponential back-off, reason: %v", err)
	time.Sleep(5 * time.Second)
	return ctrl.Result{}, err
}
```

**Context Handling:**
- Context passed as first parameter: `func (t *defaultModelDeployTask) deploy(ctx context.Context) error`
- Context used for timeouts and cancellation in async operations
- Logger extracted from context when available: `logger := contexts.NewContext(ctx).Log()` (using operator-helper)

## Logging

**Framework:**
- Primary: `github.com/sirupsen/logrus` with structured fields
- Secondary: `sigs.k8s.io/controller-runtime/pkg/log/zap` for controller-runtime integration

**Patterns:**
- Entry-based logging with fields: `logger := contexts.NewContext(ctx).Log()` returns `*logrus.Entry`
- Log levels: `Errorf()`, `Infof()`, `Warnf()`, `Debugf()`
- Error logging includes context: `logger.Errorf("failed to list nodes: %v", err)`
- Info logging for significant operations: `logger.Info("requeue immediately reason: ", reason)`

**Example from `internal/usecase/vglb_uc/vglb_uc.go`:**
```go
func (uc *vglbUseCase) InitVngcloudGlobalLoadBalancerUseCase(ctx context.Context) error {
	logger := contexts.NewContext(ctx).Log()

	err := uc.k8sRepo.ListNode(ctx, nodes)
	if err != nil {
		logger.Errorf("failed to list nodes: %v", err)
		return err
	}
}
```

## Comments

**When to Comment:**
- Non-obvious logic or design decisions
- Exported functions should have doc comments (enforced by revive linter)
- Complex business logic within functions
- Workarounds or temporary solutions (mark with TODO/FIXME)

**JSDoc/TSDoc:**
- Go uses doc comment format: `// FunctionName describes the function.`
- Multi-line doc comments for longer descriptions:
  ```go
  // deployLoadBalancer creates or ensures the load balancer exists, handling migrations if necessary.
  // It returns the load balancer ID to be used for further operations.
  // The caller should requeue if a new load balancer ID is obtained to acquire the appropriate lock.
  func (t *defaultModelDeployTask) deployLoadBalancer(ctx context.Context, createdCerts []v1alpha1.CreatedCertificate) (string, error)
  ```

**Example from `internal/usecase/lbc_uc/deploy_lb.go`:**
```go
// deployLoadBalancer creates or ensures the load balancer exists, handling migrations if necessary.
// It returns the load balancer ID to be used for further operations.
// The caller should requeue if a new load balancer ID is obtained to acquire the appropriate lock.
func (t *defaultModelDeployTask) deployLoadBalancer(ctx context.Context, createdCerts []v1alpha1.CreatedCertificate) (string, error) {
	errorNewLbIdObtained := errs.NewRequeueNeeded("new load balancer ID obtained, requeue needed")
	// ...
}
```

## Function Design

**Size:**
- Functions typically 30-100+ lines for use case implementations
- Task-oriented methods: large functions decomposed into smaller internal methods
- Example: `deploy()` calls `validateSelf()`, `deployCerts()`, `deployLoadBalancer()`, etc.

**Parameters:**
- Context as first parameter (Go standard)
- Receiver for methods: typically struct pointer with abbreviated name: `(t *defaultModelDeployTask)`, `(uc *vglbUseCase)`
- Dependency injection through struct fields, not function parameters
- Minimal parameter lists (max 3-4 parameters, often fewer)

**Return Values:**
- Error as last return value (Go convention)
- Single meaningful return value + error: `(string, error)`, `(*entity.LoadBalancer, error)`
- Multiple return values indicate related results: `(zoneID common.Zone, networkId, subnetID, subnetCIDR string, err error)` for server network info
- Boolean + error not used; use error types or empty struct returns instead

**Example from `internal/usecase/vglb_uc/build_glbc.go`:**
```go
func (t *defaultModelBuildTask) buildLoadBalancerId(_ context.Context) *string {
	var option string
	_ = t.annotationParser.ParseStringAnnotation(annotations.SuffixLoadBalancerID, &option, t.vglb.Annotations)
	if option != "" {
		return &option
	}
	return nil
}
```

## Module Design

**Exports:**
- Use case constructors are exported: `NewLoadBalancerConfigUseCase()`, `NewVngcloudGlobalLoadBalancerUseCase()`
- Interface implementations are unexported: `type vglbUseCase struct`
- Domain constants exported: `SERVICE_ANNOTATION_PREFIX`, `VGLB_ANNOTATION_PREFIX`
- Test files generate mocks via mockery: `NewMockVngCloudRepository()` (generated)

**Barrel Files:**
- Not heavily used; most packages have single responsibility
- `internal/domain/domain.go` exports constants
- `internal/repository/contracts.go` exports interfaces (K8sRepository, VngCloudRepository)

**Example Export Pattern from `internal/usecase/vglb_uc/vglb_uc.go`:**
```go
func NewVngcloudGlobalLoadBalancerUseCase(
	cfg *config.Config,
	k8sRepo repository.K8sRepository,
	vngcloudRepo repository.VngCloudRepository,
	annotationParser annotations.Parser,
	endpointResolver utils.EndpointResolver,
) usecase.VngcloudGlobalLoadBalancerUseCase {
	return &vglbUseCase{
		cfg:              cfg,
		k8sRepo:          k8sRepo,
		vngcloudRepo:     vngcloudRepo,
		annotationParser: annotationParser,
		endpointResolver: endpointResolver,
	}
}

type vglbUseCase struct {
	cfg              *config.Config
	k8sRepo          repository.K8sRepository
	vngcloudRepo     repository.VngCloudRepository
	annotationParser annotations.Parser
	endpointResolver utils.EndpointResolver
	// ...
}
```

---

*Convention analysis: 2026-03-15*

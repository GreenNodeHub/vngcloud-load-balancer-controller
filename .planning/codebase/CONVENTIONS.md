# Coding Conventions

**Analysis Date:** 2026-03-31

## Naming Patterns

**Files:**
- Snake_case for multi-word files: `build_lbc.go`, `deploy_listener.go`, `node_utils.go`
- Test files use `_test.go` suffix co-located with source: `build_lbc_test.go`
- Mock files named `mocks.go` (generated) or `<domain>_mocks.go` in their own package
- Generated files prefixed `zz_generated.` (e.g., `zz_generated.deepcopy.go`)

**Packages:**
- Snake_case package names for multi-word: `service_uc`, `lbc_uc`, `glbc_uc`, `vngcloud_repo`
- Package name matches the directory leaf name
- Mock packages in a sub-directory: `vngcloud_repo/vngcloud_mocks`

**Functions/Methods:**
- PascalCase for exported: `NewServiceUseCase`, `HandleReconcileError`, `ExtractNodeInstanceID`
- camelCase for unexported: `buildPool`, `deployListener`, `getTargetType`
- Constructor pattern: `NewXxx(...)` returns interface type
- Build methods on task structs: `(t *defaultModelBuildTask) buildXxx(...)`
- Deploy methods on task structs: `(t *defaultModelDeployTask) deployXxx(...)`

**Structs and Interfaces:**
- PascalCase for exported: `ServiceUseCase`, `VngCloudRepository`, `LoadBalancerConfigReconciler`
- camelCase for unexported implementations: `serviceUseCase`, `defaultModelBuildTask`, `defaultModelDeployTask`
- Interface contract files are named `contracts.go` at the package root
- Unexported structs implement exported interfaces (e.g., `serviceUseCase` implements `usecase.ServiceUseCase`)

**Constants:**
- UPPER_SNAKE_CASE for "configuration" constants: `SERVICE_ANNOTATION_PREFIX`, `DEFAULT_LB_PREFIX_NAME`
- PascalCase for typed-constant groups: `TargetTypeInstance`, `TargetTypeIP`
- Annotation suffix constants use kebab-case string values: `"load-balancer-name"`, `"pool-algorithm"`

**Variables:**
- camelCase for local variables and struct fields: `defaultNetworkId`, `clusterId`, `annotationParser`
- Cluster config struct field tags use `mapstructure` for viper binding

## Code Style

**Formatting:**
- `gofmt` enforced via golangci-lint formatter
- `goimports` enforced — imports are auto-grouped and sorted

**Linting:**
- Config: `.golangci.yml` (golangci-lint v2)
- Enabled linters: `dupl`, `errcheck`, `ginkgolinter`, `goconst`, `gocyclo`, `govet`, `ineffassign`, `lll`, `misspell`, `nakedret`, `prealloc`, `revive`, `staticcheck`, `unconvert`, `unparam`, `unused`
- `lll` (line length) exclusions for `api/*` and `internal/*`
- `dupl` exclusion for `internal/*`
- `revive` enforces `comment-spacings` rule

**Suppressions:**
- `//nolint:gocyclo` on complex builder/deploy methods when unavoidable
- `//nolint:unparam` on some internal deploy/build request builders
- `//nolint:lll` inline on long struct tag lines in `pkg/config/config.go`

## Import Organization

**Order (enforced by goimports):**
1. Standard library
2. Third-party packages
3. Internal project packages (`github.com/vngcloud/vngcloud-load-balancer-controller/...`)

**Dot imports:**
- Used exclusively in Ginkgo/Gomega test files: `. "github.com/onsi/ginkgo/v2"`, `. "github.com/onsi/gomega"`
- Never in production code

**Path Aliases:**
- `ctrl "sigs.k8s.io/controller-runtime"` — standard alias throughout
- `corev1 "k8s.io/api/core/v1"` — standard alias throughout
- `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` — standard alias throughout
- `loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"`
- `entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"`

## Error Handling

**Libraries:**
- `github.com/pkg/errors` — used for `errors.New`, `errors.Wrap`, `errors.Errorf`, `errors.As`
- Standard `errors` package — used in high-level usecase files for `errors.As` type checks
- `fmt.Errorf` — used occasionally in deploy methods

**Patterns:**
- Errors propagate up via `return ..., err` without wrapping at every layer
- `errors.Wrap(err, "context message")` when adding context at a boundary
- `errors.Errorf("format %s", val)` for formatted error creation
- Custom error types (`RequeueNeeded`, `RequeueNeededAfter`) in `pkg/errs/` for reconcile flow control
- `errs.HandleReconcileError(err, logger)` at the reconciler boundary to classify errors
- Nil guard checks precede operations (e.g., check `entity is nil` before dereferencing)

## Logging

**Framework:** `github.com/sirupsen/logrus` (primary), `sigs.k8s.io/controller-runtime/pkg/log/zap` (manager-level)

**Patterns:**
- Usecase/task methods receive a `*logrus.Entry` as `t.logger` or `uc.logger`
- Use `t.logger.Infof(...)`, `t.logger.Debugf(...)`, `t.logger.Warnf(...)`, `t.logger.Errorf(...)`
- Log format: structured with `logrus.TextFormatter` in test suites, default JSON in production
- Log caller enabled in test suites (`logrus.SetReportCaller(true)`)
- `ctrl.Log.WithName("...")` for controller-runtime manager-level logging

## Comments

**When to Comment:**
- Exported functions/types: godoc-style single-line or block comments
- Non-obvious business logic: inline comments explaining the "why"
- Bug fixes: comment referencing the bug ID (e.g., `// TestDeployListener_PopulatesName verifies BUG-04:`)
- TODO markers: `// TODO: description` for known deferred work (used frequently)

**Godoc Style:**
- Exported interfaces in `contracts.go` files often have no godoc (convention not enforced)
- `// HandleReconcileError will handle errors...` style comments on key exported functions

## Function Design

**Size:** Large complex functions (400–500+ lines) exist in deploy/build files (`deploy_lb.go`, `build_pool.go`); `//nolint:gocyclo` applied where unavoidable.

**Parameters:** Context `ctx context.Context` is always the first parameter on functions doing I/O.

**Return Values:**
- Pure builders return `(value, error)`
- Void operations return `error` only
- Constructors return an interface type (not concrete struct pointer)

## Module Design

**Exports:**
- Each package exposes a constructor (`NewXxx`) and interface types
- Concrete structs (`serviceUseCase`, `defaultModelBuildTask`) are unexported

**Contracts Pattern:**
- Every domain boundary (usecase, repository) has a `contracts.go` defining its interfaces
- Mocks are generated by `mockery` (vektra) and kept in the same package as the interface, or in a `_mocks` sub-package for vngcloud-specific test infrastructure

**Barrel Files:** Not used. Import the specific package directly.

---

*Convention analysis: 2026-03-31*

# Architecture

**Analysis Date:** 2026-03-31

## Pattern Overview

**Overall:** Clean Architecture / Use Case pattern within a Kubernetes controller-runtime operator

**Key 
Characteristics:**
- Strict separation into Controller → UseCase → Repository layers, each communicating only through interfaces
- All layer boundaries enforced by Go interfaces defined in `internal/usecase/contracts.go` and `internal/repository/contracts.go`
- Seven independent reconciler controllers, each wired up in `cmd/main.go` at startup via dependency injection
- Domain constants and error types centralized in `internal/domain/`; no domain structs — domain is expressed through Kubernetes CRD types in `api/v1alpha1/`

## Layers

**Controller Layer:**
- Purpose: Watches Kubernetes resources, enqueues reconcile requests, delegates all business logic to UseCase
- Location: `internal/controller/`
- Contains: `*Reconciler` structs, `SetupWithManager()`, event handler registration via `eventhandlers/` subpackages
- Depends on: `internal/usecase` interfaces, `pkg/k8s`, `pkg/metrics`, `pkg/errs`
- Used by: `cmd/main.go` (wired up at startup)

**UseCase Layer:**
- Purpose: Implements all reconciliation business logic (ensure, delete, init); orchestrates repository calls to build desired state vs actual state
- Location: `internal/usecase/`
- Contains: `*UseCase` structs per feature, `build_*.go` helpers that construct API request objects, `deploy_*.go` helpers that call vngcloud_repo
- Depends on: `internal/repository` interfaces, `api/v1alpha1`, `pkg/annotations`, `pkg/config`, `pkg/errs`
- Used by: Controller layer only

**Repository Layer:**
- Purpose: Abstracts all I/O — Kubernetes API reads/writes via `K8sRepository` and VNGCloud API calls via `VngCloudRepository`
- Location: `internal/repository/`
- Contains: Interface definitions in `internal/repository/contracts.go`; implementations in `internal/repository/k8s_repo/` and `internal/repository/vngcloud_repo/`
- Depends on: `github.com/vngcloud/vngcloud-go-sdk/v2`, `sigs.k8s.io/controller-runtime/pkg/client`
- Used by: UseCase layer only

**Domain Layer:**
- Purpose: Shared constants, finalizer strings, annotation prefixes, error sentinel values, icon strings
- Location: `internal/domain/`
- Contains: `domain.go` (constants, finalizers, labels), `error.go` (sentinel errors and error predicate helpers)
- Depends on: nothing internal
- Used by: All layers

**API / CRD Types:**
- Purpose: Defines the Kubernetes CRD schemas managed by this controller
- Location: `api/v1alpha1/`
- Contains: `LoadBalancerConfig`, `GlobalLoadBalancerConfig`, `NodeSecurityGroup`, `VngcloudGlobalLoadBalancer` types
- Depends on: `k8s.io/apimachinery`, `vngcloud-go-sdk/v2` (for field types)
- Used by: Repository and UseCase layers

**Shared Packages:**
- Purpose: Cross-cutting utilities consumed by multiple layers
- Location: `pkg/`
- Contains: `pkg/annotations` (parser), `pkg/config` (Config struct + viper loading), `pkg/errs` (requeue error types + reconcile error handler), `pkg/k8s` (FinalizerManager, node utils), `pkg/metrics/lbc` (Prometheus metrics), `pkg/logging` (logrus setup), `pkg/clusterapi` (remote cluster kubeconfig), `pkg/utils` (CNI detector, endpoint resolver)

## Data Flow

**Service/Ingress Reconciliation:**

1. Kubernetes event triggers `eventhandlers/` handler (e.g., `enqueueRequestsForServiceEvent`)
2. Handler filters irrelevant changes and enqueues `reconcile.Request`
3. `ServiceReconciler.Reconcile()` in `internal/controller/core/service_controller.go` receives the request
4. Reconciler delegates to `serviceUseCase.EnsureServiceUseCase()` or `DeleteServiceUseCase()`
5. UseCase in `internal/usecase/service_uc/service_uc.go` calls `k8sRepo` to fetch the Service and nodes
6. UseCase calls `vngcloudRepo` to fetch/create/update VNGCloud load balancer resources (LB, pools, listeners, policies)
7. UseCase updates Service status address via `k8sRepo.UpdateServiceStatusAddress()`
8. Reconciler translates UseCase errors into `ctrl.Result` using `pkg/errs.HandleReconcileError()`

**LoadBalancerConfig Reconciliation:**

1. `lbc_controller` watches `LoadBalancerConfig` CRD changes
2. `lbcUseCase.EnsureLoadBalancerConfigUseCase()` calls `ensure()` → `defaultModelDeployTask.deploy()`
3. Deploy task runs: validate → deploy certs → deploy LB → validate cross-LBCs → deploy pools → deploy listeners → deploy policies → deploy tags
4. Status (`ObservedGeneration`, `LastReconcileMessage`, `Ready` condition) is patched via `k8sRepo.PatchMutateStatusLoadBalancerConfig()`

**State Management:**
- No in-memory shared state between reconcile loops (each reconcile is stateless)
- `lbcUseCase` holds a `sync.Map` of per-LB-ID mutexes to prevent concurrent `LoadBalancerConfig` objects sharing the same LB from racing
- `ServiceReconciler` uses `atomic.Bool initDone` — if init fails (no nodes yet), reconcile requeues every 1 second until cluster is ready

## Key Abstractions

**UseCase Interfaces:**
- Purpose: Decouple controllers from business logic; enables mock injection in tests
- Definition: `internal/usecase/contracts.go`
- Pattern: Each UseCase exposes exactly three methods — `Init*`, `Ensure*`, `Delete*`
- Examples: `usecase.ServiceUseCase`, `usecase.LoadBalancerConfigUseCase`, `usecase.IngressUseCase`

**Repository Interfaces:**
- Purpose: Decouple business logic from I/O backends
- Definition: `internal/repository/contracts.go`
- Examples: `repository.VngCloudRepository` (VNGCloud API calls), `repository.K8sRepository` (Kubernetes API calls)
- Mocks: Generated with mockery in `internal/repository/vngcloud_repo/vngcloud_mocks/` and `internal/usecase/mocks.go`

**FinalizerManager:**
- Purpose: Manages Kubernetes finalizers with retry-on-conflict semantics
- Location: `pkg/k8s/finalizer.go`
- Pattern: Interface `k8s.FinalizerManager` injected into every reconciler

**EventHandlers:**
- Purpose: Filter Kubernetes watch events before enqueuing reconcile requests (e.g., skip no-op updates)
- Location: `internal/controller/<name>/eventhandlers/`
- Pattern: Implement `handler.EventHandler` interface from controller-runtime

**AnnotationParser:**
- Purpose: Reads typed values from Kubernetes annotations using suffix-based parsing
- Location: `pkg/annotations/parser.go`
- Pattern: `annotations.NewSuffixAnnotationParser(prefix)` returns a `Parser` interface consumed by UseCase

## Entry Points

**Main:**
- Location: `cmd/main.go`
- Triggers: Binary startup; Kubernetes pod lifecycle
- Responsibilities: Parses flags, loads config from `/etc/vngcloud-load-balancer-controller/config.yaml`, optionally resolves remote cluster kubeconfig via ClusterAPI, installs CRDs, wires up all repositories/usecases/reconcilers via dependency injection, starts controller-runtime Manager

**Controller SetupWithManager:**
- Location: Each `internal/controller/<name>/<name>_controller.go` implements `SetupWithManager(ctx, mgr)`
- Triggers: Called from `cmd/main.go` at startup
- Responsibilities: Registers reconciler with the manager, registers event handlers for watched resources

## Error Handling

**Strategy:** Sentinel error types that carry requeue intent, translated at the controller boundary

**Patterns:**
- `pkg/errs.RequeueNeededAfter` — requeue after a specific duration (e.g., waiting for LB to become active)
- `pkg/errs.RequeueNeeded` — requeue immediately
- All other errors — controller-runtime exponential backoff; `HandleReconcileError()` adds a 5-second sleep before returning the error to the work queue
- `domain.IgnoreErrors()` — convenience wrapper to suppress specific sentinel errors (e.g., not-found errors during cleanup)
- Repository layer exposes predicate helpers: `domain.IsLoadBalancerNotFound()`, `domain.IsSecurityGroupNotFound()`, etc.

## Cross-Cutting Concerns

**Logging:** Dual logging — `controller-runtime` zap logger (structured, used in controller layer) + logrus (used in usecase/pkg layers via `pkg/logging`). Context-scoped log entries via `github.com/anngdinh/operator-helper/contexts`.

**Validation:** Inline in UseCase layer; `validate.go` and `validate_test.go` files exist in `lbc_uc` and `ingress_uc`. Cross-resource validation (e.g., multiple LBCs sharing one LB) handled inside the deploy task.

**Authentication:** VNGCloud API auth via client credentials (ClientID/Secret) configured in `pkg/config.AuthOpts`; optional super-client credentials for INTERVPC load balancers. Kubernetes auth via controller-runtime's standard in-cluster config.

**Metrics:** Prometheus metrics via `pkg/metrics/lbc.MetricCollector`; injected into every reconciler; tracks reconcile latency, cache size, and top-talker requests.

---

*Architecture analysis: 2026-03-31*

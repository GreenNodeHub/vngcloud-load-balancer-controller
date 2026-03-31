# Technology Stack

**Analysis Date:** 2026-03-31

## Languages

**Primary:**
- Go 1.25.0 (toolchain go1.25.8) - All application code, controllers, use-cases, repositories

**Secondary:**
- YAML - Kubernetes manifests, Helm chart templates, configuration files
- Makefile - Build automation

## Runtime

**Environment:**
- Container runtime: `gcr.io/distroless/static:nonroot` (production base image)
- Build image: `golang:1.25` (multi-stage Docker build)
- Binary runs as UID/GID 65532 (nonroot)

**Package Manager:**
- Go modules (`go.mod` / `go.sum`)
- Lockfile: `go.sum` present

## Frameworks

**Core:**
- `sigs.k8s.io/controller-runtime v0.19.3` - Kubernetes controller manager, reconciler scaffolding, leader election, healthz/readyz endpoints
- `k8s.io/apimachinery v0.31.3` - Kubernetes object types, runtime scheme
- `k8s.io/client-go v0.31.3` - Kubernetes client, informers, event recording
- `k8s.io/api v0.31.3` - Kubernetes core API types (Service, Ingress, Node, etc.)
- `k8s.io/apiextensions-apiserver v0.31.3` - CRD client for programmatic CRD installation at startup
- `sigs.k8s.io/cluster-api v1.9.1` - ClusterAPI client for remote cluster kubeconfig fetching

**CRD / Code Generation:**
- `controller-gen v0.16.1` (bin/controller-gen-v0.16.1) - Generates CRD manifests and DeepCopy methods from Go type annotations
- `kubebuilder` scaffolding pattern (PROJECT file present)

**Testing:**
- `github.com/onsi/ginkgo/v2 v2.22.0` - BDD-style test suite runner
- `github.com/onsi/gomega v1.36.0` - Assertion/matcher library
- `github.com/stretchr/testify v1.11.1` - Unit test assertions and mocking helpers
- `github.com/golang/mock v1.6.0` - Mock generation (used with `.mockery.yml` for mockery-generated mocks)
- `sigs.k8s.io/controller-runtime/tools/setup-envtest` (release-0.19) - Kubernetes envtest for integration tests (ENVTEST_K8S_VERSION=1.31.0)

**Build/Dev:**
- `kustomize v5.4.3` - Kubernetes manifest generation/overlay
- `golangci-lint v1.59.1` (local) / `v2.5.0` (CI via golangci-lint-action) - Static analysis
- `helmify v0.4.5` - Converts kustomize output to Helm chart
- `govulncheck` - Vulnerability scanning (via GitHub Actions)
- `gitleaks` - Secret scanning (via GitHub Actions)

## Key Dependencies

**Critical:**
- `github.com/vngcloud/vngcloud-go-sdk/v2 v2.17.4-0.20251225102644-877dacf16698` - VNGCloud platform SDK; provides VLB, VServer, GLB, portal, IAM clients; the primary integration point for all cloud resource management
- `github.com/anngdinh/operator-helper v0.0.8-0.20250606033238-e50b218b202c` - Internal helper for context/logging patterns used across the repository layer
- `github.com/cuongpiger/joat v1.0.17` - Internal utility library (URL normalization and other helpers used in SDK configuration)

**Infrastructure:**
- `github.com/prometheus/client_golang v1.19.1` - Prometheus metrics exposition; all controller metrics registered under subsystem `vngcloud_lbc`
- `github.com/spf13/viper v1.19.0` - Config file parsing (`/etc/vngcloud-load-balancer-controller/config.yaml`)
- `github.com/sirupsen/logrus v1.9.3` - Logrus structured logging (dual logging with zap from controller-runtime)
- `k8s.io/klog/v2 v2.130.1` - klog used in metadata utility package
- `github.com/blang/semver/v4 v4.0.0` - Semantic versioning (CRD version checking)
- `github.com/huandu/go-clone v1.7.2` - Deep cloning of objects in reconcile logic
- `github.com/pkg/errors v0.9.1` - Error wrapping with stack traces
- `golang.org/x/sync v0.19.0` - `errgroup` and sync primitives used in concurrent reconcile operations

**CVE Mitigations (pinned):**
- `github.com/imroc/req/v3 v3.57.0` - HTTP client CVE fix (GO-2024-3302)
- `github.com/quic-go/quic-go v0.57.1` - QUIC CVE fix (GO-2024-3302)

## Configuration

**Environment:**
- Configured via YAML file at `/etc/vngcloud-load-balancer-controller/config.yaml` (mounted from Kubernetes Secret `vngcloud-load-balancer-controller-mysecret`)
- Config read at startup via `viper.ReadInConfig()` in `pkg/config/config.go`
- `viper.AutomaticEnv()` is enabled - environment variables override YAML values

**Key Config Fields (`pkg/config/config.go`):**
- `global.identityURL` - VNGCloud IAM endpoint (e.g., `https://iamapis.vngcloud.vn/accounts-api`)
- `global.vserverURL` - VServer API endpoint (e.g., `https://hcm-3.api.vngcloud.vn/vserver`)
- `global.clientID` / `global.clientSecret` - OAuth2 credentials for VNGCloud SDK
- `global.projectID` / `global.userID` - Optional direct project/user IDs (skips metadata service lookup)
- `global.superClientID` / `global.superClientSecret` - Optional credentials for INTERVPC LB management
- `cluster.clusterID` / `cluster.namespace` / `cluster.region` - Cluster identity
- `cluster.isRunRemote` - Enables ClusterAPI remote cluster mode
- `loadBalancerOpts.*` - Default L4/L7 package names, pool algorithm, health check thresholds, listener timeouts
- `globalLoadBalancerOpts.*` - Same defaults for GLB resources
- `maxConcurrentReconciles` - Parallelism for reconcile loops
- `chartVersion` - Used in user-agent string and CRD version checks

**CLI Flags (binary):**
- `--metrics-bind-address` (default: `0`)
- `--health-probe-bind-address` (default: `:8081`)
- `--leader-elect` (default: `false`)
- `--metrics-secure` (default: `true`)
- `--log-level` (default: `info`)
- `--sync-period` (default: `5m`)
- `--disable-*-controller` flags for each of 7 controllers

**Build:**
- `Dockerfile` - Multi-stage build; builder stage uses `golang:1.25`, final stage `gcr.io/distroless/static:nonroot`
- Version and commit injected via ldflags: `-X github.com/vngcloud/vngcloud-load-balancer-controller/pkg/version.Version` and `.Commit`
- Target binary: `manager` at repo root

## Platform Requirements

**Development:**
- Go 1.25+ (toolchain 1.25.8)
- Docker (or compatible container tool) for image builds
- `kubectl` for deployment operations
- `kustomize` / `helm` for manifest generation

**Production:**
- Kubernetes 1.31.x cluster (envtest assets target 1.31.0)
- Deployed as a `Deployment` in `kube-system` namespace
- Requires a Kubernetes `Secret` mounted at `/etc/vngcloud-load-balancer-controller/config.yaml`
- `/etc/hosts` from host node is mounted read-only into the container

**Container Registries:**
- Production images pushed to `vcr.vngcloud.vn/81-vks-public/` (HCM) and `vcr-han.vngcloud.vn/81-vks-public/` (HAN)
- Also published to `ghcr.io/vngcloud/vngcloud-load-balancer-controller`
- Helm charts pushed as OCI artifacts to `vcr.vngcloud.vn/81-vks-public/vks-helm-charts`

---

*Stack analysis: 2026-03-31*

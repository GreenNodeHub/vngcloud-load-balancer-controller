# Technology Stack

**Analysis Date:** 2026-03-15

## Languages

**Primary:**
- Go 1.22.5 - Main controller implementation language

**Build:**
- Dockerfile (multi-stage) - Container image building

## Runtime

**Environment:**
- Kubernetes 1.31 (kubebuilder assets version) - Target deployment platform
- Container runtime - Distroless static image (gcr.io/distroless/static:nonroot)

**Package Manager:**
- Go Modules (go.mod/go.sum)
- Lockfile: Present (go.sum)

## Frameworks

**Core:**
- `sigs.k8s.io/controller-runtime` v0.19.3 - Kubernetes controller framework
- `k8s.io/client-go` v0.31.3 - Kubernetes client library
- `k8s.io/api` v0.31.3 - Kubernetes API definitions
- `k8s.io/apiextensions-apiserver` v0.31.3 - Custom Resource Definition support
- `sigs.k8s.io/cluster-api` v1.9.1 - Cluster management API

**Operators & Helpers:**
- `github.com/anngdinh/operator-helper` v0.0.8 - Custom operator utilities

**Testing:**
- `github.com/onsi/ginkgo/v2` v2.22.0 - BDD testing framework
- `github.com/onsi/gomega` v1.36.0 - Matcher library for Ginkgo
- `github.com/stretchr/testify` v1.10.0 - Additional test assertions
- `github.com/golang/mock` v1.6.0 - Code generation for mocks

**Build/Dev:**
- golangci-lint - Multi-linter aggregator
- envtest (kubebuilder) - Kubernetes environment testing

## Key Dependencies

**Critical:**
- `github.com/vngcloud/vngcloud-go-sdk/v2` v2.17.4 - VNGCloud API SDK for load balancer and infrastructure management
- `github.com/go-logr/logr` v1.4.2 - Structured logging interface
- `github.com/sirupsen/logrus` v1.9.3 - Logrus logger implementation
- `sigs.k8s.io/yaml` v1.4.0 - YAML parsing and manipulation
- `github.com/prometheus/client_golang` v1.19.1 - Prometheus metrics client
- `github.com/spf13/viper` v1.19.0 - Configuration management

**Infrastructure:**
- `github.com/cuongpiger/joat` v1.0.17 - Utility library for URL normalization
- `github.com/blang/semver/v4` v4.0.0 - Semantic versioning library
- `golang.org/x/sync` v0.10.0 - Synchronization primitives
- `github.com/pkg/errors` v0.9.1 - Error wrapping utilities
- `go.opentelemetry.io/*` - OpenTelemetry observability libraries (indirect)

**Kubernetes Utilities:**
- `k8s.io/apimachinery` v0.31.3 - Kubernetes API machinery
- `k8s.io/klog/v2` v2.130.1 - Kubernetes logging
- `k8s.io/utils` v0.0.0 - Kubernetes utility functions
- `sigs.k8s.io/structured-merge-diff/v4` v4.4.1 - Strategic merge patches

## Configuration

**Environment:**
- Configuration file location: `/etc/vngcloud-load-balancer-controller/config.yaml`
- Configuration management: Viper (YAML, environment variables)
- Metadata provider: Configurable metadata search order (configDriver, metadataService)

**Build:**
- `Makefile` - Build targets and orchestration
- `Dockerfile` - Multi-stage container build with caching
- Build flags: VERSION, COMMIT (passed via LDFLAGS)
- Container image registry: `vcr.vngcloud.vn/` (VNGCloud Container Registry)

## Platform Requirements

**Development:**
- Go 1.22.5
- Docker/Container tool for builds
- kubebuilder assets (1.31.0 for testing)
- golangci-lint for linting

**Production:**
- Kubernetes 1.29+ cluster
- VNGCloud credentials and endpoints accessible
- RBAC roles and CRDs installed via Kustomize
- Distroless container image runtime (nonroot user: 65532:65532)

---

*Stack analysis: 2026-03-15*

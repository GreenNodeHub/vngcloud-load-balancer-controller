# External Integrations

**Analysis Date:** 2026-03-15

## APIs & External Services

**VNGCloud Platform:**
- VNGCloud API Services - Orchestrates load balancer, network, and compute resources
  - SDK/Client: `github.com/vngcloud/vngcloud-go-sdk/v2` v2.17.4
  - Services: VLB Gateway, VServer Gateway, GLB Gateway (Global Load Balancer), Portal
  - Auth: Client ID and Secret from config (configurable super-client credentials for cross-VPC operations)
  - Base URLs configurable via config file (identity, vserver, glb endpoints)

**Kubernetes APIs:**
- Kubernetes API Server - Cluster resource management
  - Client: `k8s.io/client-go` v0.31.3
  - CRD: Custom Resource Definitions for LoadBalancerConfig, GlobalLoadBalancerConfig, VngcloudGlobalLoadBalancer, NodeSecurityGroup
  - Authentication: kubeconfig from KUBECONFIG env or in-cluster credentials

**Kubernetes Cluster API:**
- Cluster API - Multi-cluster management support
  - Client: `sigs.k8s.io/cluster-api` v1.9.1
  - Purpose: Retrieve target cluster kubeconfig when running in remote cluster mode
  - Config location: Retrieved from management cluster via cluster-api

## Data Storage

**Databases:**
- No external database required - State stored in Kubernetes etcd via CRDs

**File Storage:**
- Local filesystem only - Configuration files and certificates managed in-cluster via Kubernetes Secrets

**Caching:**
- In-memory caching (via controller-runtime cache)
- Sync period: Configurable (default 5 minutes) for resource reconciliation

## Authentication & Identity

**Auth Provider:**
- Custom OAuth2 via VNGCloud IAM
  - Implementation: VNGCloud Go SDK handles OAuth2 flow
  - Credentials: Client ID + Client Secret from config
  - Optional: Super-client credentials for inter-VPC load balancer management
  - Endpoint: Configurable IAM endpoint (identity-url in config)

**Cluster Authentication:**
- Kubernetes RBAC - Role-based access control for controller operations
- Service Account - Deployed controller runs with dedicated K8s service account
- Leader Election - Ensures single active controller instance via Kubernetes leases

## Monitoring & Observability

**Metrics:**
- Prometheus - Metrics collection and export
  - Client: `github.com/prometheus/client_golang` v1.19.1
  - Metrics endpoint: Configurable (default: :8443 HTTPS or :8080 HTTP)
  - Metrics collected:
    - Pod readiness gate latency: `pod_readiness_flip_seconds`
    - Controller reconciliation errors: Per controller and error type
    - Controller reconciliation latency: Per stage (validation, deployment, cleanup)
    - Webhook validation/mutation errors: Per webhook
    - Cache size monitoring
    - Top talkers (high-frequency reconcilers)
  - Secure serving: HTTPS with mTLS support (configurable)

**Logging:**
- Logrus - Structured logging implementation
  - Client: `github.com/sirupsen/logrus` v1.9.3
  - Log levels: debug, info, warn, error, fatal, panic (configurable via --log-level flag)
  - Default level: info
  - Integration: Controller-runtime logger integration via `go-logr/logr`

**Tracing:**
- OpenTelemetry - Observability instrumentation (indirect dependency)
  - Packages: `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/trace`
  - Purpose: Optional distributed tracing support for instrumented client libraries
  - Not actively used by core controller logic

## CI/CD & Deployment

**Hosting:**
- Kubernetes clusters (on-premises or VNGCloud managed)
- Multi-cluster support via ClusterAPI integration

**Helm:**
- Helm Charts located: `charts/vngcloud-load-balancer-controller/`
- Chart values: `values.yaml` with configurable replicas, resource limits, image repository
- Kustomize configurations: `config/manager/` and `config/crd/` for CRD installation
- Deployment platform: Kubernetes 1.29+

**Container Registry:**
- VNGCloud Container Registry (vcr.vngcloud.vn)
- Image: `vcr.vngcloud.vn/81-vks-public/vngcloud-load-balancer-controller`
- Base image: `gcr.io/distroless/static:nonroot` (minimal, security-hardened)

**Build Pipeline:**
- Makefile targets: `docker-build`, `docker-push` for image distribution
- Multi-architecture builds supported (TARGETOS, TARGETARCH args)
- Version injection via LDFLAGS (VERSION, COMMIT)

## Environment Configuration

**Required env vars/config:**
- VNGCloud credentials: `clientID`, `clientSecret`, `identityURL`, `vserverURL`, `projectID`
- Cluster config: `clusterID`, `namespace`, `region`, `isRunRemote` (for multi-cluster mode)
- Load balancer defaults: Package names, algorithms, timeout values
- Kubernetes cluster access: kubeconfig or in-cluster service account

**Secrets location:**
- Config file: `/etc/vngcloud-load-balancer-controller/config.yaml` (mounted via ConfigMap or Secret)
- Sensitive data: Client secrets stored in Kubernetes Secrets (not in codebase)
- VNGCloud credentials: Fetched from config file or metadata service

## Webhooks & Callbacks

**Incoming:**
- Kubernetes ValidatingWebhookConfigurations - Validate CRD specs before admission
- Kubernetes MutatingWebhookConfigurations - Mutate CRD specs during admission
- Webhook server: Configured in controller-runtime webhook.Server (TLS enabled)
- Paths: Auto-registered by controller-gen annotations in CRD types

**Outgoing:**
- VNGCloud API calls - Create/update/delete load balancers, pools, listeners via REST API
- Kubernetes events - Event recording for reconciliation actions
- No external event callbacks configured

## External Health Checks

**Health Endpoints:**
- Healthz probe: `:8081/healthz` - Readiness check (Ping)
- Readyz probe: `:8081/readyz` - Liveness check (Ping)
- Metrics endpoint: `:8443/metrics` (HTTPS) or `:8080/metrics` (HTTP)

---

*Integration audit: 2026-03-15*

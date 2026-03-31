# External Integrations

**Analysis Date:** 2026-03-31

## APIs & External Services

**VNGCloud Platform (primary integration):**
- VNGCloud VLB (Virtual Load Balancer) API
  - SDK/Client: `github.com/vngcloud/vngcloud-go-sdk/v2` via `client.IClient`
  - Endpoint config: `global.vserverURL` + `vlb-gateway` suffix
  - Used in: `internal/repository/vngcloud_repo/vngcloud_loadbalancer.go`, `vngcloud_listener.go`, `vngcloud_pool.go`, `vngcloud_policy.go`
  - Auth: OAuth2 client credentials (`global.clientID` / `global.clientSecret`)

- VNGCloud VServer API
  - SDK/Client: same `client.IClient`
  - Endpoint config: `global.vserverURL` + `vserver-gateway` suffix
  - Used in: `internal/repository/vngcloud_repo/vngcloud_server.go`, `vngcloud_secgroup.go`, `vngcloud_tags.go`

- VNGCloud GLB (Global Load Balancer) API
  - SDK/Client: same `client.IClient`, configured with hardcoded GLB endpoint
  - Endpoint: `https://glb.console.vngcloud.vn/glb-controller/` (hardcoded in `internal/repository/vngcloud_repo/vngcloud_repo.go`)
  - Used in: `internal/repository/vngcloud_repo/vngcloud_global.go`

- VNGCloud IAM / Portal API
  - SDK/Client: `github.com/vngcloud/vngcloud-go-sdk/v2` portal service
  - Endpoint config: `global.identityURL` (e.g., `https://iamapis.vngcloud.vn/accounts-api`)
  - Purpose: Resolve under-project-ID to real project ID and user ID at startup
  - Entry point: `internal/repository/vngcloud_repo/vngcloud_repo.go` `setupProjectId()`

- VNGCloud Certificate API
  - SDK/Client: same `client.IClient`
  - Used in: `internal/repository/vngcloud_repo/vngcloud_certificate.go`

**INTERVPC Load Balancer (optional super-client):**
- A secondary VNGCloud client with elevated credentials for cross-VPC LB management
- Configured via `global.superClientID` / `global.superClientSecret`
- Only instantiated when both super credentials are non-empty
- Client stored as `superClient client.IClient` in `vngCloudRepository`

## Data Storage

**Databases:**
- None - no external database. State is stored exclusively in Kubernetes etcd via controller-runtime informer cache and Kubernetes API objects (Services, Ingresses, CRDs).

**File Storage:**
- Local filesystem only - configuration YAML mounted from Kubernetes Secret at `/etc/vngcloud-load-balancer-controller/config.yaml`

**Caching:**
- In-memory only via controller-runtime's built-in informer cache (sync period configurable via `--sync-period`, default 5 minutes)
- Metadata service result cached in-process via `metadataCache *Metadata` singleton in `pkg/utils/metadata/metadata_vars.go`

## Authentication & Identity

**VNGCloud OAuth2:**
- Provider: VNGCloud IAM (`global.identityURL`)
- Flow: Client credentials grant using `global.clientID` + `global.clientSecret`
- Implemented via `github.com/vngcloud/vngcloud-go-sdk/v2/client.NewSdkConfigure()`
- Token management handled internally by the SDK

**Project/User Resolution (startup):**
- If `global.projectID` is empty in config, the controller queries the VNGCloud metadata service (OpenStack-compatible) to retrieve the under-project-ID, then calls the portal API to resolve it to the real project ID
- Metadata sources (tried in order per `Metadata.SearchOrder`):
  1. `configDrive` - reads from `{tempdir}/openstack/latest/meta_data.json` (config drive)
  2. `metadataService` - fetches from `http://169.254.169.254/openstack/latest/meta_data.json` (IMDS)
- Default search order: `"configDriver,metadataService"` (set in `pkg/config/config.go` `NewConfig()`)
- Implemented in `pkg/utils/metadata/`

**Kubernetes RBAC:**
- Controller uses a `ServiceAccount` named `manager` in the deployment namespace
- RBAC manifests managed via kustomize in `config/rbac/`
- Metrics endpoint protected with Kubernetes authn/authz when `--metrics-secure=true`

**Leader Election:**
- Uses Kubernetes lease-based leader election
- Lease ID format: `n-{clusterID}-n.n-{namespace}-n.lbc.vks.vngcloud.vn`
- Disabled by default (`--leader-elect=false`), enabled in production deployment via `--leader-elect` flag

## Monitoring & Observability

**Metrics (Prometheus):**
- SDK: `github.com/prometheus/client_golang v1.19.1`
- Exposed at `--metrics-bind-address` (default disabled `0`)
- Subsystem prefix: `vngcloud_lbc`
- Metrics defined in `pkg/metrics/lbc/instruments.go`:
  - `vngcloud_lbc_readiness_gate_ready_seconds` (histogram)
  - `vngcloud_lbc_controller_reconcile_errors_total` (counter)
  - `vngcloud_lbc_controller_reconcile_stage_duration` (histogram)
  - `vngcloud_lbc_webhook_validation_failure_total` (counter)
  - `vngcloud_lbc_webhook_mutation_failure_total` (counter)
  - `vngcloud_lbc_controller_cache_object_total` (gauge)
  - `vngcloud_lbc_controller_top_talkers` (gauge)
- Prometheus scrape config manifest in `config/prometheus/`

**Logs:**
- Dual logging system:
  1. `go.uber.org/zap` via `sigs.k8s.io/controller-runtime/pkg/log/zap` - structured JSON logs for controller-runtime events
  2. `github.com/sirupsen/logrus v1.9.3` - legacy logrus logger used in some repository-layer code
  3. `k8s.io/klog/v2` - used in metadata utility package
- Log level configurable via `--log-level` flag (debug/info/warn/error/fatal/panic)
- Logger setup in `pkg/logging/`

**Health Checks:**
- Liveness: `GET /healthz` on `--health-probe-bind-address` (default `:8081`)
- Readiness: `GET /readyz` on same address
- Both use `healthz.Ping` (always returns OK)

## CI/CD & Deployment

**Hosting:**
- Kubernetes (VKS - VNGCloud Kubernetes Service)
- Deployed to `kube-system` namespace as a single-replica `Deployment`

**Container Registries:**
- `vcr.vngcloud.vn` (VNGCloud Container Registry, HCM region) - production images and Helm charts
- `vcr-han.vngcloud.vn` (VNGCloud Container Registry, HAN region) - production images and Helm charts
- `ghcr.io/vngcloud/` (GitHub Container Registry) - images on every push/release

**CI Pipeline (GitHub Actions, `.github/workflows/`):**
- `ci-dev.yml` - Builds and pushes dev image on commits containing `[build]` marker
- `publish.yaml` - Full release pipeline on git tag push: builds multi-registry images + packages/pushes Helm chart OCI artifact
- `check-golangci-lint.yml` - Runs `golangci-lint v2.5.0` on all pushes and PRs
- `check-govulncheck.yml` - Runs `govulncheck` for vulnerability scanning on all pushes and PRs
- `check-gitleaks.yml` - Scans for leaked secrets on all pushes and PRs

**Helm Deployment:**
- Chart located at `charts/vngcloud-load-balancer-controller/`
- Current chart version: `0.3.21`, app version: `v0.3.21`
- Published as OCI artifact to `vcr.vngcloud.vn/81-vks-public/vks-helm-charts` and HAN equivalent

**Kubernetes Manifest Generation:**
- `make manifests` - Generates CRD YAML and RBAC from Go type annotations (via `controller-gen`)
- `make build-installer` - Produces `dist/install.yaml` via `kustomize build config/default`
- `make helm` - Regenerates Helm chart templates from kustomize output via `helmify`

## Webhooks & Callbacks

**Incoming:**
- Kubernetes Admission Webhooks: webhook server initialized in `cmd/main.go` via `webhook.NewServer()`; HTTP/2 disabled by default for CVE protection
- Webhook TLS configured via `tlsOpts`; self-signed certs generated by default

**Outgoing:**
- VNGCloud REST APIs (VLB, VServer, GLB, IAM/Portal) over HTTPS - all initiated by the controller during reconciliation
- No external webhook push endpoints

## ClusterAPI Remote Cluster Mode

**Integration:**
- `sigs.k8s.io/cluster-api v1.9.1`
- When `cluster.isRunRemote = true` in config, the controller operates as a remote controller:
  1. Connects to the management cluster (local kubeconfig)
  2. Uses `pkg/clusterapi/client.go` to fetch the target cluster's kubeconfig from its ClusterAPI secret
  3. All reconcilers then watch the target cluster's Kubernetes resources
- Implementation: `pkg/clusterapi/client.go` using `sigs.k8s.io/cluster-api/util/kubeconfig.FromSecret()`

## Custom Resource Definitions (CRDs)

**API Group:** `vks.vngcloud.vn/v1alpha1`

**CRD types** (defined in `api/v1alpha1/`):
- `LoadBalancerConfig` - `loadbalancerconfig_types.go`
- `GlobalLoadBalancerConfig` - `globalloadbalancerconfig_types.go`
- `NodeSecurityGroup` - `nodesecuritygroup_types.go`
- `VngcloudGlobalLoadBalancer` - `vngcloudgloballoadbalancer_types.go`

**CRD Installation:**
- CRDs are installed programmatically at controller startup via `pkg/k8s/apis/vks.vngcloud.vn/InstallAllCRDs()`
- Uses `k8s.io/apiextensions-apiserver` CRD client; does not rely on `kubectl apply` at deploy time
- CRD YAML manifests stored in `config/crd/bases/`

## Environment Configuration

**Required config values (in `/etc/vngcloud-load-balancer-controller/config.yaml`):**
- `global.identityURL` - VNGCloud IAM endpoint
- `global.vserverURL` - VNGCloud VServer/VLB API endpoint
- `global.clientID` - OAuth2 client ID
- `global.clientSecret` - OAuth2 client secret
- `cluster.clusterID` - Kubernetes cluster identifier

**Optional config values:**
- `global.projectID` + `global.userID` - Skip metadata service lookup (useful in dev mode)
- `global.superClientID` + `global.superClientSecret` - INTERVPC LB management
- `cluster.isRunRemote` + `cluster.namespace` - ClusterAPI remote mode

**Secrets location:**
- Kubernetes `Secret` named `vngcloud-load-balancer-controller-mysecret` in deployment namespace
- Mounted at `/etc/vngcloud-load-balancer-controller/` (read-only)
- Secret template in `config/manager/manager.yaml`
- CI credentials stored as GitHub Actions secrets: `VCR_USER_PRO`, `VCR_PASSWORD_PRO`, `VCR_USER_HAN_PRO`, `VCR_PASSWORD_HAN_PRO`, `VCR_USER`, `VCR_PASSWORD`

---

*Integration audit: 2026-03-31*

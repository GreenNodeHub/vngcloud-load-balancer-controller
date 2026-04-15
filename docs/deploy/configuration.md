# Configuration

The controller is configured via a YAML file mounted at `/etc/vngcloud-load-balancer-controller/config.yaml` inside the pod. When deploying via Helm, this is automatically created from the values you provide.

## Configuration File Reference

```yaml
chartVersion: "0.0.1"

global:
  # VNGCloud IAM endpoint (required)
  identityURL: "https://iamapis.vngcloud.vn/accounts-api"

  # VNGCloud vServer API endpoint (required)
  vserverURL: "https://hcm-3.api.vngcloud.vn/vserver"

  # OAuth2 credentials (required)
  clientID: ""
  clientSecret: ""

  # Optional: override project/user ID (skips metadata service lookup)
  projectID: ""
  userID: ""

  # Optional: super-client credentials for InterVPC load balancers
  superClientID: ""
  superClientSecret: ""

cluster:
  # Kubernetes cluster ID (auto-detected from node labels if not set)
  clusterID: ""

  # Namespace where this controller is deployed
  namespace: "kube-system"

  # VNGCloud region
  region: "hcm-3"

  # Enable remote cluster mode (ClusterAPI)
  isRunRemote: false

loadBalancerOpts:
  # Default package for Network Load Balancers
  packageName: "lbp-f5f8a5d2-46d5-4c2b-8f37-6cf8c65b9a83"

  # Default pool algorithm: RoundRobin | LeastConnections | SourceIP
  defaultPoolAlgorithm: "RoundRobin"

  # Default health check thresholds
  healthCheckHealthyThreshold: 3
  healthCheckUnhealthyThreshold: 3
  healthCheckIntervalSeconds: 30
  healthCheckTimeoutSeconds: 5

  # Default listener timeouts (seconds)
  defaultIdleTimeoutClient: 50
  defaultIdleTimeoutMember: 50
  defaultIdleTimeoutConnection: 5

globalLoadBalancerOpts:
  packageName: ""

# Maximum number of parallel reconcile loops per controller
maxConcurrentReconciles: 1
```

## Helm Values

The Helm chart exposes the most common configuration as values. See all available values:

```bash
helm show values oci://vcr.vngcloud.vn/81-vks-public/vks-helm-charts/vngcloud-load-balancer-controller
```

Key values:

| Value | Description | Default |
|---|---|---|
| `mysecret.global.clientID` | VNGCloud client ID | `""` |
| `mysecret.global.clientSecret` | VNGCloud client secret | `""` |
| `mysecret.global.vserverURL` | vServer API endpoint | `""` |
| `manager.manager.image.repository` | Controller image repository | `vcr.vngcloud.vn/81-vks-public/vngcloud-load-balancer-controller` |
| `manager.manager.image.tag` | Controller image tag | chart's appVersion |
| `manager.replicaCount` | Number of controller replicas | `1` |

## CLI Flags

The controller binary supports the following flags:

| Flag | Default | Description |
|---|---|---|
| `--metrics-bind-address` | `0` | Address for Prometheus metrics endpoint |
| `--health-probe-bind-address` | `:8081` | Address for liveness/readiness probes |
| `--leader-elect` | `false` | Enable leader election for HA deployments |
| `--metrics-secure` | `true` | Serve metrics over HTTPS |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--sync-period` | `5m` | Resync period for the informer cache |
| `--disable-service-controller` | `false` | Disable the Service reconciler |
| `--disable-ingress-controller` | `false` | Disable the Ingress reconciler |
| `--disable-lbc-controller` | `false` | Disable the LoadBalancerConfig reconciler |
| `--disable-nsg-controller` | `false` | Disable the NodeSecurityGroup reconciler |
| `--disable-vglb-controller` | `false` | Disable the VngcloudGlobalLoadBalancer reconciler |

## Environment Variables

All config file fields can be overridden by environment variables (via `viper.AutomaticEnv()`). For example:

```bash
GLOBAL_CLIENTID=xxx
GLOBAL_CLIENTSECRET=yyy
GLOBAL_VSERVERURL=https://hcm-3.api.vngcloud.vn/vserver
```

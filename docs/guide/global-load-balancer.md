# Global Load Balancer

The VNGCloud Load Balancer Controller supports two approaches to Global Load Balancing (GLB):

1. **Service GLB annotations** — attach a GLB to a `Service` via `glb.vks.vngcloud.vn/*` annotations
2. **GlobalLoadBalancerConfig CRD** — declaratively define a multi-region GLB with full listener/pool control

!!! info
    This feature is currently in alpha (`v1alpha1`).

## Approach 1: Service GLB Annotations

Add `glb.vks.vngcloud.vn/enable: "true"` to any `Service` of type `LoadBalancer` or `NodePort`:

=== "LoadBalancer"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: my-global-service
      annotations:
        glb.vks.vngcloud.vn/enable: "true"
        glb.vks.vngcloud.vn/load-balancer-name: "my-global-lb"
        glb.vks.vngcloud.vn/package-id: "glbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        glb.vks.vngcloud.vn/pool-algorithm: "ROUND_ROBIN"
        glb.vks.vngcloud.vn/healthcheck-protocol: "HTTP"
        glb.vks.vngcloud.vn/healthcheck-path: "/healthz"
    spec:
      type: LoadBalancer
      selector:
        app: my-app
      ports:
        - port: 80
          targetPort: 8080
    ```

=== "NodePort"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: my-global-service
      annotations:
        glb.vks.vngcloud.vn/enable: "true"
        glb.vks.vngcloud.vn/load-balancer-name: "my-global-lb"
        glb.vks.vngcloud.vn/package-id: "glbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        glb.vks.vngcloud.vn/pool-algorithm: "ROUND_ROBIN"
        glb.vks.vngcloud.vn/healthcheck-protocol: "HTTP"
        glb.vks.vngcloud.vn/healthcheck-path: "/healthz"
    spec:
      type: NodePort
      selector:
        app: my-app
      ports:
        - port: 80
          targetPort: 8080
          nodePort: 30080
    ```

## Approach 2: GlobalLoadBalancerConfig CRD

For full control over listeners, pools, and multi-region members use the `GlobalLoadBalancerConfig` CRD directly.

Short name: `glbc`

```bash
kubectl get globalloadbalancerconfig -A
```

### Example

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: GlobalLoadBalancerConfig
metadata:
  name: my-glb
  namespace: default
spec:
  name: "my-global-lb"
  description: "Multi-region load balancer"
  type: L4         # or L7
  packageId: "glbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  tags:
    env: production

  globalPools:
    - name: hcm-pool
      protocol: TCP
      algorithm: RoundRobin
      healthMonitor:
        protocol: TCP
        interval: 30
        timeout: 5
        healthyThreshold: 3
        unhealthyThreshold: 3
      poolMembers:
        - name: hcm-member-group
          region: hcm-3
          vpcId: "net-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
          type: VM
          trafficDial: 100
          members:
            - name: node-1
              address: "10.0.0.1"
              port: 30080
              monitorPort: 30080
              subnetId: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              weight: 1

  globalListeners:
    - name: tcp-listener
      protocol: TCP
      protocolPort: 80
      defaultPoolName: hcm-pool
      timeoutClient: 50
      timeoutMember: 50
      timeoutConnection: 5
      allowedCidrs: "0.0.0.0/0"
```

## GlobalLoadBalancerConfig Spec Reference

### Top-level fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Name of the GLB resource in VNGCloud |
| `description` | No | Description |
| `type` | Yes | GLB type (e.g. `L4`) |
| `packageId` | No | Package/size ID |
| `paymentFlow` | No | Payment flow configuration |
| `loadBalancerId` | No | Existing GLB ID (managed by controller) |
| `tags` | No | Key-value tags |
| `globalListeners` | No | List of global listeners |
| `globalPools` | No | List of global pools |

### GlobalPool fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Pool name (unique within this GLBC) |
| `protocol` | Yes | Pool protocol |
| `algorithm` | No | `RoundRobin`, `LeastConnections`, `SourceIP` |
| `stickiness` | No | Enable sticky sessions |
| `tlsEncryption` | No | Enable TLS to pool members |
| `healthMonitor` | No | Health check configuration |
| `poolMembers` | No | List of regional pool member groups |

### GlobalPoolMember (regional group)

| Field | Required | Description |
|---|---|---|
| `name` | No | Name for this member group |
| `region` | Yes | VNGCloud region (e.g. `hcm-3`, `han-1`) |
| `vpcId` | Yes | VPC ID in that region |
| `type` | Yes | Member type (e.g. `VM`) |
| `trafficDial` | No | Traffic percentage for this region (0–100) |
| `members` | No | Individual VM members in this region |

### GlobalListener fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Listener name |
| `protocol` | Yes | Listener protocol |
| `protocolPort` | Yes | Port (1–65535) |
| `defaultPoolName` | No | Default backend pool |
| `timeoutClient` | No | Client idle timeout (seconds) |
| `timeoutMember` | No | Member idle timeout (seconds) |
| `timeoutConnection` | No | Connection timeout (seconds) |
| `allowedCidrs` | No | Allowed source CIDRs |
| `headers` | No | List of headers to insert |

## Status

```yaml
status:
  loadBalancerId: "glb-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  domains:
    - "my-glb.glb.vngcloud.vn"
  vips:
    - address: "203.0.113.1"
      region: "hcm-3"
      status: "ACTIVE"
    - address: "203.0.113.2"
      region: "han-1"
      status: "ACTIVE"
  observedGeneration: 1
  lastReconcileTime: "2026-04-15T10:00:00Z"
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileSuccess
```

## GLB Service Annotation Reference

All GLB service annotations use the `glb.vks.vngcloud.vn` prefix:

| Annotation | Description |
|---|---|
| `glb.vks.vngcloud.vn/enable` | Enable GLB for this Service (`"true"`) |
| `glb.vks.vngcloud.vn/load-balancer-id` | Use an existing GLB by ID |
| `glb.vks.vngcloud.vn/load-balancer-name` | Custom name for the GLB |
| `glb.vks.vngcloud.vn/package-id` | GLB package/size ID |
| `glb.vks.vngcloud.vn/description` | Description for the GLB |
| `glb.vks.vngcloud.vn/target-type` | `instance` or `ip` |
| `glb.vks.vngcloud.vn/pool-algorithm` | `RoundRobin`, `LeastConnections`, `SourceIP` |
| `glb.vks.vngcloud.vn/inbound-cidrs` | Allowed source CIDRs |
| `glb.vks.vngcloud.vn/enable-proxy-protocol` | Enable PROXY protocol |
| `glb.vks.vngcloud.vn/idle-timeout-client` | Client idle timeout (seconds) |
| `glb.vks.vngcloud.vn/idle-timeout-member` | Member idle timeout (seconds) |
| `glb.vks.vngcloud.vn/idle-timeout-connection` | Connection timeout (seconds) |
| `glb.vks.vngcloud.vn/healthcheck-port` | Health check port |
| `glb.vks.vngcloud.vn/healthcheck-protocol` | Health check protocol |
| `glb.vks.vngcloud.vn/healthcheck-path` | Health check path (HTTP/HTTPS) |
| `glb.vks.vngcloud.vn/healthcheck-http-method` | HTTP method for health checks |
| `glb.vks.vngcloud.vn/healthcheck-http-version` | HTTP version for health checks |
| `glb.vks.vngcloud.vn/healthcheck-http-domain-name` | Host header for health checks |
| `glb.vks.vngcloud.vn/success-codes` | HTTP codes considered healthy |
| `glb.vks.vngcloud.vn/healthy-threshold-count` | Consecutive healthy checks |
| `glb.vks.vngcloud.vn/unhealthy-threshold-count` | Consecutive unhealthy checks |
| `glb.vks.vngcloud.vn/healthcheck-interval-seconds` | Health check interval |
| `glb.vks.vngcloud.vn/healthcheck-timeout-seconds` | Health check timeout |

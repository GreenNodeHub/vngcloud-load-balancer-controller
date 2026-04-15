# LoadBalancerConfig CRD

The `LoadBalancerConfig` custom resource provides fine-grained control over VNGCloud load balancers. It allows you to define listeners, pools, pool members, and L7 routing policies declaratively in Kubernetes.

## Overview

```
kubectl get loadbalancerconfig -A
```

Short name: `lbc`

```
kubectl get lbc -A
NAME         TYPE          LOADBALANCER-ID                                   ADDRESS          ZONE    READY   AGE
my-lb        Application   lb-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx          203.0.113.42     HCM-1   True    5m
```

## Example: Application Load Balancer (L7)

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: LoadBalancerConfig
metadata:
  name: my-alb
  namespace: default
spec:
  type: Application
  subnetId: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  vpcId: "net-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  zoneId: "HCM-1"
  scheme: Internet
  packageId: "lbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"

  pools:
    - name: web-pool
      protocol: HTTP
      algorithm: RoundRobin
      healthMonitor:
        protocol: HTTP
        healthCheckPath: "/healthz"
        healthCheckMethod: GET
        interval: 30
        timeout: 5
        healthyThreshold: 3
        unhealthyThreshold: 3

  listeners:
    - name: http-listener
      protocol: HTTP
      protocolPort: 80
      defaultPoolName: web-pool
      policies:
        - name: api-redirect
          action: REDIRECT_TO_POOL
          redirectPoolName: api-pool
          l7Rules:
            - ruleType: PATH
              compareType: STARTS_WITH
              ruleValue: "/api"
```

## Example: Network Load Balancer (L4)

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: LoadBalancerConfig
metadata:
  name: my-nlb
  namespace: default
spec:
  type: Network
  subnetId: "sub-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  vpcId: "net-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  zoneId: "HCM-1"
  scheme: Internal

  pools:
    - name: tcp-pool
      protocol: TCP
      healthMonitor:
        protocol: TCP
        interval: 30
        timeout: 5
        healthyThreshold: 3
        unhealthyThreshold: 3

  listeners:
    - name: tcp-listener
      protocol: TCP
      protocolPort: 443
      defaultPoolName: tcp-pool
```

## Spec Reference

### Top-level fields

| Field | Required | Description |
|---|---|---|
| `type` | Yes | `Application` or `Network` |
| `subnetId` | Yes | Subnet ID for the load balancer's public interface |
| `vpcId` | Yes | VPC ID |
| `zoneId` | Yes | Availability zone (e.g., `HCM-1`) |
| `scheme` | No | `Internal`, `Internet` (default), or `InterVPC` |
| `packageId` | No | Load balancer package/size ID |
| `loadBalancerName` | No | Custom name for the VNGCloud LB resource |
| `enableAutoscale` | No | Enable autoscaling |
| `tags` | No | Key-value tags applied to the LB |
| `isPoc` | No | Mark as proof-of-concept |

### Pool fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Pool name (unique within this LBC) |
| `protocol` | Yes | `HTTP`, `HTTPS`, `TCP`, `UDP`, `PROXY` |
| `algorithm` | No | `RoundRobin`, `LeastConnections`, `SourceIP` |
| `stickiness` | No | Enable sticky sessions |
| `tlsEncryption` | No | Enable TLS encryption for pool members |
| `healthMonitor` | No | Health check configuration |
| `members` | No | Static pool members (IP, port) |

### Listener fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Listener name |
| `protocol` | Yes | `HTTP`, `HTTPS`, `TCP`, `UDP`, `TERMINATED_HTTPS` |
| `protocolPort` | Yes | Port number (1–65535) |
| `defaultPoolName` | No | Name of the default backend pool |
| `timeoutClient` | No | Client idle timeout (seconds) |
| `timeoutMember` | No | Member idle timeout (seconds) |
| `timeoutConnection` | No | Connection timeout (seconds) |
| `allowedCidrs` | No | Comma-separated list of allowed CIDR blocks |
| `insertHeaders` | No | Headers to inject into requests |
| `policies` | No | L7 routing policies (Application LB only) |
| `certificateDefault` | No | Default TLS certificate (Application LB only) |
| `certificateAuthorities` | No | CA certificates for mutual TLS |

### L7 Policy fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Policy name |
| `action` | Yes | `REDIRECT_TO_POOL`, `REDIRECT_TO_URL`, `REJECT` |
| `redirectPoolName` | No | Target pool for `REDIRECT_TO_POOL` |
| `redirectUrl` | No | Redirect URL for `REDIRECT_TO_URL` |
| `redirectHttpCode` | No | HTTP redirect code (e.g., `301`, `302`) |
| `position` | No | Policy priority (lower = higher priority) |
| `l7Rules` | No | List of matching rules |

### L7 Rule fields

| Field | Required | Description |
|---|---|---|
| `ruleType` | Yes | `PATH`, `HOST_NAME`, `HEADER`, `COOKIE` |
| `compareType` | Yes | `EQUAL_TO`, `STARTS_WITH`, `ENDS_WITH`, `CONTAINS`, `REGEX` |
| `ruleValue` | Yes | Value to match against |

## Status

The controller updates the status after each reconcile:

```yaml
status:
  loadBalancerId: "lb-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  address: "203.0.113.42"
  observedGeneration: 3
  lastReconcileTime: "2026-04-15T10:00:00Z"
  lastReconcileMessage: "Load balancer reconciled successfully"
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileSuccess
      lastTransitionTime: "2026-04-15T10:00:00Z"
```

# Global Load Balancer

The VNGCloud Load Balancer Controller supports Global Load Balancing (GLB) for distributing traffic across multiple regions or clusters.

!!! info
    This feature is currently in alpha (`v1alpha1`).

## Overview

Global Load Balancers distribute traffic across multiple VNGCloud regional endpoints, enabling:

- **Multi-region failover** — Route traffic to a healthy region when the primary fails
- **Geographic routing** — Direct users to the nearest region
- **Active-active** — Share load across multiple regions simultaneously

## Enabling GLB for a Service

Add the `glb.vks.vngcloud.vn/enable: "true"` annotation to any `Service` of type `LoadBalancer`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-global-service
  annotations:
    glb.vks.vngcloud.vn/enable: "true"
    glb.vks.vngcloud.vn/load-balancer-name: "my-global-lb"
    glb.vks.vngcloud.vn/package-id: "glbp-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
    glb.vks.vngcloud.vn/pool-algorithm: "RoundRobin"
spec:
  type: LoadBalancer
  selector:
    app: my-app
  ports:
    - port: 80
      targetPort: 8080
```

## VngcloudGlobalLoadBalancer CRD

For advanced configurations, use the `VngcloudGlobalLoadBalancer` CRD directly:

```yaml
apiVersion: vks.vngcloud.vn/v1alpha1
kind: VngcloudGlobalLoadBalancer
metadata:
  name: my-glb
  namespace: default
spec:
  # Global load balancer configuration
```

## GLB Annotation Reference

All GLB annotations use the `glb.vks.vngcloud.vn` prefix:

| Annotation | Description |
|---|---|
| `glb.vks.vngcloud.vn/enable` | Enable GLB for this Service (`"true"`) |
| `glb.vks.vngcloud.vn/load-balancer-id` | Use an existing GLB by ID |
| `glb.vks.vngcloud.vn/load-balancer-name` | Custom name for the GLB |
| `glb.vks.vngcloud.vn/package-id` | GLB package/size ID |
| `glb.vks.vngcloud.vn/pool-algorithm` | Load balancing algorithm |
| `glb.vks.vngcloud.vn/target-type` | `instance` or `ip` |
| `glb.vks.vngcloud.vn/inbound-cidrs` | Allowed source CIDRs |
| `glb.vks.vngcloud.vn/enable-proxy-protocol` | Enable PROXY protocol |
| `glb.vks.vngcloud.vn/healthcheck-port` | Health check port |
| `glb.vks.vngcloud.vn/healthcheck-protocol` | Health check protocol |
| `glb.vks.vngcloud.vn/healthcheck-path` | Health check path (HTTP/HTTPS) |
| `glb.vks.vngcloud.vn/success-codes` | HTTP success codes |
| `glb.vks.vngcloud.vn/healthy-threshold-count` | Consecutive healthy checks |
| `glb.vks.vngcloud.vn/unhealthy-threshold-count` | Consecutive unhealthy checks |
| `glb.vks.vngcloud.vn/healthcheck-interval-seconds` | Health check interval |
| `glb.vks.vngcloud.vn/healthcheck-timeout-seconds` | Health check timeout |
| `glb.vks.vngcloud.vn/idle-timeout-client` | Client idle timeout |
| `glb.vks.vngcloud.vn/idle-timeout-member` | Member idle timeout |
| `glb.vks.vngcloud.vn/idle-timeout-connection` | Connection timeout |

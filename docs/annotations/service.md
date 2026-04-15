# Service Annotations

All annotations use the prefix `vks.vngcloud.vn`.

## General

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/load-balancer-name` | string | Custom name for the VNGCloud load balancer |
| `vks.vngcloud.vn/load-balancer-id` | string | Attach to an existing load balancer by ID (managed by controller) |
| `vks.vngcloud.vn/package-id` | string | Load balancer package/size ID |
| `vks.vngcloud.vn/scheme` | `Internal` \| `Internet` \| `InterVPC` | Whether the LB is internal, internet-facing, or cross-VPC |
| `vks.vngcloud.vn/target-type` | `instance` \| `ip` | Route to node ports (`instance`) or pod IPs (`ip`) |
| `vks.vngcloud.vn/ignore` | `"true"` | Ignore this Service (controller will not manage it) |
| `vks.vngcloud.vn/enable-load-balancer` | `"true"` | Enable LB for `NodePort` or `ClusterIP` service types |
| `vks.vngcloud.vn/tags` | `key1=val1,key2=val2` | Tags to apply to the load balancer |
| `vks.vngcloud.vn/security-groups` | comma-separated IDs | Security groups to attach to the load balancer |
| `vks.vngcloud.vn/target-node-labels` | `key=value,...` | Only add nodes with these labels as pool members |
| `vks.vngcloud.vn/enable-autoscale` | `"true"` | Enable autoscaling for the load balancer |
| `vks.vngcloud.vn/prefer-zone-id` | zone string | Preferred availability zone |
| `vks.vngcloud.vn/prefer-subnet-id` | string | Preferred subnet ID |
| `vks.vngcloud.vn/isPOC` | `"true"` | Mark as proof-of-concept deployment |

## Timeouts

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/idle-timeout-client` | integer (seconds) | Client idle timeout |
| `vks.vngcloud.vn/idle-timeout-member` | integer (seconds) | Member idle timeout |
| `vks.vngcloud.vn/idle-timeout-connection` | integer (seconds) | Connection timeout |
| `vks.vngcloud.vn/inbound-cidrs` | CIDR list | Restrict inbound traffic to these CIDRs |

## Health Checks

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/healthcheck-port` | port number | Port used for health checks |
| `vks.vngcloud.vn/healthcheck-protocol` | `TCP` \| `HTTP` \| `HTTPS` \| `PING` | Health check protocol |
| `vks.vngcloud.vn/healthcheck-path` | path string | HTTP health check path |
| `vks.vngcloud.vn/healthcheck-http-method` | `GET` \| `HEAD` | HTTP method for health checks |
| `vks.vngcloud.vn/healthcheck-http-version` | `1.0` \| `1.1` | HTTP version for health checks |
| `vks.vngcloud.vn/healthcheck-http-domain-name` | hostname | HTTP Host header for health checks |
| `vks.vngcloud.vn/success-codes` | e.g. `200,201` | HTTP codes considered healthy |
| `vks.vngcloud.vn/healthcheck-interval-seconds` | integer | Seconds between health checks |
| `vks.vngcloud.vn/healthcheck-timeout-seconds` | integer | Health check timeout in seconds |
| `vks.vngcloud.vn/healthy-threshold-count` | integer | Consecutive successes before marking healthy |
| `vks.vngcloud.vn/unhealthy-threshold-count` | integer | Consecutive failures before marking unhealthy |

## Pool

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/pool-algorithm` | `RoundRobin` \| `LeastConnections` \| `SourceIP` | Pool load balancing algorithm |

## L4 (Network LB) Only

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/enable-proxy-protocol` | `"true"` | Enable PROXY protocol on the listener |
| `vks.vngcloud.vn/private-subnet-id` | string | Private subnet ID for InterVPC scheme |
| `vks.vngcloud.vn/private-zone-id` | zone string | Zone of the client subnet for InterVPC |

!!! info "Deprecated"
    `vks.vngcloud.vn/backend-subnet-id` is deprecated. Use `vks.vngcloud.vn/private-subnet-id` instead.

## L7 (Application LB / Ingress) Only

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/enable-sticky-session` | `"true"` | Enable sticky sessions on the pool |
| `vks.vngcloud.vn/enable-tls-encryption` | `"true"` | Enable TLS encryption to pool members |
| `vks.vngcloud.vn/certificate-ids` | comma-separated IDs | VNGCloud certificate IDs to attach to the listener |
| `vks.vngcloud.vn/client-certificate-id` | string | Client certificate ID for mutual TLS |
| `vks.vngcloud.vn/insert-headers` | `Header:Value,...` | Headers to insert into forwarded requests |
| `vks.vngcloud.vn/auto-reorder-policies` | `"true"` | Automatically reorder L7 policies by priority |

## Management (Advanced)

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/manage-pools` | `"true"` / `"false"` | Whether the controller manages pools for this resource |
| `vks.vngcloud.vn/manage-listeners` | `"true"` / `"false"` | Whether the controller manages listeners |
| `vks.vngcloud.vn/manage-dfp-members` | `"true"` / `"false"` | Whether the controller manages default pool members |
| `vks.vngcloud.vn/trigger` | any string | Force a reconcile by changing this value |

## Global Load Balancer

For GLB-specific annotations, see the [Global Load Balancer guide](../guide/global-load-balancer.md). GLB annotations use the `glb.vks.vngcloud.vn` prefix.

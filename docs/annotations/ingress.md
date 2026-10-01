# Ingress Annotations

All Ingress annotations use the prefix `vks.vngcloud.vn`. Ingress resources must also set `spec.ingressClassName: vngcloud`.

## General

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/load-balancer-name` | string | Custom name for the VNGCloud load balancer |
| `vks.vngcloud.vn/load-balancer-id` | string | Attach to an existing load balancer by ID (managed by controller) |
| `vks.vngcloud.vn/package-id` | string | Load balancer package/size ID |
| `vks.vngcloud.vn/scheme` | `Internal` \| `Internet` | Whether the ALB is internal or internet-facing |
| `vks.vngcloud.vn/target-type` | `instance` \| `ip` | Route to node ports (`instance`) or pod IPs (`ip`) |
| `vks.vngcloud.vn/ignore` | `"true"` | Ignore this Ingress (controller will not manage it) |
| `vks.vngcloud.vn/tags` | `key1=val1,key2=val2` | Tags to apply to the load balancer |
| `vks.vngcloud.vn/security-groups` | comma-separated IDs | Security groups to attach to the load balancer |
| `vks.vngcloud.vn/target-node-labels` | `key=value,...` | Only add nodes with these labels as pool members |
| `vks.vngcloud.vn/enable-autoscale` | `"true"` | Enable autoscaling for the load balancer |
| `vks.vngcloud.vn/prefer-zone-id` | zone string | Preferred availability zone |
| `vks.vngcloud.vn/prefer-subnet-id` | string | Preferred subnet ID |

## Timeouts

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/idle-timeout-client` | integer (seconds) | Client idle timeout |
| `vks.vngcloud.vn/idle-timeout-member` | integer (seconds) | Member idle timeout |
| `vks.vngcloud.vn/idle-timeout-connection` | integer (seconds) | Connection timeout |

## Access control (ACL)

Applied to every listener of this resource. The first matching rule wins: dropped CIDRs are checked first, then inbound (allowed) CIDRs; only traffic matching neither follows the default action. Dropped traffic is discarded silently (clients time out). IPv6 is not supported.

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/inbound-cidrs` | CIDR list | Allowed source CIDRs. Implies `acl-default-action: drop` unless set |
| `vks.vngcloud.vn/dropped-cidrs` | comma-separated IPv4 CIDRs or addresses | Source CIDRs dropped first. `0.0.0.0/0` drops everything. `""` keeps the list managed and empty |
| `vks.vngcloud.vn/acl-default-action` | `accept` \| `drop` | Action for traffic that matches no rule |

Removing an annotation puts back the value the listener had before the controller changed it. ACL values set on the portal are left alone while no annotation manages them.

**Shared load balancers.** Ingresses sharing a load balancer share its port 80/443 listeners, and so one ACL. Declare the ACL on one Ingress; different dropped CIDRs or default actions (including the `drop` that `inbound-cidrs` implies) on the same port are rejected. Different inbound CIDRs only produce a `SharedListenerAcl` warning event, and the listener follows whichever Ingress applied last. A resource without the annotations on such a listener also receives a `SharedListenerAcl` warning event.

If the Ingress holding the original is removed first, the last value can remain on the listener. To clear it, remove the annotation from the remaining Ingress, then clear the leftover value on the portal. Alternatively, keep `vks.vngcloud.vn/dropped-cidrs: ""` on the remaining Ingress permanently: it records the leftover value as the original, so removing it later puts the block back.

| Goal | Annotations |
|---|---|
| Drop a few addresses | `vks.vngcloud.vn/dropped-cidrs: "203.0.113.9/32,198.51.100.0/24"` |
| Allow only some ranges | `vks.vngcloud.vn/inbound-cidrs: "192.0.2.0/24"` |
| Allow a range except a few addresses | `vks.vngcloud.vn/inbound-cidrs: "192.0.2.0/24"` + `vks.vngcloud.vn/dropped-cidrs: "192.0.2.13/32"` (dropped is checked first) |
| Drop everything | `vks.vngcloud.vn/dropped-cidrs: "0.0.0.0/0"` |

## Health Checks

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/healthcheck-port` | port number | Port used for health checks |
| `vks.vngcloud.vn/healthcheck-protocol` | `HTTP` \| `HTTPS` | Health check protocol |
| `vks.vngcloud.vn/healthcheck-path` | path string | HTTP health check path |
| `vks.vngcloud.vn/healthcheck-http-method` | `GET` \| `HEAD` | HTTP method for health checks |
| `vks.vngcloud.vn/healthcheck-http-version` | `1.0` \| `1.1` | HTTP version |
| `vks.vngcloud.vn/healthcheck-http-domain-name` | hostname | Host header for health checks |
| `vks.vngcloud.vn/success-codes` | e.g. `200,201,202-204` | HTTP codes considered healthy |
| `vks.vngcloud.vn/healthcheck-interval-seconds` | integer | Seconds between health checks |
| `vks.vngcloud.vn/healthcheck-timeout-seconds` | integer | Health check timeout |
| `vks.vngcloud.vn/healthy-threshold-count` | integer | Consecutive successes before healthy |
| `vks.vngcloud.vn/unhealthy-threshold-count` | integer | Consecutive failures before unhealthy |

## TLS / Certificates

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/certificate-ids` | comma-separated IDs | Existing VNGCloud certificate IDs |
| `vks.vngcloud.vn/client-certificate-id` | string | Client certificate for mutual TLS |
| `vks.vngcloud.vn/enable-tls-encryption` | `"true"` | Enable TLS to pool members (end-to-end encryption) |

!!! tip "Auto Certificate Creation"
    If your Ingress has `spec.tls` with a `secretName`, the controller will automatically create a VNGCloud certificate from the Kubernetes TLS secret and attach it to the listener — no `certificate-ids` annotation needed.

## L7 Features

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/enable-sticky-session` | `"true"` | Enable sticky sessions |
| `vks.vngcloud.vn/insert-headers` | `Header:Value,...` | Headers to inject into forwarded requests |
| `vks.vngcloud.vn/auto-reorder-policies` | `"true"` | Automatically reorder policies by priority |
| `vks.vngcloud.vn/implementation-specific-params` | JSON string | Advanced L7 parameters |

## Pool

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/pool-algorithm` | `ROUND_ROBIN` \| `LEAST_CONNECTIONS` \| `SOURCE_IP` | Pool load balancing algorithm |

## Management

| Annotation | Values | Description |
|---|---|---|
| `vks.vngcloud.vn/manage-pools` | `"true"` / `"false"` | Whether the controller manages pools |
| `vks.vngcloud.vn/manage-listeners` | `"true"` / `"false"` | Whether the controller manages listeners |
| `vks.vngcloud.vn/trigger` | any string | Force a reconcile by changing this value |

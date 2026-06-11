# Gateway API (NLB / L4)

Phase 2 extends [Gateway API](gateway-api.md) support to VNGCloud **Network Load Balancers** (L4). A `Gateway` under the `vngcloud-nlb` GatewayClass with `TCPRoute`/`UDPRoute` produces a Layer-4 `LoadBalancerConfig` — the same CR the `Service` type=LoadBalancer path emits — so the existing LBC controller drives the cloud NLB.

| | |
|---|---|
| GatewayClass | `vngcloud-nlb` |
| Controller name | `gateway.vks.vngcloud.vn/nlb` |
| Listener protocols | `TCP`, `UDP` |
| Supported routes | `TCPRoute`, `UDPRoute` |

!!! warning "Experimental-channel CRDs are a prerequisite"
    `TCPRoute` and `UDPRoute` live only in the Gateway API **experimental** channel and are **not** installed by the standard-channel manifests used for the ALB path. Install them before enabling:

    ```bash
    kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/experimental-install.yaml
    ```

    Because of this, the NLB controller is **disabled by default**. The controller does not embed or install these CRDs.

!!! note "Phase 2 scope"
    TCP and UDP routing are supported. `TLSRoute` (SNI passthrough) is planned for a follow-up. Mixed L4+L7 on one Gateway is not supported — HTTP/HTTPS listeners on an `vngcloud-nlb` Gateway are reported `UnsupportedProtocol` and skipped (use the `vngcloud-alb` class for L7).

## Enabling

```yaml
# values.yaml
gatewayApi:
  nlb:
    enabled: true   # installs the vngcloud-nlb GatewayClass + enables the controller
```

This sets `--disable-nlb-gateway-controller=false`. Only enable it on clusters that have the experimental CRDs installed.

## Quick start

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: tcp-gateway
  namespace: default
spec:
  gatewayClassName: vngcloud-nlb
  listeners:
    - name: redis
      protocol: TCP
      port: 6379
      allowedRoutes:
        namespaces:
          from: Same
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: redis-route
  namespace: default
spec:
  parentRefs:
    - name: tcp-gateway
      sectionName: redis     # optional: pin to a listener (also matchable by port)
  rules:
    - backendRefs:
        - name: redis
          port: 6379
```

A `UDPRoute` is identical with `kind: UDPRoute` and a `UDP` listener.

```bash
kubectl get gateway tcp-gateway
NAME          CLASS          ADDRESS         PROGRAMMED   AGE
tcp-gateway   vngcloud-nlb   203.0.113.50    True         3m
```

## How routes map to listeners

Each Gateway TCP/UDP listener gets **one default pool**, built from the backendRefs of the route attached to it:

- A `TCPRoute` attaches to a `TCP` listener; a `UDPRoute` to a `UDP` listener.
- A `parentRef` attaches to a listener when its `sectionName` (if set) names the listener **and** its `port` (if set) equals the listener's port. With neither, it attaches to every compatible listener.
- If several routes target one listener, the **oldest** (by creationTimestamp) wins — an L4 listener has a single default pool.
- A listener with no attached route produces no cloud listener (an L4 listener has nothing to forward without a backend).

Weighted backendRefs across multiple Services are aggregated into one pool with scaled member weights, same as the ALB path.

## Policy CRDs

The L4 path honors three of the four [policy CRDs](gateway-api.md#policy-crds-gep-713):

| CRD | Targets | L4 effect |
|---|---|---|
| `VKSGatewayPolicy` | `Gateway` | LB creation spec (scheme, package, name, subnet/zone, BYO-LB adoption, tags), listener timeouts, allowed CIDRs |
| `VKSBackendPolicy` | `Service` | Pool algorithm, stickiness, member target type (`instance`/`ip`), node-label selection |
| `VKSHealthCheckPolicy` | `Service` | Health monitor — TCP (default), PING-UDP (UDP pools), or an HTTP/HTTPS probe; interval/timeout/thresholds/port |

`VKSRoutePolicy` does **not** apply to L4 — there are no L7 actions (Reject/Redirect) or path/host matching on a TCP/UDP listener. `insertHeaders`, certificates, and TLS termination are also L7-only and ignored here.

Health-check defaults: a TCP listener probes its members over **TCP**; a UDP listener uses **PING-UDP** (mirrors the Service controller). A `VKSHealthCheckPolicy` can override the protocol (including an HTTP/HTTPS probe over a TCP pool) and the thresholds/interval/timeout/port.

## Status

- **Gateway** — `Accepted` (always, we own the class) and `Programmed` (reflects LBC readiness); `status.addresses` carries the NLB IP. Per-listener `Accepted` is `UnsupportedProtocol` for any HTTP/HTTPS listener.
- **TCPRoute / UDPRoute** — `Accepted` (attached to a listener) and `ResolvedRefs` (`ResolvedRefs`, `BackendNotFound`, or `RefNotPermitted` for a cross-namespace backend without a [`ReferenceGrant`](gateway-api.md#cross-namespace-backends)).

## Relationship to `Service` type=LoadBalancer

Both produce a Layer-4 `LoadBalancerConfig` and reconcile through the same LBC controller. The Gateway path is declarative routing config (Gateway + routes + policies) instead of a `Service` plus `vks.vngcloud.vn/*` annotations. Pick one per workload; they don't share an LB.

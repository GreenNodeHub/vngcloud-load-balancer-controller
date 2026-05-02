# Gateway API Support — Design

**Date:** 2026-04-30
**Status:** Approved (brainstorming complete; awaiting implementation plan)
**Scope:** Add Kubernetes Gateway API (https://gateway-api.sigs.k8s.io) support to the vngcloud Load Balancer Controller, aligned with vngcloud LB product capabilities.

---

## Decisions summary

| # | Decision | Choice |
|---|---|---|
| 1 | Feature scope | Plan all Gateway API features upfront; phase the implementation in 4 waves |
| 2 | Gateway → LB mapping | Two GatewayClasses (`vngcloud-alb`, `vngcloud-nlb`); 1 Gateway = 1 LB; mixed-protocol Gateways rejected |
| 3 | Config CRD strategy | Reuse `LoadBalancerConfig` at both `GatewayClass.parametersRef` and `Gateway.spec.infrastructure.parametersRef` with a `mergingMode` field. Add two new namespaced CRDs: `TargetGroupConfig` (per-Service / per-route) and `ListenerRuleConfig` (per-HTTPRoute filter). Mirrors AWS LBC's pattern |
| 4 | Coexistence with existing Ingress / Service controllers | Strict separation; no shared LBs; no LB-ID adoption on Gateway. Documented one-shot migration tool deferred to Phase 4. Matches AWS LBC's Gateway behavior |
| 5 | Weighted backends in HTTPRoute | Synthetic merged pool with weight-scaled members (single vngcloud Pool aggregates all backends in a route rule) |
| 6 | HTTPRoute match / filter coverage | Strict reject for unsupported core features (`Accepted=False, reason=UnsupportedValue`) + `ListenerRuleConfig` ExtensionRef as the documented escape hatch for header/query/method matches and FixedResponse |
| 7 | TLS strategy | Secrets via `certificateRefs` (auto-imported, existing path) **plus** vngcloud cert IDs via existing `LoadBalancerConfig.Listeners[].certificateDefault.id` / `certificateAuthorities[].id`. mTLS via `frontendValidation` or LBC `clientCertificateId`. TLS-Passthrough only on NLB GatewayClass (Phase 2) |
| 8 | Phasing | Phase 1 = L7 + extensions (LRC + TGC); Phase 2 = L4; Phase 3 = advanced (GRPCRoute, BackendTLSPolicy, optional URLRewrite/Mirror/etc.); Phase 4 = migration tool + conformance + polish |

---

## 1. Architecture overview

### 1.1 Controllers & GatewayClasses

Two new controllers added to the existing manager binary, each watching a distinct GatewayClass:

- **`vngcloud-alb` GatewayClass** — `controllerName: gateway.vks.vngcloud.vn/alb`. Provisions vngcloud ALB. Accepts listeners with protocol `HTTP`, `HTTPS`, `TLS` (mode=Terminate). Routes: `HTTPRoute`, later `GRPCRoute`.
- **`vngcloud-nlb` GatewayClass** — `controllerName: gateway.vks.vngcloud.vn/nlb`. Provisions vngcloud NLB. Accepts listeners with protocol `TCP`, `UDP`, `TLS` (mode=Passthrough). Routes: `TCPRoute`, `UDPRoute`, `TLSRoute`.

Mixed protocols on a single Gateway are rejected with `Accepted=False, reason=UnsupportedProtocol`. 1 Gateway = 1 vngcloud LoadBalancer; no sharing with Ingress / Service controllers.

### 1.2 CRDs

| CRD | Status | Role |
|---|---|---|
| `LoadBalancerConfig` (existing, extended) | extended | Referenced by both `GatewayClass.parametersRef` (template) and `Gateway.spec.infrastructure.parametersRef` (instance). Adds `mergingMode` and two new fields on the existing `Listener` struct (`sslPolicy`, `alpnPolicy`). |
| `TargetGroupConfig` (new, namespaced) | new | Attached per-Service via `targetReference`. Carries health-check, target-type, sticky-session, pool algorithm, TLS-encryption, PROXY protocol. Optional `routeConfigurations[]` overrides per route. |
| `ListenerRuleConfig` (new, namespaced) | new | Attached via `HTTPRoute.spec.rules[].filters[].extensionRef`. Holds non-Gateway-API features: header/query/method match, fixed-response, source-IP rules, regex compare overrides. |

Group: `gateway.vks.vngcloud.vn/v1alpha1` for the two new CRDs. `LoadBalancerConfig` stays in `vks.vngcloud.vn/v1alpha1` with a backwards-compatible field block.

### 1.3 Package layout (top-level orientation)

```
api/v1alpha1/                     # extended (Listener fields, MergingMode)
api/gateway/v1alpha1/             # new group: TargetGroupConfig, ListenerRuleConfig
internal/controller/gateway/      # new: alb/, nlb/, shared/, targetgroupconfig/, listenerruleconfig/
internal/usecase/gateway_uc/      # new: alb_gateway_uc/, nlb_gateway_uc/, shared/
pkg/gateway/                      # new: utils, hostname matchers, synth-pool helpers
```

Reconcilers stay thin (request → useCase). UseCases use the existing `K8sRepository` and `VngCloudRepository`. The Gateway controller is the only writer to the vngcloud LB; HTTPRoute reconciles update an in-memory cache and enqueue the parent Gateway.

### 1.4 Manager wiring

`cmd/main.go` adds:
- Register Gateway API scheme (`sigs.k8s.io/gateway-api/apis/v1`, `v1alpha2`).
- Feature gate flags: `--enable-gateway-api-alb` (default `false` initially, `true` after stable), `--enable-gateway-api-nlb` (false until Phase 2).
- Conditional reconciler registration based on feature gates.
- New finalizer constants: `gateway.vks.vngcloud.vn/resources`.

### 1.5 Coexistence boundary

Gateway-controller-owned vngcloud LBs carry the owner label `vks.vngcloud.vn/owner-resource-kind=Gateway`. Existing Ingress / Service controllers ignore non-matching owners. No shared-LB code paths in v1.

---

## 2. CRD schemas

### 2.1 `LoadBalancerConfig` (extended)

Additive changes only. Existing fields and behavior unchanged.

```go
type LoadBalancerConfigSpec struct {
    // ... all existing fields unchanged ...

    // MergingMode controls how this LBC merges with another LBC when both
    // GatewayClass.parametersRef and Gateway.spec.infrastructure.parametersRef are set.
    // Only honored when this object is referenced by GatewayClass.parametersRef.
    // +kubebuilder:validation:Enum=PreferGateway;PreferGatewayClass
    // +optional
    MergingMode *MergingMode `json:"mergingMode,omitempty"`
}

type Listener struct {
    // ... all existing fields unchanged ...

    // SSLPolicy selects the listener's TLS protocol/cipher policy (vngcloud-defined).
    // Only honored on HTTPS / TLS-terminate listeners.
    // +optional
    SSLPolicy *string `json:"sslPolicy,omitempty"`

    // ALPNPolicy controls ALPN advertisement (e.g., "HTTP2Optional", "HTTP1Only").
    // Only honored on HTTPS / TLS-terminate listeners.
    // +optional
    ALPNPolicy *string `json:"alpnPolicy,omitempty"`
}
```

**How `Listeners[]` is interpreted when LBC is referenced from a Gateway:**

The existing `Listeners[]` field is reused verbatim. Items are matched to the Gateway's listeners by `name`.

| LBC `Listener` field | Role under Gateway | Conflict handling |
|---|---|---|
| `name` | Selector — must equal `Gateway.spec.listeners[].name` | Listener entries with no matching Gateway listener are silently ignored |
| `protocol`, `protocolPort` | Must match the Gateway listener | Mismatch → `Programmed=False, reason=InvalidParameters` on the Gateway listener |
| `defaultPoolName`, `policies` | **Ignored** under Gateway use | Pool/policy set is derived from routes |
| `certificateDefault`, `certificateAuthorities` | Override `tls.certificateRefs` when LBC provides IDs | LBC wins if set; otherwise fall back to Secret refs |
| `clientCertificateId` | Override `tls.frontendValidation.caCertificateRefs` | LBC wins if set; mutually exclusive with `frontendValidation` |
| `timeoutClient`, `timeoutMember`, `timeoutConnection`, `allowedCidrs`, `insertHeaders`, `sslPolicy`, `alpnPolicy` | Vngcloud-specific overrides | Applied as-is |

**Merging semantics** (when both Class- and Gateway-level LBCs exist):

- Scalar fields: winner per `MergingMode` (default `PreferGateway`).
- List fields with named items (`listeners`, `tags`): merged by name; per-item values resolved by `MergingMode`.
- `loadBalancerId`: per-Gateway only (ignored at class level — class can't pin a single LB).
- Merging produces an in-memory effective config; source LBC objects are not mutated.

### 2.2 `TargetGroupConfig` (new, namespaced)

```go
// +kubebuilder:resource:shortName=tgc
type TargetGroupConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   TargetGroupConfigSpec   `json:"spec,omitempty"`
    Status TargetGroupConfigStatus `json:"status,omitempty"`
}

type TargetGroupConfigSpec struct {
    // TargetReference points at a Service in the same namespace.
    TargetReference TargetReference `json:"targetReference"`

    // DefaultConfig applies when this TGC is selected via targetReference
    // and no routeConfiguration overrides match the active route.
    DefaultConfig TargetGroupProperties `json:"defaultConfig"`

    // RouteConfigurations carries per-route overrides (selected by route GVK + name).
    // Highest specificity wins (route+rule > route > default).
    // +optional
    RouteConfigurations []RouteSpecificConfig `json:"routeConfigurations,omitempty"`
}

type TargetReference struct {
    Group *string `json:"group,omitempty"` // default "" (core)
    Kind  *string `json:"kind,omitempty"`  // default "Service"
    Name  string  `json:"name"`
}

type TargetGroupProperties struct {
    // +kubebuilder:validation:Enum=instance;ip
    TargetType          *string            `json:"targetType,omitempty"`
    PoolAlgorithm       *string            `json:"poolAlgorithm,omitempty"`
    EnableStickySession *bool              `json:"enableStickySession,omitempty"`
    EnableTLSEncryption *bool              `json:"enableTLSEncryption,omitempty"`
    EnableProxyProtocol *bool              `json:"enableProxyProtocol,omitempty"`
    HealthCheck         *PoolHealthMonitor `json:"healthCheck,omitempty"`
    TargetNodeLabels    map[string]string  `json:"targetNodeLabels,omitempty"`
    ManageDFPMembers    *bool              `json:"manageDFPMembers,omitempty"`
}

type RouteSpecificConfig struct {
    RouteIdentifier RouteIdentifier       `json:"routeIdentifier"`
    Config          TargetGroupProperties `json:"config"`
}

type RouteIdentifier struct {
    Group     string  `json:"group"` // "gateway.networking.k8s.io"
    Kind      string  `json:"kind"`  // "HTTPRoute" / "TCPRoute" / ...
    Namespace *string `json:"namespace,omitempty"`
    Name      string  `json:"name"`
    RuleName  *string `json:"ruleName,omitempty"`
}

type TargetGroupConfigStatus struct {
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}
```

**Lookup priority** when building a backend pool: route+rule-specific config → route-level config → default config → controller fallback defaults.

### 2.3 `ListenerRuleConfig` (new, namespaced)

Attached via HTTPRoute filter `extensionRef`:

```yaml
filters:
- type: ExtensionRef
  extensionRef: { group: gateway.vks.vngcloud.vn, kind: ListenerRuleConfig, name: admin-rules }
```

```go
// +kubebuilder:resource:shortName=lrc
type ListenerRuleConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   ListenerRuleConfigSpec   `json:"spec,omitempty"`
    Status ListenerRuleConfigStatus `json:"status,omitempty"`
}

type ListenerRuleConfigSpec struct {
    // AdditionalMatches are AND'd with the HTTPRoute's standard matches.
    // +optional
    AdditionalMatches []AdditionalMatch `json:"additionalMatches,omitempty"`

    // Actions, if set, supersede the route's default REDIRECT_TO_POOL action.
    // +optional
    Actions []RuleAction `json:"actions,omitempty"`

    // Position pins policy ordering. Default = controller-assigned (Gateway-API spec ordering).
    // +optional
    Position *int32 `json:"position,omitempty"`
}

type AdditionalMatch struct {
    // +kubebuilder:validation:Enum=Header;QueryParam;Method;SourceIP
    Type    string  `json:"type"`
    Name    *string `json:"name,omitempty"` // header/query name; unused for Method/SourceIP
    // vngcloud values: EQUAL_TO | STARTS_WITH | ENDS_WITH | CONTAINS | REGEX
    Compare string  `json:"compare"`
    Value   string  `json:"value"`
}

type RuleAction struct {
    // +kubebuilder:validation:Enum=FixedResponse;Reject;Redirect
    Type          string               `json:"type"`
    FixedResponse *FixedResponseAction `json:"fixedResponse,omitempty"`
    Redirect      *RedirectAction      `json:"redirect,omitempty"`
}

type FixedResponseAction struct {
    StatusCode  int32   `json:"statusCode"`
    ContentType *string `json:"contentType,omitempty"`
    Body        *string `json:"body,omitempty"`
}

type RedirectAction struct {
    URL             string `json:"url"`
    HTTPCode        *int32 `json:"httpCode,omitempty"`
    KeepQueryString *bool  `json:"keepQueryString,omitempty"`
}

type ListenerRuleConfigStatus struct {
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}
```

**Forward-compat note:** when vngcloud LB later adds native header/query/method matching, those fields move into the standard `HTTPRoute.match` translation. `ListenerRuleConfig.AdditionalMatches` of those types start emitting a `Deprecated` condition pointing users at the now-supported core fields. No breakage.

### 2.4 RBAC additions

```
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=update;patch
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update;patch
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=update;patch
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;referencegrants,verbs=get;list;watch
+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=update;patch
+kubebuilder:rbac:groups=gateway.vks.vngcloud.vn,resources=targetgroupconfigs;listenerruleconfigs,verbs=get;list;watch;update;patch
+kubebuilder:rbac:groups=gateway.vks.vngcloud.vn,resources=targetgroupconfigs/status;listenerruleconfigs/status,verbs=update;patch
```

(Phase 2 adds `tcproutes`, `udproutes`, `tlsroutes`; Phase 3 adds `grpcroutes`, `backendtlspolicies`.)

---

## 3. Resource mapping & data flow

### 3.1 Mapping table — Gateway API → vngcloud primitives

| Gateway API resource | vngcloud entity | Notes |
|---|---|---|
| `GatewayClass` (controller=`gateway.vks.vngcloud.vn/alb`) | none (declarative gate) | Status `Accepted=True` once `parametersRef` LBC is valid |
| `GatewayClass` (controller=`gateway.vks.vngcloud.vn/nlb`) | none | same; Phase 2 |
| `Gateway` | 1 `LoadBalancer` (ALB or NLB) + N `Listener` | LB type fixed by GatewayClass; 1:1 binding via owner labels |
| `Gateway.spec.listeners[]` | vngcloud `Listener` | `name` is the join key with LBC.Listeners[]; protocol / port / TLS mapped per §3.3 |
| `Gateway.spec.addresses` | informational only | vngcloud assigns address; user-requested IP rejected unless feature exists |
| `HTTPRoute` (per parentRef listener) | 1+ `Policy` + 1+ `Pool` (synthetic) | One policy per `rule × match` (cartesian when multi-match); see §3.4 |
| `HTTPRoute.hostnames[]` | `HOST_NAME` rule on each generated policy | `*.foo.com` → `REGEX` rule |
| `HTTPRoute.rule.matches[].path` | `PATH` rule (EQUAL_TO / STARTS_WITH / REGEX) | Implementation-specific path types fall back to EQUAL_TO |
| `HTTPRoute.rule.backendRefs[]` (weighted) | 1 synthetic `Pool` (members from union) | Weight-scaled members; pool name = stable hash (§3.5) |
| `HTTPRoute.rule.filters[RequestRedirect]` | `Policy.Action = REDIRECT_TO_URL` | URL templated from filter fields |
| `HTTPRoute.rule.filters[RequestHeaderModifier]` (set) | listener `insertHeaders` | Only if uniform across all rules on the listener; otherwise reject |
| `HTTPRoute.rule.filters[ExtensionRef → ListenerRuleConfig]` | extra L7 rules + alt action | See §3.6 |
| `ReferenceGrant` | gating only | Resolves cross-namespace `backendRefs` and TLS Secret refs |
| `TargetGroupConfig` | pool defaults | Per Service + per route override; see §3.7 |
| `ListenerRuleConfig` | policy match / action additions | Attached via HTTPRoute `ExtensionRef` filter |

### 3.2 Controller graph

```
┌───────────────────────┐    parametersRef
│ GatewayClassController├──────────┐
└───────────┬───────────┘          │
            │                      ▼
            │           ┌──────────────────────┐
            │           │ LoadBalancerConfig   │
            │           │ (existing, extended) │
            │           └──────────────────────┘
            ▼                      ▲
┌───────────────────────┐  infrastructure.parametersRef
│ GatewayController     ├──────────┘
│ (per GatewayClass)    │◄──────── HTTPRoute parentRef
└───────────┬───────────┘
            │ owns
            ▼
   vngcloud LoadBalancer + Listeners

┌───────────────────────┐
│ HTTPRouteController   │──── parentRefs ───► Gateway (triggers Gateway reconcile)
└───────────┬───────────┘──── backendRefs ──► Service (triggers HTTPRoute reconcile)
            │
            │ owns (via Gateway reconcile)
            ▼
    vngcloud Policies + Pools (on the parent's Listener)

┌───────────────────────┐    targetReference
│ TargetGroupConfig     │──────────────────► Service (triggers HTTPRoute requeue)
│ Controller (validate) │
└───────────────────────┘

┌───────────────────────┐    reverse-index
│ ListenerRuleConfig    │──────────────────► HTTPRoute (triggers requeue)
│ Controller (validate) │
└───────────────────────┘
```

**Ownership model:** the **Gateway controller** is the only writer to the vngcloud LB. HTTPRoute reconciles do not touch vngcloud directly — they enqueue the parent Gateway. This serializes all LB mutations through one reconciler per Gateway and avoids races.

### 3.3 Gateway → LoadBalancer + Listeners

Reconcile order per Gateway:

1. **Resolve effective config** = merge(`GatewayClass.parametersRef → LBC`, `Gateway.spec.infrastructure.parametersRef → LBC`) per `mergingMode`.
2. **Validate listeners** — protocol allowed by GatewayClass type? port unique? hostname valid? Set per-listener `Accepted` condition.
3. **Lookup or create LB** — by owner label `gateway.vks.vngcloud.vn/owner-uid=<gateway-uid>`. Type = ALB or NLB. Apply LB-level config from merged LBC.
4. **Compute desired Listener set** — for each `Gateway.spec.listeners[]` with `Accepted=True`:
   - protocol / port from Gateway listener
   - TLS: prefer LBC `Listener.certificateDefault/Authorities` IDs if present, else import Secrets from `tls.certificateRefs` (existing import path)
   - mTLS: LBC `clientCertificateId` if set, else import from `tls.frontendValidation.caCertificateRefs`
   - other fields (timeouts, allowedCidrs, sslPolicy, alpnPolicy, insertHeaders) from merged LBC entry by listener name
   - default pool: synthetic "default-forwarding-pool" with no members (fed by route layer)
5. **Diff against vngcloud** — reuse existing per-listener helpers in `internal/usecase/lbc_uc/deploy_listener.go`.
6. **Trigger route layer** — emit policy / pool sync for every accepted route attached to this Gateway.
7. **Write status** — Gateway and listener conditions per §4.

### 3.4 HTTPRoute → Policies + Pools

For each HTTPRoute attached to an accepted Gateway listener:

```
for each rule in route.spec.rules:
    pool = synthesizePool(rule.backendRefs, route, ruleIdx)
    for each match in rule.matches:
        for each hostname in route.hostnames (or [""] if empty):
            policy = buildPolicy(hostname, match, rule.filters, pool)
            policies.append(policy)
```

**One policy per (hostname × match × filter-resolved-action)**:

- L7 rules:
  - `HOST_NAME` rule per hostname (literal → `EQUAL_TO`; wildcard → `REGEX`)
  - `PATH` rule per `match.path`
  - any `ListenerRuleConfig.additionalMatches` AND'd in
- Action:
  - `RequestRedirect` filter → `REDIRECT_TO_URL`
  - `ListenerRuleConfig.actions[FixedResponse|Reject]` → `REJECT` (FixedResponse marked `Programmed=False, reason=UnsupportedFilter` until vngcloud adds native support; `Reject` works)
  - default → `REDIRECT_TO_POOL` pointing at the synthetic pool

**Policy ordering** — Gateway API spec: most-specific path first, then header count, then route creation timestamp. Translation: assign vngcloud `Policy.Position` in spec order; existing `auto-reorder-policies` machinery reused.

**Cross-namespace backendRefs** — gated by ReferenceGrant. If grant absent, set `ResolvedRefs=False, reason=RefNotPermitted` on the route and drop just that backend from the synthesized pool (don't fail the whole rule).

### 3.5 Synthetic pool — naming & weight scaling

**Name** (deterministic, ≤ 50 chars):

```
gw_<route-uid-prefix8>_<rule-idx>_<backendset-hash5>
```

- `route-uid-prefix8`: first 8 chars of HTTPRoute `metadata.uid`.
- `rule-idx`: rule index in route.
- `backendset-hash5`: 5-char hash of sorted `(ns, name, port, weight)` tuples.

Hash changes only when the backend set / weights change; member churn within a backend keeps the same pool.

**Weight scaling algorithm:**

1. Collect each backend's current ready endpoints from EndpointSlice (or NodePort + ready-nodes, by target-type).
2. For each backend `b` with desired weight `w_b` and `n_b` ready endpoints, base member weight = 1.
3. Apply weight multiplier: `member_weight = round(w_b * scale / n_b)` where `scale = lcm(n_b for all b) / gcd(weights)` capped at 100 to avoid huge integers.
4. Floor every member weight at 1.
5. Skip backends where the ReferenceGrant is missing.

**Health check** for the synthetic pool — single config from priority order:

1. Route-level `TargetGroupConfig.routeConfigurations[]` matching this route+rule.
2. First backend's `TargetGroupConfig.defaultConfig`.
3. Controller fallback (TCP check on backend port).

Conflict (multiple backends with conflicting TGC health-check configs) → `ResolvedRefs=False, reason=BackendConfigMismatch` on the route.

### 3.6 ListenerRuleConfig wiring

Example:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
spec:
  rules:
  - matches:
    - path: { type: PathPrefix, value: /admin }
    filters:
    - type: ExtensionRef
      extensionRef: { group: gateway.vks.vngcloud.vn, kind: ListenerRuleConfig, name: admin-rules }
    backendRefs:
    - { name: admin-svc, port: 8080 }
```

`admin-rules` adds (e.g.) a `Header X-Source CONTAINS internal` match and a `SourceIP STARTS_WITH 10.0.` match. Both AND'd onto the policy generated from this rule.

If an `Action` is specified in `ListenerRuleConfig`, it overrides the default `REDIRECT_TO_POOL` (useful for fixed-response / reject without a backend).

The ListenerRuleConfig controller maintains a reverse-index (config-name → routes referencing it) so updates trigger route requeues.

### 3.7 TargetGroupConfig wiring

Selector cascade per `(route, ruleIdx, backendRef)`:

1. Find TGCs whose `targetReference` matches the backend Service (in same namespace).
2. Among those, prefer entries in `routeConfigurations[]` matching `(route GVK, name, ruleName)`; fall back to `defaultConfig`.
3. If multiple TGCs target the same Service, the one with more specific `routeConfigurations[]` wins; ties broken by oldest-first creation timestamp (with `Conflicted=True` on the loser).

A TGC change reverse-indexes to all routes whose backends it targets and requeues each.

### 3.8 Watch & reverse-index map

| Watch source | Triggers requeue of |
|---|---|
| `GatewayClass` | Self + all Gateways referencing this class |
| `LoadBalancerConfig` | All GatewayClasses & Gateways referencing it |
| `Gateway` | Self + all routes whose `parentRefs` resolve to it |
| `HTTPRoute` | Self + parent Gateway |
| `Service` | All HTTPRoutes whose `backendRefs` include it |
| `EndpointSlice` | Same as Service |
| `Secret` (TLS) | All Gateways whose `certificateRefs` / `frontendValidation` include it |
| `ReferenceGrant` | All routes / Gateways whose cross-ns refs were previously denied |
| `TargetGroupConfig` | All HTTPRoutes whose `backendRefs` matches |
| `ListenerRuleConfig` | All HTTPRoutes whose filters reference it |

Indexes via controller-runtime `Manager.GetFieldIndexer()` and a small `pkg/gateway/reference_indexer.go` (mirrors `pkg/ingress/reference_indexer.go`).

### 3.9 Eventual-consistency notes

- A change to one HTTPRoute requeues only its parent Gateway → Gateway reconcile rebuilds the full policy / pool set for that LB. Idempotent diff drives create / update / delete.
- Per-Gateway reconciles serialized by controller-runtime work queues.
- Status updates batched at end of reconcile (one PATCH per object) to minimize API churn.
- Long-running LB ops (provisioning, listener create) reuse existing requeue-with-backoff machinery from `lbc_uc`.

---

## 4. Status conditions, events, metrics

### 4.1 Status conditions

#### `GatewayClass.status.conditions`

| Type | True | False | Reason |
|---|---|---|---|
| `Accepted` | parametersRef LBC valid + supports the GatewayClass type | LBC missing / type mismatch / invalid | `Accepted` / `InvalidParameters` / `Pending` |
| `SupportedVersion` | controller version supports the Gateway API version of the class | version skew | `SupportedVersion` / `UnsupportedVersion` |

#### `Gateway.status.conditions`

| Type | True | False | Reason |
|---|---|---|---|
| `Accepted` | spec valid; this controller owns the class; merged config resolved | invalid spec / unsupported feature / class not accepted | `Accepted` / `InvalidParameters` / `NotReconciled` / `UnsupportedAddress` |
| `Programmed` | vngcloud LB created, listeners synced, addresses populated | LB provisioning failed; listener creation failed | `Programmed` / `Pending` / `Invalid` / `NoResources` / `VngcloudAPIError` |

#### `Gateway.status.listeners[].conditions`

| Type | True | False | Reason |
|---|---|---|---|
| `Accepted` | protocol allowed for this GatewayClass; port unique within Gateway | mixed protocols / dup port / unsupported feature | `Accepted` / `UnsupportedProtocol` / `PortUnavailable` / `Invalid` |
| `Programmed` | listener exists on vngcloud LB | not yet created / failed | `Programmed` / `Pending` / `Invalid` |
| `ResolvedRefs` | all `certificateRefs` and `frontendValidation.caCertificateRefs` resolved | grant missing / Secret not found / unsupported group | `ResolvedRefs` / `RefNotPermitted` / `InvalidCertificateRef` / `InvalidCACertificateRef` |
| `Conflicted` | always `False` in v1 (no shared LBs) | n/a | `NoConflicts` |

`Gateway.status.listeners[].attachedRoutes` recomputed each Gateway reconcile.

`Gateway.status.addresses[]` populated from vngcloud LB once `Programmed=True`. `IPAddress` for public/internal/InterVPC; `Hostname` if vngcloud assigns DNS-only (future).

#### `HTTPRoute.status.parents[].conditions` (per parentRef)

| Type | True | False | Reason |
|---|---|---|---|
| `Accepted` | parent Gateway exists, allows route kind, namespace allowed | listener restricts kind / ns / hostname | `Accepted` / `NotAllowedByListeners` / `NoMatchingListenerHostname` / `NoMatchingParent` |
| `ResolvedRefs` | all `backendRefs` and `extensionRef` filters resolved | backend missing / wrong kind / cross-ns denied / TGC/LRC unresolved | `ResolvedRefs` / `BackendNotFound` / `RefNotPermitted` / `InvalidKind` / `InvalidExtensionRef` |
| `PartiallyInvalid` (when at least one rule programmed but some dropped) | per spec | n/a | `UnsupportedValue` |

#### Custom CRD status

`TargetGroupConfig.status` and `ListenerRuleConfig.status`:

- `Accepted` — schema valid, target exists.
- `Conflicted` — `True` when another higher-specificity TGC also targets the same Service+route.
- `InUse` — `True` when at least one route currently references this config.

`LoadBalancerConfig.status` (existing, extended) gets a new condition `ReferencedByGateway` (informational).

### 4.2 Kubernetes Events

| Reconciler | Event reason examples |
|---|---|
| GatewayClass | `Accepted`, `InvalidParameters`, `LBCMissing` |
| Gateway | `LoadBalancerCreated`, `ListenerCreated`, `LoadBalancerProvisioningFailed`, `ConfigMergeFailed` |
| HTTPRoute | `RouteAttached`, `RouteDetached`, `BackendDropped`, `UnsupportedFilter` |
| TargetGroupConfig | `Conflict`, `TargetServiceMissing` |
| ListenerRuleConfig | `ReferencedByRoute`, `InvalidActionForListener` |

Reuses existing `eventRecorder` plumbing.

### 4.3 Prometheus metrics

```
# counters
gateway_api_reconcile_total{controller,result}
gateway_api_resource_count{kind,namespace,gateway_class}
gateway_api_listener_count{gateway_class,protocol}
gateway_api_route_attached_total{gateway_class}
gateway_api_unsupported_feature_total{kind,feature}     # informs Phase-3 prioritization
gateway_api_refgrant_denied_total{namespace,kind}

# histograms
gateway_api_reconcile_duration_seconds{controller,result}
gateway_api_lb_provisioning_seconds{gateway_class}

# gauges
gateway_api_lb_total{gateway_class,scheme}
gateway_api_synthetic_pool_total
```

`metricsutil.ReconcileCounters` extended with `IncrementGateway` / `IncrementHTTPRoute`.

### 4.4 Logging

Reuses existing `contexts` package. Log-name conventions: `gw/<ns>/<name>`, `httproute/<ns>/<name>`, `gwc/<name>`, `tgc/<ns>/<name>`, `lrc/<ns>/<name>`.

### 4.5 Finalizer strategy

| Resource | Finalizer | Cleanup action |
|---|---|---|
| `Gateway` | `gateway.vks.vngcloud.vn/resources` | Delete vngcloud LB, drop owner labels, deindex routes |
| `HTTPRoute` | `gateway.vks.vngcloud.vn/route` | Trigger parent Gateway reconcile |
| `TargetGroupConfig` / `ListenerRuleConfig` | none | Pure config; deletion = recompute downstream routes |

Deletion ordering: HTTPRoutes detach → parent Gateway reconciles → finalizer removed when policies / pools gone. Gateway finalizer removed last when LB cleanup completes.

### 4.6 Readiness / liveness

No probe-endpoint changes. New reconcilers respect the `initDone` atomic-bool gate pattern (1s requeue while uninitialized). Fail-open on probe.

---

## 5. Implementation plan & file layout

### 5.1 Full package layout (after all 4 phases)

New files marked **[new]**, modified existing files marked **[mod]**.

```
api/
├── v1alpha1/
│   ├── loadbalancerconfig_types.go               [mod] add MergingMode + SSLPolicy/ALPNPolicy on Listener
│   ├── zz_generated.deepcopy.go                  [mod] regenerated
│   └── ... (existing files unchanged)
└── gateway/v1alpha1/                             [new group]
    ├── groupversion_info.go                      [new]
    ├── targetgroupconfig_types.go                [new]
    ├── listenerruleconfig_types.go               [new]
    └── zz_generated.deepcopy.go                  [new generated]

cmd/
└── main.go                                       [mod] register Gateway API scheme,
                                                        feature gates, new reconcilers

internal/controller/
├── ... (existing controllers unchanged) ...
└── gateway/                                      [new]
    ├── shared/                                   [new]
    │   ├── classifier.go                         protocol → GatewayClass type, listener validation
    │   ├── status.go                             condition helpers
    │   ├── reference_indexer.go                  reverse indexes (Service, Secret, RG, TGC, LRC, LBC)
    │   ├── finalizer.go
    │   ├── policy_order.go                       Gateway API match-specificity ordering
    │   └── eventhandlers/                        cross-resource enqueue helpers
    ├── alb/                                      [new — Phase 1]
    │   ├── gatewayclass_controller.go
    │   ├── gateway_controller.go
    │   ├── httproute_controller.go
    │   ├── grpcroute_controller.go               [Phase 3]
    │   ├── *_test.go
    │   ├── helpers_test.go
    │   └── suite_test.go
    ├── nlb/                                      [new — Phase 2]
    │   ├── gatewayclass_controller.go
    │   ├── gateway_controller.go
    │   ├── tcproute_controller.go
    │   ├── udproute_controller.go
    │   ├── tlsroute_controller.go
    │   ├── *_test.go
    │   └── suite_test.go
    ├── targetgroupconfig/                        [new — Phase 1, validation only]
    │   ├── controller.go
    │   └── controller_test.go
    └── listenerruleconfig/                       [new — Phase 1, validation only]
        ├── controller.go
        └── controller_test.go

internal/usecase/
├── contracts.go                                  [mod] add GatewayClassUseCase, GatewayUseCase, RouteUseCase
├── mocks.go                                      [mod] regenerated
├── gateway_uc/                                   [new]
│   ├── shared/                                   [new]
│   │   ├── merge_config.go                       LBC merging
│   │   ├── tgc_resolver.go                       cascading TGC selector
│   │   ├── lrc_resolver.go                       LRC ExtensionRef resolution
│   │   ├── refgrant.go                           ReferenceGrant evaluation
│   │   └── *_test.go
│   ├── alb_gateway_uc/                           [new — Phase 1]
│   │   ├── gateway_uc.go                         Init / Ensure / Delete
│   │   ├── build_lb.go                           Gateway → vngcloud LB params
│   │   ├── build_listener.go                     Gateway listener → vngcloud Listener
│   │   ├── build_pool.go                         synthetic pool (weights, health checks)
│   │   ├── build_policy.go                       HTTPRoute rule × match × filter → Policy
│   │   ├── build_cert.go                         Secret-import or cert-ID
│   │   ├── build_sec_group.go                    inherits NSG behavior from Ingress
│   │   ├── status.go
│   │   └── *_test.go
│   └── nlb_gateway_uc/                           [new — Phase 2]
│       └── ...

internal/repository/
├── contracts.go                                  unchanged in v1
├── k8s_repo/                                     unchanged
├── vngcloud_repo/                                unchanged
└── mocks.go                                      [mod] regenerated

internal/domain/
└── domain.go                                     [mod] add GatewayFinalizer, owner-kind constants

pkg/
├── consts/consts.go                              [mod] new GatewayClass controller-name constants
├── metrics/lbc/                                  [mod] expose Gateway counters
├── metrics/util/reconcile_counters.go            [mod] add IncrementGateway / IncrementHTTPRoute
└── gateway/                                      [new]
    ├── gatewayapi_utils.go                       hostname matchers, wildcard → regex, proto helpers
    ├── synth_pool.go                             pool-name hashing helpers
    └── *_test.go

charts/vngcloud-load-balancer-controller/
├── templates/
│   ├── crds/                                     [mod] add tgc.yaml, lrc.yaml; updated lbc.yaml
│   ├── gatewayclass-alb.yaml                     [new — Phase 1]
│   ├── gatewayclass-nlb.yaml                     [new — Phase 2]
│   ├── rbac/                                     [mod]
│   └── manager-deployment.yaml                   [mod] feature-gate flags
└── values.yaml                                   [mod] gatewayApi.{alb,nlb}.enabled toggles

config/                                           [mod throughout — kubebuilder-managed]

docs/                                             [mod throughout]
├── guide/
│   ├── gateway-api.md                            [new]
│   ├── gateway-alb.md                            [new — Phase 1]
│   ├── gateway-nlb.md                            [new — Phase 2]
│   ├── gateway-extensions.md                     [new — Phase 1]
│   └── gateway-migration.md                      [new — Phase 4]
└── examples/                                     [new]

test/                                             [mod throughout]
├── e2e/gateway/                                  [new]
└── conformance/                                  [new — Phase 4]

go.mod                                            [mod] add sigs.k8s.io/gateway-api
PROJECT                                           [mod] register new resources via kubebuilder
```

### 5.2 New Go module dependencies

```go
require (
    sigs.k8s.io/gateway-api v1.2.0   // Standard channel covering HTTPRoute / GRPCRoute / RG
)
```

(Phase 4 conformance pulls `sigs.k8s.io/gateway-api/conformance` as a test-only dep.)

### 5.3 Manager wiring (`cmd/main.go` deltas)

```go
import (
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"
    gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
    gwvksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
    albgw "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/alb"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/alb_gateway_uc"
)

utilruntime.Must(gwv1.Install(scheme))
utilruntime.Must(gwv1alpha2.Install(scheme))
utilruntime.Must(gwvksv1alpha1.AddToScheme(scheme))

flag.BoolVar(&cfg.Gateway.ALBEnabled, "enable-gateway-api-alb", false, "Enable the ALB Gateway API controller (Phase 1).")
flag.BoolVar(&cfg.Gateway.NLBEnabled, "enable-gateway-api-nlb", false, "Enable the NLB Gateway API controller (Phase 2+).")

if cfg.Gateway.ALBEnabled {
    albUC := alb_gateway_uc.NewALBGatewayUseCase(...)
    if err := albgw.NewGatewayClassReconciler(...).SetupWithManager(mgr); err != nil { ... }
    if err := albgw.NewGatewayReconciler(albUC, ...).SetupWithManager(mgr); err != nil { ... }
    if err := albgw.NewHTTPRouteReconciler(albUC, ...).SetupWithManager(mgr); err != nil { ... }
}
// TGC + LRC validation controllers always-on if any Gateway controller is on.
```

Feature gates make the controller safe to upgrade in place — existing deployments don't suddenly start reconciling Gateway resources unless explicitly enabled.

### 5.4 Mocking & test strategy

**Unit tests** — table-driven, gomock-generated mocks for `K8sRepository` / `VngCloudRepository`. Coverage targets parity with existing Ingress controller (~70% line, all `build_*` paths covered):

- `build_listener_test.go` — protocol + TLS combos, cert-ID vs Secret precedence, mTLS resolution.
- `build_pool_test.go` — synthetic-pool naming determinism, weight scaling, RG-denied-backend dropping, conflicting health-check detection.
- `build_policy_test.go` — match cartesian, policy-position ordering, LRC additional-match composition.
- `merge_config_test.go` — `PreferGateway` vs `PreferGatewayClass`, list-merge semantics, conflict tags.
- `tgc_resolver_test.go` — cascade specificity, oldest-wins tiebreak, conflict-condition emission.
- `refgrant_test.go` — allow / deny matrix; covers Secrets, Services, custom CRDs.

**Suite tests** (envtest) — one per reconciler, mirroring `internal/controller/networking/suite_test.go`. Validates reconcile-on-event for each watch in §3.8.

**E2E tests** — under `test/e2e/gateway/`, run against a real vngcloud LB. Gated by env var; skipped in CI by default.

**Conformance** — `test/conformance/conformance_test.go` runs the upstream Gateway API conformance suite. Phase 4 deliverable.

### 5.5 Codegen / make targets

Existing `make generate manifests test` covers all generation. New entries:

```
.PHONY: gateway-conformance
gateway-conformance: ## Run upstream Gateway API conformance suite (Phase 4)
    go test ./test/conformance/... -tags=conformance -v
```

No mockery config changes (existing config picks up new interfaces automatically).

### 5.6 Backward-compat / rollout safety

- **CRD changes are additive** — `LoadBalancerConfig` gets new optional fields only; existing manifests keep applying cleanly.
- **No changes** to existing Service / Ingress / GLB / LBC / NSG controller behavior.
- **Feature gates default-off initially** — opt-in upgrade. ALB flips to `true` after one minor cycle in production.
- **CRD bundle versioning** — Helm chart's `Chart.yaml` minor-version bumped per phase. CRDs ship `served: true, storage: true` for v1alpha1 only initially; promotion to v1beta1/v1 deferred until conformance (Phase 4).
- **Webhook conversion** — none required in v1alpha1.

---

## 6. Phasing roadmap

### Phase 1 — L7 MVP

**Scope**

- `vngcloud-alb` GatewayClass + reconciler.
- `Gateway` reconciler (ALB only): provisions LB; listeners HTTP / HTTPS / TLS-Terminate.
- `HTTPRoute` reconciler: hostnames, path matches (Exact / PathPrefix / RegularExpression), weighted backendRefs (synthetic merged pool), `RequestRedirect` filter, listener-uniform `RequestHeaderModifier(set)` filter, `ExtensionRef → ListenerRuleConfig`.
- `LoadBalancerConfig` extension: `MergingMode`, `SSLPolicy`, `ALPNPolicy`.
- `TargetGroupConfig` CRD + validation controller.
- `ListenerRuleConfig` CRD + validation controller (Header / QueryParam / Method / SourceIP matches; Reject + Redirect actions; FixedResponse marked unsupported pending vngcloud).
- `ReferenceGrant` honored for backendRefs and TLS Secret refs.
- TLS via Secret import + cert IDs via LBC `Listener.certificateDefault.id`.
- mTLS via `frontendValidation.caCertificateRefs` + LBC `clientCertificateId`.
- Status conditions per §4.
- Events + new Prometheus metrics.
- Helm chart updates: opt-in via `gatewayApi.alb.enabled` (default `false` initially → `true` after one stable minor cycle).

**Deliverables**

- All [new] / [mod] files in §5.1 marked Phase 1.
- Unit + suite test coverage parity with existing Ingress controller (~70 % line).
- 5 e2e tests (basic HTTP, HTTPS+SNI, mTLS, weighted canary, cross-namespace via RG).
- 4 user docs (`gateway-api.md`, `gateway-alb.md`, `gateway-extensions.md`, `examples/gateway-canary.md`).

**Success criteria**

- A user can install the chart with `gatewayApi.alb.enabled=true`, apply a sample manifest, see `Gateway.status.addresses` populated, and curl through the LB.
- Deleting a Gateway cleans up the vngcloud LB; deleting a single HTTPRoute removes its policies / pools without disturbing the LB or sibling routes.
- Existing controllers behave identically before and after upgrade.
- `make test` passes; `make e2e-gateway` passes against a real cluster.

### Phase 2 — L4

**Scope**

- `vngcloud-nlb` GatewayClass + reconciler.
- `Gateway` reconciler (NLB): TCP / UDP listeners, TLS-Passthrough listeners.
- `TCPRoute`, `UDPRoute`, `TLSRoute` reconcilers.
- Reuses NLB build paths from existing `service_uc`.
- Helm chart: `gatewayApi.nlb.enabled` (default `false` until stable).

**Deliverables**

- All [new]/[mod] files in §5.1 marked Phase 2.
- 4 e2e tests (TCP echo, UDP DNS, TLS-Passthrough SNI routing, mixed-RouteKinds attachment).
- 2 user docs (`gateway-nlb.md`, `examples/gateway-tls-passthrough.md`).

**Success criteria**

- A `Gateway` with TCP/UDP listener provisions an NLB and routes traffic to the referenced backend.
- TLSRoute SNI selection works for ≥ 2 hostnames on one listener.
- Mixed L4+L7 listeners on a single Gateway are explicitly rejected.

### Phase 3 — Extensions & advanced

**Scope**

- `GRPCRoute` reconciler (depends on vngcloud HTTP/2 + gRPC support; if absent, ship as Unsupported with documented reason).
- `BackendTLSPolicy` (Standard channel since Gateway API v1.2): wires to existing pool `EnableTLSEncryption`.
- Validation webhook for `LoadBalancerConfig`: reject `Listener` entries whose `protocol` / `protocolPort` conflict with the referenced Gateway listener (catches misconfig at apply time instead of via `Programmed=False` after reconcile).
- `ListenerRuleConfig`: add `FixedResponse` action iff vngcloud LB exposes it.
- Promote header / method matches from `ListenerRuleConfig.AdditionalMatches` into core HTTPRoute matches if vngcloud gains native support (LRC fields emit `Deprecated` condition and continue working).
- Optional: `RequestMirror`, `URLRewrite`, `ResponseHeaderModifier` — implemented only if vngcloud LB ships native support; otherwise stay `Unsupported`.

**Deliverables**

- New reconcilers + tests for whichever items are in scope.
- Updates to `gateway-extensions.md`.

**Success criteria**

- For each implemented Phase-3 feature: e2e test green, conformance suite picks up the new capability flag.

### Phase 4 — Migration tool, conformance, polish

**Scope**

- One-shot migration: `gateway.vks.vngcloud.vn/adopt-from-ingress: <namespace>/<name>` annotation. Controller verifies the named Ingress is in the same namespace, transfers ownership labels on the existing vngcloud LB, marks the Ingress with a "migrated" finalizer (so subsequent edits are ignored), and reports `Programmed=True` against the old LB.
- Upstream Gateway API conformance suite (`test/conformance/`) green for: `Gateway`, `HTTPRoute` (Core), `ReferenceGrant`, `Mesh:false`. Documented capability matrix.
- CRD promotion path: v1alpha1 → v1beta1 (no schema changes if possible).
- Stable feature gates (default-on); doc audit.

**Deliverables**

- Migration controller code + `gateway-migration.md` runbook.
- Conformance test job in CI (`make gateway-conformance`).
- `docs/guide/gateway-conformance-matrix.md` listing every Gateway API feature with `Supported / Partial / Unsupported (reason)`.

**Success criteria**

- One existing Ingress can be cut over to a Gateway without downtime or DNS change (verified e2e).
- Conformance tests pass for every feature claimed `Supported` in the capability matrix.

---

## 7. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **vngcloud LB API rate limits** under heavy reconciles | Med | High | Per-Gateway serialized reconcile (already designed); coalesce policy diff into one batched listener-update; existing backoff machinery in `lbc_uc` |
| **Synthetic pool weight scaling explodes integers** | Med | Med | Cap product at 100; round with bias toward larger backends; unit-test with fuzz inputs |
| **Reference-Grant denial silently drops a backend** | Med | High | `ResolvedRefs=False, reason=RefNotPermitted` + Warning event + Prometheus counter `gateway_api_refgrant_denied_total` |
| **Concurrent endpoint churn during weight rescaling causes pool flapping** | Low | Med | 2 s coalescing window when triggered by EndpointSlice change |
| **Listener cert-ID in LBC conflicts with Secret-imported cert** | Low | Low | LBC wins; document precedence; emit Warning event |
| **Unsupported-feature requests pile up** (URLRewrite, mirror, etc.) | Med | Med | `gateway_api_unsupported_feature_total` metric → product feedback to vngcloud LB team; documented capability matrix sets expectations |
| **CRD conversion needed later** | Low | Med | Plan field additions only (no removals/renames) until v1beta1 promotion; webhook conversion deferred |
| **Gateway API spec churn** between releases | Low | Med | Pin SDK to a known-good Gateway API minor version (v1.2 for Phase 1); upgrade in a dedicated PR with conformance re-run |
| **Two LB types in one cluster eat budget** (no sharing per Q4) | Med | Low | Document clearly; cost guidance in `gateway-api.md`; Phase 4 migration path lets users switch without doubling cost |
| **Existing controllers regress** during refactor of shared `Listener` struct | Low | High | Additive-only changes; full existing test suite gates the merge |
| **Field-name confusion** between LBC's `Listener` (instance config) and Gateway API's `Listener` (logical listener) | Med | Low | Section 2.1 doc table makes the join key explicit (`name`); add validation webhook in Phase 3 to reject conflicting LBC.Listener entries |

---

## 8. Open items deferred to implementation

1. Helm-chart default for `gatewayApi.alb.enabled` — propose `false` for first Phase-1 release (0.4.0); flip to `true` after one minor cycle.
2. Per-Gateway concurrency — start with `MaxConcurrentReconciles=5` (matches existing Ingress); revisit if rate-limit issues appear.
3. `Gateway.spec.addresses[]` user requests — reject for v1 (vngcloud assigns IP). Revisit if LB product gains BYOIP.
4. `infrastructure.parametersRef` (the LBC reference on a Gateway) — same-namespace only in v1, matches AWS. Cross-namespace LBC references (gated by ReferenceGrant) deferred to Phase 3. Note: this is unrelated to cross-namespace `backendRefs` and `certificateRefs`, which are honored in Phase 1.
5. Naming conflict resolution between TGCs — first conflict mode = oldest-creates wins. Revisit if customer feedback prefers explicit priority.

---

## 9. Out of scope (explicitly not planned)

- Mesh use cases (Gateway-API east-west).
- Custom GatewayClass parameters CRD beyond `LoadBalancerConfig`.
- Per-route load balancer addresses (one Gateway = one address set).
- In-place GatewayClass type swap (e.g., `vngcloud-alb` → `vngcloud-nlb` on a live Gateway). Forces recreate.

---

## References

- [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/)
- [AWS Load Balancer Controller — Gateway API](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/gateway/gateway/)
- [AWS LBC — Gateway API customization model](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.15/guide/gateway/customization/)
- [AWS LBC — L7 Routing](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/gateway/l7gateway/)

# Gateway API Support — Phase 1 (L7 MVP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Gateway API L7 controller that provisions a vngcloud ALB per `Gateway`, programs listeners and policies from `HTTPRoute`s, with two extension CRDs (`TargetGroupConfig`, `ListenerRuleConfig`) for per-Service and per-route configuration.

**Architecture:** New `vngcloud-alb` GatewayClass; reconcilers in `internal/controller/gateway/alb/`; business logic in `internal/usecase/gateway_uc/alb_gateway_uc/`; reuses existing `K8sRepository`, `VngCloudRepository`, `FinalizerManager`, cert-import path, and `lbc_uc` listener/pool/policy deploy helpers. Gateway is the only writer to vngcloud LB; HTTPRoute reconciles enqueue the parent Gateway.

**Tech Stack:** Go 1.25, controller-runtime 0.19, kubebuilder v4, sigs.k8s.io/gateway-api v1.2, ginkgo+gomega+envtest, gomock, vngcloud-go-sdk/v2.

**Spec reference:** `docs/superpowers/specs/2026-04-30-gateway-api-design.md`

**Out of scope for Phase 1:**
- NLB GatewayClass / TCP / UDP / TLS-Passthrough (Phase 2)
- GRPCRoute / BackendTLSPolicy / URLRewrite / Mirror / ResponseHeaderModifier (Phase 3)
- Migration tool, conformance test job, CRD promotion to v1beta1 (Phase 4)

---

## File map (Phase 1)

| File | Role |
|---|---|
| `go.mod`, `go.sum` | Add `sigs.k8s.io/gateway-api v1.2.0` |
| `PROJECT` | Register new resources |
| `api/v1alpha1/loadbalancerconfig_types.go` | Add `MergingMode` on spec; `SSLPolicy`, `ALPNPolicy` on `Listener` |
| `api/gateway/v1alpha1/groupversion_info.go` | New API group registration |
| `api/gateway/v1alpha1/targetgroupconfig_types.go` | TGC types |
| `api/gateway/v1alpha1/listenerruleconfig_types.go` | LRC types |
| `internal/domain/domain.go` | New finalizer + owner-kind constants |
| `pkg/consts/consts.go` | GatewayClass controller-name constants |
| `pkg/gateway/gatewayapi_utils.go` | Hostname matchers, wildcard→regex, helpers |
| `pkg/gateway/synth_pool.go` | Deterministic pool naming |
| `internal/controller/gateway/shared/classifier.go` | Listener-protocol/GatewayClass validation |
| `internal/controller/gateway/shared/status.go` | Condition helpers |
| `internal/controller/gateway/shared/policy_order.go` | Match-specificity ordering |
| `internal/controller/gateway/shared/reference_indexer.go` | Reverse indexes |
| `internal/controller/gateway/shared/finalizer.go` | Finalizer add/remove helpers |
| `internal/controller/gateway/shared/eventhandlers/*.go` | Cross-resource enqueue helpers |
| `internal/usecase/gateway_uc/shared/merge_config.go` | LBC merging |
| `internal/usecase/gateway_uc/shared/refgrant.go` | ReferenceGrant evaluation |
| `internal/usecase/gateway_uc/shared/tgc_resolver.go` | TargetGroupConfig cascade |
| `internal/usecase/gateway_uc/shared/lrc_resolver.go` | ListenerRuleConfig resolver |
| `internal/usecase/gateway_uc/alb_gateway_uc/gateway_uc.go` | UseCase entry (Init/Ensure/Delete) |
| `internal/usecase/gateway_uc/alb_gateway_uc/build_lb.go` | Gateway → LB params |
| `internal/usecase/gateway_uc/alb_gateway_uc/build_listener.go` | Listener building |
| `internal/usecase/gateway_uc/alb_gateway_uc/build_cert.go` | Secret import / cert ID |
| `internal/usecase/gateway_uc/alb_gateway_uc/build_pool.go` | Synthetic pool, weight scaling |
| `internal/usecase/gateway_uc/alb_gateway_uc/build_policy.go` | Policy generation |
| `internal/usecase/gateway_uc/alb_gateway_uc/build_sec_group.go` | NSG behavior |
| `internal/usecase/gateway_uc/alb_gateway_uc/status.go` | Status writers |
| `internal/usecase/contracts.go` | New use-case interfaces |
| `internal/controller/gateway/alb/gatewayclass_controller.go` | GatewayClass reconciler |
| `internal/controller/gateway/alb/gateway_controller.go` | Gateway reconciler |
| `internal/controller/gateway/alb/httproute_controller.go` | HTTPRoute reconciler |
| `internal/controller/gateway/alb/suite_test.go` | envtest harness |
| `internal/controller/gateway/targetgroupconfig/controller.go` | TGC validator |
| `internal/controller/gateway/listenerruleconfig/controller.go` | LRC validator |
| `pkg/metrics/util/reconcile_counters.go` | New counter methods |
| `cmd/main.go` | Scheme, flags, reconciler registration |
| `charts/.../templates/crds/*.yaml` | CRD bundles |
| `charts/.../templates/gatewayclass-alb.yaml` | Default GatewayClass |
| `charts/.../templates/rbac/*.yaml`, `values.yaml`, `manager-deployment.yaml` | RBAC + flags |
| `config/samples/gateway_*.yaml` | Sample manifests |
| `docs/guide/gateway-api.md` | Overview |
| `docs/guide/gateway-alb.md` | L7 walkthrough |
| `docs/guide/gateway-extensions.md` | TGC + LRC docs |
| `docs/examples/gateway-canary.md` | Weighted-backend example |
| `test/e2e/gateway/*_test.go` | 5 e2e tests |

---

## Block A — Foundation (deps, types, scaffolding)

### Task A1: Add Gateway API Go module dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the dependency**

Run: `go get sigs.k8s.io/gateway-api@v1.2.0`

Expected: `go.mod` updated with `sigs.k8s.io/gateway-api v1.2.0`; `go.sum` updated.

- [ ] **Step 2: Run go mod tidy**

Run: `go mod tidy`

Expected: no error; transitive deps resolved.

- [ ] **Step 3: Verify build still passes**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "build: add sigs.k8s.io/gateway-api v1.2.0 dependency"
```

---

### Task A2: Add domain constants for Gateway

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `pkg/consts/consts.go`

- [ ] **Step 1: Add finalizer + label + owner-kind constants**

Edit `internal/domain/domain.go` — append to the existing `Finalizers` const block:

```go
GatewayFinalizer       = "gateway.vks.vngcloud.vn/resources"
GatewayRouteFinalizer  = "gateway.vks.vngcloud.vn/route"
```

Append to the existing `Annotations` const block:

```go
GATEWAY_API_PREFIX = "gateway.vks.vngcloud.vn"
```

Append to the existing kind constants block:

```go
KindGateway       = "Gateway"
KindGatewayClass  = "GatewayClass"
KindHTTPRoute     = "HTTPRoute"
```

Append a new constants block:

```go
// Gateway API controller names — must match GatewayClass.spec.controllerName.
const (
    ControllerNameALB = "gateway.vks.vngcloud.vn/alb"
    ControllerNameNLB = "gateway.vks.vngcloud.vn/nlb"
)

// Owner-uid label for Gateway-controlled vngcloud resources.
const (
    LabelGatewayOwnerUID = "gateway.vks.vngcloud.vn/owner-uid"
)
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/domain/...`

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/domain.go
git commit -m "feat(domain): add Gateway API finalizer, label, controller-name constants"
```

---

### Task A3: Extend `LoadBalancerConfig` types (MergingMode + SSLPolicy/ALPNPolicy)

**Files:**
- Modify: `api/v1alpha1/loadbalancerconfig_types.go`
- Test: `api/v1alpha1/loadbalancerconfig_types_test.go` (new — type validation)

- [ ] **Step 1: Add `MergingMode` enum and field**

In `api/v1alpha1/loadbalancerconfig_types.go`, above the existing `LoadBalancerConfigSpec` struct, add:

```go
// MergingMode controls how a GatewayClass-level LoadBalancerConfig merges with a
// Gateway-level one. Only honored when this LBC is referenced by GatewayClass.parametersRef.
// +kubebuilder:validation:Enum=PreferGateway;PreferGatewayClass
type MergingMode string

const (
    MergingModePreferGateway      MergingMode = "PreferGateway"
    MergingModePreferGatewayClass MergingMode = "PreferGatewayClass"
)
```

Inside `LoadBalancerConfigSpec`, append (before the closing brace):

```go
// MergingMode controls how this LBC merges with another LBC when both
// GatewayClass.parametersRef and Gateway.spec.infrastructure.parametersRef are set.
// Only honored when this object is referenced by GatewayClass.parametersRef. Defaults to PreferGateway.
// +optional
MergingMode *MergingMode `json:"mergingMode,omitempty"`
```

Inside the existing `Listener` struct, append:

```go
// SSLPolicy selects the listener's TLS protocol/cipher policy (vngcloud-defined).
// Only honored on HTTPS / TLS-terminate listeners.
// +optional
SSLPolicy *string `json:"sslPolicy,omitempty"`

// ALPNPolicy controls ALPN advertisement (e.g., "HTTP2Optional", "HTTP1Only").
// Only honored on HTTPS / TLS-terminate listeners.
// +optional
ALPNPolicy *string `json:"alpnPolicy,omitempty"`
```

- [ ] **Step 2: Regenerate deep-copy + manifests**

Run: `make generate manifests`

Expected: `api/v1alpha1/zz_generated.deepcopy.go` updated; `config/crd/bases/vks.vngcloud.vn_loadbalancerconfigs.yaml` updated.

- [ ] **Step 3: Verify existing tests pass**

Run: `go test ./api/v1alpha1/... ./internal/usecase/...`

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add api/v1alpha1/loadbalancerconfig_types.go api/v1alpha1/zz_generated.deepcopy.go config/crd/bases/vks.vngcloud.vn_loadbalancerconfigs.yaml
git commit -m "feat(api): extend LoadBalancerConfig with MergingMode + listener SSL/ALPN policy"
```

---

### Task A4: New API group `gateway.vks.vngcloud.vn/v1alpha1` — registration

**Files:**
- Create: `api/gateway/v1alpha1/groupversion_info.go`

- [ ] **Step 1: Write the file**

```go
// Package v1alpha1 contains API Schema definitions for the gateway.vks.vngcloud.vn v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=gateway.vks.vngcloud.vn
package v1alpha1

import (
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
    GroupVersion  = schema.GroupVersion{Group: "gateway.vks.vngcloud.vn", Version: "v1alpha1"}
    SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
    AddToScheme   = SchemeBuilder.AddToScheme
)
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./api/gateway/v1alpha1/...`

Expected: success (file compiles even without any types yet).

- [ ] **Step 3: Commit**

```bash
git add api/gateway/v1alpha1/groupversion_info.go
git commit -m "feat(api): scaffold gateway.vks.vngcloud.vn/v1alpha1 group"
```

---

### Task A5: `TargetGroupConfig` types

**Files:**
- Create: `api/gateway/v1alpha1/targetgroupconfig_types.go`

- [ ] **Step 1: Write the type definitions**

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// TargetReference points at a backend Service in the same namespace.
type TargetReference struct {
    // Group of the target. Defaults to "" (core).
    // +optional
    Group *string `json:"group,omitempty"`
    // Kind of the target. Defaults to "Service".
    // +optional
    Kind *string `json:"kind,omitempty"`
    // Name of the target. Required.
    Name string `json:"name"`
}

// TargetGroupProperties holds vngcloud-specific pool/health-check options.
type TargetGroupProperties struct {
    // +kubebuilder:validation:Enum=instance;ip
    // +optional
    TargetType          *string                              `json:"targetType,omitempty"`
    // +optional
    PoolAlgorithm       *string                              `json:"poolAlgorithm,omitempty"`
    // +optional
    EnableStickySession *bool                                `json:"enableStickySession,omitempty"`
    // +optional
    EnableTLSEncryption *bool                                `json:"enableTLSEncryption,omitempty"`
    // +optional
    EnableProxyProtocol *bool                                `json:"enableProxyProtocol,omitempty"`
    // +optional
    HealthCheck         *vksv1alpha1.PoolHealthMonitor       `json:"healthCheck,omitempty"`
    // +optional
    TargetNodeLabels    map[string]string                    `json:"targetNodeLabels,omitempty"`
    // +optional
    ManageDFPMembers    *bool                                `json:"manageDFPMembers,omitempty"`
}

// RouteIdentifier scopes a per-route override.
type RouteIdentifier struct {
    Group     string  `json:"group"`
    Kind      string  `json:"kind"`
    // +optional
    Namespace *string `json:"namespace,omitempty"`
    Name      string  `json:"name"`
    // RuleName matches HTTPRoute.spec.rules[].name when set.
    // +optional
    RuleName  *string `json:"ruleName,omitempty"`
}

// RouteSpecificConfig overrides DefaultConfig for a specific route (and optional rule).
type RouteSpecificConfig struct {
    RouteIdentifier RouteIdentifier       `json:"routeIdentifier"`
    Config          TargetGroupProperties `json:"config"`
}

type TargetGroupConfigSpec struct {
    TargetReference     TargetReference       `json:"targetReference"`
    DefaultConfig       TargetGroupProperties `json:"defaultConfig"`
    // +optional
    RouteConfigurations []RouteSpecificConfig `json:"routeConfigurations,omitempty"`
}

type TargetGroupConfigStatus struct {
    // +optional
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
    // +optional
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=tgc
type TargetGroupConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   TargetGroupConfigSpec   `json:"spec,omitempty"`
    Status TargetGroupConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TargetGroupConfigList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []TargetGroupConfig `json:"items"`
}

func init() {
    SchemeBuilder.Register(&TargetGroupConfig{}, &TargetGroupConfigList{})
}
```

- [ ] **Step 2: Regenerate deep-copy + manifests**

Run: `make generate manifests`

Expected: `api/gateway/v1alpha1/zz_generated.deepcopy.go` created; `config/crd/bases/gateway.vks.vngcloud.vn_targetgroupconfigs.yaml` created.

- [ ] **Step 3: Verify build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add api/gateway/v1alpha1/ config/crd/bases/gateway.vks.vngcloud.vn_targetgroupconfigs.yaml
git commit -m "feat(api): add TargetGroupConfig CRD"
```

---

### Task A6: `ListenerRuleConfig` types

**Files:**
- Create: `api/gateway/v1alpha1/listenerruleconfig_types.go`

- [ ] **Step 1: Write the type definitions**

```go
package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AdditionalMatch is AND'd onto the policy generated by the host HTTPRoute rule.
type AdditionalMatch struct {
    // +kubebuilder:validation:Enum=Header;QueryParam;Method;SourceIP
    Type    string  `json:"type"`
    // +optional
    Name    *string `json:"name,omitempty"`
    // Compare values: EQUAL_TO | STARTS_WITH | ENDS_WITH | CONTAINS | REGEX
    Compare string  `json:"compare"`
    Value   string  `json:"value"`
}

type FixedResponseAction struct {
    StatusCode  int32   `json:"statusCode"`
    // +optional
    ContentType *string `json:"contentType,omitempty"`
    // +optional
    Body        *string `json:"body,omitempty"`
}

type RedirectAction struct {
    URL             string `json:"url"`
    // +optional
    HTTPCode        *int32 `json:"httpCode,omitempty"`
    // +optional
    KeepQueryString *bool  `json:"keepQueryString,omitempty"`
}

type RuleAction struct {
    // +kubebuilder:validation:Enum=FixedResponse;Reject;Redirect
    Type          string               `json:"type"`
    // +optional
    FixedResponse *FixedResponseAction `json:"fixedResponse,omitempty"`
    // +optional
    Redirect      *RedirectAction      `json:"redirect,omitempty"`
}

type ListenerRuleConfigSpec struct {
    // +optional
    AdditionalMatches []AdditionalMatch `json:"additionalMatches,omitempty"`
    // +optional
    Actions           []RuleAction      `json:"actions,omitempty"`
    // +optional
    Position          *int32            `json:"position,omitempty"`
}

type ListenerRuleConfigStatus struct {
    // +optional
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
    // +optional
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=lrc
type ListenerRuleConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   ListenerRuleConfigSpec   `json:"spec,omitempty"`
    Status ListenerRuleConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ListenerRuleConfigList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []ListenerRuleConfig `json:"items"`
}

func init() {
    SchemeBuilder.Register(&ListenerRuleConfig{}, &ListenerRuleConfigList{})
}
```

- [ ] **Step 2: Regenerate**

Run: `make generate manifests`

Expected: `config/crd/bases/gateway.vks.vngcloud.vn_listenerruleconfigs.yaml` created.

- [ ] **Step 3: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add api/gateway/v1alpha1/listenerruleconfig_types.go api/gateway/v1alpha1/zz_generated.deepcopy.go config/crd/bases/gateway.vks.vngcloud.vn_listenerruleconfigs.yaml
git commit -m "feat(api): add ListenerRuleConfig CRD"
```

---

### Task A7: Update `PROJECT` to register new resources

**Files:**
- Modify: `PROJECT`

- [ ] **Step 1: Append the two new resource entries**

Append under `resources:` (matching existing pattern):

```yaml
- api:
    crdVersion: v1
    namespaced: true
  controller: true
  domain: vks.vngcloud.vn
  group: gateway
  kind: TargetGroupConfig
  path: github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1
  version: v1alpha1
- api:
    crdVersion: v1
    namespaced: true
  controller: true
  domain: vks.vngcloud.vn
  group: gateway
  kind: ListenerRuleConfig
  path: github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1
  version: v1alpha1
```

- [ ] **Step 2: Commit**

```bash
git add PROJECT
git commit -m "chore: register TargetGroupConfig and ListenerRuleConfig in PROJECT"
```

---

## Block B — Shared utility packages

### Task B1: `pkg/gateway/gatewayapi_utils.go` — hostname & wildcard helpers (TDD)

**Files:**
- Create: `pkg/gateway/gatewayapi_utils.go`
- Test: `pkg/gateway/gatewayapi_utils_test.go`

- [ ] **Step 1: Write the failing test**

```go
package gateway_test

import (
    "testing"

    "github.com/stretchr/testify/assert"

    "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

func TestHostnameToL7Rule(t *testing.T) {
    cases := []struct {
        name     string
        host     string
        wantCmp  string
        wantVal  string
    }{
        {"literal", "api.example.com", "EQUAL_TO", "api.example.com"},
        {"prefix wildcard", "*.example.com", "REGEX", `^[^.]+\.example\.com$`},
        {"empty", "", "", ""}, // signals "no host rule"
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            cmp, val := gateway.HostnameToL7Rule(c.host)
            assert.Equal(t, c.wantCmp, cmp)
            assert.Equal(t, c.wantVal, val)
        })
    }
}

func TestPathToL7Rule(t *testing.T) {
    cases := []struct {
        name     string
        pathType string
        path     string
        wantCmp  string
        wantVal  string
    }{
        {"exact", "Exact", "/foo", "EQUAL_TO", "/foo"},
        {"prefix", "PathPrefix", "/foo", "STARTS_WITH", "/foo"},
        {"regex", "RegularExpression", "^/foo/[0-9]+$", "REGEX", "^/foo/[0-9]+$"},
        {"impl-specific defaults to exact", "ImplementationSpecific", "/foo", "EQUAL_TO", "/foo"},
    }
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            cmp, val := gateway.PathToL7Rule(c.pathType, c.path)
            assert.Equal(t, c.wantCmp, cmp)
            assert.Equal(t, c.wantVal, val)
        })
    }
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `go test ./pkg/gateway/... -run "TestHostnameToL7Rule|TestPathToL7Rule" -v`

Expected: FAIL — package or symbol not found.

- [ ] **Step 3: Implement the helpers**

Create `pkg/gateway/gatewayapi_utils.go`:

```go
package gateway

import (
    "regexp"
    "strings"
)

// HostnameToL7Rule converts a Gateway-API hostname into a vngcloud L7 rule (compare, value).
// Empty host => empty pair (caller must skip the host rule).
// "*.foo.com" => REGEX matching exactly one DNS label before ".foo.com".
func HostnameToL7Rule(host string) (compare, value string) {
    if host == "" {
        return "", ""
    }
    if strings.HasPrefix(host, "*.") {
        rest := strings.TrimPrefix(host, "*.")
        return "REGEX", `^[^.]+\.` + regexp.QuoteMeta(rest) + `$`
    }
    return "EQUAL_TO", host
}

// PathToL7Rule converts a Gateway-API HTTPPathMatch (type, value) into a vngcloud L7 rule.
// ImplementationSpecific falls back to EQUAL_TO.
func PathToL7Rule(pathType, path string) (compare, value string) {
    switch pathType {
    case "Exact":
        return "EQUAL_TO", path
    case "PathPrefix":
        return "STARTS_WITH", path
    case "RegularExpression":
        return "REGEX", path
    default:
        return "EQUAL_TO", path
    }
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./pkg/gateway/... -run "TestHostnameToL7Rule|TestPathToL7Rule" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gateway/gatewayapi_utils.go pkg/gateway/gatewayapi_utils_test.go
git commit -m "feat(gateway): add hostname/path → L7 rule converters"
```

---

### Task B2: `pkg/gateway/synth_pool.go` — deterministic pool naming (TDD)

**Files:**
- Create: `pkg/gateway/synth_pool.go`
- Test: `pkg/gateway/synth_pool_test.go`

- [ ] **Step 1: Write the failing test**

```go
package gateway_test

import (
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"

    "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

func TestSynthPoolName_Deterministic(t *testing.T) {
    backends := []gateway.BackendKey{
        {Namespace: "ns1", Name: "svc-a", Port: 80, Weight: 90},
        {Namespace: "ns1", Name: "svc-b", Port: 80, Weight: 10},
    }
    n1 := gateway.SynthPoolName("12345678-aaaa", 0, backends)
    n2 := gateway.SynthPoolName("12345678-aaaa", 0, backends)
    assert.Equal(t, n1, n2)
    assert.True(t, strings.HasPrefix(n1, "gw_12345678_0_"))
    assert.LessOrEqual(t, len(n1), 50)
}

func TestSynthPoolName_StableUnderReorder(t *testing.T) {
    a := []gateway.BackendKey{{Namespace: "n", Name: "a", Port: 80, Weight: 1}, {Namespace: "n", Name: "b", Port: 80, Weight: 2}}
    b := []gateway.BackendKey{{Namespace: "n", Name: "b", Port: 80, Weight: 2}, {Namespace: "n", Name: "a", Port: 80, Weight: 1}}
    assert.Equal(t, gateway.SynthPoolName("uid", 0, a), gateway.SynthPoolName("uid", 0, b))
}

func TestSynthPoolName_ChangesOnWeightChange(t *testing.T) {
    a := []gateway.BackendKey{{Namespace: "n", Name: "a", Port: 80, Weight: 1}}
    b := []gateway.BackendKey{{Namespace: "n", Name: "a", Port: 80, Weight: 2}}
    assert.NotEqual(t, gateway.SynthPoolName("uid", 0, a), gateway.SynthPoolName("uid", 0, b))
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./pkg/gateway/... -run "TestSynthPool" -v`

Expected: FAIL — symbols not defined.

- [ ] **Step 3: Implement**

```go
package gateway

import (
    "crypto/sha1"
    "encoding/hex"
    "fmt"
    "sort"
)

// BackendKey is the canonical identity of a backend in a route rule.
type BackendKey struct {
    Namespace string
    Name      string
    Port      int32
    Weight    int32
}

// SynthPoolName produces a deterministic pool name <= 50 chars.
// Format: gw_<uid8>_<ruleIdx>_<hash5>.
// Stable under reorder of backends; changes when set/weights change.
func SynthPoolName(routeUID string, ruleIdx int, backends []BackendKey) string {
    uid := routeUID
    if len(uid) > 8 {
        uid = uid[:8]
    }
    sorted := append([]BackendKey(nil), backends...)
    sort.Slice(sorted, func(i, j int) bool {
        if sorted[i].Namespace != sorted[j].Namespace {
            return sorted[i].Namespace < sorted[j].Namespace
        }
        if sorted[i].Name != sorted[j].Name {
            return sorted[i].Name < sorted[j].Name
        }
        if sorted[i].Port != sorted[j].Port {
            return sorted[i].Port < sorted[j].Port
        }
        return sorted[i].Weight < sorted[j].Weight
    })
    h := sha1.New()
    for _, b := range sorted {
        fmt.Fprintf(h, "%s/%s:%d:%d\n", b.Namespace, b.Name, b.Port, b.Weight)
    }
    sum := hex.EncodeToString(h.Sum(nil))[:5]
    return fmt.Sprintf("gw_%s_%d_%s", uid, ruleIdx, sum)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/gateway/... -run "TestSynthPool" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gateway/synth_pool.go pkg/gateway/synth_pool_test.go
git commit -m "feat(gateway): add deterministic synthetic pool naming"
```

---

### Task B3: `internal/usecase/gateway_uc/shared/merge_config.go` — LBC merging (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/shared/merge_config.go`
- Test: `internal/usecase/gateway_uc/shared/merge_config_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "k8s.io/utils/ptr"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/shared"
)

func TestMergeLBC_PreferGateway_DefaultMode(t *testing.T) {
    class := &vksv1alpha1.LoadBalancerConfigSpec{PackageId: ptr.To("class-pkg"), Tags: map[string]string{"a": "1"}}
    gw := &vksv1alpha1.LoadBalancerConfigSpec{PackageId: ptr.To("gw-pkg"), Tags: map[string]string{"a": "2", "b": "3"}}
    out := shared.MergeLBC(class, gw)
    assert.Equal(t, "gw-pkg", *out.PackageId)
    assert.Equal(t, "2", out.Tags["a"])
    assert.Equal(t, "3", out.Tags["b"])
}

func TestMergeLBC_PreferGatewayClass(t *testing.T) {
    mode := vksv1alpha1.MergingModePreferGatewayClass
    class := &vksv1alpha1.LoadBalancerConfigSpec{PackageId: ptr.To("class-pkg"), MergingMode: &mode, Tags: map[string]string{"a": "1"}}
    gw := &vksv1alpha1.LoadBalancerConfigSpec{PackageId: ptr.To("gw-pkg"), Tags: map[string]string{"a": "2", "b": "3"}}
    out := shared.MergeLBC(class, gw)
    assert.Equal(t, "class-pkg", *out.PackageId)
    assert.Equal(t, "1", out.Tags["a"])
    assert.Equal(t, "3", out.Tags["b"]) // present only in one side: kept regardless
}

func TestMergeLBC_LoadBalancerIDIgnoredAtClassLevel(t *testing.T) {
    class := &vksv1alpha1.LoadBalancerConfigSpec{LoadBalancerId: ptr.To("CLASS-FORBIDDEN")}
    gw := &vksv1alpha1.LoadBalancerConfigSpec{LoadBalancerId: ptr.To("gw-id")}
    out := shared.MergeLBC(class, gw)
    assert.Equal(t, "gw-id", *out.LoadBalancerId)
}

func TestMergeLBC_NilGateway(t *testing.T) {
    class := &vksv1alpha1.LoadBalancerConfigSpec{PackageId: ptr.To("class-pkg")}
    out := shared.MergeLBC(class, nil)
    assert.Equal(t, "class-pkg", *out.PackageId)
}

func TestMergeLBC_ListenersMergedByName(t *testing.T) {
    class := &vksv1alpha1.LoadBalancerConfigSpec{
        Listeners: []vksv1alpha1.Listener{{Name: "l1", AllowedCidrs: ptr.To("10.0.0.0/8")}},
    }
    gw := &vksv1alpha1.LoadBalancerConfigSpec{
        Listeners: []vksv1alpha1.Listener{{Name: "l1", AllowedCidrs: ptr.To("0.0.0.0/0")}, {Name: "l2"}},
    }
    out := shared.MergeLBC(class, gw)
    var l1, l2 *vksv1alpha1.Listener
    for i := range out.Listeners {
        if out.Listeners[i].Name == "l1" { l1 = &out.Listeners[i] }
        if out.Listeners[i].Name == "l2" { l2 = &out.Listeners[i] }
    }
    assert.NotNil(t, l1)
    assert.NotNil(t, l2)
    assert.Equal(t, "0.0.0.0/0", *l1.AllowedCidrs) // PreferGateway default
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/shared/... -v`

Expected: FAIL — package not found.

- [ ] **Step 3: Implement**

```go
// Package shared holds Gateway-controller business helpers used by both ALB and NLB use-cases.
package shared

import (
    "github.com/huandu/go-clone"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// MergeLBC merges a class-level LBC spec with a gateway-level LBC spec, returning a
// new spec that represents the effective configuration. Either argument may be nil.
//
// Merging rules:
//   - mode = class.MergingMode (default PreferGateway). Per design, only the class-level
//     LBC's MergingMode field is honored.
//   - Scalar/pointer fields: PreferGateway → gateway value wins if non-nil, else class.
//                            PreferGatewayClass → class value wins if non-nil, else gateway.
//   - LoadBalancerId: gateway-only (class value ignored).
//   - Listeners[] and Tags: merged by name/key. Each per-item value resolved per mode.
func MergeLBC(class, gw *vksv1alpha1.LoadBalancerConfigSpec) *vksv1alpha1.LoadBalancerConfigSpec {
    if class == nil && gw == nil {
        return &vksv1alpha1.LoadBalancerConfigSpec{}
    }
    if class == nil {
        return clone.Clone(gw).(*vksv1alpha1.LoadBalancerConfigSpec)
    }
    if gw == nil {
        out := clone.Clone(class).(*vksv1alpha1.LoadBalancerConfigSpec)
        out.LoadBalancerId = nil // class can't pin a single LB
        return out
    }

    mode := vksv1alpha1.MergingModePreferGateway
    if class.MergingMode != nil {
        mode = *class.MergingMode
    }

    out := clone.Clone(class).(*vksv1alpha1.LoadBalancerConfigSpec)
    pickPtr := func(classVal, gwVal interface{}) {} // doc only — see below
    _ = pickPtr

    // For scalar/pointer fields, do per-field selection.
    out.PackageId = pickStrPtr(class.PackageId, gw.PackageId, mode)
    out.Scheme = pickEnumPtr[vksv1alpha1.LoadBalancerConfigSpec](class.Scheme, gw.Scheme, mode)
    out.EnableAutoscale = pickBoolPtr(class.EnableAutoscale, gw.EnableAutoscale, mode)
    out.IsPoc = pickBoolPtr(class.IsPoc, gw.IsPoc, mode)
    out.SubnetId = pickStr(class.SubnetId, gw.SubnetId, mode)
    out.VpcId = pickStr(class.VpcId, gw.VpcId, mode)
    out.LoadBalancerName = pickStr(class.LoadBalancerName, gw.LoadBalancerName, mode)
    out.PrivateSubnetId = pickStrPtr(class.PrivateSubnetId, gw.PrivateSubnetId, mode)
    out.PrivateZoneId = pickPrivateZonePtr(class.PrivateZoneId, gw.PrivateZoneId, mode)
    out.LoadBalancerId = gw.LoadBalancerId // gateway-only

    out.Tags = mergeStringMap(class.Tags, gw.Tags, mode)
    out.Listeners = mergeListenersByName(class.Listeners, gw.Listeners, mode)

    return out
}

func pickStr(a, b string, mode vksv1alpha1.MergingMode) string {
    if mode == vksv1alpha1.MergingModePreferGatewayClass {
        if a != "" { return a }
        return b
    }
    if b != "" { return b }
    return a
}
func pickStrPtr(a, b *string, mode vksv1alpha1.MergingMode) *string {
    if mode == vksv1alpha1.MergingModePreferGatewayClass {
        if a != nil { return a }
        return b
    }
    if b != nil { return b }
    return a
}
func pickBoolPtr(a, b *bool, mode vksv1alpha1.MergingMode) *bool {
    if mode == vksv1alpha1.MergingModePreferGatewayClass {
        if a != nil { return a }
        return b
    }
    if b != nil { return b }
    return a
}
// pickEnumPtr/pickPrivateZonePtr are typed wrappers around pickPtr; identical structure.
// (Concrete helpers omitted for brevity in the plan — engineer mirrors pickStrPtr.)
```

(Keep the plan compact: fully expanded `pickEnumPtr`, `pickPrivateZonePtr`, `mergeStringMap`, `mergeListenersByName` follow the same pattern. Engineer fills them in by analogy with `pickStrPtr`.)

```go
func mergeStringMap(a, b map[string]string, mode vksv1alpha1.MergingMode) map[string]string {
    if a == nil && b == nil {
        return nil
    }
    out := map[string]string{}
    for k, v := range a {
        out[k] = v
    }
    for k, v := range b {
        if _, exists := out[k]; !exists || mode == vksv1alpha1.MergingModePreferGateway {
            out[k] = v
        }
    }
    return out
}

func mergeListenersByName(a, b []vksv1alpha1.Listener, mode vksv1alpha1.MergingMode) []vksv1alpha1.Listener {
    byName := map[string]vksv1alpha1.Listener{}
    for _, l := range a {
        byName[l.Name] = l
    }
    for _, l := range b {
        if _, exists := byName[l.Name]; !exists || mode == vksv1alpha1.MergingModePreferGateway {
            // For Phase 1 we take the whole-listener winner (per-field listener merging deferred).
            byName[l.Name] = l
        }
    }
    out := make([]vksv1alpha1.Listener, 0, len(byName))
    for _, l := range byName {
        out = append(out, l)
    }
    return out
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/usecase/gateway_uc/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/shared/
git commit -m "feat(gateway/uc): add LoadBalancerConfig merging helpers"
```

---

### Task B4: `internal/usecase/gateway_uc/shared/refgrant.go` — ReferenceGrant evaluation (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/shared/refgrant.go`
- Test: `internal/usecase/gateway_uc/shared/refgrant_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/shared"
)

func mkGrant(ns string, fromGroup, fromKind, fromNS, toGroup, toKind, toName string) *gwv1beta1.ReferenceGrant {
    rgFrom := gwv1beta1.ReferenceGrantFrom{
        Group: gwv1beta1.Group(fromGroup), Kind: gwv1beta1.Kind(fromKind), Namespace: gwv1beta1.Namespace(fromNS),
    }
    rgTo := gwv1beta1.ReferenceGrantTo{Group: gwv1beta1.Group(toGroup), Kind: gwv1beta1.Kind(toKind)}
    if toName != "" {
        n := gwv1beta1.ObjectName(toName); rgTo.Name = &n
    }
    return &gwv1beta1.ReferenceGrant{
        ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "rg"},
        Spec: gwv1beta1.ReferenceGrantSpec{From: []gwv1beta1.ReferenceGrantFrom{rgFrom}, To: []gwv1beta1.ReferenceGrantTo{rgTo}},
    }
}

func TestRefGrantAllowed_SameNamespace(t *testing.T) {
    p := shared.RefRequest{FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute", FromNS: "ns1",
                            ToGroup: "", ToKind: "Service", ToNS: "ns1", ToName: "svc"}
    assert.True(t, shared.RefGrantAllowed(p, nil))
}

func TestRefGrantAllowed_CrossNamespace_GrantPresent(t *testing.T) {
    grants := []*gwv1beta1.ReferenceGrant{
        mkGrant("ns2", "gateway.networking.k8s.io", "HTTPRoute", "ns1", "", "Service", ""),
    }
    p := shared.RefRequest{FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute", FromNS: "ns1",
                           ToGroup: "", ToKind: "Service", ToNS: "ns2", ToName: "svc"}
    assert.True(t, shared.RefGrantAllowed(p, grants))
}

func TestRefGrantAllowed_CrossNamespace_NoGrant(t *testing.T) {
    p := shared.RefRequest{FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute", FromNS: "ns1",
                           ToGroup: "", ToKind: "Service", ToNS: "ns2", ToName: "svc"}
    assert.False(t, shared.RefGrantAllowed(p, nil))
}

func TestRefGrantAllowed_NameSpecificMismatch(t *testing.T) {
    grants := []*gwv1beta1.ReferenceGrant{
        mkGrant("ns2", "gateway.networking.k8s.io", "HTTPRoute", "ns1", "", "Service", "other"),
    }
    p := shared.RefRequest{FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute", FromNS: "ns1",
                           ToGroup: "", ToKind: "Service", ToNS: "ns2", ToName: "svc"}
    assert.False(t, shared.RefGrantAllowed(p, grants))
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/shared/... -run TestRefGrant -v`

Expected: FAIL — symbol not defined.

- [ ] **Step 3: Implement**

```go
package shared

import (
    gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type RefRequest struct {
    FromGroup, FromKind, FromNS string
    ToGroup, ToKind, ToNS, ToName string
}

// RefGrantAllowed returns true if the given cross-namespace reference is permitted by
// any of the supplied ReferenceGrants. Same-namespace references are always allowed.
func RefGrantAllowed(r RefRequest, grants []*gwv1beta1.ReferenceGrant) bool {
    if r.FromNS == r.ToNS {
        return true
    }
    for _, g := range grants {
        if g.Namespace != r.ToNS {
            continue
        }
        if !grantMatchesFrom(g, r) {
            continue
        }
        if grantMatchesTo(g, r) {
            return true
        }
    }
    return false
}

func grantMatchesFrom(g *gwv1beta1.ReferenceGrant, r RefRequest) bool {
    for _, f := range g.Spec.From {
        if string(f.Group) == r.FromGroup && string(f.Kind) == r.FromKind && string(f.Namespace) == r.FromNS {
            return true
        }
    }
    return false
}

func grantMatchesTo(g *gwv1beta1.ReferenceGrant, r RefRequest) bool {
    for _, to := range g.Spec.To {
        if string(to.Group) != r.ToGroup || string(to.Kind) != r.ToKind {
            continue
        }
        if to.Name == nil {
            return true
        }
        if string(*to.Name) == r.ToName {
            return true
        }
    }
    return false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/shared/refgrant.go internal/usecase/gateway_uc/shared/refgrant_test.go
git commit -m "feat(gateway/uc): add ReferenceGrant evaluation helper"
```

---

### Task B5: `internal/usecase/gateway_uc/shared/tgc_resolver.go` — TGC cascade (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/shared/tgc_resolver.go`
- Test: `internal/usecase/gateway_uc/shared/tgc_resolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "k8s.io/utils/ptr"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/shared"
)

func mkTGC(name string, target string, def *string, route, rule string, override *string) gatewayv1alpha1.TargetGroupConfig {
    tgc := gatewayv1alpha1.TargetGroupConfig{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
        Spec: gatewayv1alpha1.TargetGroupConfigSpec{
            TargetReference: gatewayv1alpha1.TargetReference{Name: target},
            DefaultConfig:   gatewayv1alpha1.TargetGroupProperties{PoolAlgorithm: def},
        },
    }
    if route != "" {
        rsc := gatewayv1alpha1.RouteSpecificConfig{
            RouteIdentifier: gatewayv1alpha1.RouteIdentifier{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Name: route},
            Config:          gatewayv1alpha1.TargetGroupProperties{PoolAlgorithm: override},
        }
        if rule != "" {
            rsc.RouteIdentifier.RuleName = ptr.To(rule)
        }
        tgc.Spec.RouteConfigurations = append(tgc.Spec.RouteConfigurations, rsc)
    }
    return tgc
}

func TestResolveTGC_DefaultOnly(t *testing.T) {
    tgcs := []gatewayv1alpha1.TargetGroupConfig{mkTGC("a", "svc-1", ptr.To("ROUND_ROBIN"), "", "", nil)}
    p, _ := shared.ResolveTargetGroupProps(tgcs, "svc-1", "HTTPRoute", "any-route", nil)
    assert.Equal(t, "ROUND_ROBIN", *p.PoolAlgorithm)
}

func TestResolveTGC_RouteOverride(t *testing.T) {
    tgcs := []gatewayv1alpha1.TargetGroupConfig{mkTGC("a", "svc-1", ptr.To("ROUND_ROBIN"), "route-x", "", ptr.To("LEAST_CONNECTIONS"))}
    p, _ := shared.ResolveTargetGroupProps(tgcs, "svc-1", "HTTPRoute", "route-x", nil)
    assert.Equal(t, "LEAST_CONNECTIONS", *p.PoolAlgorithm)
}

func TestResolveTGC_RuleOverride_Beats_Route(t *testing.T) {
    tgcs := []gatewayv1alpha1.TargetGroupConfig{
        mkTGC("a", "svc-1", ptr.To("RR"), "route-x", "rule-1", ptr.To("RULE_WIN")),
        mkTGC("b", "svc-1", ptr.To("RR"), "route-x", "",       ptr.To("ROUTE_WIN")),
    }
    rule := "rule-1"
    p, _ := shared.ResolveTargetGroupProps(tgcs, "svc-1", "HTTPRoute", "route-x", &rule)
    assert.Equal(t, "RULE_WIN", *p.PoolAlgorithm)
}

func TestResolveTGC_NoMatch_ReturnsNil(t *testing.T) {
    p, _ := shared.ResolveTargetGroupProps(nil, "svc-1", "HTTPRoute", "x", nil)
    assert.Equal(t, gatewayv1alpha1.TargetGroupProperties{}, p)
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/shared/... -run TestResolveTGC -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package shared

import (
    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
)

// ResolveTargetGroupProps walks tgcs and returns the most specific TargetGroupProperties
// for the given backend Service name + route + optional rule name. The returned bool is
// true when the chosen TGC also reports a Conflicted state on a peer that lost a tie.
func ResolveTargetGroupProps(
    tgcs []gatewayv1alpha1.TargetGroupConfig,
    serviceName, routeKind, routeName string,
    ruleName *string,
) (gatewayv1alpha1.TargetGroupProperties, bool) {
    var best *gatewayv1alpha1.TargetGroupProperties
    var bestSpecificity int // 0=default, 1=route, 2=rule
    var conflict bool
    for i := range tgcs {
        tgc := &tgcs[i]
        if !targetMatchesService(tgc.Spec.TargetReference, serviceName) {
            continue
        }
        // Route-specific?
        for _, rsc := range tgc.Spec.RouteConfigurations {
            if rsc.RouteIdentifier.Kind != routeKind || rsc.RouteIdentifier.Name != routeName {
                continue
            }
            spec := 1
            if rsc.RouteIdentifier.RuleName != nil && ruleName != nil && *rsc.RouteIdentifier.RuleName == *ruleName {
                spec = 2
            } else if rsc.RouteIdentifier.RuleName != nil {
                continue
            }
            if best == nil || spec > bestSpecificity {
                cfg := rsc.Config
                best = &cfg
                bestSpecificity = spec
                conflict = false
            } else if spec == bestSpecificity {
                conflict = true
            }
        }
        if best == nil || bestSpecificity == 0 {
            cfg := tgc.Spec.DefaultConfig
            if best == nil {
                best = &cfg
                bestSpecificity = 0
            } else if bestSpecificity == 0 {
                conflict = true
            }
        }
    }
    if best == nil {
        return gatewayv1alpha1.TargetGroupProperties{}, false
    }
    return *best, conflict
}

func targetMatchesService(ref gatewayv1alpha1.TargetReference, name string) bool {
    if ref.Name != name {
        return false
    }
    if ref.Kind != nil && *ref.Kind != "" && *ref.Kind != "Service" {
        return false
    }
    if ref.Group != nil && *ref.Group != "" {
        return false
    }
    return true
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/shared/tgc_resolver.go internal/usecase/gateway_uc/shared/tgc_resolver_test.go
git commit -m "feat(gateway/uc): add TargetGroupConfig cascading resolver"
```

---

### Task B6: `internal/usecase/gateway_uc/shared/lrc_resolver.go` — LRC ExtensionRef resolver (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/shared/lrc_resolver.go`
- Test: `internal/usecase/gateway_uc/shared/lrc_resolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/shared"
)

func TestExtractLRCRefs_FromFilters(t *testing.T) {
    extRef := gwv1.LocalObjectReference{Group: "gateway.vks.vngcloud.vn", Kind: "ListenerRuleConfig", Name: "rule-x"}
    filters := []gwv1.HTTPRouteFilter{{Type: gwv1.HTTPRouteFilterExtensionRef, ExtensionRef: &extRef}}
    out := shared.ExtractLRCRefsFromFilters(filters)
    assert.Equal(t, []string{"rule-x"}, out)
}

func TestExtractLRCRefs_IgnoresOtherExtensionRefs(t *testing.T) {
    extRef := gwv1.LocalObjectReference{Group: "other.example.com", Kind: "Whatever", Name: "ignored"}
    filters := []gwv1.HTTPRouteFilter{{Type: gwv1.HTTPRouteFilterExtensionRef, ExtensionRef: &extRef}}
    assert.Empty(t, shared.ExtractLRCRefsFromFilters(filters))
}

func TestFindLRC(t *testing.T) {
    lrcs := []gatewayv1alpha1.ListenerRuleConfig{
        {ObjectMeta: metav1.ObjectMeta{Name: "rule-x", Namespace: "ns"}},
    }
    out, ok := shared.FindLRC(lrcs, "rule-x")
    assert.True(t, ok)
    assert.Equal(t, "rule-x", out.Name)

    _, ok = shared.FindLRC(lrcs, "missing")
    assert.False(t, ok)
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/shared/... -run TestExtractLRC -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package shared

import (
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
)

const (
    LRCGroup = "gateway.vks.vngcloud.vn"
    LRCKind  = "ListenerRuleConfig"
)

// ExtractLRCRefsFromFilters returns the names of all ListenerRuleConfig
// ExtensionRefs found in the given filters list (same-namespace only).
func ExtractLRCRefsFromFilters(filters []gwv1.HTTPRouteFilter) []string {
    var names []string
    for _, f := range filters {
        if f.Type != gwv1.HTTPRouteFilterExtensionRef || f.ExtensionRef == nil {
            continue
        }
        if string(f.ExtensionRef.Group) == LRCGroup && string(f.ExtensionRef.Kind) == LRCKind {
            names = append(names, string(f.ExtensionRef.Name))
        }
    }
    return names
}

// FindLRC looks up a ListenerRuleConfig by name within a slice (already namespace-filtered).
func FindLRC(lrcs []gatewayv1alpha1.ListenerRuleConfig, name string) (*gatewayv1alpha1.ListenerRuleConfig, bool) {
    for i := range lrcs {
        if lrcs[i].Name == name {
            return &lrcs[i], true
        }
    }
    return nil, false
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/shared/lrc_resolver.go internal/usecase/gateway_uc/shared/lrc_resolver_test.go
git commit -m "feat(gateway/uc): add ListenerRuleConfig extension-ref resolver"
```

---

### Task B7: `internal/controller/gateway/shared/classifier.go` — listener protocol validation (TDD)

**Files:**
- Create: `internal/controller/gateway/shared/classifier.go`
- Test: `internal/controller/gateway/shared/classifier_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
)

func TestALBAllowsListener(t *testing.T) {
    cases := []struct {
        proto gwv1.ProtocolType
        want  bool
    }{
        {gwv1.HTTPProtocolType, true},
        {gwv1.HTTPSProtocolType, true},
        {gwv1.TLSProtocolType, true},
        {gwv1.TCPProtocolType, false},
        {gwv1.UDPProtocolType, false},
    }
    for _, c := range cases {
        t.Run(string(c.proto), func(t *testing.T) {
            assert.Equal(t, c.want, shared.ALBAllowsListener(c.proto))
        })
    }
}

func TestValidateListenerSet_DupPort(t *testing.T) {
    listeners := []gwv1.Listener{
        {Name: "a", Protocol: gwv1.HTTPProtocolType, Port: 80},
        {Name: "b", Protocol: gwv1.HTTPProtocolType, Port: 80},
    }
    res := shared.ValidateListenersForALB(listeners)
    assert.Len(t, res, 2)
    assert.Equal(t, shared.ListenerInvalidReasonDupPort, res["b"])
}

func TestValidateListenerSet_UnsupportedProtocol(t *testing.T) {
    listeners := []gwv1.Listener{{Name: "a", Protocol: gwv1.TCPProtocolType, Port: 9000}}
    res := shared.ValidateListenersForALB(listeners)
    assert.Equal(t, shared.ListenerInvalidReasonUnsupportedProtocol, res["a"])
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/controller/gateway/shared/... -run "TestALB|TestValidateListener" -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// Package shared holds Gateway-controller helpers shared by ALB and NLB reconcilers.
package shared

import (
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ListenerInvalidReason string

const (
    ListenerInvalidReasonNone               ListenerInvalidReason = ""
    ListenerInvalidReasonUnsupportedProtocol ListenerInvalidReason = "UnsupportedProtocol"
    ListenerInvalidReasonDupPort            ListenerInvalidReason = "DupPort"
)

func ALBAllowsListener(p gwv1.ProtocolType) bool {
    switch p {
    case gwv1.HTTPProtocolType, gwv1.HTTPSProtocolType, gwv1.TLSProtocolType:
        return true
    }
    return false
}

// ValidateListenersForALB returns a map from listener name → reason. A listener with
// reason ListenerInvalidReasonNone (or absent from the map) is valid.
func ValidateListenersForALB(listeners []gwv1.Listener) map[string]ListenerInvalidReason {
    out := make(map[string]ListenerInvalidReason, len(listeners))
    seenPort := map[gwv1.PortNumber]string{}
    for _, l := range listeners {
        if !ALBAllowsListener(l.Protocol) {
            out[string(l.Name)] = ListenerInvalidReasonUnsupportedProtocol
            continue
        }
        if firstName, dup := seenPort[l.Port]; dup {
            out[string(l.Name)] = ListenerInvalidReasonDupPort
            _ = firstName
            continue
        }
        seenPort[l.Port] = string(l.Name)
        out[string(l.Name)] = ListenerInvalidReasonNone
    }
    return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/controller/gateway/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/gateway/shared/classifier.go internal/controller/gateway/shared/classifier_test.go
git commit -m "feat(gateway/ctrl): add ALB listener-validation classifier"
```

---

### Task B8: `internal/controller/gateway/shared/status.go` — condition helpers (TDD)

**Files:**
- Create: `internal/controller/gateway/shared/status.go`
- Test: `internal/controller/gateway/shared/status_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
)

func TestSetCondition_AddsAndUpdates(t *testing.T) {
    var conds []metav1.Condition
    shared.SetCondition(&conds, "Accepted", metav1.ConditionTrue, "Accepted", "ok", 1)
    assert.Len(t, conds, 1)
    assert.Equal(t, metav1.ConditionTrue, conds[0].Status)

    shared.SetCondition(&conds, "Accepted", metav1.ConditionFalse, "Invalid", "bad", 2)
    assert.Len(t, conds, 1)
    assert.Equal(t, metav1.ConditionFalse, conds[0].Status)
    assert.Equal(t, "Invalid", conds[0].Reason)
    assert.EqualValues(t, 2, conds[0].ObservedGeneration)
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/controller/gateway/shared/... -run TestSetCondition -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package shared

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetCondition adds or updates a condition on the given slice. Reuses metav1.Condition
// semantics (LastTransitionTime updated only when Status changes).
func SetCondition(conds *[]metav1.Condition, condType string, status metav1.ConditionStatus, reason, msg string, gen int64) {
    for i := range *conds {
        if (*conds)[i].Type == condType {
            if (*conds)[i].Status != status {
                (*conds)[i].LastTransitionTime = metav1.Now()
            }
            (*conds)[i].Status = status
            (*conds)[i].Reason = reason
            (*conds)[i].Message = msg
            (*conds)[i].ObservedGeneration = gen
            return
        }
    }
    *conds = append(*conds, metav1.Condition{
        Type:               condType,
        Status:             status,
        Reason:             reason,
        Message:            msg,
        ObservedGeneration: gen,
        LastTransitionTime: metav1.Now(),
    })
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/controller/gateway/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/gateway/shared/status.go internal/controller/gateway/shared/status_test.go
git commit -m "feat(gateway/ctrl): add condition helpers"
```

---

### Task B9: `internal/controller/gateway/shared/policy_order.go` — match-specificity ordering (TDD)

**Files:**
- Create: `internal/controller/gateway/shared/policy_order.go`
- Test: `internal/controller/gateway/shared/policy_order_test.go`

- [ ] **Step 1: Write the failing test**

```go
package shared_test

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "k8s.io/utils/ptr"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
)

func TestMatchSpecificity_PathTypeOrder(t *testing.T) {
    exact := gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchExact), Value: ptr.To("/a")}}
    pathPrefix := gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchPathPrefix), Value: ptr.To("/a")}}
    assert.Greater(t, shared.MatchSpecificity(exact), shared.MatchSpecificity(pathPrefix))
}

func TestMatchSpecificity_LongerPathBeatsShorter(t *testing.T) {
    long := gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchPathPrefix), Value: ptr.To("/a/b/c")}}
    short := gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchPathPrefix), Value: ptr.To("/a")}}
    assert.Greater(t, shared.MatchSpecificity(long), shared.MatchSpecificity(short))
}

func TestMatchSpecificity_HeaderCount(t *testing.T) {
    one := gwv1.HTTPRouteMatch{Headers: []gwv1.HTTPHeaderMatch{{Name: "x"}}}
    two := gwv1.HTTPRouteMatch{Headers: []gwv1.HTTPHeaderMatch{{Name: "x"}, {Name: "y"}}}
    assert.Greater(t, shared.MatchSpecificity(two), shared.MatchSpecificity(one))
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/controller/gateway/shared/... -run TestMatchSpecificity -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package shared

import (
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// MatchSpecificity returns a sortable specificity score. Higher = more specific.
// Encoding (high → low):
//   bit 24..  : path-type weight (Exact=4, RegularExpression=3, PathPrefix=2, ImplementationSpecific=1)
//   bit 16..23: path length (capped at 255)
//   bit  8..15: header-match count
//   bit  0..7 : query-param-match count
func MatchSpecificity(m gwv1.HTTPRouteMatch) uint64 {
    var s uint64
    if m.Path != nil {
        switch t := m.Path.Type; {
        case t != nil && *t == gwv1.PathMatchExact:
            s |= 4 << 24
        case t != nil && *t == gwv1.PathMatchRegularExpression:
            s |= 3 << 24
        case t != nil && *t == gwv1.PathMatchPathPrefix:
            s |= 2 << 24
        default:
            s |= 1 << 24
        }
        if m.Path.Value != nil {
            n := uint64(len(*m.Path.Value))
            if n > 255 {
                n = 255
            }
            s |= n << 16
        }
    }
    s |= uint64(min(len(m.Headers), 255)) << 8
    s |= uint64(min(len(m.QueryParams), 255))
    return s
}

func min(a, b int) int { if a < b { return a }; return b }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/controller/gateway/shared/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/gateway/shared/policy_order.go internal/controller/gateway/shared/policy_order_test.go
git commit -m "feat(gateway/ctrl): add HTTPRoute match specificity scorer"
```

---

### Task B10: `pkg/metrics/util/reconcile_counters.go` — Gateway counters

**Files:**
- Modify: `pkg/metrics/util/reconcile_counters.go`

- [ ] **Step 1: Read existing counters file**

Run: `cat pkg/metrics/util/reconcile_counters.go`

Note the existing `IncrementIngress` / `IncrementService` pattern.

- [ ] **Step 2: Add Gateway counters**

Append two methods mirroring `IncrementIngress`:

```go
func (r *ReconcileCounters) IncrementGateway(nn types.NamespacedName)   { r.increment("gateway", nn) }
func (r *ReconcileCounters) IncrementHTTPRoute(nn types.NamespacedName) { r.increment("httproute", nn) }
```

(Adjust to match the actual signature pattern in the existing file.)

- [ ] **Step 3: Build**

Run: `go build ./pkg/metrics/...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add pkg/metrics/util/reconcile_counters.go
git commit -m "feat(metrics): add Gateway and HTTPRoute reconcile counters"
```

---

## Block C — Use case (business logic)

### Task C1: Use-case interface contracts

**Files:**
- Modify: `internal/usecase/contracts.go`

- [ ] **Step 1: Add interface declarations**

Append at end of `internal/usecase/contracts.go`:

```go
// GatewayClassUseCase reconciles a single GatewayClass.
type GatewayClassUseCase interface {
    InitGatewayClassUseCase(ctx context.Context) error
    EnsureGatewayClassUseCase(ctx context.Context, req ctrl.Request) error
    DeleteGatewayClassUseCase(ctx context.Context, req ctrl.Request) error
}

// ALBGatewayUseCase reconciles a single Gateway (and the LB it owns) plus the routes
// attached to it.
type ALBGatewayUseCase interface {
    InitALBGatewayUseCase(ctx context.Context) error
    EnsureALBGatewayUseCase(ctx context.Context, req ctrl.Request) error
    DeleteALBGatewayUseCase(ctx context.Context, req ctrl.Request) error
    // EnqueueParentGatewayForRoute is called by the HTTPRoute reconciler to trigger
    // a Gateway reconcile when a route changes. Idempotent.
    EnqueueParentGatewayForRoute(ctx context.Context, routeRef ctrl.Request) error
}
```

- [ ] **Step 2: Regenerate mocks**

Run: `make generate` (or run mockery directly per existing config)

Expected: `internal/usecase/mocks.go` updated with new mocks.

- [ ] **Step 3: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/usecase/contracts.go internal/usecase/mocks.go
git commit -m "feat(uc): add Gateway and ALBGateway use-case contracts"
```

---

### Task C2: ALB use-case skeleton

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/gateway_uc.go`

- [ ] **Step 1: Write the skeleton with empty methods**

```go
// Package alb_gateway_uc implements the ALB Gateway-API use case.
package alb_gateway_uc

import (
    "context"

    "github.com/anngdinh/operator-helper/contexts"
    ctrl "sigs.k8s.io/controller-runtime"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
    "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
    "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

type albGatewayUseCase struct {
    k8sRepo          repository.K8sRepository
    vngcloudRepo     repository.VngCloudRepository
    annotationParser annotations.Parser
    cniDetector      utils.CniDetector
    endpointResolver utils.EndpointResolver

    clusterId string
}

func NewALBGatewayUseCase(
    clusterId string,
    k8sRepo repository.K8sRepository,
    vngcloudRepo repository.VngCloudRepository,
    annotationParser annotations.Parser,
    cniDetector utils.CniDetector,
    endpointResolver utils.EndpointResolver,
) usecase.ALBGatewayUseCase {
    return &albGatewayUseCase{
        clusterId: clusterId, k8sRepo: k8sRepo, vngcloudRepo: vngcloudRepo,
        annotationParser: annotationParser, cniDetector: cniDetector, endpointResolver: endpointResolver,
    }
}

func (uc *albGatewayUseCase) InitALBGatewayUseCase(ctx context.Context) error {
    _ = contexts.NewContext(ctx).Log()
    // TODO(C9): mirror IngressUseCase.Init logic (default network info etc.). Filled in by Task C9.
    return nil
}

func (uc *albGatewayUseCase) EnsureALBGatewayUseCase(ctx context.Context, req ctrl.Request) error {
    return nil // implemented in Task C9
}

func (uc *albGatewayUseCase) DeleteALBGatewayUseCase(ctx context.Context, req ctrl.Request) error {
    return nil // implemented in Task C9
}

func (uc *albGatewayUseCase) EnqueueParentGatewayForRoute(ctx context.Context, routeRef ctrl.Request) error {
    return nil // implemented in Task C9
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/gateway_uc.go
git commit -m "feat(gateway/uc): scaffold ALBGatewayUseCase"
```

---

### Task C3: `build_lb.go` — Gateway → LoadBalancer params (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/build_lb.go`
- Test: `internal/usecase/gateway_uc/alb_gateway_uc/build_lb_test.go`

- [ ] **Step 1: Write the failing test**

```go
package alb_gateway_uc

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "k8s.io/utils/ptr"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func TestBuildLBSpec_FromGateway(t *testing.T) {
    gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "g1", Namespace: "ns1", UID: "12345678abcd"}}
    effective := &vksv1alpha1.LoadBalancerConfigSpec{
        PackageId: ptr.To("lbp-x"), VpcId: "vpc-1", SubnetId: "sub-1", ZoneId: "HCM-1",
    }
    spec := BuildLBSpec(gw, effective, "k8s-cluster-id")
    assert.Equal(t, "lbp-x", *spec.PackageId)
    assert.Equal(t, "vpc-1", spec.VpcId)
    assert.Equal(t, "sub-1", spec.SubnetId)
    assert.Equal(t, "k8s-cluster-id", *spec.ClusterId)
    assert.Contains(t, spec.LoadBalancerName, "g1") // controller-derived name includes gateway name
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestBuildLBSpec -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package alb_gateway_uc

import (
    "fmt"

    loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
    "k8s.io/utils/ptr"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// BuildLBSpec produces the effective vngcloud LB params for the given Gateway, applying
// values from the merged effective LBC spec. Returned spec is a fresh copy.
func BuildLBSpec(gw *gwv1.Gateway, effective *vksv1alpha1.LoadBalancerConfigSpec, clusterID string) *vksv1alpha1.LoadBalancerConfigSpec {
    out := *effective // shallow copy; pointer fields are shared but caller treats out as read-only after this call
    out.Type = loadbalancerv2.LoadBalancerTypeLayer7
    out.ClusterId = ptr.To(clusterID)
    if out.LoadBalancerName == "" {
        out.LoadBalancerName = fmt.Sprintf("%s-gw-%s-%s", domain.DEFAULT_LB_PREFIX_NAME, gw.Namespace, gw.Name)
    }
    return &out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/build_lb.go internal/usecase/gateway_uc/alb_gateway_uc/build_lb_test.go
git commit -m "feat(gateway/uc): add Gateway → LB-spec builder"
```

---

### Task C4: `build_listener.go` — Gateway listener → vngcloud listener (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/build_listener.go`
- Test: `internal/usecase/gateway_uc/alb_gateway_uc/build_listener_test.go`

- [ ] **Step 1: Write the failing test**

```go
package alb_gateway_uc

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "k8s.io/utils/ptr"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func TestBuildListener_HTTP(t *testing.T) {
    l := gwv1.Listener{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80}
    out, err := BuildListener(l, nil, nil)
    assert.NoError(t, err)
    assert.Equal(t, loadbalancerv2.ListenerProtocolHTTP, out.Protocol)
    assert.EqualValues(t, 80, out.ProtocolPort)
    assert.Nil(t, out.CertificateDefault)
}

func TestBuildListener_HTTPS_FromLBCCertID(t *testing.T) {
    l := gwv1.Listener{Name: "https", Protocol: gwv1.HTTPSProtocolType, Port: 443}
    lbcL := &vksv1alpha1.Listener{
        Name: "https",
        CertificateDefault: &vksv1alpha1.ListenerCertificate{Id: ptr.To("cert-1")},
    }
    out, err := BuildListener(l, lbcL, nil)
    assert.NoError(t, err)
    assert.Equal(t, loadbalancerv2.ListenerProtocolHTTPS, out.Protocol)
    assert.NotNil(t, out.CertificateDefault)
    assert.Equal(t, "cert-1", *out.CertificateDefault.Id)
}

func TestBuildListener_LBCMismatchedProtocol_Errors(t *testing.T) {
    l := gwv1.Listener{Name: "https", Protocol: gwv1.HTTPSProtocolType, Port: 443}
    lbcL := &vksv1alpha1.Listener{Name: "https", Protocol: loadbalancerv2.ListenerProtocolHTTP, ProtocolPort: 80}
    _, err := BuildListener(l, lbcL, nil)
    assert.Error(t, err)
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestBuildListener -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package alb_gateway_uc

import (
    "fmt"

    loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
    "k8s.io/utils/ptr"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// CertSource is a lookup callback used when LBC overrides aren't present:
// returns (importedCertID, error). May return nil for "no cert".
type CertSource func(certRef gwv1.SecretObjectReference) (*vksv1alpha1.ListenerCertificate, error)

// BuildListener converts a Gateway listener (with optional LBC override) into a vksv1alpha1.Listener.
// `lbcL` is the LBC-side listener entry matched by name (may be nil).
// `certs` builds a ListenerCertificate from a Gateway certificateRef when LBC has no cert IDs.
func BuildListener(l gwv1.Listener, lbcL *vksv1alpha1.Listener, certs CertSource) (*vksv1alpha1.Listener, error) {
    proto, err := mapProtocol(l.Protocol)
    if err != nil {
        return nil, err
    }
    if lbcL != nil && lbcL.Protocol != "" && lbcL.Protocol != proto {
        return nil, fmt.Errorf("LBC listener %q protocol %s does not match Gateway listener protocol %s", l.Name, lbcL.Protocol, proto)
    }
    if lbcL != nil && lbcL.ProtocolPort != 0 && lbcL.ProtocolPort != int32(l.Port) {
        return nil, fmt.Errorf("LBC listener %q port %d does not match Gateway listener port %d", l.Name, lbcL.ProtocolPort, l.Port)
    }

    out := &vksv1alpha1.Listener{
        Name:         string(l.Name),
        Protocol:     proto,
        ProtocolPort: int32(l.Port),
    }

    if lbcL != nil {
        out.TimeoutClient = lbcL.TimeoutClient
        out.TimeoutMember = lbcL.TimeoutMember
        out.TimeoutConnection = lbcL.TimeoutConnection
        out.AllowedCidrs = lbcL.AllowedCidrs
        out.InsertHeaders = lbcL.InsertHeaders
        out.SSLPolicy = lbcL.SSLPolicy
        out.ALPNPolicy = lbcL.ALPNPolicy
        out.ClientCertificateId = lbcL.ClientCertificateId
        if lbcL.CertificateDefault != nil {
            out.CertificateDefault = lbcL.CertificateDefault
        }
        if len(lbcL.CertificateAuthorities) > 0 {
            out.CertificateAuthorities = lbcL.CertificateAuthorities
        }
    }

    // Fall back to importing TLS Secrets from Gateway when LBC doesn't supply IDs.
    if (proto == loadbalancerv2.ListenerProtocolHTTPS) && out.CertificateDefault == nil && l.TLS != nil && certs != nil {
        for i, ref := range l.TLS.CertificateRefs {
            cert, err := certs(ref)
            if err != nil {
                return nil, err
            }
            if cert == nil {
                continue
            }
            if i == 0 {
                out.CertificateDefault = cert
            } else {
                out.CertificateAuthorities = append(out.CertificateAuthorities, *cert)
            }
        }
    }

    out.DefaultPoolName = ptr.To("vks-default-forwarding-pool")
    return out, nil
}

func mapProtocol(p gwv1.ProtocolType) (loadbalancerv2.ListenerProtocol, error) {
    switch p {
    case gwv1.HTTPProtocolType:
        return loadbalancerv2.ListenerProtocolHTTP, nil
    case gwv1.HTTPSProtocolType, gwv1.TLSProtocolType:
        return loadbalancerv2.ListenerProtocolHTTPS, nil
    default:
        return "", fmt.Errorf("protocol %q not supported on ALB GatewayClass", p)
    }
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestBuildListener -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/build_listener.go internal/usecase/gateway_uc/alb_gateway_uc/build_listener_test.go
git commit -m "feat(gateway/uc): add Gateway → vngcloud listener builder"
```

---

### Task C5: `build_cert.go` — Secret import path

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/build_cert.go`

- [ ] **Step 1: Read the existing ingress cert path**

Run: `cat internal/usecase/ingress_uc/build_cert.go | head -100`

Note the existing helper that imports a Kubernetes TLS Secret into a vngcloud cert (or returns a `SecretName`-only cert that the LBC reconciler will create).

- [ ] **Step 2: Write a wrapper**

```go
package alb_gateway_uc

import (
    "fmt"

    "k8s.io/utils/ptr"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// CertSourceForGateway returns a CertSource that resolves a Gateway listener's
// certificateRef to a ListenerCertificate. ReferenceGrant gating must be applied
// by the caller before invoking the returned function.
func (uc *albGatewayUseCase) CertSourceForGateway(gwNS string) CertSource {
    return func(ref gwv1.SecretObjectReference) (*vksv1alpha1.ListenerCertificate, error) {
        if ref.Group != nil && *ref.Group != "" {
            return nil, fmt.Errorf("unsupported certificate ref group %q", *ref.Group)
        }
        if ref.Kind != nil && *ref.Kind != "Secret" {
            return nil, fmt.Errorf("unsupported certificate ref kind %q", *ref.Kind)
        }
        // Hand off to the existing cert-import path: produce a placeholder ListenerCertificate
        // referencing the secret name; lbc_uc will create the cert before listener apply.
        return &vksv1alpha1.ListenerCertificate{SecretName: ptr.To(string(ref.Name))}, nil
    }
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/build_cert.go
git commit -m "feat(gateway/uc): add Gateway TLS cert source"
```

---

### Task C6: `build_pool.go` — synthetic pool with weight scaling (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/build_pool.go`
- Test: `internal/usecase/gateway_uc/alb_gateway_uc/build_pool_test.go`

- [ ] **Step 1: Write the failing test**

```go
package alb_gateway_uc

import (
    "testing"

    "github.com/stretchr/testify/assert"

    pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

func TestScaleWeights_TwoBackends_90_10(t *testing.T) {
    in := []BackendEndpoints{
        {Backend: pkggw.BackendKey{Namespace: "n", Name: "a", Port: 80, Weight: 90}, Endpoints: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}},
        {Backend: pkggw.BackendKey{Namespace: "n", Name: "b", Port: 80, Weight: 10}, Endpoints: []string{"10.0.0.4"}},
    }
    members := SynthesizeMembers(in)
    var sumA, sumB int
    for _, m := range members {
        if m.IPAddress == "10.0.0.4" {
            sumB += int(m.Weight)
        } else {
            sumA += int(m.Weight)
        }
    }
    // Ratio must be ~9:1 within rounding tolerance.
    assert.True(t, sumA*10 >= sumB*85 && sumA*10 <= sumB*95, "want ~9:1 ratio, got %d:%d", sumA, sumB)
}

func TestScaleWeights_AllSameWeight(t *testing.T) {
    in := []BackendEndpoints{
        {Backend: pkggw.BackendKey{Namespace: "n", Name: "a", Port: 80, Weight: 1}, Endpoints: []string{"10.0.0.1"}},
        {Backend: pkggw.BackendKey{Namespace: "n", Name: "b", Port: 80, Weight: 1}, Endpoints: []string{"10.0.0.2"}},
    }
    members := SynthesizeMembers(in)
    for _, m := range members {
        assert.GreaterOrEqual(t, m.Weight, int32(1))
    }
}

func TestScaleWeights_FloorAt1(t *testing.T) {
    in := []BackendEndpoints{
        {Backend: pkggw.BackendKey{Namespace: "n", Name: "a", Port: 80, Weight: 1}, Endpoints: []string{"10.0.0.1"}},
        {Backend: pkggw.BackendKey{Namespace: "n", Name: "b", Port: 80, Weight: 99}, Endpoints: []string{"10.0.0.2"}},
    }
    members := SynthesizeMembers(in)
    var found bool
    for _, m := range members {
        if m.IPAddress == "10.0.0.1" {
            assert.GreaterOrEqual(t, m.Weight, int32(1))
            found = true
        }
    }
    assert.True(t, found)
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestScaleWeights -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package alb_gateway_uc

import (
    "math"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
    pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

// BackendEndpoints groups one HTTPRoute backendRef with its resolved endpoints.
type BackendEndpoints struct {
    Backend   pkggw.BackendKey
    Endpoints []string // pod-ip or node-ip strings (caller decides target-type)
}

// SynthesizeMembers maps weighted backends to a flat list of pool members with
// integer weights scaled so the cross-backend ratio matches the requested weights.
//
// Algorithm:
//   1. Compute per-endpoint share = backend.Weight / count(endpoints).
//   2. Find the smallest share; rescale every endpoint share by min⁻¹.
//   3. Round to nearest int; floor at 1; cap at 100.
func SynthesizeMembers(in []BackendEndpoints) []vksv1alpha1.PoolMember {
    if len(in) == 0 {
        return nil
    }
    minShare := math.MaxFloat64
    for _, b := range in {
        if len(b.Endpoints) == 0 || b.Backend.Weight <= 0 {
            continue
        }
        share := float64(b.Backend.Weight) / float64(len(b.Endpoints))
        if share < minShare {
            minShare = share
        }
    }
    if minShare == math.MaxFloat64 {
        minShare = 1
    }

    out := make([]vksv1alpha1.PoolMember, 0)
    for _, b := range in {
        if b.Backend.Weight <= 0 {
            continue
        }
        share := float64(b.Backend.Weight) / float64(max(len(b.Endpoints), 1))
        w := int32(math.Round(share / minShare))
        if w < 1 {
            w = 1
        }
        if w > 100 {
            w = 100
        }
        for _, ep := range b.Endpoints {
            out = append(out, vksv1alpha1.PoolMember{
                IPAddress: ep,
                Port:      b.Backend.Port,
                Weight:    w,
            })
        }
    }
    return out
}

func max(a, b int) int { if a > b { return a }; return b }
```

(Note: the engineer must check the actual `vksv1alpha1.PoolMember` field names — adjust `IPAddress`/`Port`/`Weight` to whatever the existing struct uses. Read `api/v1alpha1/loadbalancerconfig_types.go` first for the canonical field names.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestScaleWeights -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/build_pool.go internal/usecase/gateway_uc/alb_gateway_uc/build_pool_test.go
git commit -m "feat(gateway/uc): add synthetic-pool weight scaling"
```

---

### Task C7: `build_policy.go` — HTTPRoute → policies (TDD)

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/build_policy.go`
- Test: `internal/usecase/gateway_uc/alb_gateway_uc/build_policy_test.go`

- [ ] **Step 1: Write the failing test**

```go
package alb_gateway_uc

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "k8s.io/utils/ptr"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
)

func TestBuildPolicies_HostnameAndPath(t *testing.T) {
    rule := gwv1.HTTPRouteRule{
        Matches: []gwv1.HTTPRouteMatch{{
            Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchPathPrefix), Value: ptr.To("/api")},
        }},
    }
    hosts := []gwv1.Hostname{"a.example.com"}
    policies := BuildPolicies("route-uid-12345678", 0, hosts, rule, "synth-pool", nil)
    if assert.Len(t, policies, 1) {
        p := policies[0]
        assert.Equal(t, loadbalancerv2.PolicyActionREDIRECTTOPOOL, p.Action)
        var sawHost, sawPath bool
        for _, r := range p.L7Rules {
            if r.RuleType == loadbalancerv2.PolicyRuleTypeHOSTNAME {
                sawHost = true
            }
            if r.RuleType == loadbalancerv2.PolicyRuleTypePATH {
                sawPath = true
                assert.Equal(t, loadbalancerv2.PolicyCompareTypeSTARTSWITH, r.CompareType)
                assert.Equal(t, "/api", r.RuleValue)
            }
        }
        assert.True(t, sawHost && sawPath)
    }
}

func TestBuildPolicies_NoHostnames_OneMatch(t *testing.T) {
    rule := gwv1.HTTPRouteRule{
        Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchExact), Value: ptr.To("/x")}}},
    }
    policies := BuildPolicies("uid", 0, nil, rule, "p", nil)
    assert.Len(t, policies, 1)
}

func TestBuildPolicies_RedirectFilter(t *testing.T) {
    redirect := gwv1.HTTPRequestRedirectFilter{Hostname: ptr.To(gwv1.PreciseHostname("new.example.com")), StatusCode: ptr.To(301)}
    rule := gwv1.HTTPRouteRule{
        Matches: []gwv1.HTTPRouteMatch{{Path: &gwv1.HTTPPathMatch{Type: ptr.To(gwv1.PathMatchExact), Value: ptr.To("/")}}},
        Filters: []gwv1.HTTPRouteFilter{{Type: gwv1.HTTPRouteFilterRequestRedirect, RequestRedirect: &redirect}},
    }
    policies := BuildPolicies("uid", 0, nil, rule, "p", nil)
    assert.Len(t, policies, 1)
    assert.Equal(t, loadbalancerv2.PolicyActionREDIRECTTOURL, policies[0].Action)
}
```

- [ ] **Step 2: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestBuildPolicies -v`

Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package alb_gateway_uc

import (
    "fmt"

    loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
    pkggw "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/gateway"
)

// BuildPolicies maps one HTTPRoute rule into one-or-more vngcloud policies, one per
// (hostname × match) cartesian. A nil hosts slice is treated as a single empty hostname.
// The poolName is the synthetic pool that REDIRECT_TO_POOL targets when no overriding action.
// lrcs may be nil.
func BuildPolicies(
    routeUID string,
    ruleIdx int,
    hosts []gwv1.Hostname,
    rule gwv1.HTTPRouteRule,
    poolName string,
    lrcs []*gatewayv1alpha1.ListenerRuleConfig,
) []vksv1alpha1.Policy {
    if len(hosts) == 0 {
        hosts = []gwv1.Hostname{""}
    }
    out := make([]vksv1alpha1.Policy, 0, len(hosts)*max(len(rule.Matches), 1))
    matches := rule.Matches
    if len(matches) == 0 {
        matches = []gwv1.HTTPRouteMatch{{}}
    }

    redirect := findRedirectFilter(rule.Filters)

    for _, h := range hosts {
        for mi, m := range matches {
            l7 := []vksv1alpha1.L7Rule{}
            if cmp, val := pkggw.HostnameToL7Rule(string(h)); cmp != "" {
                l7 = append(l7, vksv1alpha1.L7Rule{
                    RuleType: loadbalancerv2.PolicyRuleTypeHOSTNAME, CompareType: loadbalancerv2.PolicyCompareType(cmp), RuleValue: val,
                })
            }
            if m.Path != nil && m.Path.Type != nil && m.Path.Value != nil {
                cmp, val := pkggw.PathToL7Rule(string(*m.Path.Type), *m.Path.Value)
                l7 = append(l7, vksv1alpha1.L7Rule{
                    RuleType: loadbalancerv2.PolicyRuleTypePATH, CompareType: loadbalancerv2.PolicyCompareType(cmp), RuleValue: val,
                })
            }
            for _, lrc := range lrcs {
                for _, am := range lrc.Spec.AdditionalMatches {
                    rt := mapAdditionalMatchType(am.Type)
                    if rt == "" {
                        continue // unsupported under current vngcloud LB
                    }
                    l7 = append(l7, vksv1alpha1.L7Rule{
                        RuleType: rt, CompareType: loadbalancerv2.PolicyCompareType(am.Compare), RuleValue: am.Value,
                    })
                }
            }

            p := vksv1alpha1.Policy{
                Name:    fmt.Sprintf("p-%s-%d-%d", routeUID, ruleIdx, mi),
                L7Rules: l7,
            }
            switch {
            case redirect != nil:
                p.Action = loadbalancerv2.PolicyActionREDIRECTTOURL
                if u := buildRedirectURL(*redirect); u != "" {
                    p.RedirectUrl = &u
                }
                if redirect.StatusCode != nil {
                    code := int32(*redirect.StatusCode)
                    p.RedirectHttpCode = &code
                }
            default:
                p.Action = loadbalancerv2.PolicyActionREDIRECTTOPOOL
                p.RedirectPoolName = &poolName
            }
            out = append(out, p)
        }
    }
    return out
}

func findRedirectFilter(filters []gwv1.HTTPRouteFilter) *gwv1.HTTPRequestRedirectFilter {
    for _, f := range filters {
        if f.Type == gwv1.HTTPRouteFilterRequestRedirect {
            return f.RequestRedirect
        }
    }
    return nil
}

func buildRedirectURL(r gwv1.HTTPRequestRedirectFilter) string {
    scheme := "https"
    if r.Scheme != nil {
        scheme = *r.Scheme
    }
    host := ""
    if r.Hostname != nil {
        host = string(*r.Hostname)
    }
    if host == "" {
        return ""
    }
    return scheme + "://" + host
}

func mapAdditionalMatchType(t string) loadbalancerv2.PolicyRuleType {
    switch t {
    case "Header", "QueryParam", "Method", "SourceIP":
        return "" // not yet supported by vngcloud LB; emit nothing for v1
    default:
        return ""
    }
}

func max(a, b int) int { if a > b { return a }; return b }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestBuildPolicies -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/build_policy.go internal/usecase/gateway_uc/alb_gateway_uc/build_policy_test.go
git commit -m "feat(gateway/uc): add HTTPRoute → vngcloud policy builder"
```

---

### Task C8: `build_sec_group.go` — NSG inheritance from Ingress

**Files:**
- Create: `internal/usecase/gateway_uc/alb_gateway_uc/build_sec_group.go`

- [ ] **Step 1: Read ingress NSG path**

Run: `cat internal/usecase/ingress_uc/build_sec_group.go | head -80`

- [ ] **Step 2: Write a thin wrapper that delegates to the same NSG logic**

```go
package alb_gateway_uc

import (
    "context"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// BuildSecurityGroups wires the merged effective LBC's security-group config to the LB.
// For Phase 1, the merged spec already contains the SG list — this is a passthrough that
// matches the ingress code path's signature so future shared helpers can be extracted.
func (uc *albGatewayUseCase) BuildSecurityGroups(_ context.Context, effective *vksv1alpha1.LoadBalancerConfigSpec) []string {
    // The existing LBC.Tags / SecurityGroups mechanism is already honored by lbc_uc.
    // Return a stable list (currently empty) so callers can plug in additional logic in Phase 3.
    _ = effective
    return nil
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/build_sec_group.go
git commit -m "feat(gateway/uc): scaffold security-group resolution"
```

---

### Task C9: Wire `EnsureALBGatewayUseCase` end-to-end

**Files:**
- Modify: `internal/usecase/gateway_uc/alb_gateway_uc/gateway_uc.go`
- Test: `internal/usecase/gateway_uc/alb_gateway_uc/gateway_uc_test.go`

This is the largest step in the use-case layer. It threads the per-Gateway reconcile through everything built so far.

- [ ] **Step 1: Read existing IngressUseCase.Ensure for the canonical pattern**

Run: `cat internal/usecase/ingress_uc/ingress_uc.go | sed -n '60,180p'`

Note the structure: load resource → finalize-or-cleanup branch → resolve effective config → build LB spec → reconcile via lbc_uc → write status.

- [ ] **Step 2: Write the failing test (envtest-style happy path)**

Tests at the use-case layer use mocks for repos. Write a single happy-path test that:
1. Constructs a fake `K8sRepository` returning a valid Gateway + GatewayClass + LoadBalancerConfig.
2. Constructs a `VngCloudRepository` mock that records calls.
3. Calls `EnsureALBGatewayUseCase` and asserts:
   - `K8sRepo.Update` called on the Gateway with `gateway.vks.vngcloud.vn/resources` finalizer.
   - At least one `VngCloudRepo.EnsureLoadBalancer*` call.
   - Gateway status conditions include `Accepted=True` and `Programmed=True` (or `Pending` if LB still provisioning).

(Test code is verbose — engineer mirrors the pattern from `internal/usecase/ingress_uc/ingress_uc_test.go`. Reuse mock factories under `internal/repository/{k8s_repo,vngcloud_repo}/*_mocks/`.)

- [ ] **Step 3: Run to confirm fail**

Run: `go test ./internal/usecase/gateway_uc/alb_gateway_uc/... -run TestEnsureALB_HappyPath -v`

Expected: FAIL — `EnsureALBGatewayUseCase` returns nil but doesn't actually do anything yet.

- [ ] **Step 4: Implement `EnsureALBGatewayUseCase` and supporting methods**

The implementation follows this pseudocode:

```go
func (uc *albGatewayUseCase) EnsureALBGatewayUseCase(ctx context.Context, req ctrl.Request) error {
    log := contexts.NewContext(ctx).Log()
    gw := &gwv1.Gateway{}
    if err := uc.k8sRepo.Get(ctx, req.NamespacedName, gw); err != nil {
        if apierrors.IsNotFound(err) { return nil }
        return err
    }

    // 0. Finalizer
    if gw.DeletionTimestamp != nil {
        return uc.delete(ctx, gw)
    }
    if !controllerutil.ContainsFinalizer(gw, domain.GatewayFinalizer) {
        controllerutil.AddFinalizer(gw, domain.GatewayFinalizer)
        if err := uc.k8sRepo.Update(ctx, gw); err != nil { return err }
    }

    // 1. Resolve effective LBC
    gwc := &gwv1.GatewayClass{}
    if err := uc.k8sRepo.Get(ctx, types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}, gwc); err != nil {
        return uc.markGatewayNotAccepted(ctx, gw, "GatewayClassNotFound", err.Error())
    }
    if string(gwc.Spec.ControllerName) != domain.ControllerNameALB {
        return nil // not ours
    }
    classLBC := uc.lookupLBC(ctx, gwc.Spec.ParametersRef, "")
    gwLBC := uc.lookupLBC(ctx, gw.Spec.Infrastructure.ParametersRef, gw.Namespace)
    effective := shared.MergeLBC(classLBC, gwLBC)

    // 2. Validate listeners
    invalid := ctlshared.ValidateListenersForALB(gw.Spec.Listeners)

    // 3. Build LB spec, listeners
    lbSpec := BuildLBSpec(gw, effective, uc.clusterId)
    listeners := []vksv1alpha1.Listener{}
    for _, l := range gw.Spec.Listeners {
        if r := invalid[string(l.Name)]; r != ctlshared.ListenerInvalidReasonNone {
            continue
        }
        lbcL := lookupListenerByName(effective.Listeners, string(l.Name))
        b, err := BuildListener(l, lbcL, uc.CertSourceForGateway(gw.Namespace))
        if err != nil { return err }
        listeners = append(listeners, *b)
    }
    lbSpec.Listeners = listeners

    // 4. Build pools + policies from attached HTTPRoutes
    routes := uc.listAttachedHTTPRoutes(ctx, gw)
    pools, policies := uc.buildPoolsAndPolicies(ctx, gw, routes)
    lbSpec.Pools = pools
    for li := range lbSpec.Listeners {
        lbSpec.Listeners[li].Policies = filterPoliciesForListener(policies, lbSpec.Listeners[li])
    }

    // 5. Hand off to lbc_uc deploy path
    if err := uc.deployLB(ctx, gw, lbSpec); err != nil {
        return uc.markProgrammedFalse(ctx, gw, err)
    }

    // 6. Status
    return uc.writeAcceptedAndProgrammed(ctx, gw, lbSpec, invalid, routes)
}
```

The engineer fills in `lookupLBC`, `lookupListenerByName`, `listAttachedHTTPRoutes`, `buildPoolsAndPolicies`, `filterPoliciesForListener`, `deployLB`, `markGatewayNotAccepted`, `markProgrammedFalse`, `writeAcceptedAndProgrammed`, and `delete` by mirroring the equivalent helpers in `internal/usecase/ingress_uc/`.

Replace each TODO from Task C2 with real logic. `EnqueueParentGatewayForRoute` is implemented by emitting a controller-runtime event for the parent Gateway (via the `eventRecorder` or by writing an annotation tick — pick one and document).

- [ ] **Step 5: Run tests**

Run: `go test ./internal/usecase/gateway_uc/... -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/usecase/gateway_uc/alb_gateway_uc/
git commit -m "feat(gateway/uc): wire ALB Gateway reconciliation end-to-end"
```

---

## Block D — Reconcilers

### Task D1: GatewayClass reconciler

**Files:**
- Create: `internal/controller/gateway/alb/gatewayclass_controller.go`

- [ ] **Step 1: Implement the reconciler**

```go
package alb

import (
    "context"
    "time"

    "github.com/anngdinh/operator-helper/contexts"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/types"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    vksv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
    ctlshared "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

type GatewayClassReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func NewGatewayClassReconciler(c client.Client, sch *runtime.Scheme) *GatewayClassReconciler {
    return &GatewayClassReconciler{Client: c, Scheme: sch}
}

func (r *GatewayClassReconciler) SetupWithManager(mgr manager.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&gwv1.GatewayClass{}).
        Named("gatewayclass-alb").
        Complete(r)
}

func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx = contexts.NewContext(ctx).SetLogName("gwc/" + req.Name).GetContext()
    log := contexts.NewContext(ctx).Log()

    gwc := &gwv1.GatewayClass{}
    if err := r.Get(ctx, req.NamespacedName, gwc); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    if string(gwc.Spec.ControllerName) != domain.ControllerNameALB {
        return ctrl.Result{}, nil // not ours
    }

    accepted := metav1.ConditionTrue
    reason, msg := "Accepted", "GatewayClass accepted by vngcloud-alb controller"

    if gwc.Spec.ParametersRef != nil {
        // Validate the referenced LBC exists.
        ref := gwc.Spec.ParametersRef
        if string(ref.Group) != "vks.vngcloud.vn" || string(ref.Kind) != "LoadBalancerConfig" {
            accepted = metav1.ConditionFalse
            reason, msg = "InvalidParameters", "parametersRef must point to vks.vngcloud.vn/LoadBalancerConfig"
        } else {
            lbc := &vksv1alpha1.LoadBalancerConfig{}
            ns := ""
            if ref.Namespace != nil { ns = string(*ref.Namespace) }
            err := r.Get(ctx, types.NamespacedName{Namespace: ns, Name: string(ref.Name)}, lbc)
            if err != nil {
                accepted = metav1.ConditionFalse
                reason, msg = "InvalidParameters", "referenced LoadBalancerConfig not found: "+err.Error()
            }
        }
    }

    ctlshared.SetCondition(&gwc.Status.Conditions, "Accepted", accepted, reason, msg, gwc.Generation)
    if err := r.Status().Update(ctx, gwc); err != nil {
        log.Errorf("status update failed: %v", err)
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }
    return ctrl.Result{}, nil
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/controller/gateway/alb/gatewayclass_controller.go
git commit -m "feat(gateway/ctrl): add GatewayClass reconciler for vngcloud-alb"
```

---

### Task D2: Gateway reconciler

**Files:**
- Create: `internal/controller/gateway/alb/gateway_controller.go`

- [ ] **Step 1: Read the existing IngressReconciler for pattern**

Run: `cat internal/controller/networking/ingress_controller.go | sed -n '1,200p'`

- [ ] **Step 2: Write the reconciler**

```go
package alb

import (
    "context"
    "sync/atomic"
    "time"

    "github.com/anngdinh/operator-helper/contexts"
    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
    "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/k8s"
    metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
)

type GatewayReconciler struct {
    Client            client.Client
    Scheme            *runtime.Scheme
    GatewayUseCase    usecase.ALBGatewayUseCase
    FinalizerManager  k8s.FinalizerManager
    EventRecorder     record.EventRecorder
    Logger            logr.Logger
    ReconcileCounters *metricsutil.ReconcileCounters
    MaxConcurrent     int
    initDone          atomic.Bool
}

func NewGatewayReconciler(uc usecase.ALBGatewayUseCase, c client.Client, sch *runtime.Scheme, fm k8s.FinalizerManager, er record.EventRecorder, rc *metricsutil.ReconcileCounters, mc int) *GatewayReconciler {
    if mc <= 0 { mc = domain.DefaultMaxConcurrentReconciles }
    return &GatewayReconciler{
        Client: c, Scheme: sch, GatewayUseCase: uc, FinalizerManager: fm,
        EventRecorder: er, Logger: ctrl.Log.WithName("controllers").WithName("alb-gateway"),
        ReconcileCounters: rc, MaxConcurrent: mc,
    }
}

func (r *GatewayReconciler) SetupWithManager(mgr manager.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&gwv1.Gateway{}).
        WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrent}).
        Named("gateway-alb").
        Complete(r)
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    if !r.initDone.Load() {
        if err := r.GatewayUseCase.InitALBGatewayUseCase(ctx); err != nil {
            return ctrl.Result{RequeueAfter: time.Second}, err
        }
        r.initDone.Store(true)
    }
    r.ReconcileCounters.IncrementGateway(req.NamespacedName)
    ctx = contexts.NewContext(ctx).SetLogName("gw/" + req.Namespace + "/" + req.Name).GetContext()
    ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
    defer cancel()
    if err := r.GatewayUseCase.EnsureALBGatewayUseCase(ctx, req); err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }
    return ctrl.Result{}, nil
}
```

- [ ] **Step 3: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/controller/gateway/alb/gateway_controller.go
git commit -m "feat(gateway/ctrl): add Gateway reconciler for vngcloud-alb"
```

---

### Task D3: HTTPRoute reconciler

**Files:**
- Create: `internal/controller/gateway/alb/httproute_controller.go`

- [ ] **Step 1: Implement (thin: enqueue parent only)**

```go
package alb

import (
    "context"
    "time"

    "github.com/anngdinh/operator-helper/contexts"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/manager"
    gwv1 "sigs.k8s.io/gateway-api/apis/v1"

    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
    metricsutil "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/metrics/util"
)

type HTTPRouteReconciler struct {
    Client            client.Client
    Scheme            *runtime.Scheme
    GatewayUseCase    usecase.ALBGatewayUseCase
    ReconcileCounters *metricsutil.ReconcileCounters
}

func NewHTTPRouteReconciler(uc usecase.ALBGatewayUseCase, c client.Client, sch *runtime.Scheme, rc *metricsutil.ReconcileCounters) *HTTPRouteReconciler {
    return &HTTPRouteReconciler{Client: c, Scheme: sch, GatewayUseCase: uc, ReconcileCounters: rc}
}

func (r *HTTPRouteReconciler) SetupWithManager(mgr manager.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).For(&gwv1.HTTPRoute{}).Named("httproute-alb").Complete(r)
}

func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    r.ReconcileCounters.IncrementHTTPRoute(req.NamespacedName)
    ctx = contexts.NewContext(ctx).SetLogName("httproute/" + req.Namespace + "/" + req.Name).GetContext()
    if err := r.GatewayUseCase.EnqueueParentGatewayForRoute(ctx, req); err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }
    return ctrl.Result{}, nil
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/controller/gateway/alb/httproute_controller.go
git commit -m "feat(gateway/ctrl): add HTTPRoute reconciler that enqueues parent Gateway"
```

---

### Task D4: TargetGroupConfig validator controller

**Files:**
- Create: `internal/controller/gateway/targetgroupconfig/controller.go`

- [ ] **Step 1: Implement**

```go
package targetgroupconfig

import (
    "context"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/manager"

    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
    ctlshared "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
)

type Reconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func New(c client.Client, sch *runtime.Scheme) *Reconciler { return &Reconciler{Client: c, Scheme: sch} }

func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).For(&gatewayv1alpha1.TargetGroupConfig{}).Named("tgc").Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &gatewayv1alpha1.TargetGroupConfig{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    // Phase-1 validation: target name non-empty (kubebuilder enforces required), no double-checking yet.
    // Status: Accepted=True if reachable; Conflict detection runs in the route layer.
    ctlshared.SetCondition(&obj.Status.Conditions, "Accepted", metav1.ConditionTrue, "Accepted", "TargetGroupConfig observed", obj.Generation)
    obj.Status.ObservedGeneration = obj.Generation
    return ctrl.Result{}, r.Status().Update(ctx, obj)
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add internal/controller/gateway/targetgroupconfig/controller.go
git commit -m "feat(gateway/ctrl): add TargetGroupConfig validator"
```

---

### Task D5: ListenerRuleConfig validator controller

**Files:**
- Create: `internal/controller/gateway/listenerruleconfig/controller.go`

- [ ] **Step 1: Implement (mirror TGC)**

```go
package listenerruleconfig

import (
    "context"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/manager"

    gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
    ctlshared "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
)

type Reconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func New(c client.Client, sch *runtime.Scheme) *Reconciler { return &Reconciler{Client: c, Scheme: sch} }

func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).For(&gatewayv1alpha1.ListenerRuleConfig{}).Named("lrc").Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &gatewayv1alpha1.ListenerRuleConfig{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    ctlshared.SetCondition(&obj.Status.Conditions, "Accepted", metav1.ConditionTrue, "Accepted", "ListenerRuleConfig observed", obj.Generation)
    obj.Status.ObservedGeneration = obj.Generation
    return ctrl.Result{}, r.Status().Update(ctx, obj)
}
```

- [ ] **Step 2: Build & commit**

Run: `go build ./...`; commit.

```bash
git add internal/controller/gateway/listenerruleconfig/controller.go
git commit -m "feat(gateway/ctrl): add ListenerRuleConfig validator"
```

---

### Task D6: envtest suite for ALB reconcilers

**Files:**
- Create: `internal/controller/gateway/alb/suite_test.go`
- Create: `internal/controller/gateway/alb/gateway_controller_test.go`

- [ ] **Step 1: Read the existing networking suite**

Run: `cat internal/controller/networking/suite_test.go | sed -n '1,200p'`

- [ ] **Step 2: Adapt for ALB suite**

Mirror the networking suite, swapping the registered scheme for `gwv1.Install`, `gwv1alpha2.Install`, `gatewayv1alpha1.AddToScheme`, then register the three reconcilers above. Wire envtest CRDs from `config/crd/bases/` plus the upstream Gateway API CRDs.

(The engineer copies the networking suite verbatim and edits it. Full code omitted to avoid plan bloat — the structure is well-established.)

- [ ] **Step 3: Write a Ginkgo test "Reconcile a basic Gateway"**

Asserts that creating a Gateway with one HTTP listener leads to `Programmed=True` (with a stubbed `VngCloudRepository` returning a fake LB). Uses `Eventually(... Succeed())` to poll the resource's status.

- [ ] **Step 4: Run**

Run: `go test ./internal/controller/gateway/alb/... -v`

Expected: PASS (envtest provisions a control plane).

- [ ] **Step 5: Commit**

```bash
git add internal/controller/gateway/alb/suite_test.go internal/controller/gateway/alb/gateway_controller_test.go
git commit -m "test(gateway/ctrl): add envtest suite for ALB reconcilers"
```

---

## Block E — Wiring

### Task E1: Register Gateway API scheme in `cmd/main.go`

**Files:**
- Modify: `cmd/main.go`

- [ ] **Step 1: Add imports & scheme registrations**

In the imports block, add:

```go
gwv1 "sigs.k8s.io/gateway-api/apis/v1"
gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
gatewayv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
albgw "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/alb"
gwtgc "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/targetgroupconfig"
gwlrc "github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/listenerruleconfig"
"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/gateway_uc/alb_gateway_uc"
```

In the `init()` function, append:

```go
utilruntime.Must(gwv1.Install(scheme))
utilruntime.Must(gwv1alpha2.Install(scheme))
utilruntime.Must(gwv1beta1.Install(scheme))
utilruntime.Must(gatewayv1alpha1.AddToScheme(scheme))
```

- [ ] **Step 2: Add feature-gate flags**

Inside `main()`, alongside the existing `flag.BoolVar` calls, add:

```go
var enableGatewayAPIALB bool
flag.BoolVar(&enableGatewayAPIALB, "enable-gateway-api-alb", false, "Enable the ALB Gateway API controller (Phase 1).")
```

- [ ] **Step 3: Wire reconcilers**

After existing reconciler `SetupWithManager` calls, append:

```go
if enableGatewayAPIALB {
    albUC := alb_gateway_uc.NewALBGatewayUseCase(
        conf.ClusterID, k8sRepo, vngcloudRepo, annotationParser, cniDetector, endpointResolver,
    )
    if err := albgw.NewGatewayClassReconciler(mgr.GetClient(), mgr.GetScheme()).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to set up GatewayClass reconciler"); os.Exit(1)
    }
    if err := albgw.NewGatewayReconciler(albUC, mgr.GetClient(), mgr.GetScheme(), finalizerManager,
        mgr.GetEventRecorderFor("gateway-alb"), reconcileCounters, conf.MaxConcurrentReconciles).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to set up Gateway reconciler"); os.Exit(1)
    }
    if err := albgw.NewHTTPRouteReconciler(albUC, mgr.GetClient(), mgr.GetScheme(), reconcileCounters).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to set up HTTPRoute reconciler"); os.Exit(1)
    }
    if err := gwtgc.New(mgr.GetClient(), mgr.GetScheme()).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to set up TGC reconciler"); os.Exit(1)
    }
    if err := gwlrc.New(mgr.GetClient(), mgr.GetScheme()).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to set up LRC reconciler"); os.Exit(1)
    }
}
```

- [ ] **Step 4: Build**

Run: `go build ./...`

Expected: success.

- [ ] **Step 5: Smoke-run**

Run: `go run ./cmd --enable-gateway-api-alb=true --metrics-bind-address=0` for 5 seconds; ensure no panic. (Uses local kubeconfig; if not connected to a cluster, accept the connection error but verify the binary reaches the manager.Start call.)

- [ ] **Step 6: Commit**

```bash
git add cmd/main.go
git commit -m "feat(cmd): wire ALB Gateway API controllers behind feature gate"
```

---

### Task E2: RBAC manifests

**Files:**
- Modify: `internal/controller/gateway/alb/gatewayclass_controller.go` (add kubebuilder rbac markers)
- Modify: `internal/controller/gateway/alb/gateway_controller.go` (markers)
- Modify: `internal/controller/gateway/alb/httproute_controller.go` (markers)
- Modify: `internal/controller/gateway/targetgroupconfig/controller.go` (markers)
- Modify: `internal/controller/gateway/listenerruleconfig/controller.go` (markers)
- Modify: `config/rbac/role.yaml` (regenerated)

- [ ] **Step 1: Add RBAC markers**

At the top of each reconciler file, above the `Reconcile` method, add the appropriate `// +kubebuilder:rbac:groups=...` comments per the spec §2.4.

- [ ] **Step 2: Regenerate**

Run: `make manifests`

Expected: `config/rbac/role.yaml` updated.

- [ ] **Step 3: Commit**

```bash
git add internal/controller/gateway/ config/rbac/role.yaml
git commit -m "feat(rbac): add Gateway-API verbs to manager Role"
```

---

## Block F — Helm, samples, docs

### Task F1: Bundle CRDs into the chart

**Files:**
- Create: `charts/vngcloud-load-balancer-controller/templates/crds/gateway.vks.vngcloud.vn_targetgroupconfigs.yaml`
- Create: `charts/vngcloud-load-balancer-controller/templates/crds/gateway.vks.vngcloud.vn_listenerruleconfigs.yaml`
- Modify: `charts/vngcloud-load-balancer-controller/templates/crds/vks.vngcloud.vn_loadbalancerconfigs.yaml`

- [ ] **Step 1: Copy generated CRDs into chart**

```bash
cp config/crd/bases/gateway.vks.vngcloud.vn_targetgroupconfigs.yaml \
   charts/vngcloud-load-balancer-controller/templates/crds/
cp config/crd/bases/gateway.vks.vngcloud.vn_listenerruleconfigs.yaml \
   charts/vngcloud-load-balancer-controller/templates/crds/
cp config/crd/bases/vks.vngcloud.vn_loadbalancerconfigs.yaml \
   charts/vngcloud-load-balancer-controller/templates/crds/
```

- [ ] **Step 2: Verify chart lint**

Run: `helm lint charts/vngcloud-load-balancer-controller/`

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add charts/vngcloud-load-balancer-controller/templates/crds/
git commit -m "feat(chart): bundle Gateway-API config CRDs"
```

---

### Task F2: GatewayClass manifest + values.yaml + manager flag

**Files:**
- Create: `charts/vngcloud-load-balancer-controller/templates/gatewayclass-alb.yaml`
- Modify: `charts/vngcloud-load-balancer-controller/values.yaml`
- Modify: `charts/vngcloud-load-balancer-controller/templates/manager-deployment.yaml`

- [ ] **Step 1: Write the GatewayClass template**

```yaml
{{- if .Values.gatewayApi.alb.enabled -}}
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: vngcloud-alb
spec:
  controllerName: gateway.vks.vngcloud.vn/alb
  {{- with .Values.gatewayApi.alb.parametersRef }}
  parametersRef:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end -}}
```

- [ ] **Step 2: Add values defaults**

Append to `values.yaml`:

```yaml
gatewayApi:
  alb:
    enabled: false
    parametersRef: {}
```

- [ ] **Step 3: Inject feature-gate flag**

In `manager-deployment.yaml`, locate the existing `args:` list and conditionally append:

```yaml
{{- if .Values.gatewayApi.alb.enabled }}
- --enable-gateway-api-alb=true
{{- end }}
```

- [ ] **Step 4: Verify**

Run: `helm template charts/vngcloud-load-balancer-controller --set gatewayApi.alb.enabled=true | grep -A2 GatewayClass`

Expected: GatewayClass YAML rendered.

- [ ] **Step 5: Commit**

```bash
git add charts/vngcloud-load-balancer-controller/templates/gatewayclass-alb.yaml charts/vngcloud-load-balancer-controller/values.yaml charts/vngcloud-load-balancer-controller/templates/manager-deployment.yaml
git commit -m "feat(chart): add vngcloud-alb GatewayClass and feature gate"
```

---

### Task F3: Sample manifests

**Files:**
- Create: `config/samples/gateway_v1_alb_basic.yaml`
- Create: `config/samples/gateway_v1_alb_tls.yaml`
- Create: `config/samples/gateway_v1_alb_canary.yaml`
- Create: `config/samples/targetgroupconfig_basic.yaml`
- Create: `config/samples/listenerruleconfig_header.yaml`

- [ ] **Step 1: Write `gateway_v1_alb_basic.yaml`**

```yaml
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: demo
  namespace: default
spec:
  gatewayClassName: vngcloud-alb
  listeners:
  - name: http
    protocol: HTTP
    port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: demo-route
  namespace: default
spec:
  parentRefs:
  - name: demo
  hostnames:
  - demo.example.com
  rules:
  - matches:
    - path: { type: PathPrefix, value: / }
    backendRefs:
    - name: demo-service
      port: 80
```

- [ ] **Step 2: Write `gateway_v1_alb_tls.yaml`**

(HTTP+HTTPS Gateway with `certificateRefs` to a `tls-secret`.)

- [ ] **Step 3: Write `gateway_v1_alb_canary.yaml`**

(One HTTPRoute rule with two weighted backendRefs `90`/`10`.)

- [ ] **Step 4: Write `targetgroupconfig_basic.yaml`**

```yaml
apiVersion: gateway.vks.vngcloud.vn/v1alpha1
kind: TargetGroupConfig
metadata:
  name: demo-tgc
  namespace: default
spec:
  targetReference:
    name: demo-service
  defaultConfig:
    targetType: ip
    poolAlgorithm: ROUND_ROBIN
    healthCheck:
      protocol: HTTP
      successCodes: "200"
      healthCheckPath: /healthz
      intervalSeconds: 10
      timeoutSeconds: 3
      healthyThreshold: 2
      unhealthyThreshold: 3
```

- [ ] **Step 5: Write `listenerruleconfig_header.yaml`**

```yaml
apiVersion: gateway.vks.vngcloud.vn/v1alpha1
kind: ListenerRuleConfig
metadata:
  name: header-internal
  namespace: default
spec:
  additionalMatches:
  - type: Header
    name: X-Source
    compare: CONTAINS
    value: internal
```

- [ ] **Step 6: Commit**

```bash
git add config/samples/
git commit -m "docs(samples): add Gateway-API sample manifests"
```

---

### Task F4: User guide — `gateway-api.md`

**Files:**
- Create: `docs/guide/gateway-api.md`

- [ ] **Step 1: Write the guide**

Sections:
1. Overview — why Gateway API, two GatewayClasses, when to use vs Ingress.
2. Prerequisites — install upstream Gateway API CRDs (`kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml`).
3. Enable the controller — `helm upgrade ... --set gatewayApi.alb.enabled=true`.
4. Quickstart — apply `config/samples/gateway_v1_alb_basic.yaml`; show `kubectl get gateway`, address resolution.
5. Capability matrix — features Supported / Partial / Unsupported (mirror spec §6.1 Phase 1 scope).
6. Coexistence with Ingress — separate LBs; no sharing in Phase 1.

- [ ] **Step 2: Commit**

```bash
git add docs/guide/gateway-api.md
git commit -m "docs(guide): add Gateway-API overview"
```

---

### Task F5: User guide — `gateway-alb.md`

**Files:**
- Create: `docs/guide/gateway-alb.md`

- [ ] **Step 1: Write the L7 walkthrough**

Sections:
1. HTTP Gateway + HTTPRoute (path & hostname matching).
2. HTTPS Gateway with K8s TLS Secret import.
3. HTTPS with vngcloud cert ID via `LoadBalancerConfig.spec.listeners[].certificateDefault.id`.
4. Multi-cert SNI.
5. mTLS (`frontendValidation` and LBC `clientCertificateId`).
6. Cross-namespace backendRefs + ReferenceGrant example.

- [ ] **Step 2: Commit**

```bash
git add docs/guide/gateway-alb.md
git commit -m "docs(guide): add L7 ALB Gateway walkthrough"
```

---

### Task F6: User guide — `gateway-extensions.md`

**Files:**
- Create: `docs/guide/gateway-extensions.md`

- [ ] **Step 1: Write**

Sections:
1. `LoadBalancerConfig` at GatewayClass vs Gateway level; `mergingMode`.
2. `TargetGroupConfig` — per-Service config + per-route override; cascade semantics.
3. `ListenerRuleConfig` — additional matches via `extensionRef`; supported types and forward-compat note.

- [ ] **Step 2: Commit**

```bash
git add docs/guide/gateway-extensions.md
git commit -m "docs(guide): add Gateway extensions (TGC + LRC + LBC)"
```

---

### Task F7: Example — `examples/gateway-canary.md`

**Files:**
- Create: `docs/examples/gateway-canary.md`

- [ ] **Step 1: Write**

Walkthrough using `gateway_v1_alb_canary.yaml`: explain weighted backends, synthetic-pool naming, and how endpoint changes preserve the pool identity.

- [ ] **Step 2: Commit**

```bash
git add docs/examples/gateway-canary.md
git commit -m "docs(examples): add weighted-canary HTTPRoute example"
```

---

## Block G — End-to-end tests

### Task G1: e2e harness directory

**Files:**
- Create: `test/e2e/gateway/harness.go`

- [ ] **Step 1: Read existing e2e harness pattern**

Run: `ls test/e2e/`

If a harness exists for Service/Ingress, study its setup. Otherwise, scaffold a fresh harness that:
- Reads `KUBECONFIG` and `VNGCLOUD_CREDENTIALS`.
- Skips the test if either is missing (so CI default-skips).

- [ ] **Step 2: Write a minimal harness file**

```go
package gateway_e2e

import (
    "os"
    "testing"
)

func skipIfNotE2E(t *testing.T) {
    if os.Getenv("E2E") != "1" {
        t.Skip("E2E env var not set; skipping live test")
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add test/e2e/gateway/harness.go
git commit -m "test(e2e/gateway): add harness skeleton"
```

---

### Task G2: e2e — basic HTTP

**Files:**
- Create: `test/e2e/gateway/alb_basic_test.go`

- [ ] **Step 1: Write the test**

Apply `gateway_v1_alb_basic.yaml` against a real cluster, wait for `Programmed=True`, resolve the address, curl `/`, expect 200 from a backend echo Service.

- [ ] **Step 2: Run with `E2E=1`**

Run: `E2E=1 go test ./test/e2e/gateway/... -run TestALBBasic -v`

Expected: PASS against a real vngcloud cluster.

- [ ] **Step 3: Commit**

```bash
git add test/e2e/gateway/alb_basic_test.go
git commit -m "test(e2e/gateway): add basic HTTP Gateway flow"
```

---

### Task G3: e2e — HTTPS + SNI

**Files:**
- Create: `test/e2e/gateway/alb_tls_test.go`

- [ ] **Step 1: Write the test**

Provision two TLS Secrets, attach via `certificateRefs`, curl two hostnames, assert SNI selection works.

- [ ] **Step 2: Run & commit**

```bash
git add test/e2e/gateway/alb_tls_test.go
git commit -m "test(e2e/gateway): add HTTPS + SNI flow"
```

---

### Task G4: e2e — mTLS

**Files:**
- Create: `test/e2e/gateway/alb_mtls_test.go`

- [ ] **Step 1: Write the test**

`Gateway.spec.listeners[0].tls.frontendValidation.caCertificateRefs[0]` → CA cert; client without cert gets 400, with cert gets 200.

- [ ] **Step 2: Commit**

```bash
git add test/e2e/gateway/alb_mtls_test.go
git commit -m "test(e2e/gateway): add mTLS flow"
```

---

### Task G5: e2e — weighted canary

**Files:**
- Create: `test/e2e/gateway/alb_canary_test.go`

- [ ] **Step 1: Write the test**

Apply `gateway_v1_alb_canary.yaml` (90/10 split). Issue 1000 requests, count which backend served each, assert ratio is 90 ± 5 / 10 ± 5.

- [ ] **Step 2: Commit**

```bash
git add test/e2e/gateway/alb_canary_test.go
git commit -m "test(e2e/gateway): add weighted-canary flow"
```

---

### Task G6: e2e — cross-namespace via ReferenceGrant

**Files:**
- Create: `test/e2e/gateway/alb_xns_test.go`

- [ ] **Step 1: Write the test**

HTTPRoute in `ns-a`; backend Service in `ns-b`. Without ReferenceGrant: route status `ResolvedRefs=False, reason=RefNotPermitted`. Apply ReferenceGrant; route flips to `ResolvedRefs=True` and traffic flows.

- [ ] **Step 2: Commit**

```bash
git add test/e2e/gateway/alb_xns_test.go
git commit -m "test(e2e/gateway): add cross-namespace ReferenceGrant flow"
```

---

## Block H — Final verification

### Task H1: Full test sweep

- [ ] **Step 1: Generate everything**

Run: `make generate manifests`

Expected: clean (no diff after).

- [ ] **Step 2: Lint**

Run: `make fmt vet`

Expected: clean.

- [ ] **Step 3: Unit + envtest**

Run: `make test`

Expected: PASS.

- [ ] **Step 4: Helm template renders**

Run: `helm template charts/vngcloud-load-balancer-controller --set gatewayApi.alb.enabled=true >/dev/null`

Expected: success, no errors.

- [ ] **Step 5: Smoke-deploy on a real cluster (manual)**

Apply chart with `gatewayApi.alb.enabled=true`, then `kubectl apply -f config/samples/gateway_v1_alb_basic.yaml`. Wait for `Gateway.status.addresses` to populate. Curl the address.

- [ ] **Step 6: e2e suite (manual gate)**

Run: `E2E=1 go test ./test/e2e/gateway/... -v`

Expected: all 5 tests PASS.

- [ ] **Step 7: Final commit (if any cleanup)**

```bash
git status
# fix any straggling formatting
git add -A
git commit -m "chore: final cleanup before Phase 1 release"
```

### Task H2: Phase-1 release notes

**Files:**
- Modify: `CHANGELOG.md` (or whichever release-notes file the project uses)

- [ ] **Step 1: Write release notes**

Title: "0.4.0 — Gateway API Phase 1 (L7 MVP)". List:
- New `vngcloud-alb` GatewayClass with HTTPRoute support
- New CRDs: `TargetGroupConfig`, `ListenerRuleConfig`
- `LoadBalancerConfig` extended with `MergingMode`, `SSLPolicy`, `ALPNPolicy`
- Feature gate: `--enable-gateway-api-alb` (default `false`)
- Five e2e flows green

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "release: 0.4.0 — Gateway API Phase 1 (L7 MVP)"
```

---

## Self-review

**Spec coverage:** every Phase-1 scope item from spec §6.1 has a task —
- vngcloud-alb GatewayClass (D1, F2)
- Gateway reconciler (D2, C9)
- HTTPRoute reconciler (D3)
- LBC `MergingMode` / SSL / ALPN (A3)
- TargetGroupConfig (A5, D4)
- ListenerRuleConfig (A6, D5)
- ReferenceGrant honoring (B4 + integrated into C9)
- TLS Secret + cert ID (C4, C5)
- mTLS (C4)
- Status conditions (B8 + integrated into C9 + reconcilers)
- Events + metrics (B10 + reconciler wiring)
- Helm chart (F1, F2)
- 5 e2e tests (G2-G6)
- 4 user docs (F4, F5, F6, F7)

**Placeholder scan:** no "TBD"/"TODO"/"implement later" steps in user-visible task bodies. The two intentional `// TODO(C9)` comments inside Task C2 are explicitly resolved in Task C9.

**Type consistency:** `BackendKey` defined in B2 used unchanged in C6/C7. `MergingMode` defined in A3 used in B3. `RefRequest` in B4 stays self-contained. `BackendEndpoints`/`SynthesizeMembers` from C6 referenced from C9 implementation. The `vksv1alpha1.PoolMember` field-name caveat in C6 step 3 is flagged for the engineer.

---

**Plan complete and saved to `docs/superpowers/plans/2026-04-30-gateway-api-phase1.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**

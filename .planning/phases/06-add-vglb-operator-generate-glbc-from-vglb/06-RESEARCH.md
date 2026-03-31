# Phase 6: add vglb operator, generate glbc from vglb - Research

**Researched:** 2026-03-16
**Domain:** Go / controller-runtime / VGLB→GLBC generation, Service+Node watches
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**GLBC Generation Logic:**
- VGLB finds a Service with the same name and namespace
- Uses EndpointResolver for member address resolution
- Pool members use Node IP + NodePort (not pod IPs)
- One listener per Service port, one pool per listener (1:1 mapping)
- Pool protocol derived from Service port protocol (TCP/UDP)
- One pool member group per pool, named `{region}-{vpcId}`
- Pool member type is always PRIVATE
- Health monitor protocol is TCP by default
- Region from node label `vks.vngcloud.vn/mgmt-zone`, strip zone suffix (hcm03b → hcm)
- VPC ID from node label `vks.vngcloud.vn/network-id`
- Subnet ID from node label `vks.vngcloud.vn/subnet-id`
- GLBC uses generateName prefix (not deterministic name)
- Updates existing GLBC in-place when Service changes (find by owner labels, patch spec)

**VGLB Spec Design:**
- VngcloudGlobalLoadBalancerSpec remains empty — all config via annotations
- VGLB watches Services (all cluster-wide) and Nodes for changes
- Service watch only (no EndpointSlice watch) — endpoints resolved at reconcile time
- Matches target Service by same name + namespace
- If matching Service doesn't exist: requeue until it appears
- If Service type is ClusterIP (no NodePort): reject, requeue

**Naming Conventions:**
- GLB name (VNG Cloud portal): `vks_{namespace}_{vglb_name}` (or annotation override via `vks.vngcloud.vn/load-balancer-name`)
- Pool names: `pool-{port}-{protocol}` (e.g., pool-80-tcp, pool-443-tcp)
- Listener names: `listener-{port}` (e.g., listener-80, listener-443)
- Pool member group name: `{region}-{vpcId}` (e.g., hcm-net-86b7c84a)
- GLBC K8s name: generateName prefix from VGLB name

**Status Propagation:**
- VGLB status contains Address field only (minimal)
- Address comes from GLBC domains only (not VIPs)
- Status update follows same pattern as Service/LBC controllers
- Keep last known Address on VGLB delete (don't clear)

**Multi-GLBC Ownership:**
- Always one GLBC per VGLB (1:1 relationship)
- Multiple VGLBs can share the same load balancer via LoadBalancerId annotation
- On VGLB delete: always delete its GLBC (GLBC controller handles partial LB cleanup for shared LBs)
- VGLB adopts and updates existing GLBCs with matching owner labels

**GLBC Update Strategy:**
- Full spec replace on each reconcile (rebuild entire GLBC spec from Service state)
- Skip update if GLBC spec already matches desired state (avoid unnecessary reconcile triggers)

**Error Handling:**
- Service not found: requeue with backoff
- Service has no ports or no endpoints: requeue with backoff
- Service type is ClusterIP: reject, requeue

**Annotations (Core Set):**
- `vks.vngcloud.vn/load-balancer-id` — use existing LB by ID
- `vks.vngcloud.vn/load-balancer-name` — override GLB display name
- `vks.vngcloud.vn/package-id` — specify LB package
- `vks.vngcloud.vn/description` — LB description

**Concurrency:**
- No special locking at VGLB level — each VGLB manages its own GLBC independently

**Init Behavior:**
- Resolve network info from node labels at startup (region, VPC, subnet)

**Testing:**
- Unit tests for build_glbc.go: pool/listener generation, naming, member resolution
- Integration tests with envtest: VGLB → GLBC → mock backend flow

### Claude's Discretion
- Exact diff comparison logic for skip-if-equal optimization
- EndpointResolver integration details
- Error message formatting
- Requeue backoff durations

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

---

## Summary

Phase 6 extends the existing VGLB operator to fully generate GLBC resources from a matching Kubernetes Service. The code already has substantial scaffolding: `build_glbc.go`, `build_global_pool.go`, and `build_global_listener.go` all exist with working logic. The core gap is that the existing `InitVngcloudGlobalLoadBalancerUseCase` fetches network info from the VNG Cloud API (`GetServerNetworkInfo`), whereas the decided approach is to read it from node labels at startup. Additionally, the pool member group naming uses a hardcoded `"hcm"` region and `"default"` group name today — both must be computed from node labels.

The controller also needs Service and Node watches added to `SetupWithManager`. The existing `vglb_controller.go` only watches `VngcloudGlobalLoadBalancer` objects. The reference implementation is `internal/controller/core/service_controller.go`, which wires Service+Node event handlers inside `SetupWithManager`.

The `buildLoadBalancerName` function currently generates `glb_` prefix but CONTEXT.md mandates `vks_` prefix. This is a concrete bug to fix.

**Primary recommendation:** Treat this phase as a surgical correction phase — most code exists, fix the three concrete gaps: (1) init reads node labels not VNG Cloud API, (2) pool member group named `{region}-{vpcId}` not `"default"`, (3) SetupWithManager adds Service + Node watches. Then add tests.

## Standard Stack

### Core (already in go.mod — no new dependencies)
| Library | Purpose | Notes |
|---------|---------|-------|
| `sigs.k8s.io/controller-runtime` | Controller framework, event handlers, watches | Already used |
| `github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1` | GLB types: `GlobalPoolMemberTypePrivate`, `GlobalPoolProtocolTCP`, etc. | Already imported |
| `k8s.io/api/core/v1` | Service, Node, NodeList types | Already imported |
| `github.com/anngdinh/operator-helper/contexts` | Structured logging context | Already used throughout |
| `github.com/pkg/errors` | Error wrapping | Already used |
| `k8s.io/utils/ptr` | `ptr.To()` for pointer construction | Already used in build_global_pool.go |

### Testing (already present)
| Library | Purpose | Notes |
|---------|---------|-------|
| Ginkgo v2 + Gomega | Integration test framework | Used in glbc_controller tests |
| `sigs.k8s.io/controller-runtime/pkg/envtest` | In-process k8s API server for integration tests | Used in suite_test.go |
| `github.com/stretchr/testify/assert` | Unit test assertions | Used in build_glbc_test.go, build_global_listener_test.go |
| `vngcloud_mocks.MockProvider` | VNG Cloud API mock | Existing, fully implemented for GLB operations |

**Installation:** No new packages needed.

## Architecture Patterns

### Existing Project Structure (relevant paths)
```
internal/
├── controller/
│   └── vglb_controller/
│       ├── vglb_controller.go           # Reconciler — needs Service+Node watches
│       └── eventhandlers/
│           └── vglb_events.go           # VGLB object event handler (exists)
├── usecase/
│   └── vglb_uc/
│       ├── vglb_uc.go                   # Init/Ensure/Delete flows
│       ├── build_glbc.go                # GLBC spec construction
│       ├── build_global_pool.go         # Pool + member building
│       └── build_global_listener.go     # Listener building
└── repository/
    └── k8s_repo/k8s_repo.go             # K8s API calls
api/v1alpha1/
├── vngcloudgloballoadbalancer_types.go  # VGLB CRD (spec stays empty)
└── globalloadbalancerconfig_types.go    # GLBC CRD (full spec)
```

### Pattern 1: Controller Watch Wiring (Service + Node)
**What:** Add Service and Node watches to VGLB controller's `SetupWithManager`
**When to use:** Any controller that must react to external resource changes

**Reference from `internal/controller/core/service_controller.go`:**
```go
// In SetupWithManager, after existing vglbEventHandler registration:
nodeEventHandler := eventhandlers.NewEnqueueRequestForVglbNodeEvent(
    r.k8sClient,
    r.vglbUtils,
    r.logger.WithName("eventHandlers").WithName("node"),
)
svcEventHandler := eventhandlers.NewEnqueueRequestForVglbServiceEvent(
    r.k8sClient,
    r.vglbUtils,
    r.logger.WithName("eventHandlers").WithName("service"),
)

return ctrl.NewControllerManagedBy(mgr).
    Watches(&v1alpha1.VngcloudGlobalLoadBalancer{}, vglbEventHandler).
    Watches(&corev1.Service{}, svcEventHandler).     // NEW
    Watches(&corev1.Node{}, nodeEventHandler).        // NEW
    Named("vngcloudgloballoadbalancer").
    WithOptions(controller.Options{
        MaxConcurrentReconciles: r.maxConcurrentReconciles,
    }).
    Complete(r)
```

**Node event handler contract:** On any Node create/update/delete, enqueue all VGLBs (list all `VngcloudGlobalLoadBalancer` objects, enqueue each that passes `IsPendingFinalization || IsSupported`). Mirror `internal/controller/core/eventhandlers/node_events.go` but list VGLBs instead of Services.

**Service event handler contract:** On Service create/update, find VGLBs with same name+namespace and enqueue them. On delete, do the same (Service disappearance triggers VGLB reconcile which will requeue waiting for Service).

### Pattern 2: Node Label Extraction for Region/VPC/Subnet
**What:** Read network info from node labels instead of VNG Cloud API
**Node labels:**
- `vks.vngcloud.vn/mgmt-zone` → raw zone value (e.g., `hcm03b`) → strip digit suffix → region (e.g., `hcm`)
- `vks.vngcloud.vn/network-id` → VPC ID (e.g., `net-86b7c84a-...`)
- `vks.vngcloud.vn/subnet-id` → subnet ID

**Region stripping logic (Claude's discretion — suggested approach):**
```go
// Strip trailing digits and letters that form zone suffix
// hcm03b -> hcm, hcm03 -> hcm, sgn01a -> sgn
func stripZoneSuffix(zone string) string {
    // Find last run of digits+letters at end that looks like a zone qualifier
    // Simple approach: strip trailing [0-9a-z]+ that starts with a digit
    i := len(zone)
    for i > 0 && (zone[i-1] >= '0' && zone[i-1] <= '9' || zone[i-1] >= 'a' && zone[i-1] <= 'z') {
        i--
    }
    // Back up to first digit in this trailing run
    // hcm03b: find '0' at index 3
    // Actually: strip everything from the first digit at end
    return stripTrailingZoneCode(zone)
}
```

**Simpler approach:** Find first digit from the right sequence — `strings.TrimRightFunc` with a digit+letter set starting at the digit boundary. The concrete regex `regexp.MustCompile(`\d+[a-z]*$`)` matches zone suffix reliably.

**Where to apply:** Replace the `InitVngcloudGlobalLoadBalancerUseCase` body. The existing implementation calls `uc.vngcloudRepo.GetServerNetworkInfo(...)` — replace with node label reads.

### Pattern 3: Pool Member Group Naming
**What:** Build group name as `{region}-{vpcId}` from the first node's labels
**Current code (bug):** `build_global_pool.go` line 128 hardcodes `Name: "default"` and `Region: "hcm"`
**Fix:** Use `t.defaultNetworkId` (already present as field) and computed region:

```go
// In buildPool, replace hardcoded values:
poolMembers = append(poolMembers, v1alpha1.GlobalPoolMember{
    Name:    fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId),
    Region:  t.defaultRegion,
    VpcId:   t.defaultNetworkId,
    Type:    global.GlobalPoolMemberTypePrivate,
    Members: globalMembers,
})
```

Where `t.defaultRegion` is a new field on `defaultModelBuildTask` (alongside the existing `defaultZone`, `defaultNetworkId`, `defaultSubnetId`).

### Pattern 4: GLB Display Name Fix
**What:** CONTEXT.md mandates `vks_{namespace}_{name}`, existing code generates `glb_{namespace}_{name}`
**Location:** `build_glbc.go` line 175
**Fix:**
```go
// Change: name := "glb_" + t.vglb.Namespace + "_" + t.vglb.Name
name := "vks_" + t.vglb.Namespace + "_" + t.vglb.Name
```

Note: existing test `TestBuildLoadBalancerName` expects `"glb_default_my_vglb"` — tests must be updated to expect `"vks_default_my_vglb"`.

### Pattern 5: Service Not Found → Requeue (not continue)
**What:** Current `buildGlobalLoadBalancerConfig` treats service-not-found as soft error — sets `svc = nil` and continues building a GLBC without pools. CONTEXT.md says "If matching Service doesn't exist: requeue until it appears."
**Fix:** Return a requeue error when `GetService` returns NotFound:
```go
svc, err := t.k8sRepo.GetService(ctx, types.NamespacedName{...})
if err != nil {
    if client.IsNotFound(err) {
        return errs.NewRequeueNeededAfter("service not found, waiting", 5*time.Second)
    }
    return err
}
```

### Pattern 6: ClusterIP Rejection
**What:** Service type ClusterIP has no NodePort — must reject
**Current code:** `getTargetType` returns `TargetTypeIP` for ClusterIP (continues without error)
**Fix:** In `buildGlobalLoadBalancerConfig`, after fetching the Service:
```go
if svc.Spec.Type == corev1.ServiceTypeClusterIP {
    return errs.NewRequeueNeededAfter("service type is ClusterIP, no NodePort available", 30*time.Second)
}
```

### Pattern 7: Integration Test Structure (mirror glbc_controller)
**Reference:** `internal/controller/glbc_controller/suite_test.go` — use identical envtest bootstrap pattern
**New files needed:**
- `internal/controller/vglb_controller/suite_test.go` — envtest setup with VGLB+GLBC reconcilers
- `internal/controller/vglb_controller/helpers_test.go` — fixture builders (fake Service, fake Node, VGLB resource)
- `internal/controller/vglb_controller/vglb_controller_test.go` — Ginkgo specs

The integration test must bootstrap BOTH the VGLB reconciler AND the GLBC reconciler (since VGLB creates GLBC objects that the GLBC controller then reconciles against the mock backend). This is the key difference from a unit test.

### Anti-Patterns to Avoid
- **Returning nil when Service not found:** Current behavior (continue without pools) violates the locked decision. Must requeue.
- **Using hardcoded `"hcm"` region or `"default"` group name:** Already present in `build_global_pool.go` line 129-130 — must be replaced.
- **Calling `GetServerNetworkInfo` during init:** The VNG Cloud API approach is replaced by node label reads.
- **Registering only VGLB watches:** The controller currently has no Service or Node watches — reconciliation will not trigger when Service ports change.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Spec equality check | Custom field-by-field comparison | `reflect.DeepEqual` (already in `glbcSpecEqual`) | Already correct; DeepEqual handles nested structs |
| Node label reading | Custom label lookup | `node.Labels[labelKey]` with map access | Standard Go map access; no helper needed |
| Region suffix stripping | Complex parser | `regexp.MustCompile(`\d+[a-z]*$`).ReplaceAllString(zone, "")` | One-liner; test-covered |
| Error + requeue | Custom error type | `errs.NewRequeueNeededAfter(msg, duration)` | Already defined in `pkg/errs` |
| List VGLBs in event handler | Direct API call | `k8sClient.List(ctx, &v1alpha1.VngcloudGlobalLoadBalancerList{})` | Standard controller-runtime pattern |
| Status patching | Manual patch construction | `k8sRepo.PatchMutateStatusVngcloudGlobalLoadBalancer` | Already implemented and tested |

**Key insight:** Nearly all plumbing exists. The phase is about fixing 5 concrete gaps, not building a new system.

## Common Pitfalls

### Pitfall 1: Node label keys not matching
**What goes wrong:** Node labels use `vks.vngcloud.vn/mgmt-zone` (per CONTEXT.md) but actual cluster nodes may use different label names. If the label is absent, `node.Labels[key]` returns `""` and region/VPC extraction silently produces wrong names.
**Why it happens:** Label names can vary by VKS version or cluster config.
**How to avoid:** In `InitVngcloudGlobalLoadBalancerUseCase`, explicitly check for empty string and return an error if network info labels are absent. Log the labels actually found.
**Warning signs:** Pool member group named `-` or `hcm-` (empty VPC).

### Pitfall 2: Zone suffix stripping produces wrong region
**What goes wrong:** `hcm03b` → strip wrong suffix → `hcm0` instead of `hcm`.
**Why it happens:** Naive `strings.TrimRight` with digit+letter set strips too much or too little.
**How to avoid:** Use `regexp.MustCompile(`\d+[a-z]*$`)` — matches a run of digits optionally followed by letters at the end. Unit-test with `hcm03b`, `hcm03`, `sgn01a`, `han01`.
**Warning signs:** Region field in pool member group name doesn't match expected VNG Cloud region code.

### Pitfall 3: Integration test missing GLBC reconciler
**What goes wrong:** VGLB creates a GLBC object, but without the GLBC controller running in envtest, the GLBC's status never gets populated. Test waits for VGLB address and times out.
**Why it happens:** VGLB reads GLBC status for the address field. If GLBC controller isn't registered in the test manager, the status stays empty.
**How to avoid:** Register both VGLB and GLBC reconcilers in `suite_test.go` — mirror how `glbc_controller/suite_test.go` registers its reconciler.
**Warning signs:** Test eventually-block for VGLB address always times out.

### Pitfall 4: Service and Node watches enqueue all VGLBs on every Node change
**What goes wrong:** High-churn Node events (Node status heartbeats every 5s) trigger reconciliation for every VGLB in the cluster.
**Why it happens:** The `enqueueAllVglbs` pattern from the Node event handler is correct but potentially noisy.
**How to avoid:** In the Node event handler, filter out heartbeat-only updates: only trigger reconcile when labels, spec, addresses, or Ready condition changes (mirror `node_events.go` pattern for `Update`). The existing `equality.Semantic.DeepEqual` guards in `node_events.go` are the reference.
**Warning signs:** Excessive reconciliation loops visible in controller logs.

### Pitfall 5: generateName GLBC name not stable across tests
**What goes wrong:** Integration tests that try to `Get` a GLBC by name fail because the name is generated (`vglb-xyz-<random>`).
**Why it happens:** `generateName` produces non-deterministic names.
**How to avoid:** In tests, list GLBCs by owner labels (same pattern used in `buildGlobalLoadBalancerConfig`) rather than getting by name. The owner labels `LabelOwnerResourceName`, `LabelOwnerResourceKind`, `LabelOwnerResourceUid` are deterministic.
**Warning signs:** `k8sClient.Get(ctx, client.ObjectKey{Name: "my-vglb-..."}, glbc)` fails intermittently.

### Pitfall 6: `buildLoadBalancerName` test mismatch after `glb_` → `vks_` fix
**What goes wrong:** Existing `TestBuildLoadBalancerName` in `build_glbc_test.go` asserts `"glb_default_my_vglb"`. After the prefix fix, the test will fail.
**Why it happens:** Tests were written against old prefix.
**How to avoid:** Update all 4 test cases in `TestBuildLoadBalancerName` when fixing the prefix.
**Warning signs:** `go test ./internal/usecase/vglb_uc/...` fails after the name fix.

## Code Examples

### Region Extraction from Node Label

```go
// Source: design decision in CONTEXT.md; pattern is standard regexp usage
import "regexp"

var zoneRe = regexp.MustCompile(`\d+[a-z]*$`)

// stripZoneSuffix converts "hcm03b" -> "hcm", "sgn01a" -> "sgn", "han01" -> "han"
func stripZoneSuffix(zone string) string {
    return zoneRe.ReplaceAllString(zone, "")
}
```

### Network Info from Node Labels in Init

```go
// Replace GetServerNetworkInfo call in InitVngcloudGlobalLoadBalancerUseCase:
const (
    labelMgmtZone = "vks.vngcloud.vn/mgmt-zone"
    labelNetworkId = "vks.vngcloud.vn/network-id"
    labelSubnetId  = "vks.vngcloud.vn/subnet-id"
)

nodes := &corev1.NodeList{}
if err := uc.k8sRepo.ListNode(ctx, nodes); err != nil {
    return err
}
if len(nodes.Items) == 0 {
    return errors.New("no nodes found in cluster")
}
firstNode := &nodes.Items[0]
rawZone := firstNode.Labels[labelMgmtZone]
uc.defaultRegion = stripZoneSuffix(rawZone)         // "hcm"
uc.defaultNetworkId = firstNode.Labels[labelNetworkId] // "net-86b7c84a-..."
uc.defaultSubnetId = firstNode.Labels[labelSubnetId]  // "sub-..."
if uc.defaultRegion == "" || uc.defaultNetworkId == "" || uc.defaultSubnetId == "" {
    return fmt.Errorf("missing network info in node labels: zone=%q networkId=%q subnetId=%q",
        rawZone, uc.defaultNetworkId, uc.defaultSubnetId)
}
```

### VGLB Node Event Handler

```go
// New file: internal/controller/vglb_controller/eventhandlers/node_events.go
// Mirror of internal/controller/core/eventhandlers/node_events.go
// but enqueues VGLBs instead of Services

func (h *enqueueRequestsForVglbNodeEvent) enqueueAllVglbs(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
    vglbList := &v1alpha1.VngcloudGlobalLoadBalancerList{}
    if err := h.k8sClient.List(ctx, vglbList); err != nil {
        h.logger.Error(err, "failed to list VGLBs for node event")
        return
    }
    for _, vglb := range vglbList.Items {
        if !h.vglbUtils.IsPendingFinalization(&vglb) && !h.vglbUtils.IsSupported(&vglb) {
            continue
        }
        queue.Add(reconcile.Request{
            NamespacedName: client.ObjectKeyFromObject(&vglb),
        })
    }
}
```

### VGLB Service Event Handler

```go
// New file: internal/controller/vglb_controller/eventhandlers/service_events.go
// On Service create/update/delete: find VGLB with same name+namespace and enqueue

func (h *enqueueRequestsForVglbServiceEvent) enqueueSameNameVglb(
    ctx context.Context,
    queue workqueue.TypedRateLimitingInterface[reconcile.Request],
    svc *corev1.Service,
) {
    vglb := &v1alpha1.VngcloudGlobalLoadBalancer{}
    err := h.k8sClient.Get(ctx, types.NamespacedName{
        Namespace: svc.Namespace,
        Name:      svc.Name,
    }, vglb)
    if err != nil {
        // VGLB may not exist for this service — that's normal, ignore
        return
    }
    if !h.vglbUtils.IsPendingFinalization(vglb) && !h.vglbUtils.IsSupported(vglb) {
        return
    }
    queue.Add(reconcile.Request{
        NamespacedName: client.ObjectKeyFromObject(vglb),
    })
}
```

### Pool Member Group Name (fixed)

```go
// In buildPool, replace hardcoded group name:
groupName := fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId)
if len(globalMembers) > 0 {
    poolMembers = append(poolMembers, v1alpha1.GlobalPoolMember{
        Name:    groupName,
        Region:  t.defaultRegion,
        VpcId:   t.defaultNetworkId,
        Type:    global.GlobalPoolMemberTypePrivate,
        Members: globalMembers,
    })
}
```

### Integration Test Fixture: VGLB + Service + Nodes

```go
// In helpers_test.go for vglb_controller:
func newVGLBResource(name, namespace string) *v1alpha1.VngcloudGlobalLoadBalancer {
    return &v1alpha1.VngcloudGlobalLoadBalancer{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
        Spec:       v1alpha1.VngcloudGlobalLoadBalancerSpec{},
    }
}

func newNodePortService(name, namespace string, port int32, nodePort int32) *corev1.Service {
    return &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
        Spec: corev1.ServiceSpec{
            Type: corev1.ServiceTypeNodePort,
            Ports: []corev1.ServicePort{{
                Port:     port,
                NodePort: nodePort,
                Protocol: corev1.ProtocolTCP,
            }},
        },
    }
}

func newNodeWithLabels(name string) *corev1.Node {
    return &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: name,
            Labels: map[string]string{
                "vks.vngcloud.vn/mgmt-zone":  "hcm03b",
                "vks.vngcloud.vn/network-id": "net-test-vpc",
                "vks.vngcloud.vn/subnet-id":  "sub-test-subnet",
            },
        },
        Status: corev1.NodeStatus{
            Addresses: []corev1.NodeAddress{
                {Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
            },
        },
    }
}
```

## State of the Art

| Old Approach (existing code) | Required Approach (CONTEXT.md) | Impact |
|------------------------------|-------------------------------|--------|
| `GetServerNetworkInfo` VNG Cloud API call | Node label reads at init | Simpler, no external API dependency during init |
| Pool member group: `Name: "default"`, `Region: "hcm"` (hardcoded) | `Name: "{region}-{vpcId}"`, `Region: stripZoneSuffix(label)` | Correct multi-region naming |
| GLB display name: `glb_{ns}_{name}` | `vks_{ns}_{name}` | Matches portal conventions |
| Service-not-found: continue with empty pools | Service-not-found: requeue with backoff | Correct behavior per spec |
| No Service/Node watches in SetupWithManager | Add Service + Node watches | Controller reacts to cluster changes |

**Deprecated behaviors in existing code:**
- `"default"` as pool member group name: replace with `{region}-{vpcId}`
- `"hcm"` as hardcoded region: replace with label-derived value
- `glb_` prefix in default GLB name: replace with `vks_`
- `GetServerNetworkInfo` in init: replace with node label reads

## Open Questions

1. **`defaultZone` vs `defaultRegion` field naming**
   - What we know: `vglbUseCase` has `defaultZone common.Zone` today (set by `GetServerNetworkInfo`). CONTEXT.md says region is derived from node label, not from the zone field.
   - What's unclear: Should we rename `defaultZone` to `defaultRegion string`, or add a separate `defaultRegion` field and leave `defaultZone` for backward compat?
   - Recommendation: Add `defaultRegion string` as a new field in `vglbUseCase` and `defaultModelBuildTask`. Keep `defaultZone` unused or remove it if nothing else reads it.

2. **VPC ID format: full ID or short ID**
   - What we know: CONTEXT.md example: `hcm-net-86b7c84a`. This implies the pool group name uses a short segment of the VPC ID.
   - What's unclear: Is the full label value `net-86b7c84a-...` (long form) or is it already the short form? The example `net-86b7c84a` suggests the label may already be in short form, or we need to extract first 8 chars after `net-`.
   - Recommendation: Use the full label value from `vks.vngcloud.vn/network-id` directly as the VpcId field (it's the VPC ID for the API); for the group Name, use the full label value too (e.g., `hcm-net-86b7c84a-abc123`). Verify against actual node labels in the cluster if available.

3. **Service event handler: enqueue by name lookup or always re-list all VGLBs**
   - What we know: For Service events, only the VGLB with the same name+namespace is affected.
   - What's unclear: Whether a name-based `Get` on VGLB or listing all VGLBs with a label filter is preferred.
   - Recommendation: Use targeted `k8sClient.Get` by same name+namespace — O(1) vs O(n) list, and avoids unnecessary reconciles for other VGLBs.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (unit) + Ginkgo v2 + Gomega (integration) |
| Config file | none — `go test ./...` discovers all test files |
| Quick run command | `go test ./internal/usecase/vglb_uc/... -v -count=1` |
| Full suite command | `go test ./internal/... -v -count=1` |

### Phase Requirements → Test Map

This phase has no formal requirement IDs in REQUIREMENTS.md (phase 6 is beyond v1 scope). The behaviors to test are derived from locked decisions:

| Behavior ID | Behavior | Test Type | Automated Command | Test File |
|-------------|----------|-----------|-------------------|-----------|
| VGLB-01 | GLB name uses `vks_` prefix | unit | `go test ./internal/usecase/vglb_uc/... -run TestBuildLoadBalancerName` | `build_glbc_test.go` (update existing) |
| VGLB-02 | Pool member group name is `{region}-{vpcId}` | unit | `go test ./internal/usecase/vglb_uc/... -run TestBuildPool` | `build_global_pool_test.go` (new) |
| VGLB-03 | Region strip: `hcm03b` → `hcm` | unit | `go test ./internal/usecase/vglb_uc/... -run TestStripZoneSuffix` | `vglb_uc_test.go` (new) |
| VGLB-04 | Service not found → requeue | unit | `go test ./internal/usecase/vglb_uc/... -run TestBuildGlobalLoadBalancerConfig_ServiceNotFound` | `build_glbc_test.go` (new) |
| VGLB-05 | ClusterIP service → requeue | unit | `go test ./internal/usecase/vglb_uc/... -run TestBuildGlobalLoadBalancerConfig_ClusterIPRejected` | `build_glbc_test.go` (new) |
| VGLB-06 | VGLB → GLBC create flow (envtest) | integration | `go test ./internal/controller/vglb_controller/... -v` | `vglb_controller_test.go` (new) |
| VGLB-07 | VGLB delete → GLBC deleted (envtest) | integration | `go test ./internal/controller/vglb_controller/... -v` | `vglb_controller_test.go` (new) |
| VGLB-08 | Service port change → GLBC spec updated | integration | `go test ./internal/controller/vglb_controller/... -v` | `vglb_controller_test.go` (new) |

### Sampling Rate
- **Per task commit:** `go test ./internal/usecase/vglb_uc/... -count=1`
- **Per wave merge:** `go test ./internal/... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/usecase/vglb_uc/build_global_pool_test.go` — covers VGLB-02 (pool member group naming)
- [ ] `internal/usecase/vglb_uc/vglb_uc_test.go` — covers VGLB-03 (zone suffix stripping)
- [ ] `internal/controller/vglb_controller/suite_test.go` — envtest bootstrap
- [ ] `internal/controller/vglb_controller/helpers_test.go` — fixture builders
- [ ] `internal/controller/vglb_controller/vglb_controller_test.go` — Ginkgo specs (VGLB-06, 07, 08)
- [ ] `internal/controller/vglb_controller/eventhandlers/node_events.go` — new node event handler
- [ ] `internal/controller/vglb_controller/eventhandlers/service_events.go` — new service event handler

## Sources

### Primary (HIGH confidence)
- Direct source read: `internal/usecase/vglb_uc/build_glbc.go` — current GLBC generation code
- Direct source read: `internal/usecase/vglb_uc/build_global_pool.go` — pool building, hardcoded region bug
- Direct source read: `internal/usecase/vglb_uc/vglb_uc.go` — init/ensure/delete flows
- Direct source read: `internal/controller/vglb_controller/vglb_controller.go` — reconciler, SetupWithManager
- Direct source read: `internal/controller/core/eventhandlers/node_events.go` — Node watch reference
- Direct source read: `internal/controller/core/eventhandlers/service_events.go` — Service watch reference
- Direct source read: `internal/controller/core/service_controller.go` — SetupWithManager multi-watch reference
- Direct source read: `internal/controller/glbc_controller/suite_test.go` — envtest bootstrap reference
- Direct source read: `api/v1alpha1/vngcloudgloballoadbalancer_types.go` — VGLB CRD types
- Direct source read: `api/v1alpha1/globalloadbalancerconfig_types.go` — GLBC CRD types
- Direct source read: `internal/domain/domain.go` — labels, finalizers, constants
- Direct source read: `internal/usecase/vglb_uc/build_glbc_test.go` — existing unit tests to update

### Secondary (MEDIUM confidence)
- CONTEXT.md — user decisions document (primary constraint source)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all code read directly from source
- Architecture patterns: HIGH — reference implementations exist in the same repo
- Pitfalls: HIGH — bugs confirmed by reading the actual source code
- Test map: HIGH — test framework confirmed from existing test files

**Research date:** 2026-03-16
**Valid until:** 2026-04-16 (stable domain; internal codebase)

# Phase 07: Global Load Balancer Operator with Service Annotations - Research

**Researched:** 2026-03-17
**Domain:** Kubernetes controller-runtime, Service annotation-driven GLBC generation
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Controller Architecture**
- New dedicated controller following `core/service_controller` pattern (NOT vglb_controller pattern)
- Creates GLBC resources (not direct API calls) — existing GLBC controller handles reconciliation to VNG Cloud
- Watches Services + Nodes (same as VGLB for node label changes affecting pool members)
- Dedicated usecase package: `internal/usecase/service_glb_uc/`
- Reuse existing EndpointResolver for pool member address resolution
- Init reads node labels at startup (same as VGLB: region, VPC, subnet from node labels)
- Finalizer on Service (`glb.vks.vngcloud.vn/resources`) for cleanup on delete
- GLBC naming: `generateName: {service-name}-` prefix (same as Service controller for LBC)
- GLB display name in VNG Cloud: `vks_{namespace}_{service_name}` (same pattern as VGLB, overridable via annotation)
- Full spec rebuild on each reconcile, skip update if spec matches (same as Service controller)
- Update Service.Status.LoadBalancer.Ingress with GLB domains/VIPs
- GLBC created in same namespace as Service
- One pool + one listener per Service port (1:1 mapping, same as VGLB)

**Service Type Support**
- NodePort and LoadBalancer: support all target types (instance mode default)
- ClusterIP: must use target-type=ip (pod IPs), same as Service controller
- Support `glb.vks.vngcloud.vn/target-type` annotation with values 'instance' and 'ip'

**Annotation Design**
- Prefix: `glb.vks.vngcloud.vn`
- Trigger: `glb.vks.vngcloud.vn/enable=true` (boolean, explicit opt-in)
- Full annotation set matching Service controller capabilities
- Same suffix values as existing vks.vngcloud.vn annotations but NEW constants defined for GLB suffixes
- Parser: reuse `annotations.NewSuffixAnnotationParser("glb.vks.vngcloud.vn")`

**Service Filtering & Lifecycle**
- Watch all namespaces cluster-wide, filter by annotation in predicate
- Predicate filter: event handler checks for `glb.vks.vngcloud.vn/enable` annotation before enqueuing (IsServiceGLBSupported pattern)
- Annotation removal detection: check old annotations in Update event — if OLD had annotation but NEW doesn't, enqueue for cleanup
- Removing `enable` annotation triggers GLBC deletion (annotation on = create, off = delete)

**Relationship with VGLB**
- Fully independent of VGLB controller — zero coupling
- Both can coexist on same Service (each creates its own GLBC independently)
- Owner labels: `owner-resource-kind=Service`, `owner-resource-name={svc-name}`, `owner-resource-uid={svc-uid}`

**Code Consistency**
- Follow Service controller code structure: build_glbc.go, build_pool.go, build_listener.go
- Dedicated utils package: `pkg/service_glb/service_glb_utils.go` with IsServiceGLBSupported and IsServiceGLBPendingFinalization
- Annotation constants: new constants in `pkg/annotations/constants.go` for GLB suffixes

**Testing**
- Unit tests for GLBC building logic (pools, listeners, member resolution)
- Integration tests with envtest: Service annotation → GLBC creation/update/delete flows

### Claude's Discretion
- Exact diff comparison logic for skip-if-equal optimization
- Error message formatting
- Requeue backoff durations
- Pool member group naming (carry forward {region}-{vpcId} pattern from VGLB)
- Health monitor defaults

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope
</user_constraints>

---

## Summary

Phase 7 creates a new `ServiceGLBReconciler` that watches Kubernetes Services annotated with `glb.vks.vngcloud.vn/enable=true` and generates `GlobalLoadBalancerConfig` (GLBC) CRs. The GLBC controller from Phase 6 already handles reconciliation of GLBC resources to the VNG Cloud API — this controller is purely responsible for the Service-to-GLBC translation layer. The architecture directly mirrors the existing `core/service_controller` pattern: an event-filtering handler, annotation-based predicate, `defaultModelBuildTask` pattern in the usecase, and finalizer-based cleanup.

The key difference from the VGLB controller (Phase 6) is that the trigger object is a Kubernetes Service with an annotation (not a separate CRD). The controller reconciles the Service itself, creates/patches/deletes GLBC resources via owner labels, and updates `Service.Status.LoadBalancer.Ingress` with GLB domain addresses. The annotation annotation parser uses the new prefix `glb.vks.vngcloud.vn` but reuses all existing suffix constants — no new annotation suffixes are needed, only new prefix-binding constants.

All core building logic (pools, listeners, pool member groups, healthcheck, algorithm, idle timeouts) is copied from `vglb_uc/build_glbc.go` and `vglb_uc/build_global_pool.go` but with the build task holding a `*corev1.Service` as the primary object rather than a `*v1alpha1.VngcloudGlobalLoadBalancer`.

**Primary recommendation:** Implement in 3 plans — (1) controller scaffold + utils + annotation constants, (2) usecase + build logic, (3) integration tests.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `sigs.k8s.io/controller-runtime` | existing | Reconciler, manager, event handlers | All controllers use it |
| `github.com/anngdinh/operator-helper` | existing | `contexts`, `k8s.FinalizerManager` | Project-standard helper |
| `k8s.io/api/core/v1` | existing | `corev1.Service`, `corev1.Node` | Watched resources |
| `github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1` | existing | `GlobalLoadBalancerConfig` CRD type | Output resource |
| `github.com/sirupsen/logrus` | existing | Structured logging in usecases | Project standard |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `k8s.io/utils/ptr` | existing | Pointer helpers for optional spec fields | Pool/listener optional fields |
| `reflect` | stdlib | `reflect.DeepEqual` for `glbcSpecEqual` | Skip-if-equal optimization |
| `github.com/onsi/ginkgo/v2` + `gomega` | existing | Integration tests | Test suite |
| `sigs.k8s.io/controller-runtime/pkg/envtest` | existing | Envtest for integration tests | GLBC creation flow tests |

---

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── controller/
│   └── service_glb_controller/         # new package
│       ├── service_glb_controller.go   # ServiceGLBReconciler
│       ├── service_glb_controller_test.go
│       ├── suite_test.go
│       ├── helpers_test.go
│       └── eventhandlers/
│           ├── service_events.go       # GLB-aware service event handler
│           └── node_events.go          # node event handler (enqueue all GLB-enabled services)
├── usecase/
│   ├── contracts.go                    # add ServiceGLBUseCase interface
│   └── service_glb_uc/                 # new package
│       ├── service_glb_uc.go           # usecase struct + Init/Ensure/Delete
│       ├── build_glbc.go               # GLBC spec builder
│       ├── build_pool.go               # global pool builder
│       └── build_listener.go           # global listener builder
pkg/
├── service_glb/                        # new package
│   └── service_glb_utils.go            # IsServiceGLBSupported, IsServiceGLBPendingFinalization
├── annotations/
│   └── constants.go                    # add GLB prefix constant + SuffixGLBEnable
internal/
└── domain/
    └── domain.go                       # add ServiceGLBFinalizer, GLB_ANNOTATION_PREFIX constants
cmd/
└── main.go                             # register ServiceGLBReconciler
```

### Pattern 1: Service Utils (IsServiceGLBSupported)

The existing `service_utils.go` pattern shows: a `ServiceUtils` interface with `IsServiceSupported` and `IsServicePendingFinalization`, implemented by `defaultServiceUtils` that holds a `serviceFinalizer` string and an `annotationParser`. For the GLB controller, the same interface shape applies but the predicate logic changes:

```go
// Source: pkg/service/service_utils.go (existing pattern)
// New file: pkg/service_glb/service_glb_utils.go

type ServiceGLBUtils interface {
    IsServiceGLBSupported(object *corev1.Service) bool
    IsServiceGLBPendingFinalization(object *corev1.Service) bool
}

type defaultServiceGLBUtils struct {
    serviceFinalizer string
    annotationParser annotations.Parser
}

func (u *defaultServiceGLBUtils) IsServiceGLBPendingFinalization(object *corev1.Service) bool {
    return k8s.HasFinalizer(object, u.serviceFinalizer)
}

func (u *defaultServiceGLBUtils) IsServiceGLBSupported(object *corev1.Service) bool {
    if !object.DeletionTimestamp.IsZero() {
        return false
    }
    enabled := false
    u.annotationParser.ParseBoolAnnotation(SuffixGLBEnable, &enabled, object.Annotations)
    return enabled
}
```

**Key difference from service_utils.go:** The GLB utils checks for `glb.vks.vngcloud.vn/enable=true` instead of checking `ServiceType == LoadBalancer`. All service types (NodePort, LoadBalancer, ClusterIP) can be GLB-enabled.

### Pattern 2: Service Event Handler (GLB-Aware)

The existing core `service_events.go` uses `enqueueManagedService` which calls `IsServicePendingFinalization || IsServiceSupported`. The GLB event handler is identical in structure but replaces those calls with GLB-specific predicates. Crucially, the Update handler MUST detect annotation removal:

```go
// Source: internal/controller/core/eventhandlers/service_events.go (existing pattern)
// New: internal/controller/service_glb_controller/eventhandlers/service_events.go

func (h *enqueueRequestsForServiceGLBEvent) Update(ctx context.Context, e event.UpdateEvent, queue ...) {
    oldSvc := e.ObjectOld.(*corev1.Service)
    newSvc := e.ObjectNew.(*corev1.Service)

    isSyncEvent := oldSvc.GetResourceVersion() == newSvc.GetResourceVersion()
    if !isSyncEvent &&
        equality.Semantic.DeepEqual(oldSvc.Annotations, newSvc.Annotations) &&
        equality.Semantic.DeepEqual(oldSvc.Spec, newSvc.Spec) &&
        oldSvc.DeletionTimestamp.IsZero() == newSvc.DeletionTimestamp.IsZero() {
        return
    }

    // Enqueue if new state is GLB-relevant OR if old had GLB annotation (removal triggers cleanup)
    if h.serviceGLBUtils.IsServiceGLBPendingFinalization(newSvc) ||
        h.serviceGLBUtils.IsServiceGLBSupported(newSvc) ||
        h.hadGLBAnnotation(oldSvc) {
        queue.Add(reconcile.Request{NamespacedName: types.NamespacedName{...}})
    }
}

func (h *...) hadGLBAnnotation(svc *corev1.Service) bool {
    enabled := false
    h.serviceGLBUtils.annotationParser.ParseBoolAnnotation(SuffixGLBEnable, &enabled, svc.Annotations)
    return enabled
}
```

### Pattern 3: Controller Reconcile (ServiceGLBReconciler)

The reconciler is structurally identical to `ServiceReconciler` in `core/service_controller.go`. The Reconcile → reconcile → reconcileEnsure/reconcileDelete flow is the same. The primary object is `*corev1.Service`. The controller:

1. Fetches the Service
2. Checks `IsServiceGLBSupported` — if not supported but has finalizer, runs delete
3. If supported, runs ensure (add finalizer + call usecase)
4. SetupWithManager watches `Service` and `Node` (no LBC/GLBC watch needed)

```go
// Source: internal/controller/core/service_controller.go (template)
// Named: "service-glb"
// Watches: &corev1.Service{}, &corev1.Node{}
// Finalizer: domain.ServiceGLBFinalizer = "glb.vks.vngcloud.vn/resources"
```

### Pattern 4: UseCase (serviceGLBUseCase)

Follows `vglb_uc` pattern for Init (reads node labels) and `service_uc` pattern for Ensure/Delete. The `defaultModelBuildTask` struct holds:
- `*corev1.Service` as primary resource (not VGLB)
- `annotationParser` with `glb.vks.vngcloud.vn` prefix
- `defaultRegion`, `defaultNetworkId`, `defaultSubnetId` from Init node label read
- `endpointResolver` for pool member resolution

The `run()` method:
1. Calls `buildGlobalLoadBalancerConfig(ctx)` — list existing GLBC by owner labels, build spec, create/patch
2. Reads GLBC status domains, calls `k8sRepo.UpdateServiceStatusAddress(ctx, ...)` to update `Service.Status.LoadBalancer.Ingress`

```go
// Source: internal/usecase/vglb_uc/vglb_uc.go (Init pattern)
// Init reads: labelMgmtZone, labelNetworkId, labelSubnetId from first node
// region = stripZoneSuffix(rawZone)  — "hcm03b" -> "hcm"
```

### Pattern 5: GLBC Building (build_glbc.go)

Identical to `vglb_uc/build_glbc.go` but with `t.service` replacing `t.vglb` as annotation source:

```go
// Source: internal/usecase/vglb_uc/build_glbc.go

// Owner labels use "Service" not KindVngcloudGlobalLoadBalancer:
glbConfig.Labels[domain.LabelOwnerResourceKind] = "Service"
glbConfig.Labels[domain.LabelOwnerResourceName] = t.service.GetName()
glbConfig.Labels[domain.LabelOwnerResourceUid]  = string(t.service.GetUID())

// generateName: "{service-name}-"
glbConfig = &v1alpha1.GlobalLoadBalancerConfig{
    ObjectMeta: metav1.ObjectMeta{
        Namespace:    t.service.Namespace,
        GenerateName: t.service.Name + "-",
    },
}

// Default GLB name:
name := "vks_" + t.service.Namespace + "_" + t.service.Name
name = strings.ReplaceAll(name, "-", "_")
```

The `buildPoolsAndListeners` logic is taken verbatim from `vglb_uc/build_global_pool.go`. The `getTargetType` method reads from `t.service.Annotations` (not `t.vglb.Annotations`) and forces `TargetTypeIP` for ClusterIP services — same logic.

### Pattern 6: Delete Flow (GLBC cleanup)

Identical to `vglb_uc.deleteGlobalLoadBalancerConfig` but using `LabelOwnerResourceKind: "Service"`:

```go
// Source: internal/usecase/vglb_uc/vglb_uc.go

err = uc.k8sRepo.ListGlobalLoadBalancerConfig(ctx, glbcList,
    client.InNamespace(svc.GetNamespace()),
    client.MatchingLabels{
        domain.LabelOwnerResourceName: svc.GetName(),
        domain.LabelOwnerResourceKind: "Service",   // NOT KindVngcloudGlobalLoadBalancer
        domain.LabelOwnerResourceUid:  string(svc.UID),
    },
)
```

### Pattern 7: ServiceGLBUseCase Interface in contracts.go

```go
// Source: internal/usecase/contracts.go (existing pattern)
type ServiceGLBUseCase interface {
    InitServiceGLBUseCase(ctx context.Context) error
    EnsureServiceGLBUseCase(ctx context.Context, req ctrl.Request) error
    DeleteServiceGLBUseCase(ctx context.Context, req ctrl.Request) error
}
```

### Pattern 8: controller Registration in main.go

Follows lines 390-411 of `cmd/main.go` (VGLB registration block):

```go
if !disableServiceGLBController {
    annotationParser := annotations.NewSuffixAnnotationParser(domain.GLB_ANNOTATION_PREFIX)
    serviceGLBUtils := service_glb.NewServiceGLBUtils(domain.ServiceGLBFinalizer, annotationParser)
    serviceGLBUseCase := service_glb_uc.NewServiceGLBUseCase(
        conf, k8sRepo, vngcloudRepo, annotationParser, endpointResolver,
    )
    reconciler := service_glb_controller.NewServiceGLBReconciler(...)
    if err := reconciler.SetupWithManager(ctx, mgr); err != nil { os.Exit(1) }
}
```

### Anti-Patterns to Avoid

- **Coupling to VGLB:** Do not import or reference the vglb package — zero coupling is required.
- **Using `obj.Kind` for label values:** K8s strips TypeMeta from objects returned by Get. Hardcode `"Service"` instead of `svc.Kind` for owner labels.
- **ClusterIP with instance target type:** Forced to `TargetTypeIP` in `getTargetType()` — never use NodePort resolution for ClusterIP services.
- **Annotation removal without cleanup:** The Update event handler MUST check if OLD had GLB annotation to enqueue cleanup when annotation is removed.
- **Missing sort on pools/listeners:** Always `sort.Slice` pools and listeners by name before assigning to spec — prevents spurious spec-change reconciles.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Annotation parsing | Custom map lookup | `annotations.NewSuffixAnnotationParser("glb.vks.vngcloud.vn")` | Handles prefix/suffix, bool/int64/slice/json parsing |
| Finalizer management | Direct patch | `k8s.FinalizerManager.AddFinalizers/RemoveFinalizers` | Handles concurrent patch, retry |
| Pool member resolution | Custom node/pod IP list | `utils.EndpointResolver.ResolveNodePortEndpoints` / `ResolvePodEndpoints` | Handles node exclusion labels, readiness gates |
| Owner-label GLBC lookup | Custom name convention | `client.MatchingLabels{domain.LabelOwner*}` | Supports multiple GLBCs per Service, survives rename |
| Spec equality check | Field-by-field comparison | `reflect.DeepEqual(oldSpec, newSpec)` via `glbcSpecEqual` | Handles all nested optional pointer fields |
| Status address patch | Direct client.Update | `k8sRepo.UpdateServiceStatusAddress` | Uses status subresource, avoids conflict errors |

---

## Common Pitfalls

### Pitfall 1: Owner Label Kind Value Lost After K8s Round-Trip
**What goes wrong:** Setting `glbConfig.Labels[domain.LabelOwnerResourceKind] = svc.Kind` where `svc.Kind` is empty string because K8s strips TypeMeta from objects returned by `Get`.
**Why it happens:** Kubernetes API server returns objects without TypeMeta populated when using the typed client.
**How to avoid:** Hardcode the string `"Service"` — add a constant `KindService = "Service"` to `domain.go` or use the literal directly.
**Warning signs:** Label selector finds 0 GLBCs even though they exist; GLBC cleanup never triggers.

### Pitfall 2: Node Event Handler Enqueue Logic
**What goes wrong:** Node events trigger reconciliation of all Services with GLB annotations — if the enqueue logic is wrong (e.g., enqueues all Services blindly), it causes reconcile storms.
**Why it happens:** The VGLB node event handler enqueues all VGLBs in the cluster. For Service-based GLB, the equivalent must list Services with the `glb.vks.vngcloud.vn/enable=true` annotation.
**How to avoid:** In the node event handler, use `k8sRepo.ListService` with a label or field selector, then filter by `IsServiceGLBSupported`. Reference `vglb_controller/eventhandlers/node_events.go` for the pattern.
**Warning signs:** Excessive reconcile counter increments on node events.

### Pitfall 3: Annotation Removal Not Detected
**What goes wrong:** When a user removes `glb.vks.vngcloud.vn/enable` from a Service, the Service stops being enqueueable by `IsServiceGLBSupported` but still has the finalizer. The finalizer is never removed and GLBC is never cleaned up.
**Why it happens:** The standard `enqueueManagedService` checks the NEW state only. A Service that had the annotation removed shows `IsServiceGLBSupported=false` and `IsServiceGLBPendingFinalization=true`, so the reconciler should detect this and run delete. This works IF the Update event enqueues it when old had annotation and new doesn't.
**How to avoid:** In the Update event handler, always check if OLD had the annotation regardless of NEW state.

### Pitfall 4: ClusterIP Service with Instance Target Type
**What goes wrong:** If `getTargetType` doesn't force `TargetTypeIP` for ClusterIP services, `ResolveNodePortEndpoints` is called for a ClusterIP service which has no NodePort — returns empty members or error.
**Why it happens:** VGLB controller had this same bug documented as VGLB-06; fixed by explicit check.
**How to avoid:** In `getTargetType`: `if t.service.Spec.Type == corev1.ServiceTypeClusterIP { return domain.TargetTypeIP }`.

### Pitfall 5: Missing Sort on Pool/Listener Slices
**What goes wrong:** GLBC spec differs on every reconcile due to slice ordering non-determinism, causing continuous unnecessary patches and infinite reconcile loops.
**Why it happens:** Map iteration order is not deterministic; port ordering from the API may change.
**How to avoid:** Always `sort.Slice(allPools, ...)` and `sort.Slice(allListeners, ...)` by Name before assigning to spec, exactly as done in `vglb_uc/build_global_pool.go`.

### Pitfall 6: Envtest Node Status Not Populated
**What goes wrong:** EndpointResolver excludes nodes where `NodeReady` condition is not True — in envtest, `Node.Status.Conditions` is empty by default, so pool members come back empty.
**Why it happens:** Envtest requires explicit `Status().Update()` call after `Create()`.
**How to avoid:** In test setup, create node with `NodeReady=True` condition AND call `k8sClient.Status().Update(ctx, node)` separately. Reference `vglb_controller/suite_test.go` lines 143-166.

---

## Code Examples

Verified patterns from existing source:

### stripZoneSuffix (reuse verbatim from vglb_uc)
```go
// Source: internal/usecase/vglb_uc/vglb_uc.go
var zoneRe = regexp.MustCompile(`\d+[a-z]*$`)

func stripZoneSuffix(zone string) string {
    return zoneRe.ReplaceAllString(zone, "")
}
// "hcm03b" -> "hcm", "sgn01a" -> "sgn", "han01" -> "han"
```

### GLB default name (matches VGLB pattern)
```go
// Source: internal/usecase/vglb_uc/build_glbc.go
name := "vks_" + svc.Namespace + "_" + svc.Name
name = strings.ReplaceAll(name, "-", "_")
if len(name) > 50 {
    name = name[:50]
}
```

### Pool member group naming
```go
// Source: internal/usecase/vglb_uc/build_global_pool.go
poolMembers = append(poolMembers, v1alpha1.GlobalPoolMember{
    Name:    fmt.Sprintf("%s-%s", t.defaultRegion, t.defaultNetworkId),
    Region:  t.defaultRegion,
    VpcId:   t.defaultNetworkId,
    Type:    global.GlobalPoolMemberTypePrivate,
    Members: globalMembers,
})
```

### glbcSpecEqual
```go
// Source: internal/usecase/vglb_uc/build_glbc.go
func glbcSpecEqual(a, b v1alpha1.GlobalLoadBalancerConfigSpec) bool {
    return reflect.DeepEqual(a, b)
}
```

### Integration test suite setup (envtest)
```go
// Source: internal/controller/vglb_controller/suite_test.go
// Key: register both ServiceGLB reconciler AND GLBC reconciler in test manager
// so GLBC objects created by the controller get reconciled to fill status
glbcUseCase := glbc_uc.NewGlobalLoadBalancerConfigUseCase(mockConfig, k8sRepo, vngcloudRepo)
glbcReconciler := glbc_controller.NewGlobalLoadBalancerConfigReconciler(...)
err = glbcReconciler.SetupWithManager(ctx, k8sManager)
```

### Service with GLB annotation (test helper)
```go
// Pattern derived from helpers_test.go
func newServiceWithGLBAnnotation(name, namespace string, ports []corev1.ServicePort) *corev1.Service {
    return &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
            Annotations: map[string]string{
                "glb.vks.vngcloud.vn/enable": "true",
            },
        },
        Spec: corev1.ServiceSpec{
            Type:  corev1.ServiceTypeNodePort,
            Ports: ports,
        },
    }
}
```

### Find GLBC by Service owner labels
```go
// Pattern: same as findGLBCByOwnerLabels in helpers_test.go but Kind="Service"
err := k8sClient.List(ctx, glbcList,
    client.InNamespace(svc.Namespace),
    client.MatchingLabels{
        domain.LabelOwnerResourceName: svc.Name,
        domain.LabelOwnerResourceKind: "Service",
        domain.LabelOwnerResourceUid:  string(svc.UID),
    },
)
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| VGLB CRD as GLB trigger | Service annotation `glb.vks.vngcloud.vn/enable=true` | Phase 7 | No CRD needed for user; familiar annotation UX |
| Init via VNG Cloud API call | Init reads node labels (`vks.vngcloud.vn/mgmt-zone`, etc.) | Phase 6 | No API call at startup; faster init |
| Hardcoded `"default"` pool member group | `{region}-{vpcId}` naming | Phase 6 | Correct multi-region grouping |
| `glb_` prefix for GLB names | `vks_` prefix | Phase 6 | Consistent with VKS naming conventions |

---

## Open Questions

1. **`disableServiceGLBController` flag name**
   - What we know: main.go uses `disableVngcloudGlobalLoadBalancerController` for VGLB
   - What's unclear: Should the flag name follow the same pattern or use a shorter name?
   - Recommendation: Use `disableServiceGLBController` as the flag variable name; add `--disable-service-glb-controller` as the CLI flag

2. **Node event handler — how to list GLB-annotated Services**
   - What we know: `k8sRepo.ListService` exists; `client.MatchingLabels` only works for labels, not annotations
   - What's unclear: Annotations cannot be used in a label selector; listing all Services and filtering in-memory may be expensive at scale
   - Recommendation: List all Services cluster-wide and filter by `IsServiceGLBSupported` in the node event handler, same pattern as the VGLB node event handler that lists all VGLBs

3. **KindService constant location**
   - What we know: `domain.KindVngcloudGlobalLoadBalancer = "VngcloudGlobalLoadBalancer"` exists in domain.go
   - What's unclear: Should `"Service"` be a domain constant or hardcoded?
   - Recommendation: Add `KindService = "Service"` to domain.go for consistency

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Ginkgo v2 + Gomega (existing) |
| Config file | none — standard Go test runner |
| Quick run command | `go test ./internal/usecase/service_glb_uc/... -v` |
| Full suite command | `go test ./internal/controller/service_glb_controller/... -v -timeout 120s` |

### Phase Requirements → Test Map

Phase 7 requirements are TBD in REQUIREMENTS.md (to be added). Based on CONTEXT.md decisions, the expected behaviors map to:

| Behavior | Test Type | Automated Command | File |
|----------|-----------|-------------------|------|
| Service with `glb.../enable=true` → GLBC created | integration | `go test ./internal/controller/service_glb_controller/... -run "Create"` | Wave 0 |
| Annotation removal → GLBC deleted | integration | `go test ./internal/controller/service_glb_controller/... -run "Delete"` | Wave 0 |
| Service port change → GLBC spec updated | integration | `go test ./internal/controller/service_glb_controller/... -run "Update"` | Wave 0 |
| GLBC status domains → Service.Status.LoadBalancer.Ingress | integration | same suite | Wave 0 |
| Pool building from NodePort service | unit | `go test ./internal/usecase/service_glb_uc/... -run "Pool"` | Wave 0 |
| GLB name default `vks_{ns}_{name}` format | unit | `go test ./internal/usecase/service_glb_uc/... -run "Name"` | Wave 0 |
| stripZoneSuffix correct region extraction | unit | already tested in vglb_uc — can reuse | existing |
| ClusterIP forced to target-type=ip | unit | `go test ./internal/usecase/service_glb_uc/...` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/usecase/service_glb_uc/... -v`
- **Per wave merge:** `go test ./internal/... -v -timeout 120s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/controller/service_glb_controller/suite_test.go` — envtest setup
- [ ] `internal/controller/service_glb_controller/helpers_test.go` — test helpers
- [ ] `internal/controller/service_glb_controller/service_glb_controller_test.go` — create/update/delete flows
- [ ] `internal/usecase/service_glb_uc/service_glb_uc_test.go` — unit tests for build logic

---

## Sources

### Primary (HIGH confidence)
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/controller/core/service_controller.go` — controller struct, reconcile flow, SetupWithManager
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/usecase/service_uc/service_uc.go` — usecase Init/Ensure/Delete patterns, defaultModelBuildTask
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/usecase/vglb_uc/vglb_uc.go` — Init from node labels, GLBC delete pattern
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/usecase/vglb_uc/build_glbc.go` — GLBC spec building, owner labels, generateName
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/usecase/vglb_uc/build_global_pool.go` — pool/listener construction, pool member groups, sort
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/controller/vglb_controller/vglb_controller.go` — full controller reconciler with Service+Node watchers
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/controller/vglb_controller/suite_test.go` — envtest setup with GLBC+VGLB controllers, node setup
- `/home/stackops/vngcloud-load-balancer-controller-crd/pkg/service/service_utils.go` — ServiceUtils interface pattern
- `/home/stackops/vngcloud-load-balancer-controller-crd/pkg/annotations/parser.go` — `NewSuffixAnnotationParser` implementation
- `/home/stackops/vngcloud-load-balancer-controller-crd/pkg/annotations/constants.go` — all existing suffix constants
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/domain/domain.go` — labels, finalizer constants, annotation prefixes
- `/home/stackops/vngcloud-load-balancer-controller-crd/internal/usecase/contracts.go` — UseCase interface pattern
- `/home/stackops/vngcloud-load-balancer-controller-crd/cmd/main.go` — controller registration pattern (lines 390-411)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries are existing project dependencies
- Architecture: HIGH — directly modeled from two existing controllers (service_controller + vglb_controller), both fully readable
- Pitfalls: HIGH — pitfalls documented from Phase 6 STATE.md lessons learned and code inspection
- Test patterns: HIGH — envtest setup is readable in vglb_controller/suite_test.go

**Research date:** 2026-03-17
**Valid until:** 2026-04-17 (stable codebase; no external API changes expected)

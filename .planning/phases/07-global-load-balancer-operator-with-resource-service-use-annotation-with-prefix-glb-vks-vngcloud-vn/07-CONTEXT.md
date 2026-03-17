# Phase 7: Global Load Balancer Operator with Service Annotations - Context

**Gathered:** 2026-03-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Create a new Service-annotation-based GLB controller that watches Services with `glb.vks.vngcloud.vn/enable=true` annotation and creates/manages GlobalLoadBalancerConfig (GLBC) resources. The controller follows the same pattern as the existing Service controller (`core/service_controller`), creating GLBC resources that the existing GLBC controller reconciles to VNG Cloud. This is independent of the VGLB CRD (which will be removed in the future).

</domain>

<decisions>
## Implementation Decisions

### Controller Architecture
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

### Service Type Support
- NodePort and LoadBalancer: support all target types (instance mode default)
- ClusterIP: must use target-type=ip (pod IPs), same as Service controller
- Support `glb.vks.vngcloud.vn/target-type` annotation with values 'instance' and 'ip'

### Annotation Design
- Prefix: `glb.vks.vngcloud.vn`
- Trigger: `glb.vks.vngcloud.vn/enable=true` (boolean, explicit opt-in)
- Full annotation set matching Service controller capabilities: load-balancer-id, load-balancer-name, package-id, description, idle-timeouts, inbound-cidrs, healthcheck-*, pool-algorithm, target-type, etc.
- Same suffix values as existing vks.vngcloud.vn annotations but NEW constants defined for GLB suffixes
- Parser: reuse `annotations.NewSuffixAnnotationParser("glb.vks.vngcloud.vn")`

### Service Filtering & Lifecycle
- Watch all namespaces cluster-wide, filter by annotation in predicate
- Predicate filter: event handler checks for `glb.vks.vngcloud.vn/enable` annotation before enqueuing (IsServiceGLBSupported pattern)
- Annotation removal detection: check old annotations in Update event — if OLD had annotation but NEW doesn't, enqueue for cleanup
- Removing `enable` annotation triggers GLBC deletion (annotation on = create, off = delete)

### Relationship with VGLB
- Fully independent of VGLB controller — zero coupling
- Both can coexist on same Service (each creates its own GLBC independently)
- User's responsibility to avoid conflicts if both are active on same Service
- VGLB will be removed in the future; this is the replacement approach
- Owner labels: `owner-resource-kind=Service`, `owner-resource-name={svc-name}`, `owner-resource-uid={svc-uid}` — same label keys, different kind value from VGLB

### Code Consistency
- Follow Service controller code structure: build_glbc.go, build_pool.go, build_listener.go
- Dedicated utils package: `pkg/service_glb/service_glb_utils.go` with IsServiceGLBSupported and IsServiceGLBPendingFinalization
- Annotation constants: new constants in `pkg/annotations/constants.go` for GLB suffixes

### Testing
- Unit tests for GLBC building logic (pools, listeners, member resolution)
- Integration tests with envtest: Service annotation → GLBC creation/update/delete flows

### Claude's Discretion
- Exact diff comparison logic for skip-if-equal optimization
- Error message formatting
- Requeue backoff durations
- Pool member group naming (carry forward {region}-{vpcId} pattern from VGLB)
- Health monitor defaults

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Service Controller (primary reference pattern)
- `internal/controller/core/service_controller.go` — Service reconciler structure to follow
- `internal/usecase/service_uc/service_uc.go` — Usecase pattern: ensure/delete flows, task runner
- `internal/usecase/service_uc/build_lbc.go` — LBC builder: generateName, owner labels, annotation parsing
- `internal/usecase/service_uc/build_pool.go` — Pool building from Service ports
- `internal/usecase/service_uc/build_listener.go` — Listener building pattern
- `internal/controller/core/eventhandlers/service_events.go` — Service event handler with IsServiceSupported predicate
- `pkg/service/service_utils.go` — IsServiceSupported, IsServicePendingFinalization patterns

### VGLB Controller (GLBC generation reference)
- `internal/usecase/vglb_uc/build_glbc.go` — GLBC spec generation, annotation parsers (buildLoadBalancerName, buildLoadBalancerId, etc.)
- `internal/usecase/vglb_uc/build_global_pool.go` — Global pool building with EndpointResolver, pool member groups
- `internal/usecase/vglb_uc/vglb_uc.go` — Init pattern (node label reading), ensure/delete flows

### CRD Types
- `api/v1alpha1/globalloadbalancerconfig_types.go` — GLBC CRD spec (pools, listeners, members)
- `api/v1alpha1/vngcloudgloballoadbalancer_types.go` — VGLB CRD (reference only)

### Domain & Annotations
- `internal/domain/domain.go` — Labels, finalizers, annotation prefix constants
- `pkg/annotations/constants.go` — Existing annotation suffix constants
- `pkg/annotations/parser.go` — SuffixAnnotationParser implementation

### Integration Test Patterns
- `internal/controller/vglb_controller/vglb_controller_test.go` — Envtest setup for GLBC-creating controllers
- `internal/controller/glbc_controller/glbc_controller_test.go` — GLBC controller test patterns

### Controller Registration
- `cmd/main.go` — Controller setup and registration pattern

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `EndpointResolver` — resolves node IPs / pod IPs from Services, already wired in VGLB usecase
- `SuffixAnnotationParser` — generic parser, just needs new prefix instance
- Owner label constants (`LabelOwnerResourceKind`, `LabelOwnerResourceName`, `LabelOwnerResourceUid`) in domain.go
- Mock provider with full GLB API simulation — reusable for integration tests
- `NameHelper` utility for generating resource names

### Established Patterns
- Controller → UseCase → Repository layering used by all controllers
- Finalizer-based cleanup: add finalizer on ensure, cleanup + remove on delete
- `defaultModelBuildTask` pattern in service_uc: struct with all dependencies, `run()` method
- Status patching via `PatchMutateStatus*` methods
- Event handler: enqueue on create/update, check IsSupported/IsPendingFinalization predicates

### Integration Points
- `cmd/main.go` — add controller registration (same pattern as VGLB controller lines 390-411)
- `internal/usecase/contracts.go` — add new usecase interface
- `internal/domain/domain.go` — add GLB annotation prefix, finalizer constants
- `pkg/annotations/constants.go` — add GLB annotation suffix constants

</code_context>

<specifics>
## Specific Ideas

- Follow Service controller pattern exactly — this IS a Service controller, just for global LBs
- VGLB will be deprecated/removed in the future; this controller is the replacement
- Same suffix values as existing annotations, just new prefix (`glb.vks.vngcloud.vn` instead of `vks.vngcloud.vn`)
- Pool member group naming carries forward from VGLB: `{region}-{vpcId}`
- Region from node label zone via stripZoneSuffix (hcm03b → hcm)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 07-global-load-balancer-operator-with-resource-service-use-annotation-with-prefix-glb-vks-vngcloud-vn*
*Context gathered: 2026-03-17*

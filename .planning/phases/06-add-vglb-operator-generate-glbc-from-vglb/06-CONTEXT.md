# Phase 6: add vglb operator, generate glbc from vglb - Context

**Gathered:** 2026-03-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Enhance the existing VngcloudGlobalLoadBalancer (VGLB) operator to properly generate GlobalLoadBalancerConfig (GLBC) resources from a matching Service. VGLB finds a Service with the same name+namespace, reads its NodePorts, and builds a complete GLBC spec with pools, listeners, and pool members. The VGLB controller watches Services and Nodes for changes and updates the GLBC in-place.

</domain>

<decisions>
## Implementation Decisions

### GLBC Generation Logic
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

### VGLB Spec Design
- VngcloudGlobalLoadBalancerSpec remains empty — all config via annotations
- VGLB watches Services (all cluster-wide) and Nodes for changes
- Service watch only (no EndpointSlice watch) — endpoints resolved at reconcile time
- Matches target Service by same name + namespace
- If matching Service doesn't exist: requeue until it appears
- If Service type is ClusterIP (no NodePort): reject, requeue

### Naming Conventions
- GLB name (VNG Cloud portal): `vks_{namespace}_{vglb_name}` (or annotation override via `vks.vngcloud.vn/load-balancer-name`)
- Pool names: `pool-{port}-{protocol}` (e.g., pool-80-tcp, pool-443-tcp)
- Listener names: `listener-{port}` (e.g., listener-80, listener-443)
- Pool member group name: `{region}-{vpcId}` (e.g., hcm-net-86b7c84a)
- GLBC K8s name: generateName prefix from VGLB name

### Status Propagation
- VGLB status contains Address field only (minimal)
- Address comes from GLBC domains only (not VIPs)
- Status update follows same pattern as Service/LBC controllers
- Keep last known Address on VGLB delete (don't clear)

### Multi-GLBC Ownership
- Always one GLBC per VGLB (1:1 relationship)
- Multiple VGLBs can share the same load balancer via LoadBalancerId annotation
- On VGLB delete: always delete its GLBC (GLBC controller handles partial LB cleanup for shared LBs)
- VGLB adopts and updates existing GLBCs with matching owner labels

### GLBC Update Strategy
- Full spec replace on each reconcile (rebuild entire GLBC spec from Service state)
- Skip update if GLBC spec already matches desired state (avoid unnecessary reconcile triggers)

### Error Handling
- Service not found: requeue with backoff
- Service has no ports or no endpoints: requeue with backoff
- Service type is ClusterIP: reject, requeue

### Annotations (Core Set)
- `vks.vngcloud.vn/load-balancer-id` — use existing LB by ID
- `vks.vngcloud.vn/load-balancer-name` — override GLB display name
- `vks.vngcloud.vn/package-id` — specify LB package
- `vks.vngcloud.vn/description` — LB description

### Concurrency
- No special locking at VGLB level — each VGLB manages its own GLBC independently
- GLBC controller handles LB-level locking for shared LB scenarios

### Init Behavior
- Resolve network info from node labels at startup (region, VPC, subnet)

### Testing
- Unit tests for build_glbc.go: pool/listener generation, naming, member resolution
- Integration tests with envtest: VGLB → GLBC → mock backend flow

### Claude's Discretion
- Exact diff comparison logic for skip-if-equal optimization
- EndpointResolver integration details
- Error message formatting
- Requeue backoff durations

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### VGLB Operator (existing code)
- `internal/usecase/vglb_uc/build_glbc.go` — Current GLBC generation logic, ownership labels, annotation parsing
- `internal/usecase/vglb_uc/build_global_pool.go` — Current pool building with EndpointResolver
- `internal/usecase/vglb_uc/vglb_uc.go` — VGLB usecase: ensure/delete flows
- `internal/controller/vglb_controller/vglb_controller.go` — VGLB reconciler structure

### GLBC Controller (downstream consumer)
- `internal/usecase/glbc_uc/deploy_lb.go` — How GLBC processes the spec (LB creation, package, etc.)
- `internal/usecase/glbc_uc/deploy_pool.go` — How GLBC deploys pools from spec
- `internal/usecase/glbc_uc/deploy_listener.go` — How GLBC deploys listeners from spec
- `internal/usecase/glbc_uc/deploy_pool_member.go` — Pool member merge logic

### CRD Types
- `api/v1alpha1/vngcloudgloballoadbalancer_types.go` — VGLB CRD (currently minimal)
- `api/v1alpha1/globalloadbalancerconfig_types.go` — GLBC CRD (full spec with pools/listeners/members)

### Reference Patterns
- `internal/usecase/service_uc/build_lbc.go` — Service→LBC generation pattern (similar to VGLB→GLBC)
- `internal/usecase/service_uc/build_pool.go` — Pool building from Service ports (reference for naming)
- `internal/controller/vglb_controller/eventhandlers/vglb_events.go` — Existing VGLB event handler

### Domain Constants
- `internal/domain/domain.go` — Labels, finalizers, annotation prefix

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `EndpointResolver` — already wired into VGLB usecase, resolves endpoints from Services
- `buildLoadBalancerId()`, `buildLoadBalancerName()`, `buildPackageId()`, `buildDescription()` — annotation parsers in build_glbc.go
- Owner labels (`LabelOwnerResourceKind`, `LabelOwnerResourceName`, `LabelOwnerResourceUid`) — already defined in domain.go
- `vglb_events.go` — existing event handler for VGLB objects
- Mock provider with full GLB API simulation — reusable for integration tests

### Established Patterns
- Controller → UseCase → Repository pattern used by all controllers
- Finalizer-based cleanup (ensure on create, cleanup on delete)
- Status patching via `PatchMutateStatus*` methods
- Event handler pattern: enqueue on create/update, check IsSupported/IsPendingFinalization

### Integration Points
- `cmd/main.go` lines 390-411 — VGLB controller registration (already exists)
- `SetupWithManager` — needs Service and Node watchers added
- `internal/usecase/contracts.go` — VngcloudGlobalLoadBalancerUseCase interface

</code_context>

<specifics>
## Specific Ideas

- Pool member group naming uses stripped region + VPC ID: e.g., "hcm-net-86b7c84a"
- Region derived by stripping availability zone suffix from node label (hcm03b → hcm)
- VGLB→GLBC relationship mirrors existing Service→LBC pattern in service_uc
- GLB display name format: `vks_{namespace}_{name}` (not `glb_` prefix)

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 06-add-vglb-operator-generate-glbc-from-vglb*
*Context gathered: 2026-03-16*

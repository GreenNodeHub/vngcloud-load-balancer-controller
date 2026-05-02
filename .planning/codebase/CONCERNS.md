# Codebase Concerns

**Analysis Date:** 2026-03-31

## Tech Debt

**Unimplemented rate-limit error detection:**
- Issue: `IsRateLimitExceeded` always returns `false`. Any API rate-limit errors from VNGCloud are treated as generic errors and trigger a full requeue instead of being handled gracefully (e.g., with exponential back-off).
- Files: `internal/domain/error.go:72`
- Impact: Under heavy load, the controller can enter a tight retry loop that amplifies the rate-limit condition.
- Fix approach: Inspect the VNGCloud SDK error body for a rate-limit indicator and return `true` accordingly.

**Annotations parsed repeatedly on every reconcile:**
- Issue: `getTargetType` and similar helpers re-parse annotations on every call rather than caching results in the build-task struct. The same annotation string is parsed across multiple sub-functions within a single reconcile cycle.
- Files: `internal/usecase/service_uc/build_pool.go:166`, `internal/usecase/ingress_uc/build_pool.go:268`, `internal/usecase/vglb_uc/build_global_pool.go:195`
- Impact: Negligible CPU cost today; becomes a correctness risk if parsing grows more complex or if calls have side effects.
- Fix approach: Cache parsed values in the `defaultModelBuildTask` struct during `run()` and share them across helpers.

**Hardcoded fallback package ID for Global Load Balancer:**
- Issue: When no default package name is configured and the API lookup fails, the code falls back to a hardcoded package ID (`pkg-b02e62ab-a282-4faf-8732-a172ef497a7b`). This ID is specific to one environment and will silently fail in other regions/environments.
- Files: `internal/usecase/glbc_uc/deploy_lb.go:343`
- Impact: GLB creation silently uses the wrong package in environments where that ID does not exist, causing a confusing API error.
- Fix approach: Remove the hardcoded fallback; require `GlobalLoadBalancerOpts.DefaultL4PackageName` to be set or return a clear configuration error.

**`TargetType` declared in `domain` package but not used there:**
- Issue: `TargetType` and its constants are declared in `internal/domain/domain.go` with a `TODO: refactor` comment. All consumers reference the domain type but the abstraction is incomplete.
- Files: `internal/domain/domain.go:3`
- Impact: Low today; becomes drift risk as new controllers are added.
- Fix approach: Audit all usages and move type to the correct layer or delete the comment once the design is finalised.

**`LbcFinalizer` name unplanned:**
- Issue: The finalizer name `lbc.vngcloud.vn/resources` carries a `TODO: plan this finalizer name` comment. Finalizer names are sticky once applied to live objects; changing them later requires a migration.
- Files: `internal/domain/domain.go:17`
- Impact: Changing this name in future will leave orphan finalizers on existing resources unless a migration reconciler is written.
- Fix approach: Decide on the canonical name and remove the TODO before the next major release.

**Incomplete INTERVPC LBC creation (listener/pool not inlined):**
- Issue: The INTERVPC `buildCreateLoadBalancerRequest` skips attaching listener and pool to the creation request with a `TODO: add more fields` comment. The API likely supports it but the SDK types are not yet wired in.
- Files: `internal/usecase/lbc_uc/deploy_lb.go:434`
- Impact: INTERVPC load balancers are created without initial listener/pool, requiring separate API round-trips and increasing provisioning time.
- Fix approach: Wire `inter.ListenerRequest` and `inter.CreatePoolRequest` into the creation request once SDK types are verified.

**Tags not set on GLB creation (SDK limitation):**
- Issue: Tag propagation when creating a GlobalLoadBalancer is commented out with the note `sdk not support add tags when create load balancer`. Tags are never applied to new GLBs.
- Files: `internal/usecase/glbc_uc/deploy_lb.go:304`
- Impact: GLBs created via this controller cannot be tagged at creation time, breaking any billing/ownership tagging workflows.
- Fix approach: Track SDK support for `WithTags` on `CreateGlobalLoadBalancer`; uncomment the block when available.

**Pool status not updated after update in GLBC deploy path:**
- Issue: `statusAddPool` call is commented out with `// TODO: uncomment me` after a pool update in the global LB deploy flow. Pool status is therefore not reflected in the CRD after updates.
- Files: `internal/usecase/glbc_uc/deploy_pool.go:87`
- Impact: CRD status drifts from actual cloud state after pool updates; dependent controllers or operators relying on status will operate on stale data.
- Fix approach: Uncomment and verify `statusAddPool` is safe to call after the pool update path.

**`GlobalLoadBalancerConfig` CRD types carry placeholder `TODO` comments:**
- Issue: Several fields in `GlobalPoolMember` and `GlobalMember` structs carry bare `// TODO` comments instead of real documentation. This affects `Region`, `TrafficDial`, `VpcId`, `Type`, and the `GlobalMember` type itself.
- Files: `api/v1alpha1/globalloadbalancerconfig_types.go:222-249`
- Impact: Generated documentation and user-facing kubebuilder markers are incomplete; schema validation markers may be missing.
- Fix approach: Replace each `// TODO` with actual godoc and add kubebuilder validation markers.

## Known Bugs

**`ErrorNotFound` has a debug error message:**
- Symptoms: The sentinel error `ErrorNotFound` contains the message `"heheh not found"` — clearly a debug placeholder that was never replaced.
- Files: `internal/domain/error.go:13`
- Trigger: Any code path that surfaces this error to a log or status condition emits an unprofessional and confusing message.
- Workaround: None; consumers check for the error value by identity not by string.

**`GetLoadBalancerByName` does not use tag-filtered listing:**
- Symptoms: `GetLoadBalancerByName` calls `ListLoadBalancers` with `nil` tags and iterates the first page (up to 1000 items) doing a string match. In large projects with many load balancers, this is both slow and may return the wrong result if names collide across clusters.
- Files: `internal/repository/vngcloud_repo/vngcloud_loadbalancer.go:40-51`
- Trigger: Any code path that looks up a load balancer by name rather than ID.
- Workaround: Where possible, callers should look up by ID stored in CRD status.

## Security Considerations

**Client secret passed via config file:**
- Risk: `ClientSecret` and `SuperClientSecret` are read from a plain config file (`/etc/vngcloud-load-balancer-controller/config.yaml`). If the file is world-readable inside the container or the ConfigMap/Secret is misconfigured, credentials leak.
- Files: `internal/repository/vngcloud_repo/vngcloud_repo.go:48-66`, `pkg/config/config.go`
- Current mitigation: Kubernetes Secret mount, but no code-level enforcement.
- Recommendations: Add a startup check that the config file has restrictive permissions; consider accepting credentials via environment variable or Workload Identity instead.

**`superClient` created with `context.Background()` ignoring operator lifecycle:**
- Risk: The super-client SDK client is initialised with `context.Background()` instead of the controller-manager root context, meaning it is not cancelled when the manager shuts down. Long-running background SDK requests may continue after the process is logically stopped.
- Files: `internal/repository/vngcloud_repo/vngcloud_repo.go:66`
- Current mitigation: None.
- Recommendations: Pass the root context from `NewVngCloudRepository` to the super-client initialisation.

## Performance Bottlenecks

**Unbounded load-balancer listing (page size 1000, no pagination):**
- Problem: All `ListLoadBalancers` and `ListGlobalLoadBalancers` calls use a fixed page size of 1000 with offset 0. If a project has more than 1000 LBs, only the first page is returned, silently truncating results.
- Files: `internal/repository/vngcloud_repo/vngcloud_repo.go:22`, `internal/repository/vngcloud_repo/vngcloud_loadbalancer.go:23`, `internal/repository/vngcloud_repo/vngcloud_global.go:30`
- Cause: No pagination loop is implemented; `defaultPageSize = 1000` is treated as "all".
- Improvement path: Implement a pagination loop that accumulates pages until a page smaller than `pageSize` is returned.

**No caching for repeated network/subnet lookups:**
- Problem: The `vngCloudRepository` struct has commented-out `sync.Map` caches for subnet-to-CIDR, subnet-to-zone, and instance-to-subnet lookups. Every reconcile fetches these values from the API.
- Files: `internal/repository/vngcloud_repo/vngcloud_repo.go:87-93`
- Cause: Cache not implemented.
- Improvement path: Uncomment the cache fields and add TTL-based invalidation; alternatively use the already-present metadata provider cache.

**SDK retry count set to 1:**
- Problem: Both the primary and super clients are configured with `.WithRetryCount(1)`, meaning only one retry per API call. Transient cloud-API errors will cause unnecessary reconcile failures.
- Files: `internal/repository/vngcloud_repo/vngcloud_repo.go:54,66`
- Cause: Conservative default, not tuned for production.
- Improvement path: Increase to 3 or configure via a `config.yaml` field; implement exponential back-off once `IsRateLimitExceeded` is implemented.

## Fragile Areas

**`compareSecgroupRule` uses O(n²) nested loop:**
- Files: `internal/usecase/nsg_uc/nsg_uc.go:268-308`
- Why fragile: Compares current and new rules with two nested loops. With many rules (e.g., many allowed CIDRs) this degrades. The `Description` field comparison is explicitly commented out, meaning description changes are silently ignored.
- Safe modification: Any change to comparison logic must be mirrored in the corresponding integration test `internal/usecase/nsg_uc/nsg_uc_integration_test.go`.
- Test coverage: Integration test exists but uses real API by default; no unit test covers the diff logic directly.

**`buildSubnetAndZone` relies on existing subnet ID across both Service and Ingress controllers:**
- Files: `internal/usecase/service_uc/build_lbc.go:125`, `internal/usecase/ingress_uc/build_lbc.go` (equivalent path)
- Why fragile: Subnet and zone selection logic is duplicated across `service_uc` and `ingress_uc`. Tests for this path exist (`build_lbc_subnet_zone_test.go` in each package) but cover the happy path; edge cases around zone resolution with existing LBC IDs are partially tested.
- Safe modification: Changes to subnet/zone logic must be applied to both packages and both test files.
- Test coverage: Tests exist but `ingress_uc` and `service_uc` top-level use-case test files (`ingress_uc_test.go`, `service_uc_test.go`) are empty stubs.

**GLBC deletion path: `DeleteGlobalLoadBalancer` vs `DeleteLoadBalancer` confusion:**
- Files: `internal/usecase/glbc_uc/delete_lb.go`, test at `internal/usecase/glbc_uc/delete_lb_test.go:15`
- Why fragile: BUG-03 (documented in the test) identified that the wrong delete method was called. The fix was applied, but the path that calls `DeleteGlobalLoadBalancer` in `deleteLoadBalancerWhenNotEmpty` is only covered by a regression test; the `canDeleteWholeLoadBalancer` path is not separately tested.
- Safe modification: Any refactoring of `deleteLoadBalancer` must keep the `DeleteGlobalLoadBalancer` / `DeleteLoadBalancer` distinction intact.
- Test coverage: Regression test exists; broader deletion flow coverage is incomplete.

## Scaling Limits

**Single-instance listing cap at 1000 load balancers:**
- Current capacity: Up to 1000 LBs per `ListLoadBalancers` call.
- Limit: Projects with >1000 LBs will have reconciliation silently skip LBs beyond the first page.
- Scaling path: Implement pagination in `ListLoadBalancers` / `ListGlobalLoadBalancers` repository methods.

## Dependencies at Risk

**`github.com/cuongpiger/joat` (personal utility library):**
- Risk: The URL normalisation utility `cuongpigerutils.NormalizeURL` is sourced from a personal GitHub repository with no organisational backing. The repo could be deleted or go unmaintained.
- Impact: Build failure if the repository becomes unavailable; no alternative normalisation exists in the codebase.
- Migration plan: Inline the `NormalizeURL` function directly or replace with `strings.TrimSuffix` / `url.Parse`.

**`github.com/anngdinh/operator-helper` (personal utility library):**
- Risk: Contextual logging via `contexts.NewContext(ctx).Log()` is sourced from a personal repository. Same supply-chain risk as above.
- Impact: Logging is used pervasively across all repository methods; replacing it would touch many files.
- Migration plan: Vendor the package or replace with a standard `logr` / `slog` approach.

**`wait.PollUntilContextTimeout` marked `staticcheck` deprecated:**
- Risk: `crdhelpers.go` suppresses the `staticcheck` warning with `//nolint:staticcheck`. The function is deprecated in newer versions of `k8s.io/apimachinery`.
- Impact: Will break when the function is eventually removed from the library.
- Migration plan: Replace with `wait.PollUntilContextCancel` as recommended by the deprecation notice.

## Missing Critical Features

**No rate-limit handling:**
- Problem: The cloud API can return rate-limit errors, but `IsRateLimitExceeded` is a stub returning `false`. There is no back-off beyond the controller-runtime default requeue delay.
- Blocks: Safe operation at high reconcile frequency; responsible behaviour when multiple clusters share the same VNGCloud project quota.

**No inter-LBC listener/pool inlined at creation for INTERVPC:**
- Problem: `buildCreateLoadBalancerRequest` for INTERVPC LBs skips attaching the initial listener and pool to the creation request.
- Blocks: Efficient single-request provisioning of INTERVPC load balancers.

## Test Coverage Gaps

**`service_glb_uc` package has zero tests:**
- What's not tested: All of `build_glbc.go`, `build_listener.go`, `build_pool.go`, and `service_glb_uc.go` in `internal/usecase/service_glb_uc/`.
- Files: `internal/usecase/service_glb_uc/`
- Risk: Any regression in the Service GLB build/deploy flow goes undetected.
- Priority: High

**`service_uc` and `ingress_uc` top-level use-case test files are empty stubs:**
- What's not tested: The `run()` orchestration logic, error propagation between build phases, and annotation-ignore flow in `internal/usecase/service_uc/service_uc.go` and `internal/usecase/ingress_uc/ingress_uc.go`.
- Files: `internal/usecase/service_uc/service_uc_test.go`, `internal/usecase/ingress_uc/ingress_uc_test.go`
- Risk: High-level orchestration changes break silently.
- Priority: High

**Cilium native-routing security group test disabled:**
- What's not tested: Multi-target-port Cilium native routing path in `buildDefaultSecurityGroupRule` with both instance and pod CIDRs.
- Files: `internal/usecase/service_uc/build_sec_group_test.go:301`
- Risk: Cilium-native-routing security group rules could be miscalculated when multiple ports are exposed.
- Priority: Medium

**Ingress health-check protocol defaulting not covered:**
- What's not tested: The `// TODO review it` path in `internal/usecase/ingress_uc/build_pool.go:233` where TCP is hardcoded as the default health-check protocol for L7 pools. No test verifies that HTTP health-check settings are correctly ignored when the protocol falls back to TCP.
- Files: `internal/usecase/ingress_uc/build_pool.go:233`
- Risk: Ingress L7 pools may have incorrect health-check configuration under some annotation combinations.
- Priority: Medium

---

*Concerns audit: 2026-03-31*

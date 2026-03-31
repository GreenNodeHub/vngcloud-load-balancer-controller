# Phase 4: Add Test from Controller Use VngCloud Mock Repository - Research

**Researched:** 2026-03-16
**Domain:** Go controller integration testing — envtest + Ginkgo/Gomega + MockProvider
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Use envtest + Ginkgo/Gomega + vngcloud_mocks.MockProvider — matches the existing NSG controller test pattern
- Full controller manager setup (SetupWithManager + start manager in goroutine) — tests create GlobalLoadBalancerConfig objects, manager triggers reconcile automatically
- Extend the existing MockProvider in vngcloud_mock_glb.go — do NOT create a separate mock
- **Create flow**: Create a GLBC object with minimal spec (1 pool, 1 pool member group with 1 member, 1 listener referencing that pool) → reconcile creates LB/pools/listeners in mock backend → verify status is populated
- **Delete flow (full)**: Controller owns the LB exclusively → delete GLBC object → reconcile calls DeleteGlobalLoadBalancer → verify mock backend is empty
- **Delete flow (partial)**: Shared LB scenario — some resources belong to another GLBC → delete one GLBC → only its resources are cleaned up, shared resources preserved
- Happy paths only — no error case tests (API failures better tested at usecase level)
- Implement `UpdateGlobalPool` and `UpdateGlobalListener` in MockProvider (currently return ErrorNotImplemented) — needed for reconcile flows
- Mock fixture data (sample GLBC CR specs, expected states) goes in the vngcloud_mocks package — reusable, not inline in test files

### Claude's Discretion
- Exact GLBC spec field values for test fixtures
- Helper functions for creating/verifying GLBC objects in envtest
- Assertion granularity (check specific status fields vs overall status)
- Suite/test file organization within glbc_controller directory

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

---

## Summary

Phase 4 adds controller-level integration tests for the GLBC reconciler using the established NSG controller test pattern. The work is purely additive: three new test files in `internal/controller/glbc_controller/`, two mock method implementations in `vngcloud_mock_glb.go`, and one fixtures file in `vngcloud_mocks/`.

The NSG controller test suite is a near-complete template. The GLBC suite setup follows exactly the same structure — `envtest.Environment`, Ginkgo `BeforeSuite/AfterSuite`, controller manager startup in a goroutine, and `MockProvider` injection. The critical difference is that GLBC has an `initDone` atomic flag gating reconcile dispatch; the test suite must wait for it to flip `true` before creating objects, or use `Eventually` with long enough timeouts.

The two unimplemented mock methods (`UpdateGlobalPool`, `UpdateGlobalListener`) both follow the pattern established by `PatchGlobalPoolMembers` — parse the request, mutate in-memory state, call `updatingGlobalStatus`, launch `go readyGlobalAfterTime`. These are required because the reconcile update path calls them on second reconcile (when pool or listener already exists and needs no field changes other than identity).

**Primary recommendation:** Copy suite_test.go from nsg_controller verbatim, replace NSG-specific wiring with GLBC wiring, define one minimal GLBC CR fixture in vngcloud_mocks, implement UpdateGlobalPool/UpdateGlobalListener in mock, write three `It` blocks for Create/DeleteFull/DeletePartial.

---

## Standard Stack

### Core (all already present in go.mod)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/onsi/ginkgo/v2` | v2.x | BDD test framework | Already used by NSG tests |
| `github.com/onsi/gomega` | v1.x | Assertion library | Paired with Ginkgo; `Eventually` for async |
| `sigs.k8s.io/controller-runtime/pkg/envtest` | matched to ctrl-runtime | In-process kube API server | Standard for controller integration tests |
| `sigs.k8s.io/controller-runtime` | project version | Manager, reconciler infrastructure | Core dependency |
| `github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository/vngcloud_repo/vngcloud_mocks` | local | MockProvider | In-repo, matches project pattern |

No new dependencies required.

**Installation:** Nothing — all imports already in go.mod.

---

## Architecture Patterns

### Recommended File Structure
```
internal/controller/glbc_controller/
├── glbc_controller.go          (existing — do NOT modify)
├── suite_test.go               (NEW — envtest bootstrap, manager start)
├── glbc_controller_test.go     (NEW — Describe/It test blocks)
└── helpers_test.go             (NEW — timeout consts, clean-state helpers)

internal/repository/vngcloud_repo/vngcloud_mocks/
├── vngcloud_mock_glb.go        (MODIFY — implement UpdateGlobalPool, UpdateGlobalListener)
└── mock_glbc_fixtures.go       (NEW — MockGLBCSpec, MockGLBCSpec2 for shared-LB scenario)
```

### Pattern 1: Suite Bootstrap (suite_test.go)

Exact copy of NSG suite, substituting GLBC-specific wiring.

```go
// Source: modeled directly on internal/controller/nsg_controller/suite_test.go
package glbc_controller

import (
    // same imports as NSG suite, plus:
    "github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/glbc_uc"
    "github.com/vngcloud/vngcloud-load-balancer-controller/pkg/glbc"
)

var (
    ctx               context.Context
    cancel            context.CancelFunc
    testEnv           *envtest.Environment
    cfg               *rest.Config
    k8sClient         client.Client
    mockGLBCReconciler *GlobalLoadBalancerConfigReconciler
    vngcloudRepo      *vngcloud_mocks.MockProvider
)

// mockConfig uses GlobalLoadBalancerOpts (not LoadBalancerOpts)
var mockConfig = &config.Config{
    GlobalLoadBalancerOpts: config.GlobalLoadBalancerOpts{
        DefaultL4PackageName:      "glb-small",   // matches MockProvider.ListGlobalPackages
        DefaultPoolAlgorithm:      "ROUND_ROBIN",
        DefaultHealthyThreshold:   3,
        DefaultUnhealthyThreshold: 3,
        DefaultInterval:           30,
        DefaultTimeout:            5,
        DefaultTimeoutClient:      50,
        DefaultTimeoutMember:      50,
        DefaultTimeoutConnection:  5,
        DefaultAllowedCidrs:       "0.0.0.0/0",
    },
}

func TestGLBCController(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "GlobalLoadBalancerConfig Controller Suite")
}
```

Key differences from NSG suite:
- Use `GlobalLoadBalancerOpts` not `LoadBalancerOpts` in mockConfig (see `deploy_lb.go:buildPackageId`)
- The package name `"glb-small"` must match what `MockProvider.ListGlobalPackages` returns (line 32 of mock: `{ID: "glb-pkg-001", Name: "glb-small"}`)
- Wire `glbc_uc.NewGlobalLoadBalancerConfigUseCase` instead of nsg_uc
- Wire `glbc.NewGlobalLoadBalancerConfigUtils(domain.GlbcFinalizer)` instead of nsg utils
- Do NOT create mock nodes (GLBC tests don't need them)
- No `maxConcurrentReconciles` field in NSG reconciler was passed as `1`, GLBC constructor has no such param — pass only what `NewGlobalLoadBalancerConfigReconciler` accepts

### Pattern 2: initDone Gate

The GLBC controller has `r.initDone` atomic flag. `SetupWithManager` registers an init runnable that calls `InitGlobalLoadBalancerConfigUseCase` before setting the flag. This completes immediately (the impl returns nil). However, the manager must be started and the runnable must run before reconcile can proceed.

```go
// In BeforeSuite: after go k8sManager.Start(ctx)
// No explicit wait needed — Eventually in tests absorbs the brief delay.
// Use timeout*2 minimum for first assertions (same as NSG pattern).
```

### Pattern 3: Test Structure (glbc_controller_test.go)

```go
package glbc_controller

var _ = Describe("GlobalLoadBalancerConfig Controller", func() {
    AfterEach(func() {
        expectNoGLBs()
        expectNoGLBCObjects()
    })

    Context("Create flow", func() {
        It("should create LB, pool, and listener in mock backend and populate status", func() {
            glbc := newGLBCResource("test-glbc", "default")
            Expect(k8sClient.Create(ctx, glbc)).Should(Succeed())

            Eventually(func(g Gomega) {
                updated := &v1alpha1.GlobalLoadBalancerConfig{}
                g.Expect(k8sClient.Get(ctx, client.ObjectKey{...}, updated)).Should(Succeed())
                // verify status.loadBalancerId is set
                // verify status.createdPools has 1 entry
                // verify status.createdListeners has 1 entry
                // verify Ready condition = True
                // verify mock backend has 1 LB, 1 pool, 1 listener
            }, timeout*4, interval).Should(Succeed())

            Expect(k8sClient.Delete(ctx, glbc)).Should(Succeed())
        })
    })

    Context("Delete flow (full — controller owns LB)", func() {
        It("should delete LB entirely from mock backend", func() { ... })
    })

    Context("Delete flow (partial — shared LB)", func() {
        It("should clean up only owned resources, leave shared ones", func() { ... })
    })
})
```

### Pattern 4: UpdateGlobalPool Implementation

`UpdateGlobalPool` receives an `IUpdateGlobalPoolRequest`. The concrete type is `*global.UpdateGlobalPoolRequest` which carries `Algorithm` and `HealthMonitor`.

```go
// Source: inspected glb_pool_requests.go UpdateGlobalPoolRequest
func (m *MockProvider) UpdateGlobalPool(ctx context.Context, glbID, poolID string, opt global.IUpdateGlobalPoolRequest) error {
    logger := contexts.NewContext(ctx).Log()
    logger.Infof("%s Request update global pool %s of load balancer %s", domain.RequestIcon, poolID, glbID)
    req := opt.ToRequestBody().(*global.UpdateGlobalPoolRequest)

    m.mu.Lock()
    defer m.mu.Unlock()
    for _, p := range m.globalPools {
        if p.lbID == glbID && p.ID == poolID {
            p.Algorithm = string(req.Algorithm)
            if req.HealthMonitor != nil {
                health := req.HealthMonitor.ToRequestBody().(*global.GlobalHealthMonitorRequest)
                p.Health.HealthyThreshold   = health.HealthyThreshold
                p.Health.UnhealthyThreshold = health.UnhealthyThreshold
                p.Health.IntervalTime       = health.Interval
                p.Health.Timeout            = health.Timeout
                p.Health.Protocol           = string(health.HealthCheckProtocol)
                p.Health.HTTPMethod         = (*string)(health.HttpMethod)
                p.Health.HTTPVersion        = (*string)(health.HttpVersion)
                p.Health.Path               = health.Path
                p.Health.SuccessCode        = health.SuccessCode
                p.Health.DomainName         = health.DomainName
                p.Health.UpdatedAt          = time.Now().Format(time.RFC3339)
            }
            p.UpdatedAt = time.Now().Format(time.RFC3339)
            m.updatingGlobalStatus(glbID)    // must be called before unlock
            go m.readyGlobalAfterTime(glbID)
            return nil
        }
    }
    logger.Error("Pool not found")
    return domain.ErrorNotFound
}
```

Note: `updatingGlobalStatus` also acquires `m.mu` — call it AFTER releasing the lock, or restructure to avoid deadlock. The existing `PatchGlobalPoolMembers` calls `m.updatingGlobalStatus` and `go m.readyGlobalAfterTime` OUTSIDE the lock. Follow the same pattern.

### Pattern 5: UpdateGlobalListener Implementation

`UpdateGlobalListener` receives an `IUpdateGlobalListenerRequest`. The concrete type used by the controller is `*global.UpdateGlobalListenerRequest` (used directly in `deploy_listener.go:buildListenerUpdateRequest`).

```go
// Source: inspected glb_listener_request.go, deploy_listener.go
func (m *MockProvider) UpdateGlobalListener(ctx context.Context, glbID, listenerID string, opt global.IUpdateGlobalListenerRequest) error {
    logger := contexts.NewContext(ctx).Log()
    logger.Infof("%s Request update global listener %s of load balancer %s", domain.RequestIcon, listenerID, glbID)
    req := opt.ToRequestBody().(*global.UpdateGlobalListenerRequest)

    found := false
    m.mu.Lock()
    for _, l := range m.globalListeners {
        if l.lbID == glbID && l.ID == listenerID {
            l.AllowedCidrs      = req.AllowedCidrs
            l.TimeoutClient     = req.TimeoutClient
            l.TimeoutMember     = req.TimeoutMember
            l.TimeoutConnection = req.TimeoutConnection
            l.GlobalPoolID      = req.GlobalPoolId
            l.Headers           = req.Headers
            l.UpdatedAt         = time.Now().Format(time.RFC3339)
            found = true
            break
        }
    }
    m.mu.Unlock()

    if !found {
        logger.Error("Listener not found")
        return domain.ErrorNotFound
    }
    m.updatingGlobalStatus(glbID)
    go m.readyGlobalAfterTime(glbID)
    return nil
}
```

Note: `UpdateGlobalListenerRequest` (from `deploy_listener.go`) is used as `*global.UpdateGlobalListenerRequest` (direct struct, not via constructor). Field names: `AllowedCidrs string`, `TimeoutClient int`, `TimeoutMember int`, `TimeoutConnection int`, `GlobalPoolId string`, `Headers *string`. The `Headers` field on the entity is `*string` (comma-joined), matching what the controller stores.

### Pattern 6: GLBC Fixtures (mock_glbc_fixtures.go)

```go
// Source: modeled on mocks.go MockNode pattern
package vngcloud_mocks

import (
    global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/utils/ptr"
    "github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// MockGLBCMinimalSpec returns the minimal GLBC spec for testing:
// 1 pool, 1 pool member group with 1 member, 1 listener referencing that pool.
func MockGLBCMinimalSpec() v1alpha1.GlobalLoadBalancerConfigSpec {
    return v1alpha1.GlobalLoadBalancerConfigSpec{
        Name: "test-glb",
        Type: global.GlobalLoadBalancerTypeLayer4,
        GlobalPools: []v1alpha1.GlobalPool{
            {
                Name:     "test-pool",
                Protocol: global.GlobalPoolProtocolTCP,
                HealthMonitor: v1alpha1.GlobalPoolHealthMonitor{
                    Protocol: global.GlobalPoolHealthCheckProtocolTCP,
                },
                PoolMembers: []v1alpha1.GlobalPoolMember{
                    {
                        Name:   "test-pool-member-group",
                        Region: "HCM-03",
                        VpcId:  MockNetID,
                        Type:   global.GlobalPoolMemberTypePrivate,
                        Members: []v1alpha1.GlobalMember{
                            {
                                Name:     "test-member",
                                Address:  "10.0.0.1",
                                SubnetID: MockSubnetID,
                                Port:     8080,
                            },
                        },
                    },
                },
            },
        },
        GlobalListeners: []v1alpha1.GlobalListener{
            {
                Name:            "test-listener",
                Protocol:        global.GlobalListenerProtocolTCP,
                ProtocolPort:    80,
                DefaultPoolName: ptr.To("test-pool"),
            },
        },
    }
}
```

### Pattern 7: Shared-LB Delete (Partial) Scenario

The partial delete test requires two GLBC objects sharing the same LB. The second GLBC must use `Spec.LoadBalancerId` referencing the LB created by the first GLBC.

Sequence:
1. Create GLBC-A → wait for status.LoadBalancerId to be set
2. Create GLBC-B with `Spec.LoadBalancerId = *glbcA.Status.LoadBalancerId`, different pool/listener names
3. Wait for GLBC-B status to populate
4. Delete GLBC-A → wait for GLBC-A to be fully deleted (no finalizer)
5. Assert: mock backend still has GLBC-B's pool and listener; GLBC-A's pool and listener are gone; LB still exists

Note: `canDeleteWholeLoadBalancer` (in `delete_lb.go`) checks that all current listeners/pools on the LB are owned by the deleting GLBC's status. If GLBC-B's listeners exist, GLBC-A cannot delete the whole LB and falls through to `deleteLoadBalancerWhenNotEmpty`.

### Anti-Patterns to Avoid

- **Sharing vngcloudRepo state between test runs without reset**: `AfterEach` must verify mock state is empty. If a test leaves orphan resources, subsequent tests will fail unpredictably.
- **Creating fixtures inline in test body**: Specs say fixtures go in vngcloud_mocks package. Keep test bodies readable.
- **Not waiting for initDone**: The reconciler queues a 1-second requeue until `initDone` is true. `Eventually` with `timeout*2` (10 seconds) is sufficient; `timeout` (5 seconds) may be too short for the first object.
- **Using `time.Sleep` instead of `Eventually`**: Controller reconcile is async. Always use `Eventually(..., timeout, interval)`.
- **Hardcoding pool/listener IDs**: MockProvider assigns random IDs (`randID()`). Always read IDs from status, not from expectation.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Async K8s reconcile assertion | Polling loop + time.Sleep | `Eventually(func(g Gomega){...}, timeout, interval)` | Gomega handles retry + failure reporting |
| Envtest setup | Custom fake API server | `envtest.Environment` from controller-runtime | Standard, maintained, handles CRD loading |
| Mock backend | New mock struct | Extend `MockProvider` in vngcloud_mock_glb.go | Consistency, reuse existing in-memory state |

---

## Common Pitfalls

### Pitfall 1: mockConfig Uses Wrong Config Section
**What goes wrong:** The GLBC reconciler's `buildPackageId` reads `cfg.GlobalLoadBalancerOpts.DefaultL4PackageName`, not `cfg.LoadBalancerOpts.DefaultL4PackageName`. If you copy the NSG mockConfig and only set `LoadBalancerOpts`, the package resolution falls back to hardcoded ID `"pkg-b02e62ab..."` which doesn't exist in `MockProvider.ListGlobalPackages`. This causes reconcile to return `NoNeedRequeue` error and tests to time out waiting for status.
**How to avoid:** Set `GlobalLoadBalancerOpts.DefaultL4PackageName = "glb-small"` (matches mock's package name at line 32 of vngcloud_mock_glb.go).

### Pitfall 2: UpdateGlobalPool/UpdateGlobalListener Deadlock
**What goes wrong:** Both `updatingGlobalStatus` and the caller hold `m.mu`. If `UpdateGlobalPool` acquires the lock and then calls `updatingGlobalStatus` (which also acquires the lock), the goroutine deadlocks.
**How to avoid:** Follow the pattern in `PatchGlobalPoolMembers` — unlock before calling `updatingGlobalStatus`. Structure as: lock → mutate → unlock → call updatingGlobalStatus → go readyGlobalAfterTime.

### Pitfall 3: initDone Gate Causes First Reconcile Requeue
**What goes wrong:** The reconciler returns `{RequeueAfter: 1s}` until `initDone` is true. This means the first reconcile of any object may do nothing. Tests using `timeout*1` (5 seconds) may see the object created but not reconciled yet.
**How to avoid:** Use `timeout*2` (10 seconds) minimum for all `Eventually` blocks that check the results of the first reconcile. The init runnable runs immediately after the manager starts, so the delay is usually under 100ms, but the test framework itself has overhead.

### Pitfall 4: Shared-LB Scenario Race on LB ID
**What goes wrong:** GLBC-B is created with `Spec.LoadBalancerId` referencing GLBC-A's LB. If you create GLBC-B before GLBC-A's status has the `loadBalancerId` written, GLBC-B's reconcile treats it as a new LB and creates a second one.
**How to avoid:** Wait for GLBC-A's `Status.LoadBalancerId` to be non-nil before creating GLBC-B. Read GLBC-A's status in an `Eventually` block, then create GLBC-B with the retrieved ID.

### Pitfall 5: Delete Leaves Finalizer → AfterEach Loop
**What goes wrong:** If the delete reconcile fails (e.g., UpdateGlobalPool returns ErrorNotImplemented during the delete path's redundant-listener cleanup), the finalizer is never removed. The object stays in K8s indefinitely. `AfterEach`'s `expectNoGLBCObjects` will hang.
**How to avoid:** Implement UpdateGlobalPool and UpdateGlobalListener BEFORE writing tests. Verify they compile and return nil for valid inputs. This is a prerequisite task.

### Pitfall 6: maxConcurrentReconciles Field Missing
**What goes wrong:** The NSG reconciler constructor takes `maxConcurrentReconciles int` as the last parameter. The GLBC reconciler constructor (`NewGlobalLoadBalancerConfigReconciler`) does NOT include this parameter (it defaults to 0, which controller-runtime treats as 1).
**How to avoid:** Do not pass `1` as the last argument when constructing the GLBC reconciler — it would be misinterpreted as `reconcileCounters`.

---

## Code Examples

### Suite Setup Wiring (GLBC-specific portion)

```go
// Source: internal/controller/nsg_controller/suite_test.go adapted for GLBC
glbcUtils := glbc.NewGlobalLoadBalancerConfigUtils(domain.GlbcFinalizer)
glbcUseCase := glbc_uc.NewGlobalLoadBalancerConfigUseCase(
    mockConfig,
    k8sRepo,
    vngcloudRepo,
)
mockGLBCReconciler = NewGlobalLoadBalancerConfigReconciler(
    k8sManager.GetClient(),
    k8sManager.GetScheme(),
    glbcUseCase,
    k8sManager.GetEventRecorderFor("glbc-controller"),
    finalizerManager,
    glbcUtils,
    lbcMetricsCollector,
    reconcileCounters,
)
err = mockGLBCReconciler.SetupWithManager(ctx, k8sManager)
Expect(err).ToNot(HaveOccurred())
```

### Verifying LB Exists in Mock Backend

```go
// Source: pattern from nsg_controller_test.go verifySecurityGroupAttachedToServers
func verifyGLBExists(g Gomega, lbID string) {
    lb, err := vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbID)
    g.Expect(err).ShouldNot(HaveOccurred())
    g.Expect(lb).ShouldNot(BeNil())
}

func verifyGLBNotExists(g Gomega, lbID string) {
    _, err := vngcloudRepo.GetGlobalLoadBalancerByID(ctx, lbID)
    g.Expect(err).Should(MatchError(domain.ErrorNotFound))
}
```

### Helper: newGLBCResource

```go
// Source: pattern from nsg_controller_test.go newNodeSecurityGroupResource
func newGLBCResource(name, namespace string) *v1alpha1.GlobalLoadBalancerConfig {
    spec := vngcloud_mocks.MockGLBCMinimalSpec()
    return &v1alpha1.GlobalLoadBalancerConfig{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        TypeMeta: metav1.TypeMeta{
            Kind:       "GlobalLoadBalancerConfig",
            APIVersion: "vks.vngcloud.vn/v1alpha1",
        },
        Spec: spec,
    }
}
```

### Clean-State Helpers (helpers_test.go)

```go
// Source: pattern from internal/controller/nsg_controller/helpers_test.go
const (
    timeout  = time.Second * 5
    interval = time.Millisecond * 250
)

func expectNoGLBs() {
    Eventually(func() int {
        lbs, err := vngcloudRepo.ListGlobalLoadBalancers(ctx, nil)
        if err != nil || lbs == nil { return -1 }
        return len(lbs.Items)
    }, timeout*4, interval).Should(Equal(0), "Expected no GLBs in mock backend")
}

func expectNoGLBCObjects() {
    Eventually(func() int {
        list := &v1alpha1.GlobalLoadBalancerConfigList{}
        if err := k8sClient.List(ctx, list); err != nil { return -1 }
        return len(list.Items)
    }, timeout*4, interval).Should(Equal(0), "Expected no GLBC objects in K8s")
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Unit tests with controller.Reconcile() called directly | envtest + full manager + automatic reconcile dispatch | This phase | Tests exercise the full goroutine-based reconcile loop including requeueing |
| Stub mock returning hardcoded responses | Stateful in-memory mock simulating real backend | Already in MockProvider | Tests verify actual state transitions |

---

## Open Questions

1. **UpdateGlobalListenerRequest concrete type assertion**
   - What we know: `deploy_listener.go` constructs the update request as `&global.UpdateGlobalListenerRequest{...}` (direct struct literal, not via constructor). The mock type-asserts `opt.ToRequestBody().(*global.UpdateGlobalListenerRequest)`.
   - What's unclear: Whether `ToRequestBody()` returns the pointer itself (it does — `return s`). Verified: `ToRequestBody() interface{} { return s }` is on the concrete type.
   - Recommendation: Type assertion is safe. Use `opt.ToRequestBody().(*global.UpdateGlobalListenerRequest)`.

2. **Region value for MockGLBCMinimalSpec PoolMember**
   - What we know: `GlobalPoolMember.Region` is a required string. The MockProvider doesn't validate it — it's passed through.
   - What's unclear: What string value to use that looks realistic.
   - Recommendation: Use `"HCM-03"` — matches the zone naming convention visible in `mocks.go` (`common.HCM_03_1A_ZONE`).

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Ginkgo v2 + Gomega (already installed) |
| Config file | No separate config — `go test ./internal/controller/glbc_controller/...` |
| Quick run command | `go test ./internal/controller/glbc_controller/... -v -timeout 120s` |
| Full suite command | `go test ./internal/controller/... -v -timeout 120s` |

### Phase Requirements → Test Map

No formal REQ-IDs were assigned. Phase scope from CONTEXT.md:

| Scenario | Behavior | Test Type | Automated Command | File Exists? |
|----------|----------|-----------|-------------------|-------------|
| Create flow | GLBC CR → mock backend LB+pool+listener created, status populated | integration | `go test ./internal/controller/glbc_controller/... -run "Create" -v` | Wave 0 |
| Delete full | GLBC deleted, sole owner → DeleteGlobalLoadBalancer called, mock empty | integration | `go test ./internal/controller/glbc_controller/... -run "Delete.*full" -v` | Wave 0 |
| Delete partial | GLBC deleted, shared LB → only owned resources removed | integration | `go test ./internal/controller/glbc_controller/... -run "Delete.*partial" -v` | Wave 0 |
| UpdateGlobalPool | pool update request mutates mock in-memory state correctly | unit (implicit — exercised by 2nd reconcile in create flow) | same suite | Wave 0 |
| UpdateGlobalListener | listener update request mutates mock state | unit (implicit) | same suite | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/controller/glbc_controller/... -v -timeout 120s`
- **Per wave merge:** `go test ./internal/controller/... -v -timeout 120s`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/controller/glbc_controller/suite_test.go` — covers envtest bootstrap
- [ ] `internal/controller/glbc_controller/glbc_controller_test.go` — covers all three scenario tests
- [ ] `internal/controller/glbc_controller/helpers_test.go` — covers timeout consts + clean-state helpers
- [ ] `internal/repository/vngcloud_repo/vngcloud_mocks/mock_glbc_fixtures.go` — covers MockGLBCMinimalSpec fixture
- [ ] `UpdateGlobalPool` implementation in `vngcloud_mock_glb.go` — unblocks create+update paths
- [ ] `UpdateGlobalListener` implementation in `vngcloud_mock_glb.go` — unblocks listener update path

---

## Sources

### Primary (HIGH confidence)
- Directly read: `internal/controller/nsg_controller/suite_test.go` — exact envtest setup template
- Directly read: `internal/controller/nsg_controller/nsg_controller_test.go` — test block pattern
- Directly read: `internal/controller/nsg_controller/helpers_test.go` — helper pattern
- Directly read: `internal/repository/vngcloud_repo/vngcloud_mocks/vngcloud_mock_glb.go` — existing mock methods, mutex pattern, field structures
- Directly read: `internal/controller/glbc_controller/glbc_controller.go` — initDone gate, constructor signature
- Directly read: `internal/usecase/glbc_uc/glbc_uc.go` — constructor, config field usage
- Directly read: `internal/usecase/glbc_uc/deploy_lb.go` — buildPackageId reads GlobalLoadBalancerOpts
- Directly read: `internal/usecase/glbc_uc/deploy_pool.go` — UpdateGlobalPool call path
- Directly read: `internal/usecase/glbc_uc/deploy_listener.go` — UpdateGlobalListener call path, concrete request struct
- Directly read: `internal/usecase/glbc_uc/delete_lb.go` — canDeleteWholeLoadBalancer logic, partial-delete path
- Directly read: `api/v1alpha1/globalloadbalancerconfig_types.go` — all spec/status field names
- Directly read: `pkg/config/config.go` — GlobalLoadBalancerOpts struct fields
- Directly read: SDK `glb/v1/irequest.go` — IUpdateGlobalPoolRequest, IUpdateGlobalListenerRequest interfaces
- Directly read: SDK `glb/v1/glb_pool_requests.go` — UpdateGlobalPoolRequest concrete struct
- Directly read: SDK `glb/v1/glb_listener_request.go` — UpdateGlobalListenerRequest concrete struct

### Secondary (MEDIUM confidence)
None required — all findings from direct code inspection.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all libraries already present, confirmed in imports
- Architecture: HIGH — NSG test suite is a near-perfect template; GLBC controller structure confirmed
- Mock implementation: HIGH — concrete request structs and mutex pattern confirmed from source
- Pitfalls: HIGH — derived from direct reading of production code logic (initDone, config fields, mutex, delete paths)

**Research date:** 2026-03-16
**Valid until:** 2026-04-16 (stable codebase)

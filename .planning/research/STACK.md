# Stack Research: GLBC Operator

**Confidence:** HIGH — based on direct codebase reads

## Current Stack (Fixed)

No new libraries needed for this milestone.

- **Go** 1.22.5
- **controller-runtime** v0.19.3
- **k8s client-go/api/apimachinery** v0.31.3
- **vngcloud-go-sdk/v2** v2.17.4
- **Testing:** testify v1.10.0, ginkgo v2.22.0, gomega v1.36.0
- **Mocks:** mockery/testify style (current), golang/mock (deprecated, archived)

## Key Constraints

- Every VNG Cloud API mutation MUST be followed by `WaitGlobalLoadBalancerActive` — hard API constraint
- Status updates must go through `PatchMutateStatusGlobalLoadBalancerConfig`, never `Status().Update()` on stale objects
- New mocks should use mockery/testify style, not extend golang/mock

## Confirmed Bugs (Not Library Gaps)

- `convertMember` missing `SubnetID` — code bug
- `canDeleteWholeListener` returns `ErrorNotImplemented` — code bug
- `DeleteLoadBalancer` called instead of `DeleteGlobalLoadBalancer` in shared-LB cleanup

## Recommendations

- Standardize on mockery/testify for new test mocks
- No dependency changes needed for this milestone
- `go.uber.org/mock` v0.4.0 already in go.mod as replacement for deprecated golang/mock

---
*Research: 2026-03-15*

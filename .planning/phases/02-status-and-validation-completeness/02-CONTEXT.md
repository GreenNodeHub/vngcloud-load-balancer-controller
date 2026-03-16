# Phase 2: Status and Validation Completeness - Context

**Gathered:** 2026-03-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Activate pool member status tracking on pool creation (STAT-01) and implement headers comparison in listener update detection (STAT-02). This phase completes the two stubbed status/validation gaps left after Phase 1 bug fixes.

</domain>

<decisions>
## Implementation Decisions

### Status Tracking Scope
- Uncomment 4 of the 5 commented-out status calls:
  1. `statusAddPoolMember` on pool creation (deploy_pool.go:53-56)
  2. `CreatedPoolMembers` population on pool creation (deploy_pool.go:64-65)
  3. ~~`statusAddPool` on pool update (deploy_pool.go:78-81)~~ — **EXCEPTION**: Leave commented. Research found `deployPoolMembers` already calls `statusUpdatePoolMember` on the update path, making this redundant and causing spurious status patches every reconcile.
  4. `statusAddListener` on listener creation (deploy_listener.go:54-57)
  5. `statusAddListener` on listener update (deploy_listener.go:86-89)
- If `statusAddListener` or `statusAddPool` functions don't exist in the GLBC use case, implement them following the LBC pattern

### Status Save Timing
- Save status immediately after each resource creation/update, BEFORE calling `WaitGlobalLoadBalancerActive`
- **EXCEPTION for pool member status (STAT-01):** `ListGlobalPoolMembers` requires the pool to be active, so the status save must come AFTER `WaitGlobalLoadBalancerActive` on the pool creation path. This is a structural constraint — the user decision applies to listener paths where resource IDs are available immediately.
- If reconcile crashes mid-way, status should reflect what was actually created

### Status Mutation Method
- Use the existing status mutation helper function (not direct Status().Update or Patch calls)

### Pool Member Status Data
- Store both IDs and address:port tuples for each member in status
- The existing `CreatedGlobalPoolMember` type has `Id`, `Name`, and `CreatedMembers` (with `GlobalMember` entries) — populate all fields from the API create response

### Headers Comparison (STAT-02)
- Join spec `Headers []string` with comma separator (sorted, lowercase), compare against entity `Headers *string`
- Comparison is case-insensitive (normalize to lowercase before comparing)
- When headers drift is detected, set `updateOptions.Headers` to the joined spec headers and trigger update
- Treat nil entity headers as empty string — if spec has headers and entity is nil, that's drift triggering an update

### Error Handling on Status Save
- If status write fails, return error and requeue (don't log-and-continue)
- Next reconcile will see the resource already exists via API and just update status
- No special conditions or events for status write failures — standard error propagation and logging is sufficient

### Claude's Discretion
- Exact implementation of the status mutation helper calls
- How to extract member data (IDs, addresses, ports) from the CreateGlobalPool API response
- Order of status saves within the deploy flow
- Any necessary type conversions between API response types and CRD status types

</decisions>

<specifics>
## Specific Ideas

- "There is a function that helps mutate the status — use it" (existing helper, don't create new K8s API call patterns)
- The TODO at deploy_listener.go:214 says "compare headers, somewhere is []string, somewhere is *string" — this is the exact type mismatch to resolve

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `deploy_pool.go` lines 53-56, 64-65, 78-81: Commented-out status calls with correct function signatures
- `deploy_listener.go` lines 54-57, 86-89: Commented-out statusAddListener calls
- `CreatedGlobalPool` type (api/v1alpha1): Already has `CreatedPoolMembers` field ready for population
- `CreatedGlobalPoolMember` type: Has `Id`, `Name`, `CreatedMembers []GlobalMember` fields
- Existing status mutation helper function in the codebase

### Established Patterns
- LBC use case has `statusAddListener` and related functions — follow same pattern for GLBC
- `buildListenerUpdateRequest` already handles cidrs, timeouts, pool ID comparisons — headers comparison follows same pattern
- Status is saved via subresource helper, not direct K8s client calls

### Integration Points
- `deploy_pool.go:deployPool()` — pool creation path where status must be saved
- `deploy_listener.go:deployListener()` — listener creation/update path where status must be saved
- `deploy_listener.go:buildListenerUpdateRequest()` line 214 — where headers comparison must be added
- `api/v1alpha1/globalloadbalancerconfig_types.go` — CRD types for status fields

</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 02-status-and-validation-completeness*
*Context gathered: 2026-03-16*

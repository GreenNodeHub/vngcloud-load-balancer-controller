# Phase 1: P0 Bug Fixes - Context

**Gathered:** 2026-03-15
**Status:** Ready for planning

<domain>
## Phase Boundary

Fix the four critical bugs (BUG-01 through BUG-04) blocking the delete cycle and preventing status from stabilizing. The reconciler's create, update, and delete flows must complete without code-level failures; status must stabilize after each reconcile.

</domain>

<decisions>
## Implementation Decisions

### Delete Ownership Model (BUG-01: canDeleteWholeListener)

The delete flow operates bottom-up across four levels: **Listener → Pool → PoolMember → Member**.

**Cross-cluster constraint:** GLBCs can run in different clusters sharing the same LB. Cannot query other GLBC specs — ownership decisions must rely ONLY on:
1. This GLBC's spec
2. This GLBC's status (created resources)
3. Current API state from VNG Cloud

**Complete delete flow when a GLBC is deleted:**

1. **canDeleteWholeLoadBalancer**: Check if ALL current listeners and ALL current pools (with full member-level verification) are in this GLBC's status.
   - If YES → `DeleteGlobalLoadBalancer()` (delete entire GLB)
   - If NO → partial cleanup (step 2)

2. **For each listener this GLBC created** (in status.createdListeners):
   - **canDeleteWholeListener**: Bottom-up check — does this GLBC own ALL members in the listener's referenced pool?
     - If YES → delete listener + delete its pool (cascade delete)
     - If NO → listener and pool are kept. Go to member-level cleanup (step 3)

3. **For each PoolMember group** in the pool:
   - If ALL members in the group were created by this GLBC → delete the entire PoolMember group
   - If only SOME members are owned → patch: remove only this GLBC's members, keep others

4. **After cleanup**: if LB has no resources left → delete the empty GLB

**Key rules:**
- Pool deletion is tied to listener deletion — never independent. A pool only gets deleted when its listener is also being deleted.
- Empty PoolMember groups (zero members after cleanup) → delete the group
- Pool stays even if empty of members, as long as its listener stays (another GLBC may add members later)

### Resource Matching Keys

Consistent pattern across all levels: **ID first, fallback to Name**.

| Level | Primary Key | Fallback Key |
|-------|------------|--------------|
| Listener | ID match against status.createdListeners | Name |
| Pool | ID match against status.createdPools | Name |
| PoolMember | ID match against status.createdPoolMembers | Name |
| Member | **Address + Port** (no ID, no name fallback) | — |

Member matching is different: members are identified by **Address + Port** tuple, not by name or ID.

### Delete API Call (BUG-03)

When the LB is empty after cleanup, call `DeleteGlobalLoadBalancer()` instead of the current `DeleteLoadBalancer()`. This is a simple method swap — the existing ownership check logic before the call is correct.

### Listener Name in Status (BUG-04)

Populate the `Name` field in `CreatedGlobalListener` returned by `deployListener`. Straightforward — extract from the API response.

### SubnetID in convertMember (BUG-02)

Add `SubnetID` field to the `convertMember` function output. Currently missing, causing infinite spurious member patches when SubnetID is unchanged in spec.

### Claude's Discretion
- Member-level verification implementation details in `canDeleteWholeLoadBalancer`
- Exact patch request construction for partial member removal
- Error handling and logging within the new ownership checks
- Order of operations within the partial cleanup path

</decisions>

<specifics>
## Specific Ideas

- "Imagine 2 GLBCs with same LB ID, same listener port 80 referencing the same pool, but different pool members. Expected: 1 GLB, 1 listener, 1 pool, merged members. When deleting 1 GLBC, just delete its pool members — keep pool, keep listener because there are members not created by this GLBC."
- The hierarchy is: Pool → PoolMember (group) → Member (individual entry). PoolMember is a container/group, Member is each IP:port entry within it.
- Cross-cluster sharing is a real scenario — cannot assume we can query other GLBC specs.

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `canDeleteWholeLoadBalancer()` (delete_lb.go:78-156): Existing listener/pool ownership check — needs member-level verification uncommented and completed
- `canDeleteWholePool()` (delete_pool.go:108-159): Existing member ownership check — needs key matching updated from name to address+port
- `deleteRedundantListeners()` (delete_listener.go:14-70): Existing framework for iterating listeners — needs `canDeleteWholeListener` implementation
- `deleteRedundantPools()` (delete_pool.go:13-103): Existing framework for pool cleanup with bulk patch support
- `convertMember()` (deploy_pool_member.go:305-314): Member conversion — needs SubnetID field added
- `statusAddListener()` (status.go:12-29): Status update for listeners — needs Name population

### Established Patterns
- Bottom-up ownership checking pattern already exists in `canDeleteWholePool` — extend to listener level
- Bulk patch via `PatchGlobalPoolMembers` for partial member removal
- Status tracking via `status.Created*` fields for all resource levels
- Per-LB mutex locking for concurrent modification safety

### Integration Points
- `delete_lb.go:deleteLoadBalancer()` — main delete flow orchestrator, calls each level
- `delete_listener.go:canDeleteWholeListener()` — currently returns ErrorNotImplemented, needs full implementation
- `delete_lb.go:66` — calls `DeleteLoadBalancer()`, needs swap to `DeleteGlobalLoadBalancer()`
- `deploy_pool_member.go:305-314` — `convertMember()` missing SubnetID
- `status.go` / `deploy_listener.go` — listener name not populated in CreatedGlobalListener

</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 01-p0-bug-fixes*
*Context gathered: 2026-03-15*

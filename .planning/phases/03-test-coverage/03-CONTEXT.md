# Phase 3: Test Coverage - Context

**Gathered:** 2026-03-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Add unit tests for pool member 3-way merge edge cases (TEST-01). The merge logic in `mergePoolMembers` already exists — this phase only adds tests, no production code changes.

</domain>

<decisions>
## Implementation Decisions

### Test Scope — 4 Required Edge Cases
- **Add new member**: member in spec but not in current → appears in merge output
- **Remove deleted member**: member in current AND in created, but not in spec → removed from merge output
- **Update existing member**: member in both current and spec, but with different field values → spec values used. Test ALL comparable fields (Weight, BackupRole, SubnetID, MonitorPort, Port, Address). If a spec field is nil, keep the current value (no update needed); only update when spec value is not nil and different from current.
- **Preserve manually-added member**: member in current but NOT in created and NOT in spec → preserved in merge output (added via portal, not by this GLBC)

### Test Structure
- Table-driven: one `TestMergePoolMembers` function with `t.Run` sub-tests for each case
- Use `testify/assert` (matches existing codebase pattern)
- No additional edge cases beyond the 4 required (empty inputs, duplicates, combinations are out of scope)

### Coverage Boundary
- Test `mergePoolMembers` only — it's a pure function, no mocks needed
- Do NOT test `buildPatchGlobalPoolMemberRequest` or `deployPoolMembers` (those require mock setup, out of scope for TEST-01)
- Test file: `deploy_pool_member_test.go` (add to existing file alongside `TestConvertMember_IncludesSubnetID`)

### Claude's Discretion
- Exact member field values in test fixtures
- Helper functions for creating test members if needed
- Assertion granularity (check full slice vs individual members)

</decisions>

<specifics>
## Specific Ideas

- `mergePoolMembers` signature: `(createdMembers, currentMembers, poolMemberSpec []v1alpha1.GlobalMember) []v1alpha1.GlobalMember`
- `checkIfPoolMemberExist` matches on Address, Port, Weight, BackupRole, SubnetID, MonitorPort (6 fields)
- The merge logic: keep current if in spec OR not in created, then add new spec members not already merged

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets
- `deploy_pool_member_test.go`: Already has `TestConvertMember_IncludesSubnetID` with table-driven pattern
- `v1alpha1.GlobalMember` type: struct with Name, Address, Port, BackupRole, Weight (*int), MonitorPort (*int), SubnetID
- `mergePoolMembers` (deploy_pool_member.go:245-263): Pure function, testable without mocks

### Established Patterns
- Table-driven tests with `t.Run` sub-tests
- `testify/assert` for assertions
- `*int` pointer fields for optional values (Weight, MonitorPort)

### Integration Points
- `deploy_pool_member_test.go` — existing test file to add to
- `mergePoolMembers` is a method on `defaultModelDeployTask` but only uses the context parameter (ignored), so a minimal task struct suffices

</code_context>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 03-test-coverage*
*Context gathered: 2026-03-16*

# Phase 3: Test Coverage - Research

**Researched:** 2026-03-16
**Domain:** Go unit testing — table-driven tests for a pure merge function
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **Test Scope:** Exactly 4 edge cases — add new member, remove deleted member, update existing member, preserve manually-added member
- **Test Structure:** Table-driven `TestMergePoolMembers` function with `t.Run` sub-tests
- **Assertion library:** `testify/assert` (matches existing codebase pattern)
- **No additional edge cases:** Empty inputs, duplicates, combinations are out of scope
- **Coverage boundary:** Test `mergePoolMembers` only — no mocks, no `buildPatchGlobalPoolMemberRequest`, no `deployPoolMembers`
- **Test file:** `deploy_pool_member_test.go` — add alongside existing `TestConvertMember_IncludesSubnetID`

### Claude's Discretion
- Exact member field values in test fixtures
- Helper functions for creating test members if needed
- Assertion granularity (check full slice vs individual members)

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| TEST-01 | Unit tests for pool member 3-way merge edge cases (add, remove, update, preserve manual members) | `mergePoolMembers` is a pure function at line 245 of `deploy_pool_member.go`, fully testable without any mocks. All 4 edge cases mapped to concrete fixture designs below. |
</phase_requirements>

---

## Summary

Phase 3 adds `TestMergePoolMembers` to the existing `deploy_pool_member_test.go` file. The target function, `mergePoolMembers`, is a pure function — it takes three `[]v1alpha1.GlobalMember` slices and returns one. No mocks, interfaces, or dependency injection are required. The existing test file already demonstrates the exact table-driven pattern to follow.

The most important research finding is a pointer comparison trap in `checkIfPoolMemberExist`: the function compares `Weight` and `MonitorPort` fields with `==` on `*int` values, which compares pointer addresses, not dereferenced values. Two separately-allocated `*int` pointers with the same integer value will NOT match. Test fixtures must account for this by using `nil` for `Weight` and `MonitorPort` when the intent is "same member", or by sharing the exact same pointer variable.

**Primary recommendation:** Use `nil` for `Weight` and `MonitorPort` in all test fixtures unless the test case specifically exercises those fields; keep "different field" tests to non-pointer fields (Address, Port, BackupRole, SubnetID).

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` | stdlib | Test runner, `t.Run` sub-tests | Built-in Go testing package |
| `github.com/stretchr/testify/assert` | already in go.mod | Assertion helpers | Already used in `TestConvertMember_IncludesSubnetID` |

### No New Dependencies
No new packages required. The existing `go.mod` already has `testify`. The test file already imports `testing` and `testify/assert`.

**Run tests:**
```bash
go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers -v
```

**Full package suite:**
```bash
go test ./internal/usecase/glbc_uc/... -v
```

---

## Architecture Patterns

### Existing File Structure
```
internal/usecase/glbc_uc/
├── deploy_pool_member.go        # mergePoolMembers at line 245
├── deploy_pool_member_test.go   # ADD TestMergePoolMembers here
└── deploy_lb.go                 # defaultModelDeployTask struct definition
```

### Pattern: Table-Driven Tests with t.Run (exact existing pattern)

**What:** Struct slice of named test cases, iterated with `for _, tt := range tests { t.Run(tt.name, ...) }`
**When to use:** Always — matches existing `TestConvertMember_IncludesSubnetID` pattern in the same file

**Existing pattern to follow verbatim:**
```go
// Source: internal/usecase/glbc_uc/deploy_pool_member_test.go (lines 11-81)
func TestMergePoolMembers(t *testing.T) {
    tests := []struct {
        name            string
        createdMembers  []v1alpha1.GlobalMember
        currentMembers  []v1alpha1.GlobalMember
        poolMemberSpec  []v1alpha1.GlobalMember
        wantLen         int
        // additional assertion fields per case
    }{
        // ... cases ...
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            task := &defaultModelDeployTask{}
            result := task.mergePoolMembers(context.Background(), tt.createdMembers, tt.currentMembers, tt.poolMemberSpec)
            // assert ...
        })
    }
}
```

### Pattern: Calling mergePoolMembers in tests

`mergePoolMembers` is a method on `*defaultModelDeployTask`. The struct requires `logger`, `cfg`, `vngcloudRepo`, `k8sRepo`, `lbConfig` fields (from `deploy_lb.go:20`). However, `mergePoolMembers` ignores its receiver entirely — it only calls the package-level `checkIfPoolMemberExist` function. A zero-value struct works:

```go
task := &defaultModelDeployTask{}
result := task.mergePoolMembers(context.Background(), created, current, spec)
```

`context.Background()` satisfies the `context.Context` parameter (which is also ignored via `_ context.Context`).

### Pattern: Constructing GlobalMember Fixtures

`v1alpha1.GlobalMember` struct fields:
- `Address string` — use distinct IPs like `"10.0.0.1"`
- `BackupRole bool`
- `Description *string` — NOT compared by `checkIfPoolMemberExist`, safe to omit
- `MonitorPort *int` — POINTER compared by address (see pitfalls)
- `Name string` — NOT compared by `checkIfPoolMemberExist`, safe to vary
- `Port int`
- `SubnetID string`
- `Weight *int` — POINTER compared by address (see pitfalls)

**Safe fixture pattern** (avoid pointer address trap):
```go
// Use nil for pointer fields when testing "same member" matching
member := v1alpha1.GlobalMember{
    Address:    "10.0.0.1",
    Port:       8080,
    BackupRole: false,
    SubnetID:   "subnet-abc",
    // Weight: nil, MonitorPort: nil — safe for matching
}

// To share a pointer (same value, guaranteed same address):
w := 1
member.Weight = &w
// Now copy the SAME pointer to other fixtures that should match
```

### Anti-Patterns to Avoid
- **Separate `*int` allocations for "matching" members:** `Weight: intPtr(1)` in one fixture and `Weight: intPtr(1)` in another will NOT match because `checkIfPoolMemberExist` compares `*int == *int` (pointer address, not value). These are distinct pointers.
- **Helper `intPtr(n int) *int` with duplicate calls for matching fields:** Only safe if matching members use `nil` for those fields, or share the same variable.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Assertions | Manual `if result != expected` | `assert.Equal`, `assert.Len` from testify | Already imported, readable diff output |
| Test runner | Custom test harness | stdlib `testing` + `t.Run` | Matches existing pattern exactly |

---

## Common Pitfalls

### Pitfall 1: `*int` Pointer Comparison in checkIfPoolMemberExist
**What goes wrong:** `checkIfPoolMemberExist` (line 284 in `deploy_pool_member.go`) uses `r.Weight == member.Weight` which compares pointer addresses. Two distinct `*int` variables with value `1` will NOT match.
**Why it happens:** Go's `==` on pointer types checks address equality, not value equality.
**How to avoid:** Use `nil` for `Weight` and `MonitorPort` in fixtures where matching is required. Only use non-nil pointers when the test case explicitly tests that these fields differ (and you want non-matching behavior).
**Warning signs:** Test for "remove deleted member" or "preserve manual member" fails unexpectedly — current member not matching created/spec as intended.

**Concrete illustration:**
```go
// WRONG — these two members will NOT match in checkIfPoolMemberExist
created := v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 8080, Weight: intPtr(1)}
current := v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 8080, Weight: intPtr(1)}
// created.Weight != current.Weight (different pointers)

// CORRECT — share the nil zero value, or share the same pointer variable
w := 1
created := v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 8080, Weight: &w}
current := v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 8080, Weight: &w}
// OR: both nil
created := v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 8080}
current := v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 8080}
```

### Pitfall 2: Missing context.Background() import
**What goes wrong:** `mergePoolMembers` takes `context.Context` as first arg; tests must import `"context"`.
**How to avoid:** Add `"context"` to imports in `deploy_pool_member_test.go`. The existing file does NOT import it yet — it must be added.

### Pitfall 3: Confusing "update" semantics
**What goes wrong:** The "update existing member" case is NOT about mutating a member in place. The merge function either keeps or discards currentMembers based on whether they match spec. An "updated" member means: current has member X, spec has member Y (same address but different port/subnet/backupRole), so X is dropped (not in spec match) and Y is added as a new spec member.
**How to avoid:** Design the "update" case with current member having different non-pointer fields (Address+Port OR BackupRole OR SubnetID) from spec member. Do not try to test Weight/MonitorPort changes as the trigger for "different from spec" due to the pointer pitfall above.

### Pitfall 4: Description and Name not compared
**What goes wrong:** Over-specifying fixtures by varying Name or Description expecting it to affect merging behavior.
**Why it happens:** `checkIfPoolMemberExist` only compares: Address, Port, Weight, BackupRole, SubnetID, MonitorPort. Name and Description are not part of the match key.
**How to avoid:** Use Name only for human-readable fixture labels. Do not rely on Name differences to make members "not match."

---

## Code Examples

### The 4 Test Cases — Fixture Design

```go
// Source: analysis of mergePoolMembers (deploy_pool_member.go:245-263)
//         and checkIfPoolMemberExist (deploy_pool_member.go:280-292)

// Case 1: Add new member
// Spec has member NOT in current → appears in merge output
createdMembers: []v1alpha1.GlobalMember{},
currentMembers: []v1alpha1.GlobalMember{},
poolMemberSpec: []v1alpha1.GlobalMember{
    {Address: "10.0.0.1", Port: 8080, SubnetID: "subnet-a"},
},
// expect: result contains the spec member

// Case 2: Remove deleted member
// Member is in current AND in created, but NOT in spec → removed
memberA := v1alpha1.GlobalMember{Address: "10.0.0.2", Port: 8080, SubnetID: "subnet-b"}
createdMembers: []v1alpha1.GlobalMember{memberA},  // same pointer fields
currentMembers: []v1alpha1.GlobalMember{memberA},
poolMemberSpec: []v1alpha1.GlobalMember{},
// expect: result is empty

// Case 3: Update existing member (old version dropped, new spec version added)
oldMember := v1alpha1.GlobalMember{Address: "10.0.0.3", Port: 8080, SubnetID: "subnet-c"}
newMember := v1alpha1.GlobalMember{Address: "10.0.0.3", Port: 9090, SubnetID: "subnet-c"}  // Port changed
createdMembers: []v1alpha1.GlobalMember{oldMember},
currentMembers: []v1alpha1.GlobalMember{oldMember},
poolMemberSpec: []v1alpha1.GlobalMember{newMember},
// expect: result contains newMember (Port=9090), NOT oldMember (Port=8080)

// Case 4: Preserve manually-added member
// Member is in current, NOT in created, NOT in spec → preserved
manualMember := v1alpha1.GlobalMember{Address: "10.0.0.4", Port: 8080, SubnetID: "subnet-d"}
createdMembers: []v1alpha1.GlobalMember{},  // not in created (added via portal)
currentMembers: []v1alpha1.GlobalMember{manualMember},
poolMemberSpec: []v1alpha1.GlobalMember{},
// expect: result contains manualMember (preserved)
```

### Full Function Skeleton
```go
// Source: pattern from TestConvertMember_IncludesSubnetID + mergePoolMembers analysis
func TestMergePoolMembers(t *testing.T) {
    tests := []struct {
        name           string
        created        []v1alpha1.GlobalMember
        current        []v1alpha1.GlobalMember
        spec           []v1alpha1.GlobalMember
        wantLen        int
        wantContains   []v1alpha1.GlobalMember  // members expected in result
        wantExcludes   []v1alpha1.GlobalMember  // members expected absent from result
    }{
        {
            name: "add new member",
            // ...
        },
        {
            name: "remove deleted member",
            // ...
        },
        {
            name: "update existing member",
            // ...
        },
        {
            name: "preserve manually-added member",
            // ...
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            task := &defaultModelDeployTask{}
            result := task.mergePoolMembers(context.Background(), tt.created, tt.current, tt.spec)
            assert.Len(t, result, tt.wantLen)
            for _, m := range tt.wantContains {
                assert.True(t, checkIfPoolMemberExist(result, &m), "expected %+v in result", m)
            }
            for _, m := range tt.wantExcludes {
                assert.False(t, checkIfPoolMemberExist(result, &m), "expected %+v absent from result", m)
            }
        })
    }
}
```

**Note on assertion approach:** Using `checkIfPoolMemberExist` for assertions is safe only when fixtures avoid the `*int` pointer pitfall. Alternatively, use `assert.Equal` on specific members by index when output order is deterministic (it is: currentMembers first, then new spec members).

---

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| No tests for mergePoolMembers | TestMergePoolMembers with 4 edge cases | Catches regression in 3-way merge logic |

---

## Open Questions

1. **Assertion granularity: `checkIfPoolMemberExist` vs `assert.Equal` by index**
   - What we know: `mergePoolMembers` output order is deterministic (current members first, new spec members appended)
   - What's unclear: Whether the planner prefers index-based equality checks or membership checks
   - Recommendation: Claude's discretion — either works. Index-based `assert.Equal` is simpler when the slice order is known. Membership-based `checkIfPoolMemberExist` is more robust if order ever changes.

2. **Helper function for member construction**
   - What we know: All 4 cases use nil Weight/MonitorPort to avoid pointer pitfall
   - Recommendation: A simple inline struct literal suffices; no helper needed unless fixtures get verbose.

---

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `testify/assert` v1.x (already in go.mod) |
| Config file | none — uses `go test` directly |
| Quick run command | `go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers -v` |
| Full suite command | `go test ./internal/usecase/glbc_uc/... -v` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TEST-01 | mergePoolMembers adds new spec member | unit | `go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers/add_new_member -v` | Wave 0 |
| TEST-01 | mergePoolMembers removes deleted member (in created+current, not in spec) | unit | `go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers/remove_deleted_member -v` | Wave 0 |
| TEST-01 | mergePoolMembers replaces updated member (old dropped, new spec added) | unit | `go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers/update_existing_member -v` | Wave 0 |
| TEST-01 | mergePoolMembers preserves manually-added member (in current, not in created/spec) | unit | `go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers/preserve_manually-added_member -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/usecase/glbc_uc/... -run TestMergePoolMembers -v`
- **Per wave merge:** `go test ./internal/usecase/glbc_uc/... -v`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `TestMergePoolMembers` function in `internal/usecase/glbc_uc/deploy_pool_member_test.go` — covers all 4 sub-cases of TEST-01
- [ ] `"context"` import added to `deploy_pool_member_test.go` (not currently present)

---

## Sources

### Primary (HIGH confidence)
- Direct code read: `internal/usecase/glbc_uc/deploy_pool_member.go` lines 245–292 — mergePoolMembers and checkIfPoolMemberExist implementation
- Direct code read: `internal/usecase/glbc_uc/deploy_pool_member_test.go` — existing table-driven pattern
- Direct code read: `api/v1alpha1/globalloadbalancerconfig_types.go` lines 250–282 — GlobalMember struct with `*int` pointer fields
- Live test run: `go test ./internal/usecase/glbc_uc/... -run TestConvertMember -v` — confirmed existing tests pass
- Go language spec (pointer comparison): Verified via runtime test that `*int == *int` compares addresses, not dereferenced values

### Secondary (MEDIUM confidence)
- CONTEXT.md 03-CONTEXT.md — user-locked decisions on scope and structure

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — testify already in go.mod, existing test file confirmed
- Architecture: HIGH — pattern directly observed in same file
- Pitfalls: HIGH — pointer comparison behavior verified by runtime experiment

**Research date:** 2026-03-16
**Valid until:** 2026-04-16 (stable Go testing patterns)

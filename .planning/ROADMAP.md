# Roadmap: GLBC Operator — Verify and Fix Milestone

## Overview

The GLBC reconciler is substantially complete but has four P0 bugs that collectively block the delete cycle and prevent status from stabilizing. This milestone fixes those bugs, completes the two stubbed status/validation gaps, and adds test coverage for the pool member merge logic. Three tightly-scoped phases ordered by dependency depth: fix blockers first, then complete correctness gaps, then verify with tests.

## Phases

- [x] **Phase 1: P0 Bug Fixes** - Fix the four critical bugs blocking the delete and status flows (completed 2026-03-15)
- [x] **Phase 2: Status and Validation Completeness** - Implement stubbed status tracking and listener update headers comparison (completed 2026-03-16)
- [x] **Phase 3: Test Coverage** - Add unit tests verifying pool member 3-way merge edge cases (completed 2026-03-16)

## Phase Details

### Phase 1: P0 Bug Fixes
**Goal**: The reconciler's create, update, and delete flows complete without code-level failures; status stabilizes after each reconcile
**Depends on**: Nothing (first phase)
**Requirements**: BUG-01, BUG-02, BUG-03, BUG-04
**Success Criteria** (what must be TRUE):
  1. A GLBC object with a redundant listener (no longer in spec) reaches `Ready` and the listener is deleted — no `ErrorNotImplemented` in logs
  2. Pool members survive a re-reconcile without spurious bulk-patch calls when SubnetID is unchanged in spec
  3. A GLBC deletion where the controller owns the LB exclusively calls `DeleteGlobalLoadBalancer` (not `DeleteLoadBalancer`)
  4. `status.createdListeners` entries have their `name` field populated after a successful `deployListener` run
**Plans:** 2/2 plans complete

Plans:
- [ ] 01-01-PLAN.md — Fix trivial bugs: convertMember SubnetID (BUG-02), DeleteGlobalLoadBalancer/Pool API calls (BUG-03), listener Name in status (BUG-04)
- [ ] 01-02-PLAN.md — Implement canDeleteWholeListener ownership check (BUG-01) + uncomment canDeleteWholeLoadBalancer member verification

### Phase 2: Status and Validation Completeness
**Goal**: Pool member IDs are recorded in status on first pool creation, and listener updates correctly detect header drift
**Depends on**: Phase 1
**Requirements**: STAT-01, STAT-02
**Success Criteria** (what must be TRUE):
  1. After a pool is created, `status.createdPools[*].createdMembers` contains the IDs of all members returned by the create response — no second reconcile needed to populate them
  2. Changing a listener's allowed headers in the GLBC spec triggers a listener update API call on the next reconcile
**Plans:** 1/1 plans complete

Plans:
- [ ] 02-01-PLAN.md — Activate pool member status tracking on pool creation (STAT-01) + implement headers comparison in buildListenerUpdateRequest (STAT-02)

### Phase 3: Test Coverage
**Goal**: The pool member 3-way merge logic is verified by unit tests covering all edge cases, giving confidence the merge produces correct bulk-patch payloads
**Depends on**: Phase 2
**Requirements**: TEST-01
**Success Criteria** (what must be TRUE):
  1. Unit tests cover: add new member, remove deleted member, update existing member, and preserve manually-added member not in spec
  2. All tests pass with `go test ./internal/usecase/glbc_uc/...`
  3. No regressions in existing tests after the fix changes from Phases 1 and 2
**Plans:** 1/1 plans complete

Plans:
- [ ] 03-01-PLAN.md — Add TestMergePoolMembers with 4 table-driven sub-tests for 3-way merge edge cases

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. P0 Bug Fixes | 2/2 | Complete    | 2026-03-15 |
| 2. Status and Validation Completeness | 1/1 | Complete   | 2026-03-16 |
| 3. Test Coverage | 1/1 | Complete   | 2026-03-16 |

### Phase 4: Add test from controller use vngcloud mock repository

**Goal:** Controller-level integration tests exercise the full GLBC reconcile loop (envtest + manager) with mocked VNG Cloud backend, verifying create, full-delete, and shared-LB partial-delete flows
**Requirements**: CTRL-TEST-01, CTRL-TEST-02, CTRL-TEST-03, CTRL-TEST-04, CTRL-TEST-05
**Depends on:** Phase 3
**Plans:** 2 plans

Plans:
- [ ] 04-01-PLAN.md — Implement UpdateGlobalPool/UpdateGlobalListener in MockProvider + create GLBC test fixtures
- [ ] 04-02-PLAN.md — Create GLBC controller test suite (envtest setup + create/delete-full/delete-partial scenarios)

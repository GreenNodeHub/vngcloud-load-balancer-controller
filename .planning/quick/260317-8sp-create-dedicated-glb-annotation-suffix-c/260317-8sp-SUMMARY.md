---
quick_task: 260317-8sp
subsystem: annotations, service_glb_uc
tags: [annotations, constants, service-glb, refactor]
dependency_graph:
  requires: []
  provides: [dedicated-glb-annotation-suffix-constants]
  affects: [pkg/annotations/constants.go, internal/usecase/service_glb_uc/build_glbc.go, internal/usecase/service_glb_uc/build_pool.go]
tech_stack:
  added: []
  patterns: [dedicated-constant-namespacing]
key_files:
  created: []
  modified:
    - pkg/annotations/constants.go
    - internal/usecase/service_glb_uc/build_glbc.go
    - internal/usecase/service_glb_uc/build_pool.go
key_decisions:
  - "SuffixGLB* constants use identical string values to shared Suffix* counterparts — zero behavioral change, pure naming isolation"
metrics:
  duration: 5min
  completed: 2026-03-17T06:25:39Z
  tasks_completed: 2
  files_modified: 3
---

# Quick Task 260317-8sp: Create Dedicated GLB Annotation Suffix Constants Summary

**One-liner:** Added 22 dedicated SuffixGLB* annotation constants to pkg/annotations/constants.go and updated all service_glb_uc references to use them exclusively.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Add SuffixGLB* constants to pkg/annotations/constants.go | 1d8ebf4 | pkg/annotations/constants.go |
| 2 | Update service_glb_uc to use SuffixGLB* constants | fd3e52b | build_glbc.go, build_pool.go |

## What Was Done

### Task 1: Add SuffixGLB* constants

Added 22 new constants in `pkg/annotations/constants.go` inside a new comment block after `SuffixGLBEnable`. All constants have the same string values as their shared `Suffix*` counterparts:

- `SuffixGLBLoadBalancerID`, `SuffixGLBLoadBalancerName`, `SuffixGLBPackageID`, `SuffixGLBDescription`
- `SuffixGLBTargetType`, `SuffixGLBHealthcheckPort`, `SuffixGLBPoolAlgorithm`
- `SuffixGLBIdleTimeoutClient/Member/Connection`
- `SuffixGLBInboundCIDRs`, `SuffixGLBEnableProxyProtocol`
- `SuffixGLBHealthcheckProtocol`, `SuffixGLBHealthyThresholdCount`, `SuffixGLBUnhealthyThresholdCount`
- `SuffixGLBHealthcheckIntervalSeconds`, `SuffixGLBHealthcheckTimeoutSeconds`
- `SuffixGLBHealthcheckHttpMethod`, `SuffixGLBHealthcheckPath`, `SuffixGLBSuccessCodes`
- `SuffixGLBHealthcheckHttpVersion`, `SuffixGLBHealthcheckHttpDomainName`

Total: 23 SuffixGLB* constants (SuffixGLBEnable + 22 new).

### Task 2: Update service_glb_uc

Replaced all `annotations.Suffix*` references in both files:

- `build_glbc.go`: 4 replacements (LoadBalancerID, LoadBalancerName, PackageID, Description)
- `build_pool.go`: 30 replacements across 19 distinct constant names

No logic was changed. `go build ./...` passes.

## Deviations from Plan

None — plan executed exactly as written.

## Verification

- `grep -c "SuffixGLB" pkg/annotations/constants.go` returns 23
- `grep -n "annotations\.Suffix[^G]"` on both files returns no matches
- `go build ./...` passes with no errors

## Self-Check: PASSED

- pkg/annotations/constants.go: modified, contains 23 SuffixGLB constants
- internal/usecase/service_glb_uc/build_glbc.go: modified, uses SuffixGLB* exclusively
- internal/usecase/service_glb_uc/build_pool.go: modified, uses SuffixGLB* exclusively
- Commits 1d8ebf4 and fd3e52b exist in git log

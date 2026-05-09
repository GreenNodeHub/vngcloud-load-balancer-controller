# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build / test / lint

```bash
make test            # full unit + envtest (regenerates manifests + deepcopy first)
make lint lint-fix   # golangci-lint
make run             # run controller locally against current kubeconfig
make generate manifests   # regenerate deepcopy + CRDs after editing api/v1alpha1/*.go
```

`make test` writes its envtest binaries via `setup-envtest` into `bin/`. PRs target the `main` branch.

### Running a single test

The controller suites use Ginkgo. To focus on one spec without running 250s of envtest setup:

```bash
go clean -testcache && go test -v ./internal/controller/networking/... \
  -ginkgo.focus="When node status changes from not ready to ready"
```

### Test parallelism gotcha

`go test ./...` runs packages in parallel by default. Multiple controller packages spin up their own envtest API server, and **they collide on shared resources** — you'll see ~12 specs all fail with timeouts that look real but aren't. Always use `go test -p=1 ./...` when running the full suite, or test packages individually.

### `make manifests` scans `.worktrees/`

`controller-gen` is invoked with `paths="./..."` which Go interprets to include `.worktrees/` (gitignored but visible to Go tooling). If a worktree contains code with kubebuilder markers, `make manifests` will produce diffs in `config/rbac/role.yaml` and create stray CRD YAMLs unrelated to your change. **Always check `git status` after `make manifests` and revert unrelated files.**

## Architecture

Layered **Controller → UseCase → Repository** (see `README.md` for the diagram). Concretely:

- `internal/controller/{core,networking,lbc_controller,nsg_controller,glbc_controller,...}` — reconcilers and event handlers. Filter watch events here (e.g. skip status-only updates) to avoid hot-looping.
- `internal/usecase/{ingress_uc,service_uc,lbc_uc,nsg_uc,glbc_uc,...}` — business logic. `ingress_uc` and `service_uc` create/maintain `LoadBalancerConfig` (LBC) and `NodeSecurityGroup` (NSG) CRs as side-effect of reconciling Ingress/Service objects.
- `internal/repository/k8s_repo` — Kubernetes API access (uses `client.PatchMutateStatusObject` / `PatchMutateObject` helpers with optimistic locking).
- `internal/repository/vngcloud_repo` — VNGCloud SDK calls. Has `vngcloud_mocks/` for envtests.
- `api/v1alpha1` — CRD type definitions. Edit these, then `make generate manifests`.

Controller wiring lives in `cmd/main.go` — that's where reconcilers, eventhandlers, repositories, and use cases are constructed and `SetupWithManager`'d.

### Spec/Status contract (don't break this — it caused a production reconcile loop)

The LBC controller is the owner of `LoadBalancerConfig` and **must only write to Status, never to Spec**. The Ingress/Service controllers (acting as the "user" / owner-controller) own Spec — they translate annotations into desired state.

If you find code in `internal/usecase/lbc_uc/` patching `Spec.*`, that's a bug. Mirror cloud-observed values into `Status.*` (see `syncLBCStatusFromLoadBalancer` in `deploy_lb.go` for the pattern). Eventhandlers in both controllers filter status-only updates to avoid loops — preserve those filters when editing.

The same Spec/Status discipline applies to the other CRDs (`NodeSecurityGroup`, `GlobalLoadBalancerConfig`).

### Two CRD locations stay in sync via `make manifests`

CRDs live in two places:

1. `config/crd/bases/*.yaml` — for `kubectl apply -f` / Helm install.
2. `pkg/k8s/apis/vks.vngcloud.vn/crds/*.yaml` — **embedded into the binary** (`//go:embed` in `register.go`) and applied by the controller at startup.

`make manifests` regenerates (1) and then automatically syncs to (2) via the `sync-embedded-crds` target. If you ever edit the bases YAML by hand, run `make sync-embedded-crds` to propagate.

If they fall out of sync, the API server will silently strip new fields when the controller patches Status — the bug looks fixed locally but breaks once deployed.

## Annotations & feature flags

User-facing knobs are `vks.vngcloud.vn/*` annotations on Ingress/Service. Constants in `pkg/annotations/constants.go`; parsing in `pkg/annotations/parser.go` (`ParseStringAnnotation` returns `bool`, **not** `(bool, error)` — different from `ParseBoolAnnotation`).

Controllers can be individually disabled via `--disable-{service,ingress,load-balancer-config,node-security-group}-controller` flags (see commented-out flags in `make run`).

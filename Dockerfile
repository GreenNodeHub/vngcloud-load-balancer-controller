# Build the manager binary
# Pinned to the patch, not the 1.26 line. The floor itself comes from the toolchain directive
# in go.mod, which the Makefile restores with `override GOTOOLCHAIN = auto` - the official
# images set GOTOOLCHAIN=local, which would otherwise ignore it. Two reasons the pin stays on
# top of that: `go mod download` below runs outside make and so has only this tag to go on,
# and a floating tag makes the compiler that produced a given image unknowable afterwards.
# Removing either one leaves a gap; they are not two spellings of the same guard.
FROM golang:1.26.9 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod CGO_ENABLED=0 go mod download

# Copy the go source
COPY Makefile ./
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY pkg/ pkg/
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN --mount=type=cache,target=/go/pkg/mod make build-pro CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} VERSION=${VERSION} COMMIT=${COMMIT}

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]

# Build the manager binary
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT

# The SDK is a private module: fetch it straight from git, bypassing the public proxy and
# checksum database.
ENV GOPRIVATE=github.com/GreenNodeHub/*

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
#
# Credentials for the private SDK come from the BuildKit secret gh_token (a GitHub token with read
# access to GreenNodeHub/vngcloud-go-sdk), e.g. `docker build --secret id=gh_token,env=GH_TOKEN`.
# They are handed to git through GIT_CONFIG_* for this RUN only, so the token is never written to
# a layer or to /root/.gitconfig. Without the secret the download fails on the private module.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=gh_token,required=false \
    if [ -s /run/secrets/gh_token ]; then \
      export GIT_CONFIG_COUNT=1 \
        GIT_CONFIG_KEY_0="url.https://x-access-token:$(cat /run/secrets/gh_token)@github.com/GreenNodeHub/.insteadOf" \
        GIT_CONFIG_VALUE_0="https://github.com/GreenNodeHub/"; \
    else \
      echo "gh_token secret not provided; private module github.com/GreenNodeHub/vngcloud-go-sdk cannot be downloaded" >&2; \
    fi; \
    CGO_ENABLED=0 go mod download

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
#
# Every module comes from the cache filled above. GOPROXY=off plus GONOPROXY=none forbid any fetch
# (GOPROXY=off alone still lets go clone GOPRIVATE modules straight from git, without credentials),
# so a missing module fails here with "module lookup disabled by GOPROXY=off".
RUN --mount=type=cache,target=/go/pkg/mod GOPROXY=off GONOPROXY=none GOFLAGS=-mod=readonly \
    make build-pro CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} VERSION=${VERSION} COMMIT=${COMMIT}

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]

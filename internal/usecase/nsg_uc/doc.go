// Package nsg_uc implements the NodeSecurityGroup reconciliation use-case.
//
// # Running tests
//
// The integration test (nsg_uc_integration_test.go) spins up a real Kubernetes
// API server via controller-runtime's envtest. It requires the kubebuilder
// binaries (etcd + kube-apiserver) to be present locally.
//
// Download the binaries once (idempotent):
//
//	./bin/setup-envtest use 1.31.0 --bin-dir ./bin/k8s
//
// Then run the tests — KUBEBUILDER_ASSETS must be an absolute path:
//
//	KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/k8s/1.31.0-linux-amd64" \
//	  go test ./internal/usecase/nsg_uc/... -v
//
// Using a relative path causes envtest to fail with
// "fork/exec bin/k8s/.../etcd: no such file or directory"
// because the test binary changes its working directory at startup.
//
// Alternatively, run all non-e2e tests at once via:
//
//	make test
package nsg_uc

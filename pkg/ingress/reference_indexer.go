package ingress

import (
	"context"

	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// IndexKeyServiceRefName is index key for services referenced by Ingress.
	IndexKeyServiceRefName = "ingress.serviceRef.name"
	// IndexKeySecretRefName is index key for secrets referenced by Ingress or Service.
	IndexKeySecretRefName = "ingress.secretRef.name"
)

// ReferenceIndexer has the ability to index Ingresses with referenced objects.
type ReferenceIndexer interface {
	// BuildServiceRefIndexes returns the name of related Service objects.
	BuildServiceRefIndexes(ctx context.Context, ing *networking.Ingress) []string
	// BuildSecretRefIndexes returns the name of related Secret objects.
	BuildSecretRefIndexes(ctx context.Context, ingOrSvc *networking.Ingress) []string
}

// NewDefaultReferenceIndexer constructs new defaultReferenceIndexer.
func NewDefaultReferenceIndexer() *defaultReferenceIndexer {
	return &defaultReferenceIndexer{}
}

var _ ReferenceIndexer = &defaultReferenceIndexer{}

// default implementation for ReferenceIndexer
type defaultReferenceIndexer struct{}

func (i *defaultReferenceIndexer) BuildServiceRefIndexes(ctx context.Context, ing *networking.Ingress) []string {
	var backends []networking.IngressBackend
	if ing.Spec.DefaultBackend != nil {
		backends = append(backends, *ing.Spec.DefaultBackend)
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			backends = append(backends, path.Backend)
		}
	}

	serviceNames := sets.NewString()
	for _, backend := range backends {
		if backend.Service != nil && backend.Service.Name != "" {
			serviceNames.Insert(backend.Service.Name)
		}
	}
	return serviceNames.List()
}

func (i *defaultReferenceIndexer) BuildSecretRefIndexes(ctx context.Context, ingOrSvc *networking.Ingress) []string {
	secretNames := sets.NewString()

	// Extract secrets from Ingress TLS configuration
	if ingOrSvc != nil {
		for _, tls := range ingOrSvc.Spec.TLS {
			if tls.SecretName != "" {
				secretNames.Insert(tls.SecretName)
			}
		}
	}

	return secretNames.List()
}

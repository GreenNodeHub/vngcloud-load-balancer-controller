package ingress_uc

import (
	"context"
	"slices"
	"sort"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func (t *defaultModelBuildTask) buildCreateCertificates(ctx context.Context) ([]v1alpha1.CreateCertificate, error) {
	isValid, err := t.isValidToBuildHTTPSListener(ctx)
	if err != nil {
		return nil, err
	}
	if !isValid {
		return nil, nil
	}

	// if there are certIDs from annotation, do not create new certificates
	if len(t.buildAnnotationCertificateIds(ctx)) > 0 {
		return nil, nil
	}

	secretNames := make([]string, 0)
	for _, tls := range t.ingress.Spec.TLS {
		if !slices.Contains(secretNames, tls.SecretName) {
			secretNames = append(secretNames, tls.SecretName)
		}
	}

	createCertificates := make([]v1alpha1.CreateCertificate, 0, len(secretNames))
	for _, secretName := range secretNames {
		createCertificates = append(createCertificates, v1alpha1.CreateCertificate{
			SecretName: secretName,
		})
	}

	// Sort by SecretName to ensure deterministic ordering
	sort.Slice(createCertificates, func(i, j int) bool {
		return createCertificates[i].SecretName < createCertificates[j].SecretName
	})

	return createCertificates, nil
}

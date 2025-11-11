package lbc_uc

import (
	"context"
	"fmt"
	"strings"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/consts"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/errs"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func (t *defaultModelDeployTask) deployCerts(ctx context.Context) ([]v1alpha1.CreatedCertificate, error) {
	currentCerts, err := t.vngcloudRepo.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}

	createdCerts := make([]v1alpha1.CreatedCertificate, 0)
	for _, reqCreateCert := range t.lbConfig.Spec.CreateCertificates {
		createdCert, err := t.deployCert(ctx, reqCreateCert, currentCerts)
		if err != nil {
			return nil, err
		}
		createdCerts = append(createdCerts, *createdCert)
	}

	return createdCerts, nil
}

func (t *defaultModelDeployTask) deployCert(ctx context.Context, reqCreateCert v1alpha1.CreateCertificate, currentCerts *entity.ListCertificates) (*v1alpha1.CreatedCertificate, error) {
	secret, err := t.k8sRepo.GetSecret(ctx, types.NamespacedName{
		Name:      reqCreateCert.SecretName,
		Namespace: t.lbConfig.GetNamespace(),
	})
	if err != nil {
		return nil, err
	}

	certName := t.generateCertName(reqCreateCert.SecretName, secret.ResourceVersion)
	// check if certificate already exists
	for _, cert := range currentCerts.Certificates {
		if cert.Name == certName {
			t.logger.Infof("certificate %s already exists with ID %s", cert.Name, cert.UUID)
			return &v1alpha1.CreatedCertificate{
				Id:              cert.UUID,
				SecretName:      reqCreateCert.SecretName,
				ResourceVersion: secret.ResourceVersion,
				CertificateName: &cert.Name,
			}, nil
		}
	}

	// validate secret
	if secret.Type != corev1.SecretTypeTLS {
		return nil, errs.NewNoNeedRequeue("secret type must be tls")
	}

	if len(secret.Data[corev1.TLSCertKey]) == 0 || len(secret.Data[corev1.TLSPrivateKeyKey]) == 0 {
		return nil, errs.NewNoNeedRequeue("secret must have tls.crt and tls.key")
	}

	formatString := func(s string) string {
		return strings.ReplaceAll(s, "\r", "")
	}
	certificateData := formatString(string(secret.Data[corev1.TLSCertKey]))
	privateKeyData := formatString(string(secret.Data[corev1.TLSPrivateKeyKey]))

	// t.logger.Debugf(" #################### Certificate data: %q #################### ", certificateData)
	// t.logger.Debugf(" #################### Private key data: %q #################### ", privateKeyData)

	// create certificate
	createRequest := loadbalancerv2.CreateCertificateRequest{
		Name:             certName,
		Type:             loadbalancerv2.ImportOptsTypeOptTLS,
		Certificate:      certificateData,
		PrivateKey:       &privateKeyData,
		CertificateChain: nil,
		Passphrase:       nil,
	}

	createdCert, err := t.vngcloudRepo.ImportCertificate(ctx, &createRequest)
	if err != nil {
		return nil, err
	}

	t.logger.Infof("imported certificate %s with ID %s", createdCert.Name, createdCert.UUID)
	return &v1alpha1.CreatedCertificate{
		Id:              createdCert.UUID,
		SecretName:      reqCreateCert.SecretName,
		ResourceVersion: secret.ResourceVersion,
		CertificateName: &createdCert.Name,
	}, nil
}

// Delete created certificates that are no longer needed
func (t *defaultModelDeployTask) deployDeleteRedundantCerts(ctx context.Context) error {
	certList, err := t.vngcloudRepo.ListCertificates(ctx)
	if err != nil {
		return err
	}

	for _, createdCert := range t.lbConfig.Status.CreatedCertificates {
		for _, cert := range certList.Certificates {
			if cert.UUID == createdCert.Id {
				if cert.InUse {
					t.logger.Infof("certificate %s is still in use, skipping deletion", cert.UUID)
					break
				}
				err = t.vngcloudRepo.DeleteCertificate(ctx, cert.UUID)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// this function is used to generate certificate name based on secret name and resource version
func (t *defaultModelDeployTask) generateCertName(secretName string, resourceVersion string) string {
	clusterId := ""
	if t.lbConfig.Spec.ClusterId != nil {
		clusterId = *t.lbConfig.Spec.ClusterId
	}
	ingressName := ""
	// try to get from annotation first
	if val, ok := t.lbConfig.GetLabels()[consts.LabelOwnerResourceName]; ok {
		ingressName = val
	}

	generateHash := func() string {
		fullName := fmt.Sprintf("%s_%s_%s_%s", clusterId, t.lbConfig.GetNamespace(), ingressName, "ingress")
		hash := utils.HashString(fullName)
		return utils.TrimString(hash, consts.DEFAULT_HASH_NAME_LENGTH)
	}
	hash := generateHash()
	hashSecret := utils.TrimString(utils.HashString(secretName), consts.DEFAULT_HASH_NAME_LENGTH)

	name := fmt.Sprintf("%s-%s-%s-%s",
		consts.DEFAULT_LB_PREFIX_NAME,
		hash,
		hashSecret,
		resourceVersion)
	return utils.ValidateName(name)
}

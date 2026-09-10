package lbc_uc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
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
			t.logger.Debugf("certificate %s already exists with ID %s", cert.Name, cert.UUID)
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

// deleteRedundantCerts removes certificates this LBC imported and no longer wants. keep is the
// set this reconcile resolved; passing it explicitly is what keeps a live certificate off the
// candidate list, so a lagging inUse flag on the cloud can never take down the certificate the
// listener is currently serving. One certificate now backs every Ingress sharing a secret, so
// that mistake would break TLS for all of them at once. The delete path passes nothing -
// everything the LBC still holds is redundant by then.
//
// A candidate the cloud still reports in use is another owner's: it is left alone, and that
// owner - which holds the same id in its own status - cleans it up when it is done.
func (t *defaultModelDeployTask) deleteRedundantCerts(ctx context.Context, keep []v1alpha1.CreatedCertificate) error {
	wanted := sets.New[string]()
	for _, cert := range keep {
		wanted.Insert(cert.Id)
	}

	candidates := make([]string, 0, len(t.lbConfig.Status.CreatedCertificates))
	for _, held := range t.lbConfig.Status.CreatedCertificates {
		if !wanted.Has(held.Id) {
			candidates = append(candidates, held.Id)
		}
	}
	if len(candidates) < 1 {
		return nil
	}

	certList, err := t.vngcloudRepo.ListCertificates(ctx)
	if err != nil {
		return err
	}
	onCloud := make(map[string]*entity.Certificate, len(certList.Certificates))
	for i := range certList.Certificates {
		onCloud[certList.Certificates[i].UUID] = &certList.Certificates[i]
	}

	// Same reasoning as deleteRedundantPools: the candidates are independent, so one that will
	// not go must not stop the others from going. Failures are collected and returned at the
	// end, which still fails the reconcile and retries it.
	failures := make([]error, 0)
	for _, candidateId := range candidates {
		cert, onCloudNow := onCloud[candidateId]
		if !onCloudNow {
			t.logger.Debugf("certificate %s not found on the cloud, skip delete", candidateId)
			continue
		}
		if cert.InUse {
			t.logger.Debugf("certificate %s is still in use, skip delete", candidateId)
			continue
		}
		if err := t.vngcloudRepo.DeleteCertificate(ctx, candidateId); err != nil {
			failures = append(failures, fmt.Errorf("certificate %s: delete: %w", candidateId, err))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%w: %w", errPartialDelete, errors.Join(failures...))
	}
	return nil
}

// generateCertName names the cloud certificate after the Kubernetes secret it is imported
// from - cluster, namespace, secret name and the secret's resourceVersion - and deliberately
// not after the Ingress that references it. Several Ingresses may share one TLS secret on one
// shared load balancer; naming per Ingress gave each LBC its own copy of the same certificate,
// and the two then overwrote the listener's single defaultCertificateAuthority with their own
// id on every reconcile, forever.
func (t *defaultModelDeployTask) generateCertName(secretName string, resourceVersion string) string {
	clusterId := ""
	if t.lbConfig.Spec.ClusterId != nil {
		clusterId = *t.lbConfig.Spec.ClusterId
	}

	fullName := fmt.Sprintf("%s_%s_%s", clusterId, t.lbConfig.GetNamespace(), "ingress")
	hash := utils.TrimString(utils.HashString(fullName), domain.DEFAULT_HASH_NAME_LENGTH)
	hashSecret := utils.TrimString(utils.HashString(secretName), domain.DEFAULT_HASH_NAME_LENGTH)

	name := fmt.Sprintf("%s-%s-%s-%s",
		domain.DEFAULT_LB_PREFIX_NAME,
		hash,
		hashSecret,
		resourceVersion)
	return utils.ValidateName(name)
}

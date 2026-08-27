package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// Two Ingresses in one namespace, both terminating TLS with the same secret on one shared load
// balancer - the arrangement that made a production listener rewrite its default certificate
// every seven minutes for hours. A cloud certificate is identified by the secret it came from,
// so both owners have to land on the same one; when the name carried the owning Ingress
// instead, each LBC imported its own copy and then overwrote the listener's single
// defaultCertificateAuthority with its own id, forever.
// 0.3.24 has no deploy_tag_test.go, so the cluster ids these tests need are defined here.
const (
	thisClusterId  = "k8s-10eafaef-56e8-4dfc-878d-dd1c86fcb810"
	otherClusterId = "k8s-4bb03c1f-7463-46c1-8bfb-ca3fc16fb085"
)

const (
	certSecretName = "tls-secret-2026"
	certSecretRV   = "8275634"

	// Hand-derived, independently of the code under test:
	//   sha256("<thisClusterId>_default_ingress")[:5] = 540c8
	//   sha256("tls-secret-2026")[:5]                 = 44cf6
	// so the name is vks-<clusterNsHash>-<secretHash>-<resourceVersion>.
	certNameForDefaultNs = "vks-540c8-44cf6-8275634"
)

func certTask(owner string, vngcloudRepo *repository.MockVngCloudRepository, k8sRepo *repository.MockK8sRepository) *defaultModelDeployTask {
	return &defaultModelDeployTask{
		logger:       logrus.NewEntry(logrus.New()),
		vngcloudRepo: vngcloudRepo,
		k8sRepo:      k8sRepo,
		lbConfig: &v1alpha1.LoadBalancerConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      owner + "-abcde",
				Namespace: "default",
				Labels:    map[string]string{domain.LabelOwnerResourceName: owner},
			},
			Spec: v1alpha1.LoadBalancerConfigSpec{
				ClusterId:          ptr.To(thisClusterId),
				CreateCertificates: []v1alpha1.CreateCertificate{{SecretName: certSecretName}},
			},
		},
	}
}

func tlsSecret(resourceVersion string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            certSecretName,
			Namespace:       "default",
			ResourceVersion: resourceVersion,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("-----BEGIN CERTIFICATE-----\nnot-a-real-cert\n-----END CERTIFICATE-----\n"),
			corev1.TLSPrivateKeyKey: []byte("-----BEGIN RSA PRIVATE KEY-----\nnot-a-real-key\n-----END RSA PRIVATE KEY-----\n"),
		},
	}
}

// The name is the whole mechanism: deployCert looks the certificate up by name, so the owning
// Ingress must not reach it. Both owners are asserted against the same literal, so a function
// that merely returned some constant would not satisfy this.
func TestCertNameIgnoresTheOwningIngress(t *testing.T) {
	for _, owner := range []string{"fms-stream-zlmediakit-vn", "fms-stream-evidence-file-http-ingestor"} {
		t.Run(owner, func(t *testing.T) {
			task := certTask(owner, nil, nil)
			assert.Equal(t, certNameForDefaultNs, task.generateCertName(certSecretName, certSecretRV))
		})
	}
}

// The identity that remains: sharing one certificate is right only for owners that really do
// mean the same secret. Each row changes exactly one component and must change the name, or the
// fix has merged certificates that hold different bytes.
func TestCertNameSeparatesDifferentSecrets(t *testing.T) {
	tests := []struct {
		name       string
		clusterId  string
		namespace  string
		secretName string
		rv         string
		want       string
	}{
		{"baseline", thisClusterId, "default", certSecretName, certSecretRV, "vks-540c8-44cf6-8275634"},
		{"other namespace", thisClusterId, "kube-system", certSecretName, certSecretRV, "vks-1ea29-44cf6-8275634"},
		{"other cluster", otherClusterId, "default", certSecretName, certSecretRV, "vks-52771-44cf6-8275634"},
		{"other secret", thisClusterId, "default", "tls-secret-other", certSecretRV, "vks-540c8-fed51-8275634"},
		{"rotated secret", thisClusterId, "default", certSecretName, "9000001", "vks-540c8-44cf6-9000001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := certTask("some-ingress", nil, nil)
			task.lbConfig.Spec.ClusterId = ptr.To(tt.clusterId)
			task.lbConfig.Namespace = tt.namespace

			assert.Equal(t, tt.want, task.generateCertName(tt.secretName, tt.rv))
		})
	}
}

// The behaviour that ends the ping-pong: the second Ingress to reconcile adopts the certificate
// the first one imported. The mock is strict, so leaving ImportCertificate undeclared is itself
// the assertion that no duplicate is imported.
func TestDeployCertsAdoptsTheCertificateImportedByTheOtherOwner(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	k8sRepo := repository.NewMockK8sRepository(t)

	// What the other owner already put on the cloud for this same secret.
	existing := entityv2.Certificate{
		UUID:            "secret-90042715-9a64-46c8-8e81-1665fe93d3f6",
		Name:            certNameForDefaultNs,
		CertificateType: "TLS/SSL",
		DomainName:      "*.example.com",
		InUse:           true,
	}

	second := certTask("fms-stream-evidence-file-http-ingestor", vngcloudRepo, k8sRepo)

	vngcloudRepo.EXPECT().
		ListCertificates(mock.Anything).
		Return(&entityv2.ListCertificates{Certificates: []entityv2.Certificate{existing}}, nil).
		Once()
	k8sRepo.EXPECT().
		GetSecret(mock.Anything, mock.Anything).
		Return(tlsSecret(certSecretRV), nil).
		Once()

	createdCerts, err := second.deployCerts(context.Background())

	assert.NoError(t, err)
	assert.Len(t, createdCerts, 1)
	assert.Equal(t, existing.UUID, createdCerts[0].Id,
		"the second owner must reuse the existing certificate, not import a second copy of the same secret")
	assert.Equal(t, certSecretName, createdCerts[0].SecretName)
	assert.Equal(t, certSecretRV, createdCerts[0].ResourceVersion)
}

// ---------------------------------------------------------------------------
// Which certificate the sweep may remove.
// ---------------------------------------------------------------------------

func certTaskWithStatus(owner string, vngcloudRepo *repository.MockVngCloudRepository, held ...v1alpha1.CreatedCertificate) *defaultModelDeployTask {
	task := certTask(owner, vngcloudRepo, nil)
	task.lbConfig.Status.CreatedCertificates = held
	return task
}

func heldCert(id string) v1alpha1.CreatedCertificate {
	return v1alpha1.CreatedCertificate{Id: id, SecretName: certSecretName, ResourceVersion: certSecretRV}
}

func cloudCert(id string, inUse bool) entityv2.Certificate {
	return entityv2.Certificate{UUID: id, Name: certNameForDefaultNs, CertificateType: "TLS/SSL", InUse: inUse}
}

// The sweep has to decide from desired state, not from the cloud's inUse flag. A certificate this
// reconcile still wants is not a deletion candidate at all, so an inUse reading that lags behind
// the listener update it follows cannot take down the certificate the listener is serving. That
// matters more now than it did: one certificate backs every Ingress sharing a secret, so the
// mistake would break TLS for all of them at once rather than for one.
func TestDeleteRedundantCertsKeepsACertificateStillWanted(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	task := certTaskWithStatus("repro", vngcloudRepo, heldCert("secret-live"))

	// Nothing is declared on the mock: with no candidate to consider there is nothing to ask
	// the cloud, so ListCertificates going uncalled is part of the assertion, and a
	// DeleteCertificate would fail the test outright.
	assert.NoError(t, task.deleteRedundantCerts(context.Background(), []v1alpha1.CreatedCertificate{heldCert("secret-live")}))
}

// What the sweep is actually for: the certificate left over from a previous generation - a
// rotated secret, or the rename this change causes - still has to go.
func TestDeleteRedundantCertsDeletesTheOneNoLongerWanted(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	task := certTaskWithStatus("repro", vngcloudRepo, heldCert("secret-old"))

	vngcloudRepo.EXPECT().
		ListCertificates(mock.Anything).
		Return(&entityv2.ListCertificates{Certificates: []entityv2.Certificate{cloudCert("secret-old", false)}}, nil).
		Once()
	vngcloudRepo.EXPECT().
		DeleteCertificate(mock.Anything, "secret-old").
		Return(nil).
		Once()

	assert.NoError(t, task.deleteRedundantCerts(context.Background(), []v1alpha1.CreatedCertificate{heldCert("secret-new")}))
}

// A certificate this LBC has stopped wanting but something else still has attached is not ours
// to remove.
func TestDeleteRedundantCertsLeavesOneStillAttached(t *testing.T) {
	vngcloudRepo := repository.NewMockVngCloudRepository(t)
	task := certTaskWithStatus("repro", vngcloudRepo, heldCert("secret-shared"))

	vngcloudRepo.EXPECT().
		ListCertificates(mock.Anything).
		Return(&entityv2.ListCertificates{Certificates: []entityv2.Certificate{cloudCert("secret-shared", true)}}, nil).
		Once()

	assert.NoError(t, task.deleteRedundantCerts(context.Background(), nil))
}

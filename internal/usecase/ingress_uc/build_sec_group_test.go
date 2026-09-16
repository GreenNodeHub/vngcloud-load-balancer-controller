package ingress_uc

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

// Security group rules are the other half of "one bad backend must not take the Ingress down".
// Pools decide what the load balancer forwards to; these rules decide whether the node accepts
// it. If an unresolvable backend aborted this build the way it aborted pool building, the
// Ingress would come up with routes the nodes refuse - so the skip has to happen on both paths,
// and only this file covers the security group one.
const (
	secGroupNamespace = "magnet-common-sit"
	secGroupSubnet    = "192.168.1.0/24"
)

// ingressWithPaths builds an Ingress whose rule points at each named backend in turn, so a test
// only has to say which Services resolve.
func ingressWithPaths(backends ...networkingv1.IngressServiceBackend) *networkingv1.Ingress {
	paths := make([]networkingv1.HTTPIngressPath, 0, len(backends))
	for i := range backends {
		backend := backends[i]
		paths = append(paths, networkingv1.HTTPIngressPath{
			Backend: networkingv1.IngressBackend{Service: &backend},
		})
	}
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "prod-magnet", Namespace: secGroupNamespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths},
				},
			}},
		},
	}
}

func backend(name string, port int32) networkingv1.IngressServiceBackend {
	return networkingv1.IngressServiceBackend{
		Name: name,
		Port: networkingv1.ServiceBackendPort{Number: port},
	}
}

func serviceWithNodePort(name string, port, nodePort int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: secGroupNamespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: port, Protocol: corev1.ProtocolTCP, NodePort: nodePort}},
		},
	}
}

// allowNodePort is the rule the controller opens for one resolved backend: the load balancer's
// subnet reaching that node port. Written out by hand rather than taken from the code, so a
// change in what gets opened shows up as a failure here.
func allowNodePort(nodePort int32) v1alpha1.NodeSecurityGroupRule {
	return v1alpha1.NodeSecurityGroupRule{
		Protocol:    networkv2.SecgroupRuleProtocolTCP,
		FromPort:    nodePort,
		ToPort:      nodePort,
		CIDR:        secGroupSubnet,
		Description: "Allow load balancer access to port " + strconv.Itoa(int(nodePort)),
		Direction:   networkv2.SecgroupRuleDirectionIngress,
		EtherType:   networkv2.SecgroupRuleEtherTypeIPv4,
	}
}

// The two egress rules the controller always appends, whatever the backends are.
func defaultEgressRules() []v1alpha1.NodeSecurityGroupRule {
	return []v1alpha1.NodeSecurityGroupRule{
		{
			Protocol:    networkv2.SecgroupRuleProtocolAll,
			FromPort:    0,
			ToPort:      65535,
			CIDR:        "0.0.0.0/0",
			Description: "Default egress security group rule for IPv4",
			Direction:   networkv2.SecgroupRuleDirectionEgress,
			EtherType:   networkv2.SecgroupRuleEtherTypeIPv4,
		},
		{
			Protocol:    networkv2.SecgroupRuleProtocolAll,
			FromPort:    0,
			ToPort:      65535,
			CIDR:        "::/0",
			Description: "Default egress security group rule for IPv6",
			Direction:   networkv2.SecgroupRuleDirectionEgress,
			EtherType:   networkv2.SecgroupRuleEtherTypeIPv6,
		},
	}
}

func secGroupTask(t *testing.T, ingress *networkingv1.Ingress, k8sRepo *repository.MockK8sRepository) *defaultModelBuildTask {
	t.Helper()

	resolver := utils.NewMockEndpointResolver(t)
	resolver.EXPECT().
		ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, svc types.NamespacedName, _ intstr.IntOrString, _ ...utils.EndpointResolveOption) ([]utils.EndpointAddress, error) {
			// One node, and the port depends on which Service was resolved - so a rule built
			// for the wrong backend would not match the expected set.
			return []utils.EndpointAddress{{IP: "10.0.0.1", Port: nodePortOf(svc.Name), Name: "node-1"}}, nil
		}).Maybe()

	cni := utils.NewMockCniDetector(t)
	cni.EXPECT().DetectCNIType(mock.Anything).Return(utils.CalicoOverlay, nil).Maybe()

	return &defaultModelBuildTask{
		logger:            logrus.NewEntry(logrus.New()),
		ingress:           ingress,
		k8sRepo:           k8sRepo,
		annotationParser:  annotations.NewSuffixAnnotationParser(domain.INGRESS_ANNOTATION_PREFIX),
		endpointResolver:  resolver,
		cniDetector:       cni,
		defaultSubnetCIDR: secGroupSubnet,
	}
}

// The node ports the fixture Services expose, kept in one place so the resolver and the
// expected rules cannot drift apart.
func nodePortOf(service string) int {
	switch service {
	case "checkout":
		return 30080
	case "search":
		return 30081
	}
	return 0
}

// QC-6d: an Ingress with a typo'd backend still has to produce the rules for the backends that
// do resolve. Before the fix the whole build returned the error, so the NodeSecurityGroup came
// out empty and the nodes dropped traffic for every path - including the ones still working.
func TestBuildDefaultSecurityGroupRuleSkipsAnUnresolvableBackend(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		GetService(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, key types.NamespacedName) (*corev1.Service, error) {
			switch key.Name {
			case "checkout":
				return serviceWithNodePort("checkout", 80, 30080), nil
			case "search":
				return serviceWithNodePort("search", 8080, 30081), nil
			}
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, key.Name)
		})

	task := secGroupTask(t, ingressWithPaths(
		backend("checkout", 80),
		backend("nfk", 443), // the typo: no such Service
		backend("search", 8080),
	), k8sRepo)

	rules, err := task.buildDefaultSecurityGroupRule(context.Background(), secGroupSubnet, nil)

	require.NoError(t, err, "a backend the Ingress names wrongly is the Ingress author's problem, not a build failure")
	want := append(defaultEgressRules(), allowNodePort(30080), allowNodePort(30081))
	assert.ElementsMatch(t, want, rules,
		"the two resolvable backends keep their rules, and the unresolvable one contributes none")
}

// The default backend is built by a separate branch above the rule loop, so it needs its own
// case: an Ingress whose default backend is gone must still open the ports its rules need.
func TestBuildDefaultSecurityGroupRuleSkipsAnUnresolvableDefaultBackend(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		GetService(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, key types.NamespacedName) (*corev1.Service, error) {
			if key.Name == "checkout" {
				return serviceWithNodePort("checkout", 80, 30080), nil
			}
			return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, key.Name)
		})

	ingress := ingressWithPaths(backend("checkout", 80))
	missing := backend("nfk", 443)
	ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{Service: &missing}

	task := secGroupTask(t, ingress, k8sRepo)

	rules, err := task.buildDefaultSecurityGroupRule(context.Background(), secGroupSubnet, nil)

	require.NoError(t, err)
	assert.ElementsMatch(t, append(defaultEgressRules(), allowNodePort(30080)), rules)
}

// The counterpart the skip must not swallow. A Service the API server refuses to report on says
// nothing about whether the backend is valid, so building a partial rule set from it would quietly
// close ports that should be open. That stays fatal, and the reconcile retries.
func TestBuildDefaultSecurityGroupRuleFailsOnAnErrorThatIsNotTheIngressAuthorsFault(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		GetService(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, key types.NamespacedName) (*corev1.Service, error) {
			if key.Name == "checkout" {
				return serviceWithNodePort("checkout", 80, 30080), nil
			}
			return nil, apierrors.NewForbidden(schema.GroupResource{Resource: "services"}, key.Name, errors.New("nope"))
		})

	task := secGroupTask(t, ingressWithPaths(
		backend("checkout", 80),
		backend("locked-down", 443),
	), k8sRepo)

	rules, err := task.buildDefaultSecurityGroupRule(context.Background(), secGroupSubnet, nil)

	require.Error(t, err)
	assert.Nil(t, rules, "a partial rule set must never be returned as if it were complete")
	assert.False(t, errors.Is(err, errBackendUnresolvable),
		"only the Ingress author's mistakes are skippable; everything else has to reach the caller")
}

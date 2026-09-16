package ingress_uc

import (
	"context"
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

// buildPool alone reporting a backend as skippable is only half the contract. What the incident
// was actually about is the loop above it: an Ingress with ten paths and one typo used to come
// out with no routing at all, because the first unresolvable backend returned from the whole
// build. These tests pin the caller's behaviour - the paths that resolve still become pools and
// policies, and nothing points at a pool that was never built.

// pathTo is one Ingress path sending a prefix at a backend Service.
func pathTo(urlPath, service string, port int32) networkingv1.HTTPIngressPath {
	return networkingv1.HTTPIngressPath{
		Path:     urlPath,
		PathType: ptr.To(networkingv1.PathTypePrefix),
		Backend: networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: service,
				Port: networkingv1.ServiceBackendPort{Number: port},
			},
		},
	}
}

// An empty host keeps the Ingress on the plain HTTP listener, so these tests stay about path
// isolation rather than about TLS.
func ingressWithRulePaths(paths ...networkingv1.HTTPIngressPath) *networkingv1.Ingress {
	return &networkingv1.Ingress{
		ObjectMeta: ingressWithPaths().ObjectMeta,
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths},
				},
			}},
		},
	}
}

// isolationTask wires the collaborators buildPoolsAndListeners needs. Only the Services listed
// in resolvable exist; anything else comes back NotFound, which is the typo case.
func isolationTask(t *testing.T, ingress *networkingv1.Ingress, resolvable map[string]int32) *defaultModelBuildTask {
	t.Helper()

	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		GetService(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, key types.NamespacedName) (*corev1.Service, error) {
			port, ok := resolvable[key.Name]
			if !ok {
				return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "services"}, key.Name)
			}
			return serviceWithNodePort(key.Name, port, 30000+port), nil
		})

	resolver := utils.NewMockEndpointResolver(t)
	resolver.EXPECT().
		ResolveNodePortEndpoints(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, svc types.NamespacedName, _ intstr.IntOrString, _ ...utils.EndpointResolveOption) ([]utils.EndpointAddress, error) {
			return []utils.EndpointAddress{{IP: "10.0.0.1", Port: int(30000 + resolvable[svc.Name]), Name: "node-1"}}, nil
		}).Maybe()

	names := utils.NewMockNameHelper(t)
	names.EXPECT().
		GenL7PoolName(mock.Anything, mock.Anything).
		RunAndReturn(func(service string, port int) string { return fmt.Sprintf("pool-%s-%d", service, port) }).
		Maybe()
	names.EXPECT().
		GenL7PolicyName(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(https bool, rule, path int) string { return fmt.Sprintf("policy-%d-%d", rule, path) }).
		Maybe()

	return &defaultModelBuildTask{
		logger:           logrus.NewEntry(logrus.New()),
		ingress:          ingress,
		k8sRepo:          k8sRepo,
		annotationParser: annotations.NewSuffixAnnotationParser(domain.INGRESS_ANNOTATION_PREFIX),
		endpointResolver: resolver,
		nameHelper:       names,
	}
}

// poolNames and policyPoolRefs read the result the way the deploy side later will.
func poolNames(pools []v1alpha1.Pool) []string {
	out := make([]string, 0, len(pools))
	for _, p := range pools {
		out = append(out, p.Name)
	}
	return out
}

// QC-6a: three paths, the middle one naming a Service that does not exist. The two good paths
// have to survive with a pool each and a policy each.
func TestBuildPoolsAndListenersDropsOnlyTheUnresolvablePath(t *testing.T) {
	task := isolationTask(t, ingressWithRulePaths(
		pathTo("/checkout", "checkout", 80),
		pathTo("/nfk", "nfk", 443), // no such Service
		pathTo("/search", "search", 8080),
	), map[string]int32{"checkout": 80, "search": 8080})

	pools, listeners, err := task.buildPoolsAndListeners(context.Background(), nil)

	require.NoError(t, err, "one path the Ingress author got wrong must not fail the build")
	assert.Equal(t, []string{"pool-checkout-80", "pool-search-8080"}, poolNames(pools),
		"a pool for each resolvable backend, and none for the one that does not resolve")

	require.Len(t, listeners, 1)
	require.Len(t, listeners[0].Policies, 2,
		"the unresolvable path contributes no policy - a policy with no pool would be a dangling route")

	built := map[string]bool{}
	for _, p := range pools {
		built[p.Name] = true
	}
	for _, policy := range listeners[0].Policies {
		require.NotNil(t, policy.RedirectPoolName, "every surviving policy still routes somewhere")
		assert.True(t, built[*policy.RedirectPoolName],
			"policy %s points at pool %s, which was never built", policy.Name, *policy.RedirectPoolName)
	}
}

// The default backend takes its own branch, and its pool is what the listener's DefaultPoolName
// refers to. If an unresolvable default backend left that pointing at a pool nobody built, the
// listener would carry a route to nothing.
func TestBuildPoolsAndListenersServesTheRulesWhenTheDefaultBackendIsUnresolvable(t *testing.T) {
	ingress := ingressWithRulePaths(pathTo("/checkout", "checkout", 80))
	ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: "nfk",
			Port: networkingv1.ServiceBackendPort{Number: 443},
		},
	}

	task := isolationTask(t, ingress, map[string]int32{"checkout": 80})

	pools, listeners, err := task.buildPoolsAndListeners(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"pool-checkout-80"}, poolNames(pools),
		"the default backend contributes no pool, the rule still does")
	require.Len(t, listeners, 1)
	assert.Nil(t, listeners[0].DefaultPoolName,
		"with no default pool built, the listener must not name one")
	require.Len(t, listeners[0].Policies, 1, "the rule is still served")
}

// The guard on the other side: a backend that cannot be read for reasons that have nothing to do
// with the Ingress must not be treated as a typo. Skipping it would quietly remove a working
// route from the load balancer and report success.
func TestBuildPoolsAndListenersFailsWhenABackendCannotBeRead(t *testing.T) {
	k8sRepo := repository.NewMockK8sRepository(t)
	k8sRepo.EXPECT().
		GetService(mock.Anything, mock.Anything).
		Return(nil, apierrors.NewInternalError(fmt.Errorf("etcd leader changed")))

	names := utils.NewMockNameHelper(t)
	names.EXPECT().
		GenL7PolicyName(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(https bool, rule, path int) string { return fmt.Sprintf("policy-%d-%d", rule, path) }).
		Maybe()

	task := &defaultModelBuildTask{
		logger:           logrus.NewEntry(logrus.New()),
		ingress:          ingressWithRulePaths(pathTo("/checkout", "checkout", 80)),
		k8sRepo:          k8sRepo,
		annotationParser: annotations.NewSuffixAnnotationParser(domain.INGRESS_ANNOTATION_PREFIX),
		nameHelper:       names,
	}

	pools, listeners, err := task.buildPoolsAndListeners(context.Background(), nil)

	require.Error(t, err)
	assert.Nil(t, pools)
	assert.Nil(t, listeners)
}

package alb_gateway_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	gwv1alpha1 "github.com/vngcloud/vngcloud-load-balancer-controller/api/gateway/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/controller/gateway/shared"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// newFakeClientWithHTTPRouteIndex builds a fake client that has the HTTPRoute parent-gateway
// field index registered — required by listAttachedHTTPRoutes.
func newFakeClientWithHTTPRouteIndex(objs ...client.Object) client.Client {
	s := newTestScheme()
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&gwv1.Gateway{}).
		WithIndex(&gwv1.HTTPRoute{}, shared.IndexHTTPRouteByParentGateway, func(obj client.Object) []string {
			r := obj.(*gwv1.HTTPRoute)
			var keys []string
			for _, p := range r.Spec.ParentRefs {
				ns := r.Namespace
				if p.Namespace != nil {
					ns = string(*p.Namespace)
				}
				if p.Kind == nil || *p.Kind == "Gateway" {
					keys = append(keys, ns+"/"+string(p.Name))
				}
			}
			return keys
		}).
		Build()
}

// newFullTask builds a task with a real fake client containing the gateway policy objs.
func newFullTask(t *testing.T, gw *gwv1.Gateway, objs ...client.Object) (*defaultGatewayBuildTask, *repository.MockK8sRepository) {
	s := newTestScheme()
	builder := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&gwv1.Gateway{})
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	fakeClient := builder.Build()
	mockK8s := repository.NewMockK8sRepository(t)
	mockVng := repository.NewMockVngCloudRepository(t)
	uc := &albGatewayUseCase{
		k8sRepo:           mockK8s,
		vngcloudRepo:      mockVng,
		k8sClient:         fakeClient,
		defaultZone:       "HCM03-1C",
		defaultNetworkId:  "net-1",
		defaultSubnetId:   "subnet-1",
		defaultSubnetCIDR: "10.0.0.0/24",
		clusterId:         "cluster-1",
	}
	task := &defaultGatewayBuildTask{
		uc:               uc,
		gw:               gw,
		logger:           logrus.NewEntry(logrus.New()),
		listenerPolicies: make(map[string]*gwv1alpha1.VKSGatewayPolicy),
	}
	return task, mockK8s
}

func TestResolveGatewayPolicies_NoPolicies(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw", UID: "uid-1"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}
	task, _ := newFullTask(t, gw)
	err := task.resolveGatewayPolicies(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, task.unscopedPolicy)
	assert.Empty(t, task.listenerPolicies)
}

func TestResolveGatewayPolicies_WithUnscopedPolicy(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      "my-gw",
			UID:       "uid-1",
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}

	// VKSGatewayPolicy targeting this Gateway (no sectionName = unscoped)
	policy := &gwv1alpha1.VKSGatewayPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw-policy"},
		Spec: gwv1alpha1.VKSGatewayPolicySpec{
			TargetRefs: []gwv1alpha1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gwv1alpha1.LocalPolicyTargetReference{
					Group: "gateway.networking.k8s.io",
					Kind:  "Gateway",
					Name:  "my-gw",
				},
			}},
		},
	}

	task, _ := newFullTask(t, gw, policy)
	err := task.resolveGatewayPolicies(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, task.unscopedPolicy)
}

func TestApplyLoadBalancerSpec_NilPolicy(t *testing.T) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw"}}
	task := newTestTask(t, gw)
	task.unscopedPolicy = nil
	lbc := &v1alpha1.LoadBalancerConfig{}
	task.applyLoadBalancerSpec(lbc)
	// No-op
	assert.Nil(t, lbc.Spec.Scheme)
	assert.Nil(t, lbc.Spec.PackageId)
}

func TestApplyLoadBalancerSpec_NilLoadBalancerSpec(t *testing.T) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw"}}
	task := newTestTask(t, gw)
	task.unscopedPolicy = &gwv1alpha1.VKSGatewayPolicy{} // no LoadBalancerSpec
	lbc := &v1alpha1.LoadBalancerConfig{}
	task.applyLoadBalancerSpec(lbc)
	assert.Nil(t, lbc.Spec.Scheme)
}

func TestApplyLoadBalancerSpec_WithFields(t *testing.T) {
	gw := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw"}}
	task := newTestTask(t, gw)

	scheme := "Internet"
	pkg := "pkg-1"
	lbID := "lb-existing"
	task.unscopedPolicy = &gwv1alpha1.VKSGatewayPolicy{
		Spec: gwv1alpha1.VKSGatewayPolicySpec{
			LoadBalancerSpec: &gwv1alpha1.VKSLoadBalancerSpec{
				Scheme:         &scheme,
				PackageID:      &pkg,
				LoadBalancerID: &lbID,
				Tags:           map[string]string{"env": "prod"},
			},
		},
	}
	lbc := &v1alpha1.LoadBalancerConfig{}
	task.applyLoadBalancerSpec(lbc)
	assert.NotNil(t, lbc.Spec.Scheme)
	assert.NotNil(t, lbc.Spec.PackageId)
	assert.Equal(t, "pkg-1", *lbc.Spec.PackageId)
	assert.NotNil(t, lbc.Spec.LoadBalancerId)
	assert.Equal(t, map[string]string{"env": "prod"}, lbc.Spec.Tags)
}

// TestBuildLoadBalancerConfig_CreatePath tests the happy path where no LBC exists yet.
func TestBuildLoadBalancerConfig_CreatePath(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      "my-gw",
			UID:       types.UID("gw-uid-1"),
		},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}

	// The Gateway itself is in the fake client for status patch; index required for HTTPRoute listing.
	fakeClient := newFakeClientWithHTTPRouteIndex(gw)
	mockK8s := repository.NewMockK8sRepository(t)
	mockVng := repository.NewMockVngCloudRepository(t)

	// listOwnedLBCs → empty (new gateway)
	mockK8s.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	// CreateLoadBalancerConfig called
	mockK8s.EXPECT().
		CreateLoadBalancerConfig(mock.Anything, mock.Anything).
		Return(nil).Once()

	uc := &albGatewayUseCase{
		k8sRepo:           mockK8s,
		vngcloudRepo:      mockVng,
		k8sClient:         fakeClient,
		defaultZone:       "HCM03-1C",
		defaultNetworkId:  "net-1",
		defaultSubnetId:   "subnet-1",
		defaultSubnetCIDR: "10.0.0.0/24",
		clusterId:         "cluster-1",
	}
	task := &defaultGatewayBuildTask{
		uc:               uc,
		gw:               gw,
		logger:           logrus.NewEntry(logrus.New()),
		listenerPolicies: make(map[string]*gwv1alpha1.VKSGatewayPolicy),
	}

	err := task.buildLoadBalancerConfig(context.Background())
	assert.NoError(t, err)
}

// TestBuildLoadBalancerConfig_PatchPath tests the happy path where the LBC already exists.
func TestBuildLoadBalancerConfig_PatchPath(t *testing.T) {
	gwUID := types.UID("gw-uid-2")
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "my-gw", UID: gwUID},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}

	existingLBC := &v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      "my-gw-lbc",
			Labels: map[string]string{
				domain.LabelOwnerResourceKind: domain.OwnerKindGateway,
				domain.LabelOwnerResourceUid:  string(gwUID),
			},
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{VpcId: "net-old"},
	}

	// Index required for HTTPRoute listing.
	fakeClient := newFakeClientWithHTTPRouteIndex(gw)
	mockK8s := repository.NewMockK8sRepository(t)
	mockVng := repository.NewMockVngCloudRepository(t)

	// listOwnedLBCs returns one existing LBC
	mockK8s.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, list *v1alpha1.LoadBalancerConfigList, _ ...client.ListOption) error {
			list.Items = []v1alpha1.LoadBalancerConfig{*existingLBC}
			return nil
		}).Once()

	// Spec changed (VpcId) → PatchLoadBalancerConfig called
	mockK8s.EXPECT().
		PatchLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	uc := &albGatewayUseCase{
		k8sRepo:           mockK8s,
		vngcloudRepo:      mockVng,
		k8sClient:         fakeClient,
		defaultZone:       "HCM03-1C",
		defaultNetworkId:  "net-1",
		defaultSubnetId:   "subnet-1",
		defaultSubnetCIDR: "10.0.0.0/24",
		clusterId:         "cluster-1",
	}
	task := &defaultGatewayBuildTask{
		uc:               uc,
		gw:               gw,
		logger:           logrus.NewEntry(logrus.New()),
		listenerPolicies: make(map[string]*gwv1alpha1.VKSGatewayPolicy),
	}

	err := task.buildLoadBalancerConfig(context.Background())
	assert.NoError(t, err)
}

// TestBuildLoadBalancerConfig_MultipleOwnedLBCsError ensures >1 LBC returns error.
func TestBuildLoadBalancerConfig_MultipleOwnedLBCsError(t *testing.T) {
	gwUID := types.UID("gw-uid-3")
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "my-gw", UID: gwUID},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}

	s := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(gw).Build()
	mockK8s := repository.NewMockK8sRepository(t)
	mockVng := repository.NewMockVngCloudRepository(t)

	mockK8s.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, list *v1alpha1.LoadBalancerConfigList, _ ...client.ListOption) error {
			list.Items = []v1alpha1.LoadBalancerConfig{
				{ObjectMeta: metav1.ObjectMeta{Name: "lbc-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "lbc-2"}},
			}
			return nil
		}).Once()

	uc := &albGatewayUseCase{k8sRepo: mockK8s, vngcloudRepo: mockVng, k8sClient: fakeClient}
	task := &defaultGatewayBuildTask{
		uc: uc, gw: gw,
		logger:           logrus.NewEntry(logrus.New()),
		listenerPolicies: make(map[string]*gwv1alpha1.VKSGatewayPolicy),
	}
	err := task.buildLoadBalancerConfig(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiple LBCs")
}

// TestRun exercises the top-level task.run() path.
func TestRun_NoSubnet(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw-run", UID: "uid-run"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}
	fakeClient := newFakeClientWithHTTPRouteIndex(gw)
	mockK8s := repository.NewMockK8sRepository(t)
	mockVng := repository.NewMockVngCloudRepository(t)

	mockK8s.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	uc := &albGatewayUseCase{
		k8sRepo:      mockK8s,
		vngcloudRepo: mockVng,
		k8sClient:    fakeClient,
		// No subnet/zone → error in buildLoadBalancerConfig
	}
	task := &defaultGatewayBuildTask{
		uc:               uc,
		gw:               gw,
		logger:           logrus.NewEntry(logrus.New()),
		listenerPolicies: make(map[string]*gwv1alpha1.VKSGatewayPolicy),
	}
	err := task.run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not resolve default subnet")
}

// TestBuildLoadBalancerConfig_MissingSubnet tests error when no subnet available.
func TestBuildLoadBalancerConfig_MissingSubnet(t *testing.T) {
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "gw", UID: "uid"},
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Protocol: gwv1.HTTPProtocolType, Port: 80},
			},
		},
	}

	s := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(gw).Build()
	mockK8s := repository.NewMockK8sRepository(t)
	mockVng := repository.NewMockVngCloudRepository(t)

	mockK8s.EXPECT().
		ListLoadBalancerConfig(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).Once()

	uc := &albGatewayUseCase{
		k8sRepo:      mockK8s,
		vngcloudRepo: mockVng,
		k8sClient:    fakeClient,
		// No defaults set — subnet/network/zone all empty
	}
	task := &defaultGatewayBuildTask{
		uc: uc, gw: gw,
		logger:           logrus.NewEntry(logrus.New()),
		listenerPolicies: make(map[string]*gwv1alpha1.VKSGatewayPolicy),
	}
	err := task.buildLoadBalancerConfig(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not resolve default subnet")
}

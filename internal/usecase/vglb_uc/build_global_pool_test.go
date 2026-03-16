package vglb_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/utils"
)

func newNodePortService(name, namespace string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeNodePort,
			Ports: ports,
		},
	}
}

func newVGLB(name, namespace string) *v1alpha1.VngcloudGlobalLoadBalancer {
	return &v1alpha1.VngcloudGlobalLoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newBuildTask(vglb *v1alpha1.VngcloudGlobalLoadBalancer, svc *corev1.Service, resolver utils.EndpointResolver) *defaultModelBuildTask {
	return &defaultModelBuildTask{
		logger:           logrus.NewEntry(logrus.New()),
		vglb:             vglb,
		servicePointer:   svc,
		annotationParser: annotations.NewSuffixAnnotationParser("vks.vngcloud.vn"),
		endpointResolver: resolver,
		defaultRegion:    "hcm",
		defaultNetworkId: "net-abc123",
		defaultSubnetId:  "sub-xyz",
	}
}

func TestBuildPool_PoolMemberGroupNaming(t *testing.T) {
	ctx := context.Background()

	mockResolver := utils.NewMockEndpointResolver(t)
	// buildPool calls ResolveNodePortEndpoints with a node selector option
	// Use mock.Anything for the variadic opts argument (passed as a slice by testify)
	mockResolver.On("ResolveNodePortEndpoints",
		mock.Anything, mock.Anything, intstr.FromInt(80), mock.Anything,
	).Return([]utils.EndpointAddress{
		{IP: "10.0.0.1", Port: 30080, Name: "node-1"},
	}, nil)

	vglb := newVGLB("my-vglb", "default")
	svc := newNodePortService("my-vglb", "default", []corev1.ServicePort{
		{Protocol: corev1.ProtocolTCP, Port: 80, NodePort: 30080},
	})
	task := newBuildTask(vglb, svc, mockResolver)

	pool, err := task.buildPool(ctx, svc.Spec.Ports[0], nil)

	assert.NoError(t, err)
	assert.NotNil(t, pool)
	assert.Len(t, pool.PoolMembers, 1)

	pm := pool.PoolMembers[0]
	assert.Equal(t, "hcm-net-abc123", pm.Name, "pool member group name should be {region}-{vpcId}")
	assert.Equal(t, "hcm", pm.Region)
	assert.Equal(t, "net-abc123", pm.VpcId)
	assert.Equal(t, global.GlobalPoolMemberTypePrivate, pm.Type)
	assert.Len(t, pm.Members, 1)
	assert.Equal(t, "10.0.0.1", pm.Members[0].Address)
}

func TestBuildPoolsAndListeners_1to1Mapping(t *testing.T) {
	ctx := context.Background()

	mockResolver := utils.NewMockEndpointResolver(t)
	mockResolver.On("ResolveNodePortEndpoints",
		mock.Anything, mock.Anything, intstr.FromInt(80), mock.Anything,
	).Return([]utils.EndpointAddress{
		{IP: "10.0.0.1", Port: 30080, Name: "node-1"},
	}, nil)
	mockResolver.On("ResolveNodePortEndpoints",
		mock.Anything, mock.Anything, intstr.FromInt(443), mock.Anything,
	).Return([]utils.EndpointAddress{
		{IP: "10.0.0.1", Port: 30443, Name: "node-1"},
	}, nil)

	vglb := newVGLB("my-vglb", "default")
	svc := newNodePortService("my-vglb", "default", []corev1.ServicePort{
		{Protocol: corev1.ProtocolTCP, Port: 80, NodePort: 30080},
		{Protocol: corev1.ProtocolTCP, Port: 443, NodePort: 30443},
	})
	task := newBuildTask(vglb, svc, mockResolver)

	pools, listeners, err := task.buildPoolsAndListeners(ctx, nil)

	assert.NoError(t, err)
	assert.Len(t, pools, 2, "should create one pool per service port")
	assert.Len(t, listeners, 2, "should create one listener per service port")

	poolNames := []string{pools[0].Name, pools[1].Name}
	assert.Contains(t, poolNames, "pool-TCP-80-tcp")
	assert.Contains(t, poolNames, "pool-TCP-443-tcp")

	listenerNames := []string{listeners[0].Name, listeners[1].Name}
	assert.Contains(t, listenerNames, "listener-TCP-80")
	assert.Contains(t, listenerNames, "listener-TCP-443")
}

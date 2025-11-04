package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// ============================================================================
// LAYER 1 Tests: Self-validation (internal consistency)
// ============================================================================

func TestValidateSelf_DuplicatePortWithinSameLBC(t *testing.T) {
	ctx := context.Background()

	// Create a LBC with duplicate ports (TCP and UDP on port 53)
	lbc := &v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-lbc",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "dns-tcp",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 53,
				},
				{
					Name:         "dns-udp",
					Protocol:     loadbalancerv2.ListenerProtocolUDP,
					ProtocolPort: 53,
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:   logrus.NewEntry(logrus.New()),
		lbConfig: lbc,
	}

	err := task.validateSelf(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate listener port 53")
	assert.Contains(t, err.Error(), "VNGCloud limitation")
}

func TestValidateSelf_NoConflict(t *testing.T) {
	ctx := context.Background()

	// Create a LBC with different ports
	lbc := &v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-lbc",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "http",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 80,
				},
				{
					Name:         "https",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 443,
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:   logrus.NewEntry(logrus.New()),
		lbConfig: lbc,
	}

	err := task.validateSelf(ctx)
	assert.NoError(t, err)
}

// ============================================================================
// LAYER 2 Tests: Cross-LBC validation (shared load balancer conflicts)
// ============================================================================

func TestValidateCrossLBCs_PortConflictAcrossLBCs(t *testing.T) {
	ctx := context.Background()
	lbId := "lb-shared"

	// LBC 1 - already deployed with port 80
	existingLBC := v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-lbc",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "http",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 80,
				},
			},
		},
		Status: v1alpha1.LoadBalancerConfigStatus{
			LoadBalancerId: ptr.To(lbId),
		},
	}

	// LBC 2 - trying to use the same port 80
	newLBC := &v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-lbc",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "http",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 80, // Conflict!
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:   logrus.NewEntry(logrus.New()),
		lbConfig: newLBC,
	}

	allLBCs := &v1alpha1.LoadBalancerConfigList{
		Items: []v1alpha1.LoadBalancerConfig{existingLBC, *newLBC},
	}

	err := task.validateCrossListenerPorts(ctx, lbId, allLBCs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port 80 is already in use")
	assert.Contains(t, err.Error(), "existing-lbc")
}

func TestValidateCrossLBCs_NoConflict(t *testing.T) {
	ctx := context.Background()
	lbId := "lb-shared"

	// LBC 1 - using port 80
	existingLBC := v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-lbc",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "http",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 80,
				},
			},
		},
		Status: v1alpha1.LoadBalancerConfigStatus{
			LoadBalancerId: ptr.To(lbId),
		},
	}

	// LBC 2 - using different port 443
	newLBC := &v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-lbc",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "https",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 443, // No conflict
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:   logrus.NewEntry(logrus.New()),
		lbConfig: newLBC,
	}

	allLBCs := &v1alpha1.LoadBalancerConfigList{
		Items: []v1alpha1.LoadBalancerConfig{existingLBC, *newLBC},
	}

	err := task.validateCrossListenerPorts(ctx, lbId, allLBCs)
	assert.NoError(t, err)
}

func TestValidateCrossLBCs_DifferentLoadBalancers(t *testing.T) {
	ctx := context.Background()

	// LBC 1 - on load balancer lb-1
	lbc1 := v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lbc-1",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "http",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 80,
				},
			},
		},
		Status: v1alpha1.LoadBalancerConfigStatus{
			LoadBalancerId: ptr.To("lb-1"),
		},
	}

	// LBC 2 - on load balancer lb-2 (different LB, same port is OK)
	lbc2 := &v1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lbc-2",
			Namespace: "default",
		},
		Spec: v1alpha1.LoadBalancerConfigSpec{
			Listeners: []v1alpha1.Listener{
				{
					Name:         "http",
					Protocol:     loadbalancerv2.ListenerProtocolTCP,
					ProtocolPort: 80, // Same port but different LB - OK
				},
			},
		},
	}

	task := &defaultModelDeployTask{
		logger:   logrus.NewEntry(logrus.New()),
		lbConfig: lbc2,
	}

	allLBCs := &v1alpha1.LoadBalancerConfigList{
		Items: []v1alpha1.LoadBalancerConfig{lbc1, *lbc2},
	}

	// Validating lbc2 on lb-2 should pass (lbc1 is on lb-1)
	err := task.validateCrossListenerPorts(ctx, "lb-2", allLBCs)
	assert.NoError(t, err)
}

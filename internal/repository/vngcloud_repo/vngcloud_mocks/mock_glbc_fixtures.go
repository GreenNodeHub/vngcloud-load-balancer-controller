package vngcloud_mocks

import (
	global "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/glb/v1"
	"k8s.io/utils/ptr"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// MockGLBCMinimalSpec returns a minimal valid GlobalLoadBalancerConfigSpec with
// 1 pool, 1 member group, 1 member, and 1 listener. Used as the primary fixture
// for controller integration tests.
func MockGLBCMinimalSpec() v1alpha1.GlobalLoadBalancerConfigSpec {
	return v1alpha1.GlobalLoadBalancerConfigSpec{
		Name: "test-glb",
		Type: global.GlobalLoadBalancerTypeLayer4,
		GlobalPools: []v1alpha1.GlobalPool{
			{
				Name:     "test-pool",
				Protocol: global.GlobalPoolProtocolTCP,
				HealthMonitor: v1alpha1.GlobalPoolHealthMonitor{
					Protocol: global.GlobalPoolHealthCheckProtocolTCP,
				},
				PoolMembers: []v1alpha1.GlobalPoolMember{
					{
						Name:   "test-pool-member-group",
						Region: "HCM-03",
						VpcId:  MockNetID,
						Type:   global.GlobalPoolMemberTypePrivate,
						Members: []v1alpha1.GlobalMember{
							{
								Name:     "test-member",
								Address:  "10.0.0.1",
								SubnetID: MockSubnetID,
								Port:     8080,
							},
						},
					},
				},
			},
		},
		GlobalListeners: []v1alpha1.GlobalListener{
			{
				Name:            "test-listener",
				Protocol:        global.GlobalListenerProtocolTCP,
				ProtocolPort:    80,
				DefaultPoolName: ptr.To("test-pool"),
			},
		},
	}
}

// MockGLBCSharedSpec returns a second GlobalLoadBalancerConfigSpec with different
// pool and listener names. Used for shared-LB (partial delete) testing scenarios
// where two GLBCs share the same underlying load balancer.
// Note: different listener port (8080 vs 80) prevents port conflicts.
func MockGLBCSharedSpec() v1alpha1.GlobalLoadBalancerConfigSpec {
	return v1alpha1.GlobalLoadBalancerConfigSpec{
		Name: "test-glb",
		Type: global.GlobalLoadBalancerTypeLayer4,
		GlobalPools: []v1alpha1.GlobalPool{
			{
				Name:     "test-pool-shared",
				Protocol: global.GlobalPoolProtocolTCP,
				HealthMonitor: v1alpha1.GlobalPoolHealthMonitor{
					Protocol: global.GlobalPoolHealthCheckProtocolTCP,
				},
				PoolMembers: []v1alpha1.GlobalPoolMember{
					{
						Name:   "shared-pool-member-group",
						Region: "HCM-03",
						VpcId:  MockNetID,
						Type:   global.GlobalPoolMemberTypePrivate,
						Members: []v1alpha1.GlobalMember{
							{
								Name:     "shared-member",
								Address:  "10.0.0.2",
								SubnetID: MockSubnetID,
								Port:     9090,
							},
						},
					},
				},
			},
		},
		GlobalListeners: []v1alpha1.GlobalListener{
			{
				Name:            "test-listener-shared",
				Protocol:        global.GlobalListenerProtocolTCP,
				ProtocolPort:    8080,
				DefaultPoolName: ptr.To("test-pool-shared"),
			},
		},
	}
}

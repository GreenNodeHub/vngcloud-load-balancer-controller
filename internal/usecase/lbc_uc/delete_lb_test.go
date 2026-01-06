package lbc_uc

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

func TestCanDeleteWholeLoadBalancer(t *testing.T) {
	tests := []struct {
		name             string
		lbType           loadbalancerv2.LoadBalancerType
		createdListeners []v1alpha1.CreatedListener
		createdPools     []v1alpha1.CreatedPool
		setupMock        func(*repository.MockVngCloudRepository)
		wantCanDelete    bool
		wantErr          bool
	}{
		{
			name:             "L7 - empty LB - no listeners, no pools - can delete",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools:     []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L7 - all resources created by us - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{
					Id: "listener-1",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-1"},
					},
				},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-1",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-1", Name: "policy-1"},
						},
					}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
						},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L7 - has listener not created by us - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
							{UUID: "listener-external", Name: "external-listener"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:   "L7 - has policy not created by us - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{
					Id: "listener-1",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-1"},
					},
				},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-1", Name: "policy-1"},
							{UUID: "policy-external", Name: "external-policy"},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:             "L7 - has pool not created by us - cannot delete",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-1", CreatedMembers: []v1alpha1.PoolMember{}},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
							{UUID: "pool-external", Name: "external-pool"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:             "L7 - has member not created by us - cannot delete",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-1",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
							{Name: "external-member", Address: "10.0.0.99", ProtocolPort: 9090, MonitorPort: 9090},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:             "L7 - error - ListListenerOfLB fails",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools:     []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(nil, errors.New("API error"))
			},
			wantCanDelete: false,
			wantErr:       true,
		},
		{
			name:   "L7 - error - ListPolicyOfListener fails",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(nil, errors.New("API error"))
			},
			wantCanDelete: false,
			wantErr:       true,
		},
		{
			name:             "L7 - error - ListPool fails",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools:     []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(nil, errors.New("API error"))
			},
			wantCanDelete: false,
			wantErr:       true,
		},
		{
			name:             "L7 - error - GetPoolMembers fails",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-1", CreatedMembers: []v1alpha1.PoolMember{}},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(nil, errors.New("API error"))
			},
			wantCanDelete: false,
			wantErr:       true,
		},
		{
			name:   "L7 - multiple listeners all created by us - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{{Id: "policy-1"}}},
				{Id: "listener-2", CreatedPolicies: []v1alpha1.CreatedPolicy{{Id: "policy-2"}}},
				{Id: "listener-3", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
							{UUID: "listener-2", Name: "listener-2"},
							{UUID: "listener-3", Name: "listener-3"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{{UUID: "policy-1", Name: "policy-1"}},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-2").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{{UUID: "policy-2", Name: "policy-2"}},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-3").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:             "L7 - multiple pools all created by us - can delete",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-1",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
				{
					Id: "pool-2",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
						{Name: "member-3", IP: "10.0.0.3", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
							{UUID: "pool-2", Name: "pool-2"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-2").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
							{Name: "member-3", Address: "10.0.0.3", ProtocolPort: 8080, MonitorPort: 8080},
						},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L7 - complex scenario - all resources created by us - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{
					Id: "listener-1",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-1"},
						{Id: "policy-2"},
					},
				},
				{
					Id: "listener-2",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-3"},
					},
				},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-1",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
						{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
					},
				},
				{
					Id: "pool-2",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-3", IP: "10.0.0.3", Port: 9090, MonitorPort: 9090},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
							{UUID: "listener-2", Name: "listener-2"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-1", Name: "policy-1"},
							{UUID: "policy-2", Name: "policy-2"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-2").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-3", Name: "policy-3"},
						},
					}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
							{UUID: "pool-2", Name: "pool-2"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
							{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-2").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-3", Address: "10.0.0.3", ProtocolPort: 9090, MonitorPort: 9090},
						},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L7 - only listeners no pools - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:             "L7 - only pools no listeners - can delete",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-1", CreatedMembers: []v1alpha1.PoolMember{}},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:             "L7 - current LB has no resources but status has - can delete",
			lbType:           loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{{Id: "listener-old"}},
			createdPools:     []v1alpha1.CreatedPool{{Id: "pool-old"}},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{Items: []*entityv2.Listener{}}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L7 - listener with no policies in status but has policies in current - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-1").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-unknown", Name: "unknown-policy"},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		// Layer 4 test cases - L4 has no policies, skip policy check
		{
			name:   "L4 - listener created by us - can delete (no policy check)",
			lbType: loadbalancerv2.LoadBalancerTypeLayer4,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				// No ListPolicyOfListener call for L4
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{Items: []*entityv2.Pool{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L4 - multiple listeners with pools - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer4,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
				{Id: "listener-2", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-1",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
							{UUID: "listener-2", Name: "listener-2"},
						},
					}, nil)
				// No ListPolicyOfListener calls for L4
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
						},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "L4 - has external listener - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer4,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
							{UUID: "listener-external", Name: "external-listener"},
						},
					}, nil)
				// No ListPolicyOfListener calls for L4 - fails before reaching policy check
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:   "L4 - has external member - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer4,
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-1", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-1",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListListenerOfLB(mock.Anything, "lb-123").
					Return(&entityv2.ListListeners{
						Items: []*entityv2.Listener{
							{UUID: "listener-1", Name: "listener-1"},
						},
					}, nil)
				m.EXPECT().
					ListPool(mock.Anything, "lb-123").
					Return(&entityv2.ListPools{
						Items: []*entityv2.Pool{
							{UUID: "pool-1", Name: "pool-1"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-1").
					Return(&entityv2.ListMembers{
						Items: []*entityv2.Member{
							{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
							{Name: "external", Address: "10.0.0.99", ProtocolPort: 9090, MonitorPort: 9090},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVngcloudRepo := repository.NewMockVngCloudRepository(t)
			tt.setupMock(mockVngcloudRepo)

			task := &defaultModelDeployTask{
				logger:       logrus.NewEntry(logrus.New()),
				vngcloudRepo: mockVngcloudRepo,
				lbConfig: &v1alpha1.LoadBalancerConfig{
					Status: v1alpha1.LoadBalancerConfigStatus{
						CreatedListeners: tt.createdListeners,
						CreatedPools:     tt.createdPools,
					},
				},
			}

			canDelete, err := task.canDeleteWholeLoadBalancer(context.Background(), "lb-123", tt.lbType)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCanDelete, canDelete, "canDelete mismatch")
		})
	}
}

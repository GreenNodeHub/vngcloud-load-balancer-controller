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

func TestCanDeleteWholeListener(t *testing.T) {
	tests := []struct {
		name             string
		lbType           loadbalancerv2.LoadBalancerType
		listener         *entityv2.Listener
		newCreatedPools  []v1alpha1.CreatedPool
		createdListeners []v1alpha1.CreatedListener
		createdPools     []v1alpha1.CreatedPool
		setupMock        func(*repository.MockVngCloudRepository)
		wantCanDelete    bool
		wantErr          bool
	}{
		{
			name:   "layer4 listener - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer4,
			listener: &entityv2.Listener{
				UUID: "listener-123",
			},
			newCreatedPools:  []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools:     []v1alpha1.CreatedPool{},
			setupMock:        func(m *repository.MockVngCloudRepository) {},
			wantCanDelete:    true,
			wantErr:          false,
		},
		{
			name:   "layer7 - no policies, no default pool - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "",
			},
			newCreatedPools:  []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools:     []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "layer7 - all policies created by us - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{
					Id: "listener-123",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-1"},
						{Id: "policy-2"},
					},
				},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-1", Name: "policy-1"},
							{UUID: "policy-2", Name: "policy-2"},
						},
					}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "layer7 - has policy not created by us - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{
					Id: "listener-123",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-1"},
					},
				},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
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
			name:   "layer7 - default pool in newCreatedPools - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-123",
			},
			newCreatedPools: []v1alpha1.CreatedPool{
				{Id: "pool-123"},
			},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-123"},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:   "layer7 - default pool not created by us - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-external",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-123"}, // different pool
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:   "layer7 - default pool created by us and can delete whole pool - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-123",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-123",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-123").
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
			name:   "layer7 - default pool created by us but cannot delete whole pool - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-123",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-123",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-123").
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
			name:   "error - ListPolicyOfListener fails",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "",
			},
			newCreatedPools:  []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{},
			createdPools:     []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(nil, errors.New("API error"))
			},
			wantCanDelete: false,
			wantErr:       true,
		},
		{
			name:   "error - GetPoolMembers fails",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-123",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-123", CreatedMembers: []v1alpha1.PoolMember{}},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-123").
					Return(nil, errors.New("API error"))
			},
			wantCanDelete: false,
			wantErr:       true,
		},
		{
			name:   "layer7 - multiple policies all created by us with default pool - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-123",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{
					Id: "listener-123",
					CreatedPolicies: []v1alpha1.CreatedPolicy{
						{Id: "policy-1"},
						{Id: "policy-2"},
						{Id: "policy-3"},
					},
				},
			},
			createdPools: []v1alpha1.CreatedPool{
				{
					Id: "pool-123",
					CreatedMembers: []v1alpha1.PoolMember{
						{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
					},
				},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-1", Name: "policy-1"},
							{UUID: "policy-2", Name: "policy-2"},
							{UUID: "policy-3", Name: "policy-3"},
						},
					}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-123").
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
			name:   "layer7 - listener not in createdListeners but no policies exist - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-456",
				DefaultPoolId: "",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-456").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
			},
			wantCanDelete: true,
			wantErr:       false,
		},
		{
			name:   "layer7 - listener not in createdListeners and has policies - cannot delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-456",
				DefaultPoolId: "",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-456").
					Return(&entityv2.ListPolicies{
						Items: []*entityv2.Policy{
							{UUID: "policy-external", Name: "external-policy"},
						},
					}, nil)
			},
			wantCanDelete: false,
			wantErr:       false,
		},
		{
			name:   "layer7 - empty default pool - can delete",
			lbType: loadbalancerv2.LoadBalancerTypeLayer7,
			listener: &entityv2.Listener{
				UUID:          "listener-123",
				DefaultPoolId: "pool-123",
			},
			newCreatedPools: []v1alpha1.CreatedPool{},
			createdListeners: []v1alpha1.CreatedListener{
				{Id: "listener-123", CreatedPolicies: []v1alpha1.CreatedPolicy{}},
			},
			createdPools: []v1alpha1.CreatedPool{
				{Id: "pool-123", CreatedMembers: []v1alpha1.PoolMember{}},
			},
			setupMock: func(m *repository.MockVngCloudRepository) {
				m.EXPECT().
					ListPolicyOfListener(mock.Anything, "lb-123", "listener-123").
					Return(&entityv2.ListPolicies{Items: []*entityv2.Policy{}}, nil)
				m.EXPECT().
					GetPoolMembers(mock.Anything, "lb-123", "pool-123").
					Return(&entityv2.ListMembers{Items: []*entityv2.Member{}}, nil)
			},
			wantCanDelete: true,
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
					Spec: v1alpha1.LoadBalancerConfigSpec{
						Type: tt.lbType,
					},
					Status: v1alpha1.LoadBalancerConfigStatus{
						CreatedListeners: tt.createdListeners,
						CreatedPools:     tt.createdPools,
					},
				},
			}

			canDelete, err := task.canDeleteWholeListener(
				context.Background(),
				"lb-123",
				tt.createdListeners,
				tt.listener,
				tt.newCreatedPools,
			)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCanDelete, canDelete, "canDelete mismatch")
		})
	}
}

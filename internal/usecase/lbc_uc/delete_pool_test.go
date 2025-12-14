package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func TestCanDeleteWholePool(t *testing.T) {
	tests := []struct {
		name               string
		currentListMembers *entity.ListMembers
		createdMembers     []v1alpha1.PoolMember
		newCreatedMembers  []v1alpha1.PoolMember
		wantCanDelete      bool
		wantUpdateRequest  bool
	}{
		{
			name: "can delete - all members created by us and not in new spec",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     true,
			wantUpdateRequest: false,
		},
		{
			name: "cannot delete - has external members not created by us",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "external-member", Address: "10.0.0.99", ProtocolPort: 9090, MonitorPort: 9090},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "cannot delete - some members still in new spec",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "no update needed - current state matches desired state",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			wantCanDelete:     false,
			wantUpdateRequest: false,
		},
		{
			name: "can delete - empty pool",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{},
			},
			createdMembers:    []v1alpha1.PoolMember{},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     true,
			wantUpdateRequest: false,
		},
		{
			name:               "can delete - nil current members",
			currentListMembers: nil,
			createdMembers:     []v1alpha1.PoolMember{},
			newCreatedMembers:  []v1alpha1.PoolMember{},
			wantCanDelete:      true,
			wantUpdateRequest:  false,
		},
		{
			name: "mixed scenario - external members + partial spec",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "external", Address: "10.0.0.99", ProtocolPort: 9090, MonitorPort: 9090},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "cannot delete - all members are external",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "external-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "external-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers:    []v1alpha1.PoolMember{},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     false,
			wantUpdateRequest: false,
		},
		{
			name: "can delete - single member created by us not in new spec",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     true,
			wantUpdateRequest: false,
		},
		{
			name: "same IP different ports - treated as different members",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.1", ProtocolPort: 9090, MonitorPort: 9090},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "same IP same port different monitor port - treated as different members",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 9090},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "new spec adds members not in current - needs update",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
			},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "new spec replaces all members - needs update",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "old-member", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "old-member", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{
				{Name: "new-member", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
			},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "partial created members - only remove what we created",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-3", Address: "10.0.0.3", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "nil items in current members list",
			currentListMembers: &entity.ListMembers{
				Items: nil,
			},
			createdMembers:    []v1alpha1.PoolMember{},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     true,
			wantUpdateRequest: false,
		},
		{
			name: "many members - all created by us and removed",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-3", Address: "10.0.0.3", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-4", Address: "10.0.0.4", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-5", Address: "10.0.0.5", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
				{Name: "member-3", IP: "10.0.0.3", Port: 8080, MonitorPort: 8080},
				{Name: "member-4", IP: "10.0.0.4", Port: 8080, MonitorPort: 8080},
				{Name: "member-5", IP: "10.0.0.5", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     true,
			wantUpdateRequest: false,
		},
		{
			name: "scale down - keep some members remove others",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "member-1", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-2", Address: "10.0.0.2", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "member-3", Address: "10.0.0.3", ProtocolPort: 8080, MonitorPort: 8080},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
				{Name: "member-2", IP: "10.0.0.2", Port: 8080, MonitorPort: 8080},
				{Name: "member-3", IP: "10.0.0.3", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{
				{Name: "member-1", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
		{
			name: "external member with same IP as created but different port",
			currentListMembers: &entity.ListMembers{
				Items: []*entity.Member{
					{Name: "created", Address: "10.0.0.1", ProtocolPort: 8080, MonitorPort: 8080},
					{Name: "external", Address: "10.0.0.1", ProtocolPort: 443, MonitorPort: 443},
				},
			},
			createdMembers: []v1alpha1.PoolMember{
				{Name: "created", IP: "10.0.0.1", Port: 8080, MonitorPort: 8080},
			},
			newCreatedMembers: []v1alpha1.PoolMember{},
			wantCanDelete:     false,
			wantUpdateRequest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelDeployTask{
				logger: logrus.NewEntry(logrus.New()),
			}

			canDelete, updateRequest := task.canDeleteWholePool(
				context.Background(),
				"lb-123",
				"pool-123",
				tt.currentListMembers,
				tt.createdMembers,
				tt.newCreatedMembers,
			)

			assert.Equal(t, tt.wantCanDelete, canDelete, "canDelete mismatch")
			if tt.wantUpdateRequest {
				assert.NotNil(t, updateRequest, "expected updateRequest to be non-nil")
			} else {
				assert.Nil(t, updateRequest, "expected updateRequest to be nil")
			}
		})
	}
}

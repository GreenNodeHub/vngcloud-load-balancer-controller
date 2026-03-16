package glbc_uc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func TestMergePoolMembers(t *testing.T) {
	tests := []struct {
		name    string
		created []v1alpha1.GlobalMember
		current []v1alpha1.GlobalMember
		spec    []v1alpha1.GlobalMember
		wantLen int
		check   func(t *testing.T, result []v1alpha1.GlobalMember)
	}{
		{
			name:    "add_new_member",
			created: []v1alpha1.GlobalMember{},
			current: []v1alpha1.GlobalMember{},
			spec: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 8080, SubnetID: "subnet-a"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []v1alpha1.GlobalMember) {
				assert.Equal(t, "10.0.0.1", result[0].Address)
				assert.Equal(t, 8080, result[0].Port)
			},
		},
		{
			name: "remove_deleted_member",
			created: func() []v1alpha1.GlobalMember {
				memberA := v1alpha1.GlobalMember{Address: "10.0.0.2", Port: 8080, SubnetID: "subnet-b"}
				return []v1alpha1.GlobalMember{memberA}
			}(),
			current: func() []v1alpha1.GlobalMember {
				memberA := v1alpha1.GlobalMember{Address: "10.0.0.2", Port: 8080, SubnetID: "subnet-b"}
				return []v1alpha1.GlobalMember{memberA}
			}(),
			spec:    []v1alpha1.GlobalMember{},
			wantLen: 0,
			check:   func(t *testing.T, result []v1alpha1.GlobalMember) {},
		},
		{
			name: "update_existing_member",
			created: func() []v1alpha1.GlobalMember {
				oldMember := v1alpha1.GlobalMember{Address: "10.0.0.3", Port: 8080, SubnetID: "subnet-c"}
				return []v1alpha1.GlobalMember{oldMember}
			}(),
			current: func() []v1alpha1.GlobalMember {
				oldMember := v1alpha1.GlobalMember{Address: "10.0.0.3", Port: 8080, SubnetID: "subnet-c"}
				return []v1alpha1.GlobalMember{oldMember}
			}(),
			spec: []v1alpha1.GlobalMember{
				{Address: "10.0.0.3", Port: 9090, SubnetID: "subnet-c"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []v1alpha1.GlobalMember) {
				assert.Equal(t, 9090, result[0].Port)
			},
		},
		{
			name:    "preserve_manually-added_member",
			created: []v1alpha1.GlobalMember{},
			current: []v1alpha1.GlobalMember{
				{Address: "10.0.0.4", Port: 8080, SubnetID: "subnet-d"},
			},
			spec:    []v1alpha1.GlobalMember{},
			wantLen: 1,
			check: func(t *testing.T, result []v1alpha1.GlobalMember) {
				assert.Equal(t, "10.0.0.4", result[0].Address)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &defaultModelDeployTask{}
			result := task.mergePoolMembers(context.Background(), tt.created, tt.current, tt.spec)
			assert.Len(t, result, tt.wantLen)
			tt.check(t, result)
		})
	}
}

func TestConvertMember_IncludesSubnetID(t *testing.T) {
	tests := []struct {
		name           string
		input          *entityv2.GlobalPoolMemberDetail
		wantSubnetID   string
		wantAddress    string
		wantPort       int
		wantBackupRole bool
	}{
		{
			name: "member with SubnetID is preserved",
			input: &entityv2.GlobalPoolMemberDetail{
				Name:        "member-1",
				Address:     "10.0.0.1",
				Port:        8080,
				BackupRole:  false,
				Weight:      1,
				MonitorPort: 8080,
				SubnetID:    "subnet-123",
			},
			wantSubnetID:   "subnet-123",
			wantAddress:    "10.0.0.1",
			wantPort:       8080,
			wantBackupRole: false,
		},
		{
			name: "member with different SubnetID is preserved",
			input: &entityv2.GlobalPoolMemberDetail{
				Name:        "member-2",
				Address:     "10.0.0.2",
				Port:        9090,
				BackupRole:  true,
				Weight:      5,
				MonitorPort: 9090,
				SubnetID:    "subnet-456",
			},
			wantSubnetID:   "subnet-456",
			wantAddress:    "10.0.0.2",
			wantPort:       9090,
			wantBackupRole: true,
		},
		{
			name: "member with empty SubnetID returns empty SubnetID",
			input: &entityv2.GlobalPoolMemberDetail{
				Name:        "member-3",
				Address:     "10.0.0.3",
				Port:        8080,
				BackupRole:  false,
				Weight:      1,
				MonitorPort: 8080,
				SubnetID:    "",
			},
			wantSubnetID:   "",
			wantAddress:    "10.0.0.3",
			wantPort:       8080,
			wantBackupRole: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMember(tt.input)

			assert.NotNil(t, result)
			assert.Equal(t, tt.wantSubnetID, result.SubnetID, "SubnetID must be populated from input member")
			assert.Equal(t, tt.wantAddress, result.Address)
			assert.Equal(t, tt.wantPort, result.Port)
			assert.Equal(t, tt.wantBackupRole, result.BackupRole)
		})
	}
}

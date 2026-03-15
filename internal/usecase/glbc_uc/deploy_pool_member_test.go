package glbc_uc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
)

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

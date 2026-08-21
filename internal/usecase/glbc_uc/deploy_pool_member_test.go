package glbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
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
		{
			name: "no_duplicate_when_pointer_fields_differ",
			created: func() []v1alpha1.GlobalMember {
				w := 1
				mp := 32556
				return []v1alpha1.GlobalMember{
					{Address: "10.0.5.5", Port: 32556, SubnetID: "subnet-e", Weight: &w, MonitorPort: &mp},
				}
			}(),
			current: func() []v1alpha1.GlobalMember {
				w := 1
				mp := 32556
				return []v1alpha1.GlobalMember{
					{Address: "10.0.5.5", Port: 32556, SubnetID: "subnet-e", Weight: &w, MonitorPort: &mp},
				}
			}(),
			spec: []v1alpha1.GlobalMember{
				{Address: "10.0.5.5", Port: 32556, SubnetID: "subnet-e"},
			},
			wantLen: 1,
			check: func(t *testing.T, result []v1alpha1.GlobalMember) {
				assert.Equal(t, "10.0.5.5", result[0].Address)
				assert.Equal(t, 32556, result[0].Port)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// mergePoolMembers logs the members it keeps that this cluster did not create, so
			// the task needs a logger the way the real one always has.
			task := &defaultModelDeployTask{logger: logrus.NewEntry(logrus.New())}
			result := task.mergePoolMembers(context.Background(), tt.created, tt.current, tt.spec)
			assert.Len(t, result, tt.wantLen)
			tt.check(t, result)
		})
	}
}

func intPtr(v int) *int { return &v }

func TestPtrIntEqual(t *testing.T) {
	tests := []struct {
		name string
		a    *int
		b    *int
		want bool
	}{
		{name: "both_nil", a: nil, b: nil, want: true},
		{name: "first_nil", a: nil, b: intPtr(1), want: false},
		{name: "second_nil", a: intPtr(1), b: nil, want: false},
		{name: "equal_values", a: intPtr(1), b: intPtr(1), want: true},
		{name: "different_values", a: intPtr(1), b: intPtr(2), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ptrIntEqual(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComparePoolMembers_PointerFields(t *testing.T) {
	tests := []struct {
		name  string
		listA []v1alpha1.GlobalMember
		listB []v1alpha1.GlobalMember
		want  bool
	}{
		{
			name: "matching_with_different_pointer_allocations",
			listA: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(1), MonitorPort: intPtr(80)},
			},
			listB: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(1), MonitorPort: intPtr(80)},
			},
			want: true,
		},
		{
			name: "nil_vs_populated_weight",
			listA: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(1), MonitorPort: nil},
			},
			listB: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: nil, MonitorPort: nil},
			},
			want: false,
		},
		{
			name: "nil_vs_populated_monitorport",
			listA: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: nil, MonitorPort: intPtr(80)},
			},
			listB: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: nil, MonitorPort: nil},
			},
			want: false,
		},
		{
			name: "both_nil_optional_fields",
			listA: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: nil, MonitorPort: nil},
			},
			listB: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: nil, MonitorPort: nil},
			},
			want: true,
		},
		{
			name: "different_weight_values",
			listA: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(1), MonitorPort: nil},
			},
			listB: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(2), MonitorPort: nil},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := comparePoolMembers(tt.listA, tt.listB)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckIfPoolMemberExist_MixedPointers(t *testing.T) {
	tests := []struct {
		name   string
		list   []v1alpha1.GlobalMember
		member v1alpha1.GlobalMember
		want   bool
	}{
		{
			name: "nil_weight_does_not_match_populated",
			list: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(1), MonitorPort: nil},
			},
			member: v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: nil, MonitorPort: nil},
			want:   false,
		},
		{
			name: "matching_values_different_pointers",
			list: []v1alpha1.GlobalMember{
				{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(5), MonitorPort: nil},
			},
			member: v1alpha1.GlobalMember{Address: "10.0.0.1", Port: 80, SubnetID: "sub-1", Weight: intPtr(5), MonitorPort: nil},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkIfPoolMemberExist(tt.list, &tt.member)
			assert.Equal(t, tt.want, got)
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

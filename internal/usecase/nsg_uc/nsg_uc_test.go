package nsg_uc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	networkv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/network/v2"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

// ── resolveStringArrayChange ────────────────────────────────────────────────

func TestResolveStringArrayChange(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		old           []string
		new           []string
		wantRemove    []string
		wantNotChange []string
		wantAdd       []string
	}{
		{
			name:          "empty old and new",
			old:           []string{},
			new:           []string{},
			wantRemove:    nil,
			wantNotChange: nil,
			wantAdd:       nil,
		},
		{
			name:          "all added — empty old",
			old:           []string{},
			new:           []string{"a", "b"},
			wantRemove:    nil,
			wantNotChange: nil,
			wantAdd:       []string{"a", "b"},
		},
		{
			name:          "all removed — empty new",
			old:           []string{"a", "b"},
			new:           []string{},
			wantRemove:    []string{"a", "b"},
			wantNotChange: nil,
			wantAdd:       nil,
		},
		{
			name:          "no change",
			old:           []string{"a", "b"},
			new:           []string{"a", "b"},
			wantRemove:    nil,
			wantNotChange: []string{"a", "b"},
			wantAdd:       nil,
		},
		{
			name:          "mixed: remove, keep, add",
			old:           []string{"1", "2", "3"},
			new:           []string{"2", "3", "4"},
			wantRemove:    []string{"1"},
			wantNotChange: []string{"2", "3"},
			wantAdd:       []string{"4"},
		},
		{
			name:          "completely replaced",
			old:           []string{"a", "b"},
			new:           []string{"c", "d"},
			wantRemove:    []string{"a", "b"},
			wantNotChange: nil,
			wantAdd:       []string{"c", "d"},
		},
		{
			name:          "single element unchanged",
			old:           []string{"x"},
			new:           []string{"x"},
			wantRemove:    nil,
			wantNotChange: []string{"x"},
			wantAdd:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remove, notChange, add := resolveStringArrayChange(ctx, tt.old, tt.new)
			assert.ElementsMatch(t, tt.wantRemove, remove)
			assert.ElementsMatch(t, tt.wantNotChange, notChange)
			assert.ElementsMatch(t, tt.wantAdd, add)
		})
	}
}

// ── mergeStringArray ────────────────────────────────────────────────────────

func TestMergeStringArray(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		current     []string
		remove      []string
		add         []string
		wantResult  []string
		wantChanged bool
	}{
		{
			name:        "no change — add already present, nothing to remove",
			current:     []string{"a", "b"},
			remove:      []string{},
			add:         []string{"a", "b"},
			wantResult:  []string{"a", "b"},
			wantChanged: false,
		},
		{
			name:        "add new element",
			current:     []string{"a"},
			remove:      []string{},
			add:         []string{"b"},
			wantResult:  []string{"a", "b"},
			wantChanged: true,
		},
		{
			name:        "remove existing element",
			current:     []string{"a", "b"},
			remove:      []string{"a"},
			add:         []string{},
			wantResult:  []string{"b"},
			wantChanged: true,
		},
		{
			name:        "remove and add simultaneously",
			current:     []string{"a", "b"},
			remove:      []string{"a"},
			add:         []string{"c"},
			wantResult:  []string{"b", "c"},
			wantChanged: true,
		},
		{
			name:        "remove element not in current — no change",
			current:     []string{"a"},
			remove:      []string{"x"},
			add:         []string{},
			wantResult:  []string{"a"},
			wantChanged: false,
		},
		{
			name:        "remove all elements",
			current:     []string{"a", "b"},
			remove:      []string{"a", "b"},
			add:         []string{},
			wantResult:  []string{},
			wantChanged: true,
		},
		{
			name:        "empty current, add elements",
			current:     []string{},
			remove:      []string{},
			add:         []string{"a", "b"},
			wantResult:  []string{"a", "b"},
			wantChanged: true,
		},
		{
			name:        "three-way merge: preserves external secgroups not in remove list",
			current:     []string{"external", "old-managed"},
			remove:      []string{"old-managed"},
			add:         []string{"new-managed"},
			wantResult:  []string{"external", "new-managed"},
			wantChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changed := mergeStringArray(ctx, tt.current, tt.remove, tt.add)
			assert.ElementsMatch(t, tt.wantResult, result)
			assert.Equal(t, tt.wantChanged, changed)
		})
	}
}

// ── compareSecgroupRule ─────────────────────────────────────────────────────

func ingressRule(id string, from, to int) *entityv2.SecgroupRule {
	return &entityv2.SecgroupRule{
		Id:             id,
		Direction:      "ingress",
		EtherType:      "IPv4",
		Protocol:       "tcp",
		RemoteIPPrefix: "0.0.0.0/0",
		PortRangeMin:   from,
		PortRangeMax:   to,
	}
}

func egressRule(id string) *entityv2.SecgroupRule {
	return &entityv2.SecgroupRule{
		Id:           id,
		Direction:    "egress",
		EtherType:    "IPv4",
		Protocol:     "any",
		PortRangeMin: 0,
		PortRangeMax: 0,
	}
}

func desiredRule(cidr string, from, to int32) v1alpha1.NodeSecurityGroupRule {
	return v1alpha1.NodeSecurityGroupRule{
		Direction: networkv2.SecgroupRuleDirectionIngress,
		EtherType: networkv2.SecgroupRuleEtherTypeIPv4,
		Protocol:  networkv2.SecgroupRuleProtocol("tcp"),
		CIDR:      cidr,
		FromPort:  from,
		ToPort:    to,
	}
}

func TestCompareSecgroupRule(t *testing.T) {
	ctx := context.Background()
	uc := &nsgUseCase{}

	tests := []struct {
		name       string
		current    []*entityv2.SecgroupRule
		desired    []v1alpha1.NodeSecurityGroupRule
		wantDelete []string // IDs expected in needDelete
		wantCreate int      // count expected in needCreate
	}{
		{
			name:       "no current rules, no desired — nothing to do",
			current:    []*entityv2.SecgroupRule{},
			desired:    []v1alpha1.NodeSecurityGroupRule{},
			wantDelete: []string{},
			wantCreate: 0,
		},
		{
			name:       "desired rule already exists — no-op",
			current:    []*entityv2.SecgroupRule{ingressRule("r1", 80, 80)},
			desired:    []v1alpha1.NodeSecurityGroupRule{desiredRule("0.0.0.0/0", 80, 80)},
			wantDelete: []string{},
			wantCreate: 0,
		},
		{
			name:       "current rule not in desired — should delete",
			current:    []*entityv2.SecgroupRule{ingressRule("r1", 80, 80)},
			desired:    []v1alpha1.NodeSecurityGroupRule{},
			wantDelete: []string{"r1"},
			wantCreate: 0,
		},
		{
			name:       "desired rule missing from current — should create",
			current:    []*entityv2.SecgroupRule{},
			desired:    []v1alpha1.NodeSecurityGroupRule{desiredRule("0.0.0.0/0", 443, 443)},
			wantDelete: []string{},
			wantCreate: 1,
		},
		{
			name: "mixed: keep one, delete one, create one",
			current: []*entityv2.SecgroupRule{
				ingressRule("r1", 80, 80),
				ingressRule("r2", 22, 22),
			},
			desired: []v1alpha1.NodeSecurityGroupRule{
				desiredRule("0.0.0.0/0", 80, 80),   // matches r1 — keep
				desiredRule("0.0.0.0/0", 443, 443), // no match — create
			},
			wantDelete: []string{"r2"},
			wantCreate: 1,
		},
		{
			name: "egress rules not in desired are deleted — controller owns all rules",
			current: []*entityv2.SecgroupRule{
				ingressRule("r1", 80, 80),
				egressRule("egress-1"),
				egressRule("egress-2"),
			},
			desired: []v1alpha1.NodeSecurityGroupRule{
				desiredRule("0.0.0.0/0", 80, 80),
			},
			wantDelete: []string{"egress-1", "egress-2"}, // unmatched egress rules are removed
			wantCreate: 0,
		},
		{
			name: "egress-only current rules with empty desired — all deleted",
			current: []*entityv2.SecgroupRule{
				egressRule("egress-1"),
			},
			desired:    []v1alpha1.NodeSecurityGroupRule{},
			wantDelete: []string{"egress-1"},
			wantCreate: 0,
		},
		{
			name: "matching is case-insensitive for direction, protocol, ethertype",
			current: []*entityv2.SecgroupRule{
				{
					Id:             "r1",
					Direction:      "INGRESS",
					EtherType:      "ipv4",
					Protocol:       "TCP",
					RemoteIPPrefix: "10.0.0.0/8",
					PortRangeMin:   8080,
					PortRangeMax:   8080,
				},
			},
			desired: []v1alpha1.NodeSecurityGroupRule{
				desiredRule("10.0.0.0/8", 8080, 8080),
			},
			wantDelete: []string{},
			wantCreate: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			del, create, err := uc.compareSecgroupRule(ctx, tt.current, tt.desired)
			require.NoError(t, err)

			delIDs := make([]string, 0, len(del))
			for _, r := range del {
				delIDs = append(delIDs, r.Id)
			}
			assert.ElementsMatch(t, tt.wantDelete, delIDs)
			assert.Len(t, create, tt.wantCreate)
		})
	}
}

// ── nodeInfosEqual ───────────────────────────────────────────────────────────

func TestNodeInfosEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []v1alpha1.NodeInfo
		b    []v1alpha1.NodeInfo
		want bool
	}{
		{
			name: "both nil", a: nil, b: nil, want: true,
		},
		{
			name: "both empty",
			a:    []v1alpha1.NodeInfo{},
			b:    []v1alpha1.NodeInfo{},
			want: true,
		},
		{
			name: "identical single entry",
			a:    []v1alpha1.NodeInfo{{Name: "n1", ServerId: "ins-1"}},
			b:    []v1alpha1.NodeInfo{{Name: "n1", ServerId: "ins-1"}},
			want: true,
		},
		{
			name: "different length",
			a:    []v1alpha1.NodeInfo{{Name: "n1"}},
			b:    []v1alpha1.NodeInfo{},
			want: false,
		},
		{
			name: "same entries different order",
			a: []v1alpha1.NodeInfo{
				{Name: "n1", ServerId: "ins-1"},
				{Name: "n2", ServerId: "ins-2"},
			},
			b: []v1alpha1.NodeInfo{
				{Name: "n2", ServerId: "ins-2"},
				{Name: "n1", ServerId: "ins-1"},
			},
			want: true,
		},
		{
			name: "different server IDs",
			a:    []v1alpha1.NodeInfo{{Name: "n1", ServerId: "ins-1"}},
			b:    []v1alpha1.NodeInfo{{Name: "n1", ServerId: "ins-X"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nodeInfosEqual(tt.a, tt.b))
		})
	}
}

// ── errorToStringPtr ────────────────────────────────────────────────────────

func TestErrorToStringPtr(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		assert.Nil(t, errorToStringPtr(nil))
	})

	t.Run("non-nil error returns pointer to message", func(t *testing.T) {
		ptr := errorToStringPtr(assert.AnError)
		require.NotNil(t, ptr)
		assert.Equal(t, assert.AnError.Error(), *ptr)
	})
}

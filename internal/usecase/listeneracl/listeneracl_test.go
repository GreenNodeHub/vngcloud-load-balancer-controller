package listeneracl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

func parse(t *testing.T, anns map[string]string) (Fields, error) {
	t.Helper()
	return FromAnnotations(annotations.NewSuffixAnnotationParser("vks.vngcloud.vn"), anns)
}

func TestNoAnnotationsManageNothing(t *testing.T) {
	f, err := parse(t, map[string]string{})
	require.NoError(t, err)
	assert.Nil(t, f.AllowedCidrs)
	assert.Nil(t, f.BlockedCidrs)
	assert.Nil(t, f.DefaultAction)
}

func TestDroppedCidrsAreCleanedAndJoined(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/dropped-cidrs": " 192.0.2.1/32, ,198.51.100.0/24,"})
	require.NoError(t, err)
	require.NotNil(t, f.BlockedCidrs)
	assert.Equal(t, "192.0.2.1/32,198.51.100.0/24", *f.BlockedCidrs)
	assert.Nil(t, f.DefaultAction, "dropping alone says nothing about traffic that matches no rule")
}

func TestBareIpIsAccepted(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/dropped-cidrs": "203.0.113.9"})
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.9", *f.BlockedCidrs)
}

// vLB does not support IPv6; sending it would fail on every reconcile.
func TestIPv6IsRejected(t *testing.T) {
	_, err := parse(t, map[string]string{"vks.vngcloud.vn/dropped-cidrs": "2001:db8::/32"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IPv4")
}

// Dropping everything needs no default action: the dropped list is checked first.
func TestDropAllIsAccepted(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/dropped-cidrs": "0.0.0.0/0"})
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0/0", *f.BlockedCidrs)
}

// An empty value still means "managed": it is how a user clears the list without giving it up.
func TestEmptyDroppedCidrsIsManagedAndEmpty(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/dropped-cidrs": ""})
	require.NoError(t, err)
	require.NotNil(t, f.BlockedCidrs)
	assert.Equal(t, "", *f.BlockedCidrs)
}

// A typo in a block list must never turn into an unblock, so a bad entry fails the whole build.
func TestInvalidDroppedEntryIsAnError(t *testing.T) {
	_, err := parse(t, map[string]string{"vks.vngcloud.vn/dropped-cidrs": "192.0.2.1/32,192.0.2.0/33"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "192.0.2.0/33")
}

func TestDefaultActionIsNormalised(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/acl-default-action": " DROP "})
	require.NoError(t, err)
	assert.Equal(t, ActionDrop, *f.DefaultAction)
}

func TestInvalidDefaultActionIsAnError(t *testing.T) {
	_, err := parse(t, map[string]string{"vks.vngcloud.vn/acl-default-action": "reject"})
	require.Error(t, err)
}

// inbound-cidrs is a whitelist; with the portal's default of accept it would let everyone in.
func TestInboundCidrsImpliesDrop(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/inbound-cidrs": "192.0.2.0/24"})
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.0/24", *f.AllowedCidrs)
	assert.Equal(t, ActionDrop, *f.DefaultAction)
}

func TestExplicitDefaultActionWinsOverTheImpliedOne(t *testing.T) {
	f, err := parse(t, map[string]string{
		"vks.vngcloud.vn/inbound-cidrs":      "192.0.2.0/24",
		"vks.vngcloud.vn/acl-default-action": "accept",
	})
	require.NoError(t, err)
	assert.Equal(t, ActionAccept, *f.DefaultAction)
}

// inbound-cidrs keeps its old, unvalidated parsing so no running manifest breaks on upgrade.
func TestInboundCidrsIsNotValidated(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/inbound-cidrs": "not-a-cidr"})
	require.NoError(t, err)
	assert.Equal(t, "not-a-cidr", *f.AllowedCidrs)
}

func TestEmptyInboundCidrsImpliesNothing(t *testing.T) {
	f, err := parse(t, map[string]string{"vks.vngcloud.vn/inbound-cidrs": ""})
	require.NoError(t, err)
	assert.Nil(t, f.DefaultAction)
}

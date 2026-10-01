package lbc_uc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func noPeers(aclField) bool { return false }

func cur(allowed, blocked, action string) v1alpha1.ListenerAcl {
	return currentAcl(&entityv2.Listener{AllowedCidrs: allowed, BlockedCidrs: blocked, DefaultAction: action})
}

func TestPlanAclTakesOverAndRecordsWhatItDisplaces(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32")},
		cur("0.0.0.0/0", "192.0.2.1/32", "accept"), nil, noPeers)
	assert.Equal(t, "203.0.113.9/32", *p.Desired.BlockedCidrs)
	assert.Equal(t, "192.0.2.1/32", *p.RecordBefore.BlockedCidrs, "the user's list is what goes back")
	assert.Equal(t, "192.0.2.1/32", *p.RecordAfter.BlockedCidrs)
	assert.Nil(t, p.Desired.AllowedCidrs)
	assert.Nil(t, p.Desired.DefaultAction)
}

func TestPlanAclRecordsNothingWhenTheValueIsAlreadyThere(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32")},
		cur("0.0.0.0/0", "203.0.113.9/32", "accept"), nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.Nil(t, p.RecordBefore, "we did not author a value we found in place")
}

func TestPlanAclReappliesSpecOverAPortalEdit(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")}
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32")},
		cur("0.0.0.0/0", "203.0.113.8/32", "accept"), rec, noPeers)
	assert.Equal(t, "203.0.113.9/32", *p.Desired.BlockedCidrs)
	assert.Equal(t, "", *p.RecordBefore.BlockedCidrs, "the original is recorded once and never rewritten")
}

func TestPlanAclPutsTheOriginalBackWhenTheAnnotationIsGone(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1/32")}
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("0.0.0.0/0", "203.0.113.9/32", "accept"), rec, noPeers)
	assert.Equal(t, "192.0.2.1/32", *p.Desired.BlockedCidrs)
	assert.True(t, p.RecordBefore.Equal(rec))
	assert.True(t, p.RecordAfter.IsEmpty(), "released after the put")
}

func TestPlanAclReleasesWithoutAPutWhenTheOriginalIsAlreadyBack(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")}
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("0.0.0.0/0", "", "accept"), rec, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.True(t, p.RecordAfter.IsEmpty())
}

// Review Focus 4: a cluster upgraded from a release without records must see no change.
func TestPlanAclLeavesUnmanagedFieldsAlone(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("192.0.2.0/24", "203.0.113.9/32", "drop"), nil, noPeers)
	assert.True(t, p.Desired.IsEmpty())
	assert.True(t, p.RecordBefore.IsEmpty())
	assert.True(t, p.RecordAfter.IsEmpty())
}

// Shared listener: another LBC still declares it, so putting the original back would only open
// the listener until that LBC re-applies. Fail closed: drop our record, leave the value.
func TestPlanAclDoesNotRevertWhileAPeerStillDeclaresTheField(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")}
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("0.0.0.0/0", "203.0.113.9/32", "accept"), rec,
		func(f aclField) bool { return f == aclBlocked })
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.True(t, p.RecordAfter.IsEmpty())
}

// Review Focus 1: vLB may echo the list back reformatted; that is not a change.
func TestPlanAclComparesCidrsAsSets(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32,192.0.2.0/24")},
		cur("0.0.0.0/0", "192.0.2.0/24, 203.0.113.9/32", "accept"), nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
}

// Review Focus 3: a listener from before ACL existed reports no default action.
func TestPlanAclRecordsAcceptForAListenerWithNoDefaultAction(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{DefaultAction: ptrTo("drop")}, cur("192.0.2.0/24", "", ""), nil, noPeers)
	assert.Equal(t, "drop", *p.Desired.DefaultAction)
	assert.Equal(t, "accept", *p.RecordBefore.DefaultAction)
}

func TestSpecAclTreatsAnEmptyAllowListAsUnmanaged(t *testing.T) {
	assert.Nil(t, specAcl(v1alpha1.Listener{AllowedCidrs: ptrTo("")}).AllowedCidrs)
}

func TestNeutralAclCoversOnlyTheFieldsTheSpecSets(t *testing.T) {
	n := neutralAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32"), DefaultAction: ptrTo("drop")}, "0.0.0.0/0")
	assert.Nil(t, n.AllowedCidrs)
	assert.Equal(t, "", *n.BlockedCidrs)
	assert.Equal(t, "accept", *n.DefaultAction)
}

package lbc_uc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	entityv2 "github.com/GreenNodeHub/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
)

func noPeers(aclField) bool { return false }

func cur(allowed, blocked, action string) v1alpha1.ListenerAcl {
	return currentAcl(&entityv2.Listener{AllowedCidrs: allowed, BlockedCidrs: blocked, DefaultAction: action})
}

func TestPlanAclTakesOverAndRecordsWhatItDisplaces(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32")},
		cur("0.0.0.0/0", "192.0.2.1/32", "accept"), nil, nil, noPeers)
	assert.Equal(t, "203.0.113.9/32", *p.Desired.BlockedCidrs)
	assert.Equal(t, "192.0.2.1/32", *p.RecordBefore.BlockedCidrs, "the user's list is what goes back")
	assert.Equal(t, "192.0.2.1/32", *p.RecordAfter.BlockedCidrs)
	assert.Nil(t, p.Desired.AllowedCidrs)
	assert.Nil(t, p.Desired.DefaultAction)
}

func TestPlanAclRecordsNothingWhenTheValueIsAlreadyThere(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32")},
		cur("0.0.0.0/0", "203.0.113.9/32", "accept"), nil, nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.Nil(t, p.RecordBefore, "we did not author a value we found in place")
}

func TestPlanAclReappliesSpecOverAPortalEdit(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")}
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32")},
		cur("0.0.0.0/0", "203.0.113.8/32", "accept"), rec, nil, noPeers)
	assert.Equal(t, "203.0.113.9/32", *p.Desired.BlockedCidrs)
	assert.Equal(t, "", *p.RecordBefore.BlockedCidrs, "the original is recorded once and never rewritten")
}

func TestPlanAclPutsTheOriginalBackWhenTheAnnotationIsGone(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1/32")}
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("0.0.0.0/0", "203.0.113.9/32", "accept"), rec, nil, noPeers)
	assert.Equal(t, "192.0.2.1/32", *p.Desired.BlockedCidrs)
	assert.True(t, p.RecordBefore.Equal(rec))
	assert.True(t, p.RecordAfter.IsEmpty(), "released after the put")
}

func TestPlanAclReleasesWithoutAPutWhenTheOriginalIsAlreadyBack(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")}
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("0.0.0.0/0", "", "accept"), rec, nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.True(t, p.RecordAfter.IsEmpty())
}

// Review Focus 4: a cluster upgraded from a release without records must see no change.
func TestPlanAclLeavesUnmanagedFieldsAlone(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("192.0.2.0/24", "203.0.113.9/32", "drop"), nil, nil, noPeers)
	assert.True(t, p.Desired.IsEmpty())
	assert.True(t, p.RecordBefore.IsEmpty())
	assert.True(t, p.RecordAfter.IsEmpty())
}

// Shared listener: another LBC still declares it, so putting the original back would only open
// the listener until that LBC re-applies. Fail closed: drop our record, leave the value.
func TestPlanAclDoesNotRevertWhileAPeerStillDeclaresTheField(t *testing.T) {
	rec := &v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("")}
	p := planListenerAcl(v1alpha1.ListenerAcl{}, cur("0.0.0.0/0", "203.0.113.9/32", "accept"), rec, nil,
		func(f aclField) bool { return f == aclBlocked })
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.True(t, p.RecordAfter.IsEmpty())
}

// Review Focus 1: vLB may echo the list back reformatted; that is not a change.
func TestPlanAclComparesCidrsAsSets(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32,192.0.2.0/24")},
		cur("0.0.0.0/0", "192.0.2.0/24, 203.0.113.9/32", "accept"), nil, nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
}

// Final review R9: the validator accepts a bare IP and host bits; vLB may echo either back in
// canonical form, which must not read as a change on every reconcile.
func TestPlanAclTreatsABareIPAsSlash32(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1")},
		cur("0.0.0.0/0", "192.0.2.1/32", "accept"), nil, nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.Nil(t, p.RecordBefore)
}

func TestPlanAclIgnoresHostBitsInACidr(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1/22")},
		cur("0.0.0.0/0", "192.0.2.0/22", "accept"), nil, nil, noPeers)
	assert.Nil(t, p.Desired.BlockedCidrs)
	assert.Nil(t, p.RecordBefore)
}

func TestPlanAclStillSeesADifferentBlock(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("192.0.2.1/22")},
		cur("0.0.0.0/0", "192.0.2.0/24", "accept"), nil, nil, noPeers)
	assert.Equal(t, "192.0.2.1/22", *p.Desired.BlockedCidrs, "the user's text is what is sent")
}

// Review Focus 3: a listener reporting no default action is recorded as drop (vLB API default, measured 2026-10-01).
func TestPlanAclRecordsDropForAListenerWithNoDefaultAction(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{DefaultAction: ptrTo("drop")}, cur("192.0.2.0/24", "", ""), nil, nil, noPeers)
	assert.Equal(t, "drop", *p.Desired.DefaultAction)
	assert.Equal(t, "drop", *p.RecordBefore.DefaultAction)
}

func TestSpecAclTreatsAnEmptyAllowListAsUnmanaged(t *testing.T) {
	assert.Nil(t, specAcl(v1alpha1.Listener{AllowedCidrs: ptrTo("")}).AllowedCidrs)
}

func TestNeutralAclCoversOnlyTheFieldsTheSpecSets(t *testing.T) {
	n := neutralAcl(v1alpha1.ListenerAcl{BlockedCidrs: ptrTo("203.0.113.9/32"), DefaultAction: ptrTo("drop")}, "0.0.0.0/0")
	assert.Nil(t, n.AllowedCidrs)
	assert.Equal(t, "", *n.BlockedCidrs)
	assert.Equal(t, "drop", *n.DefaultAction)
}

// Review C1: a listener the controller created has no original other than the neutral listener. A
// whitelist an older release applied (no record) is not the user's value to put back.
func TestPlanAclTakesOverACreatedListenerFromItsNeutralAcl(t *testing.T) {
	neutral := &v1alpha1.ListenerAcl{AllowedCidrs: ptrTo("0.0.0.0/0"), BlockedCidrs: ptrTo(""), DefaultAction: ptrTo("drop")}
	p := planListenerAcl(v1alpha1.ListenerAcl{AllowedCidrs: ptrTo("203.0.113.0/24")},
		cur("192.0.2.0/24", "", "drop"), nil, neutral, noPeers)
	assert.Equal(t, "203.0.113.0/24", *p.Desired.AllowedCidrs)
	assert.Equal(t, "0.0.0.0/0", *p.RecordBefore.AllowedCidrs, "the neutral listener, not the legacy whitelist")
	assert.Equal(t, "0.0.0.0/0", *p.RecordAfter.AllowedCidrs)
	assert.Nil(t, p.RecordBefore.BlockedCidrs, "only the field taken over is recorded")
	assert.Nil(t, p.RecordBefore.DefaultAction)
}

func TestPlanAclTakesOverAnAdoptedListenerFromWhatItHas(t *testing.T) {
	p := planListenerAcl(v1alpha1.ListenerAcl{AllowedCidrs: ptrTo("203.0.113.0/24")},
		cur("192.0.2.0/24", "", "drop"), nil, nil, noPeers)
	assert.Equal(t, "192.0.2.0/24", *p.RecordBefore.AllowedCidrs, "the user's value is what goes back")
}

// An existing record still wins over the neutral fallback.
func TestPlanAclKeepsAnExistingRecordOverTheNeutralAcl(t *testing.T) {
	neutral := &v1alpha1.ListenerAcl{AllowedCidrs: ptrTo("0.0.0.0/0"), BlockedCidrs: ptrTo(""), DefaultAction: ptrTo("drop")}
	rec := &v1alpha1.ListenerAcl{AllowedCidrs: ptrTo("198.51.100.0/24")}
	p := planListenerAcl(v1alpha1.ListenerAcl{AllowedCidrs: ptrTo("203.0.113.0/24")},
		cur("192.0.2.0/24", "", "drop"), rec, neutral, noPeers)
	assert.Equal(t, "198.51.100.0/24", *p.RecordBefore.AllowedCidrs)
}

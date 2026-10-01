package v1alpha1

import "testing"

func s(v string) *string { return &v }

func TestCreatedListenerEqualSeesTheAclRecord(t *testing.T) {
	a := CreatedListener{Id: "lis-1", Port: 80, OriginalAcl: &ListenerAcl{BlockedCidrs: s("")}}
	b := CreatedListener{Id: "lis-1", Port: 80}
	if a.Equal(b) {
		t.Fatal("a record of what to put back must make two listeners differ, or status never saves it")
	}
	if !a.Equal(CreatedListener{Id: "lis-1", Port: 80, OriginalAcl: &ListenerAcl{BlockedCidrs: s("")}}) {
		t.Fatal("same record must compare equal")
	}
}

func TestEmptyAclRecordEqualsNone(t *testing.T) {
	if !(&ListenerAcl{}).Equal(nil) {
		t.Fatal("a record with no field is no record")
	}
}

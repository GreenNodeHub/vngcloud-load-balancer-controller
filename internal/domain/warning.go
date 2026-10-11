package domain

import "strings"

// LBCEventReasonSharedListenerAcl is the reason of the Warning event raised when a listener ACL
// declared by another LoadBalancerConfig on the same load balancer also filters this one.
const LBCEventReasonSharedListenerAcl = "SharedListenerAcl"

// ReconcileWarning is returned by a reconcile that succeeded but has something to tell the user.
// The caller reports it as Warning events instead of treating it as a failure.
type ReconcileWarning struct {
	Messages []string
}

func (w *ReconcileWarning) Error() string {
	return strings.Join(w.Messages, "; ")
}

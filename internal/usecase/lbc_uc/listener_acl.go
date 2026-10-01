package lbc_uc

import (
	"fmt"
	"slices"
	"strings"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase/listeneracl"
)

type aclField int

const (
	aclAllowed aclField = iota
	aclBlocked
	aclDefaultAction
)

var aclFields = []aclField{aclAllowed, aclBlocked, aclDefaultAction}

func (f aclField) String() string {
	return [...]string{"allowed cidrs", "blocked cidrs", "default action"}[f]
}

func (f aclField) get(a *v1alpha1.ListenerAcl) *string {
	if a == nil {
		return nil
	}
	return [...]*string{a.AllowedCidrs, a.BlockedCidrs, a.DefaultAction}[f]
}

func (f aclField) set(a *v1alpha1.ListenerAcl, v *string) {
	switch f {
	case aclAllowed:
		a.AllowedCidrs = v
	case aclBlocked:
		a.BlockedCidrs = v
	case aclDefaultAction:
		a.DefaultAction = v
	}
}

// specAcl is the ACL the spec asks for. An empty allow list was ignored before ACL existed, and
// sending "" there would allow nobody, so it stays unmanaged.
func specAcl(l v1alpha1.Listener) v1alpha1.ListenerAcl {
	acl := v1alpha1.ListenerAcl{BlockedCidrs: l.BlockedCidrs, DefaultAction: l.DefaultAction}
	if l.AllowedCidrs != nil && *l.AllowedCidrs != "" {
		acl.AllowedCidrs = l.AllowedCidrs
	}
	return acl
}

func currentAcl(l *entityv2.Listener) v1alpha1.ListenerAcl {
	acl := v1alpha1.ListenerAcl{AllowedCidrs: &l.AllowedCidrs, BlockedCidrs: &l.BlockedCidrs}
	if l.DefaultAction != "" {
		acl.DefaultAction = &l.DefaultAction
	}
	return acl
}

// neutralAcl is what a listener the controller creates would have had without the annotations,
// recorded so that removing them later opens the listener back up.
func neutralAcl(spec v1alpha1.ListenerAcl, defaultAllowed string) *v1alpha1.ListenerAcl {
	neutral := [...]string{defaultAllowed, "", listeneracl.ActionAccept}
	rec := &v1alpha1.ListenerAcl{}
	for _, f := range aclFields {
		if f.get(&spec) != nil {
			v := neutral[f]
			f.set(rec, &v)
		}
	}
	if rec.IsEmpty() {
		return nil
	}
	return rec
}

// aclDeclaration is one ACL field another LBC on the same load balancer declares for a listener.
type aclDeclaration struct{ lbcName, value string }

// peerDeclaresAcl reports, for the listener on port, whether another LBC declares a field - the
// planner then leaves that field to the peer instead of restoring it.
func (t *defaultModelDeployTask) peerDeclaresAcl(port int32) func(aclField) bool {
	return func(f aclField) bool { return t.aclPeers[port][f] != nil }
}

type aclPlan struct {
	Desired      v1alpha1.ListenerAcl  // value to PUT; nil = keep the current one
	RecordBefore *v1alpha1.ListenerAcl // record to store BEFORE the PUT
	RecordAfter  *v1alpha1.ListenerAcl // record to store AFTER a successful PUT
	Changes      []string
}

func planListenerAcl(spec, current v1alpha1.ListenerAcl, record *v1alpha1.ListenerAcl, peerDeclares func(aclField) bool) aclPlan {
	var plan aclPlan
	before, after := cloneAcl(record), cloneAcl(record)

	for _, f := range aclFields {
		want, have, orig := f.get(&spec), f.get(&current), f.get(record)
		switch {
		case want != nil:
			if aclValueEqual(f, have, want) {
				continue
			}
			if orig == nil {
				// The value we are about to displace, or accept for a listener that predates ACL.
				o := ""
				if have != nil {
					o = *have
				} else if f == aclDefaultAction {
					o = listeneracl.ActionAccept
				}
				f.set(before, &o)
				f.set(after, &o)
			}
			f.set(&plan.Desired, want)
			plan.Changes = append(plan.Changes, fmt.Sprintf("%s (%s -> %s)", f, deref(have), *want))
		case orig != nil:
			f.set(after, nil)
			if peerDeclares(f) || aclValueEqual(f, have, orig) {
				continue
			}
			f.set(&plan.Desired, orig)
			plan.Changes = append(plan.Changes, fmt.Sprintf("%s restored (%s -> %s)", f, deref(have), *orig))
		}
	}

	plan.RecordBefore, plan.RecordAfter = nilIfEmpty(before), nilIfEmpty(after)
	return plan
}

// aclValueEqual compares CIDR lists as sets, since vLB may echo a list back reformatted.
func aclValueEqual(f aclField, a, b *string) bool {
	if f == aclDefaultAction {
		return deref(a) == deref(b)
	}
	return slices.Equal(cidrSet(deref(a)), cidrSet(deref(b)))
}

func cidrSet(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func cloneAcl(a *v1alpha1.ListenerAcl) *v1alpha1.ListenerAcl {
	if a == nil {
		return &v1alpha1.ListenerAcl{}
	}
	return a.DeepCopy()
}

func nilIfEmpty(a *v1alpha1.ListenerAcl) *v1alpha1.ListenerAcl {
	if a.IsEmpty() {
		return nil
	}
	return a
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Package listeneracl turns the ACL annotations of a Service or Ingress into the listener ACL
// fields of a LoadBalancerConfig. A nil field means the annotations do not manage it.
package listeneracl

import (
	"fmt"
	"net"
	"strings"

	"github.com/vngcloud/vngcloud-load-balancer-controller/pkg/annotations"
)

const (
	ActionAccept = "accept"
	ActionDrop   = "drop"
)

type Fields struct {
	AllowedCidrs  *string
	BlockedCidrs  *string
	DefaultAction *string
}

func FromAnnotations(p annotations.Parser, anns map[string]string) (Fields, error) {
	var f Fields

	inbound := []string{}
	if p.ParseStringSliceAnnotation(annotations.SuffixInboundCIDRs, &inbound, anns) {
		s := strings.Join(inbound, ",")
		f.AllowedCidrs = &s
	}

	dropped := []string{}
	if p.ParseStringSliceAnnotation(annotations.SuffixDroppedCIDRs, &dropped, anns) {
		for _, c := range dropped {
			if !isIPv4CidrOrIP(c) {
				return Fields{}, fmt.Errorf("annotation %s/%s: %q is not an IPv4 CIDR or IPv4 address",
					p.GetPrefix(), annotations.SuffixDroppedCIDRs, c)
			}
		}
		s := strings.Join(dropped, ",")
		f.BlockedCidrs = &s
	}

	action := ""
	if p.ParseStringAnnotation(annotations.SuffixACLDefaultAction, &action, anns) {
		action = strings.ToLower(strings.TrimSpace(action))
		if action != ActionAccept && action != ActionDrop {
			return Fields{}, fmt.Errorf("annotation %s/%s: %q must be %q or %q",
				p.GetPrefix(), annotations.SuffixACLDefaultAction, action, ActionAccept, ActionDrop)
		}
		f.DefaultAction = &action
	} else if f.AllowedCidrs != nil && *f.AllowedCidrs != "" {
		// A whitelist only means "nobody else" when what matches no rule is dropped; the
		// portal's default is accept.
		drop := ActionDrop
		f.DefaultAction = &drop
	}

	return f, nil
}

// isIPv4CidrOrIP accepts what vLB accepts: IPv4 only, host bits allowed ("192.0.2.1/22").
func isIPv4CidrOrIP(s string) bool {
	if ip, _, err := net.ParseCIDR(s); err == nil {
		return ip.To4() != nil
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

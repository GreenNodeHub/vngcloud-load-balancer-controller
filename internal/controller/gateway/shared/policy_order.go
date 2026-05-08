package shared

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RankedMatch struct {
	Match        gwv1.HTTPRouteMatch
	RouteCreated metav1.Time
}

type byMatchSpecificity []RankedMatch

func (s byMatchSpecificity) Len() int      { return len(s) }
func (s byMatchSpecificity) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byMatchSpecificity) Less(i, j int) bool {
	a, b := s[i], s[j]
	if r := pathRank(a.Match.Path) - pathRank(b.Match.Path); r != 0 {
		return r < 0
	}
	if pl, pr := pathLen(a.Match.Path), pathLen(b.Match.Path); pl != pr {
		return pl > pr
	}
	if hi, hj := len(a.Match.Headers), len(b.Match.Headers); hi != hj {
		return hi > hj
	}
	if qi, qj := len(a.Match.QueryParams), len(b.Match.QueryParams); qi != qj {
		return qi > qj
	}
	return a.RouteCreated.Before(&b.RouteCreated)
}

func pathRank(p *gwv1.HTTPPathMatch) int {
	if p == nil || p.Type == nil {
		return 99
	}
	switch *p.Type {
	case gwv1.PathMatchExact:
		return 0
	case gwv1.PathMatchRegularExpression:
		return 1
	case gwv1.PathMatchPathPrefix:
		return 2
	}
	return 99
}

func pathLen(p *gwv1.HTTPPathMatch) int {
	if p == nil || p.Value == nil {
		return 0
	}
	return len(*p.Value)
}

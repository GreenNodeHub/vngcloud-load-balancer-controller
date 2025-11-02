package domain

// TODO: refactor TargetType to use in LoadBalancer domain
type TargetType string

const (
	TargetTypeInstance TargetType = "instance"
	TargetTypeIP       TargetType = "ip"
)

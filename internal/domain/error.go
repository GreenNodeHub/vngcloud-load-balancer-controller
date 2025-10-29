package domain

import "github.com/pkg/errors"

var (
	ErrorInvalidInput            = errors.New("invalid input")
	ErrorNotImplemented          = errors.New("not implemented yet")
	ErrorNotFound                = errors.New("heheh not found")
	ErrorLoadBalancerStatusError = errors.New("load balancer status is error")
)

const (
	RequestIcon = "🌐"
	WaitIcon    = "⏳"
	ReadyIcon   = "👍"
	SuccessIcon = "✅"
	ErrorIcon   = "❌"
)

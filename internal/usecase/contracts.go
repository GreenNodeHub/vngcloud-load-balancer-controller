package usecase

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

type ServiceUseCase interface {
	InitServiceUseCase(ctx context.Context) error
	EnsureServiceUseCase(ctx context.Context, req ctrl.Request) error
	DeleteServiceUseCase(ctx context.Context, req ctrl.Request) error
}

type LoadBalancerConfigUseCase interface {
	InitLoadBalancerConfigUseCase(ctx context.Context) error
	EnsureLoadBalancerConfigUseCase(ctx context.Context, req ctrl.Request) error
	DeleteLoadBalancerConfigUseCase(ctx context.Context, req ctrl.Request) error
}

type NodeSecurityGroupUseCase interface {
	InitNodeSecurityGroupUseCase(ctx context.Context) error
	EnsureNodeSecurityGroupUseCase(ctx context.Context, req ctrl.Request) error
	DeleteNodeSecurityGroupUseCase(ctx context.Context, req ctrl.Request) error
}

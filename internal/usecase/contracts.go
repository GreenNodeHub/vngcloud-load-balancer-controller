package usecase

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

type ServiceUseCase interface {
	Init(ctx context.Context) error
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

type LoadBalancerConfigUseCase interface {
	Init(ctx context.Context) error
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

type NodeSecurityGroupUseCase interface {
	Init(ctx context.Context) error
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

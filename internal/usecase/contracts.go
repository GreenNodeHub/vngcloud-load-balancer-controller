package usecase

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

type IServiceUseCase interface {
	Init(ctx context.Context) error
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

type IVLBConfigUseCase interface {
	Init(ctx context.Context) error
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

type NodeSecurityGroupUseCase interface {
	Init(ctx context.Context) error
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

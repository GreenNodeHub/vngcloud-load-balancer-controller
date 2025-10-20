package usecase

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

type IServiceUseCase interface {
	Ensure(ctx context.Context, req ctrl.Request) error
	Delete(ctx context.Context, req ctrl.Request) error
}

package service_uc

import (
	"context"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/usecase"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ServiceUseCase struct {
}

func NewServiceUseCase() usecase.IServiceUseCase {
	return &ServiceUseCase{}
}

func (uc *ServiceUseCase) Ensure(ctx context.Context, req ctrl.Request) error {
	return nil
}
func (uc *ServiceUseCase) Delete(ctx context.Context, req ctrl.Request) error {
	return nil
}

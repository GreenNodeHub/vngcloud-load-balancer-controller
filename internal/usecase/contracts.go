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

type IngressUseCase interface {
	InitIngressUseCase(ctx context.Context) error
	EnsureIngressUseCase(ctx context.Context, req ctrl.Request) error
	DeleteIngressUseCase(ctx context.Context, req ctrl.Request) error
}

type VngcloudGlobalLoadBalancerUseCase interface {
	InitVngcloudGlobalLoadBalancerUseCase(ctx context.Context) error
	EnsureVngcloudGlobalLoadBalancerUseCase(ctx context.Context, req ctrl.Request) error
	DeleteVngcloudGlobalLoadBalancerUseCase(ctx context.Context, req ctrl.Request) error
}

type GlobalLoadBalancerConfigUseCase interface {
	InitGlobalLoadBalancerConfigUseCase(ctx context.Context) error
	EnsureGlobalLoadBalancerConfigUseCase(ctx context.Context, req ctrl.Request) error
	DeleteGlobalLoadBalancerConfigUseCase(ctx context.Context, req ctrl.Request) error
}

// ServiceGLBUseCase handles reconciliation of Services with the glb.vks.vngcloud.vn/enable=true
// annotation. It creates and manages GlobalLoadBalancerConfig resources owned by the Service.
type ServiceGLBUseCase interface {
	InitServiceGLBUseCase(ctx context.Context) error
	EnsureServiceGLBUseCase(ctx context.Context, req ctrl.Request) error
	DeleteServiceGLBUseCase(ctx context.Context, req ctrl.Request) error
}

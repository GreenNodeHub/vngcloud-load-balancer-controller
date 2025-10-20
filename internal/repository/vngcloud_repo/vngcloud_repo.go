package vngcloud_repo

import (
	"context"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	loadbalancerv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

func NewVngCloudRepository() repository.IVngCloudRepository {
	return &VngCloudRepository{}
}

type VngCloudRepository struct {
}

func (r *VngCloudRepository) CreateLoadBalancer(ctx context.Context, lbOptions loadbalancerv2.ICreateLoadBalancerRequest) (*entityv2.LoadBalancer, error) {
	return nil, nil
}
func (r *VngCloudRepository) DeleteLoadBalancer(ctx context.Context, lbID string) error {
	return nil
}

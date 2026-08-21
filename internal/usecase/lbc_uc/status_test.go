package lbc_uc

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// A cloud resource we cannot name is one we have lost: nothing later can find it, update it
// or delete it. Recording it with an empty id is worse than failing, because id is the key of
// a map-list - the API server rejects the whole status patch, every reconcile, and the LBC
// never moves again. deployLoadBalancer already guards the load balancer's own id this way.
//
// The strict mock asserts no patch is even attempted.
func TestStatusAddRefusesAnEmptyId(t *testing.T) {
	tests := map[string]func(*defaultModelDeployTask) error{
		"pool": func(task *defaultModelDeployTask) error {
			return task.statusAddPool(context.Background(), "", "vks-a-b-80")
		},
		"listener": func(task *defaultModelDeployTask) error {
			return task.statusAddListener(context.Background(), "", 80)
		},
		"policy": func(task *defaultModelDeployTask) error {
			return task.statusAddPolicy(context.Background(), "listener-1", 80, "")
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			task := &defaultModelDeployTask{
				logger:   logrus.NewEntry(logrus.New()),
				k8sRepo:  repository.NewMockK8sRepository(t),
				lbConfig: &v1alpha1.LoadBalancerConfig{},
			}

			err := call(task)

			assert.Error(t, err, "an unnameable %s must not be recorded", name)
			assert.Contains(t, err.Error(), "need to retry")
		})
	}
}

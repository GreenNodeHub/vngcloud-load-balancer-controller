package glbc_uc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vngcloud/vngcloud-load-balancer-controller/api/v1alpha1"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// id is the key of a map-list the CRD marks required, so recording an entry with an empty
// id wedges the GLBC: the API server rejects the whole status patch on every reconcile.
// The strict mock asserts no patch is even attempted. Mirrors the lbc_uc guard.
func TestGlbcStatusAddRefusesAnEmptyId(t *testing.T) {
	tests := map[string]func(*defaultModelDeployTask) error{
		"listener": func(task *defaultModelDeployTask) error {
			return task.statusAddListener(context.Background(), "", 80, "vks-glb-80")
		},
		"pool": func(task *defaultModelDeployTask) error {
			return task.statusUpdatePoolMember(context.Background(), "", "vks-glb-80", nil)
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			task := &defaultModelDeployTask{
				k8sRepo:  repository.NewMockK8sRepository(t),
				lbConfig: &v1alpha1.GlobalLoadBalancerConfig{},
			}

			err := call(task)

			assert.Error(t, err, "an unnameable %s must not be recorded", name)
			assert.Contains(t, err.Error(), "need to retry")
		})
	}
}

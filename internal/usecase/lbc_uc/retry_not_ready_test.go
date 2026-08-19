package lbc_uc

import (
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	entityv2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/entity"
	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/repository"
)

// notReadyErr has the shape domain.IsLoadBalancerNotReady recognises.
func notReadyErr() error {
	return errors.New("The load balancer id lb-87f329e4-9ada-41b0-b8e7-5419439ce499 is not ready")
}

// vLB rejects a write while it is still applying the previous one. deployPool issues three
// writes in a row against the same load balancer, so the second and third routinely arrive
// while it is busy - measured at roughly one rejection in ten on a live cluster. That is
// transient, and treating it as fatal is what aborts the reconcile before the cleanup runs.
func TestRetryOnLoadBalancerNotReady(t *testing.T) {
	tests := []struct {
		name       string
		errs       []error // one per attempt
		wantCalls  int
		wantWaits  int
		wantErr    bool
	}{
		{
			name:      "accepted first time - no wait, no retry",
			errs:      []error{nil},
			wantCalls: 1,
			wantWaits: 0,
		},
		{
			name:      "busy once then accepted",
			errs:      []error{notReadyErr(), nil},
			wantCalls: 2,
			wantWaits: 1,
		},
		{
			name:      "busy every time - gives up and reports it",
			errs:      []error{notReadyErr(), notReadyErr(), notReadyErr()},
			wantCalls: 3,
			wantWaits: 3,
			wantErr:   true,
		},
		{
			// A real failure must not be retried, or a genuine misconfiguration would be
			// hidden behind three attempts and a delay.
			name:      "a different error is not retried",
			errs:      []error{errors.New("pool protocol is invalid")},
			wantCalls: 1,
			wantWaits: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := repository.NewMockVngCloudRepository(t)
			if tt.wantWaits > 0 {
				repo.EXPECT().
					WaitForLBActive(mock.Anything, "lb-1").
					Return(&entityv2.LoadBalancer{UUID: "lb-1"}, nil).
					Times(tt.wantWaits)
			}

			task := &defaultModelDeployTask{
				logger:       logrus.NewEntry(logrus.New()),
				vngcloudRepo: repo,
			}

			calls := 0
			err := task.retryOnLoadBalancerNotReady(context.Background(), "lb-1", func() error {
				e := tt.errs[calls]
				calls++
				return e
			})

			assert.Equal(t, tt.wantCalls, calls, "number of write attempts")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

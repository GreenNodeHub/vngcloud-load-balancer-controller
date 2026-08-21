package lbc_uc

import (
	"context"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// maxLoadBalancerBusyRetries bounds how many times a single write waits out a busy load
// balancer. Three is enough to ride out the previous write settling; beyond that the load
// balancer is not merely busy and the reconcile should back off and try again later.
const maxLoadBalancerBusyRetries = 3

// retryOnLoadBalancerNotReady runs write, waiting the load balancer out and trying again
// while it rejects the call as not ready.
//
// vLB refuses a write while it is still applying the previous one, and deployPool issues
// several writes in a row against the same load balancer - so the later ones routinely
// arrive while it is busy. Measured on a live cluster, about one write sequence in ten hit
// this. Treating it as fatal is what aborted the whole reconcile, which in turn skipped the
// cleanup that removes stale pools and members: a pool that lost this race stayed dirty
// because the pass never got far enough to fix it.
//
// Only the not-ready rejection is retried. Any other error is returned untouched, so a
// genuine misconfiguration still fails fast instead of hiding behind three attempts.
func (t *defaultModelDeployTask) retryOnLoadBalancerNotReady(ctx context.Context, lbId string, write func() error) error {
	var err error
	for attempt := 1; attempt <= maxLoadBalancerBusyRetries; attempt++ {
		err = write()
		if err == nil || !domain.IsLoadBalancerNotReady(err) {
			return err
		}

		t.logger.Warnf("load balancer %s was busy, waiting for it to settle and retrying (%d/%d)",
			lbId, attempt, maxLoadBalancerBusyRetries)
		if _, waitErr := t.vngcloudRepo.WaitForLBActive(ctx, lbId); waitErr != nil {
			return waitErr
		}
	}
	return err
}

package errs

import (
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vngcloud/vngcloud-load-balancer-controller/internal/domain"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(err error, log *logrus.Entry) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var rateLimit *domain.RateLimitError
	if errors.As(err, &rateLimit) {
		wait := domain.RateLimitRequeueAfter(rateLimit.RetryAfter)
		log.Warnf("rate limited by VngCloud API, requeue after %s (server hint: %s, %s %s)",
			wait, rateLimit.RetryAfter, rateLimit.Method, rateLimit.URL)
		// The warning above is the line an operator reads, so it stays short; the server's own
		// reason goes here. Nothing else prints it - the CR message is built from Error(), which
		// leaves it out on purpose - so without this line it would only be reachable through
		// Unwrap, by no one.
		if cause := errors.Unwrap(rateLimit); cause != nil {
			log.Debugf("rate limit cause from the API: %v", cause)
		}
		return ctrl.Result{RequeueAfter: wait}, nil
	}

	var requeueNeededAfter *RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		log.Debug("requeue after duration: ", requeueNeededAfter.Duration(), ", reason: ", requeueNeededAfter.Reason())
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	var requeueNeeded *RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		log.Debug("requeue immediately, reason: ", requeueNeeded.Reason())
		return ctrl.Result{Requeue: true}, nil
	}

	// The single place a reconcile failure is logged; the layers below wrap
	// the error with context instead of logging it themselves.
	log.Errorf("reconcile failed, requeue with exponential back-off: %v", err)
	time.Sleep(5 * time.Second)
	return ctrl.Result{}, err
}

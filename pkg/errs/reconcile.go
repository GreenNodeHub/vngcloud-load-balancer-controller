package errs

import (
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	ctrl "sigs.k8s.io/controller-runtime"
)

// HandleReconcileError will handle errors from reconcile handlers, which respects runtime errors.
func HandleReconcileError(err error, log *logrus.Entry) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeueNeededAfter *RequeueNeededAfter
	if errors.As(err, &requeueNeededAfter) {
		log.Info("requeue after duration: ", requeueNeededAfter.Duration(), ", reason: ", requeueNeededAfter.Reason())
		return ctrl.Result{RequeueAfter: requeueNeededAfter.Duration()}, nil
	}

	var requeueNeeded *RequeueNeeded
	if errors.As(err, &requeueNeeded) {
		log.Info("requeue immediately reason: ", requeueNeeded.Reason())
		return ctrl.Result{Requeue: true}, nil
	}

	log.Infof("requeue after 5 seconds + exponential back-off, reason: %v", err)
	time.Sleep(5 * time.Second)
	return ctrl.Result{}, err
}

package controller

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PeriodicReconciler is a struct that manages periodic reconciliation
type PeriodicReconciler struct {
	Reconciler reconcile.Reconciler
	Interval   time.Duration
	StopChan   chan struct{}

	// GetReconcileRequests returns a list of reconcile requests
	GetReconcileRequests func() []reconcile.Request
}

// NewPeriodicReconciler creates a new instance of PeriodicReconciler
func NewPeriodicReconciler(reconciler reconcile.Reconciler, interval time.Duration,
	GetReconcileRequests func() []reconcile.Request) *PeriodicReconciler {
	return &PeriodicReconciler{
		Reconciler:           reconciler,
		Interval:             interval,
		StopChan:             make(chan struct{}),
		GetReconcileRequests: GetReconcileRequests,
	}
}

// Start begins the periodic reconcile process
func (p *PeriodicReconciler) Start(ctx context.Context) {
	ticker := time.NewTicker(p.Interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				// Get the list of reconcile requests to process
				requests := p.GetReconcileRequests()

				// Log reconciliation start using klog
				logrus.Infof("Starting periodic reconciliation for requests: %v", requests)

				// Call Reconcile for each request and pass the context
				for _, req := range requests {
					if _, err := p.Reconciler.Reconcile(ctx, req); err != nil {
						logrus.Errorf("Reconcile failed for request %v: %v", req, err)
					}
				}

			case <-ctx.Done():
				// Stop the ticker when the context is canceled
				p.Stop()
				return

			case <-p.StopChan:
				// Stop the ticker when stop is requested
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops the periodic reconciler
func (p *PeriodicReconciler) Stop() {
	close(p.StopChan)
}

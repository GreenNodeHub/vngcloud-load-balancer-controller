package controller

import (
	"context"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type InitReconciler interface {
	Init(client.Client) error
}

// InitRunnable runs after the cache is started and handles the node initialization
type InitRunnable struct {
	Client     client.Client
	Reconciler InitReconciler
}

// Start is where you initialize and store the nodes, after cache has started
func (r *InitRunnable) Start(ctx context.Context) error {
	if err := r.Reconciler.Init(r.Client); err != nil {
		logrus.Errorf("Failed to initialize: %v", err)
		return err
	}
	logrus.Info("InitRunnable function ran successfully.")
	return nil
}

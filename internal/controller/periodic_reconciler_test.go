package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MockReconciler is a mock implementation of the reconcile.Reconciler interface
type MockReconciler struct {
	mock.Mock
}

// Reconcile is the mock implementation of the Reconcile function
func (m *MockReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	args := m.Called(ctx, req)
	return reconcile.Result{}, args.Error(1)
}

// TestPeriodicReconciler tests the periodic reconciler behavior
func TestPeriodicReconciler(t *testing.T) {
	// Mock the Reconciler
	mockReconciler := new(MockReconciler)

	// Mock Reconcile requests
	requests := []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "resource1", Namespace: "default"}},
		{NamespacedName: types.NamespacedName{Name: "resource2", Namespace: "default"}},
	}

	// Define GetReconcileRequests mock function
	getReconcileRequests := func() []reconcile.Request {
		return requests
	}

	// Mock that Reconcile will be called for each request
	mockReconciler.On("Reconcile", mock.Anything, requests[0]).Return(reconcile.Result{}, nil)
	mockReconciler.On("Reconcile", mock.Anything, requests[1]).Return(reconcile.Result{}, nil)

	// Create the periodic reconciler
	periodicReconciler := NewPeriodicReconciler(mockReconciler, 1*time.Second, getReconcileRequests)

	// Create a context to control the execution
	ctx, cancel := context.WithCancel(context.Background())

	// Start the periodic reconciler
	go periodicReconciler.Start(ctx)

	// Let the periodic reconciler run for 2 seconds (2 ticks)
	time.Sleep(2 * time.Second)

	// Stop the reconciler after some time
	cancel()

	// Assert that Reconcile was called twice for each request during the 2-second run
	// mockReconciler.AssertNumberOfCalls(t, "Reconcile", 4) // 2 requests per tick, 2 ticks

	assert.Eventually(t, func() bool {
		// We expect Reconcile to be called twice per tick (once for each request)
		return len(mockReconciler.Calls) == 4 // Two ticks, two calls per tick (2 requests)
	}, 5*time.Second, 100*time.Millisecond)

	// Check that Reconcile was called with the correct requests
	mockReconciler.AssertCalled(t, "Reconcile", mock.Anything, requests[0])
	mockReconciler.AssertCalled(t, "Reconcile", mock.Anything, requests[1])

	// Ensure that no more calls were made
	mockReconciler.AssertExpectations(t)
}

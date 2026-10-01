package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestReconcileWarningUnwrapsThroughWrapping(t *testing.T) {
	var w *ReconcileWarning
	if !errors.As(fmt.Errorf("wrap: %w", &ReconcileWarning{Messages: []string{"x"}}), &w) {
		t.Fatal("errors.As did not find the ReconcileWarning")
	}
	if got := w.Error(); got != "x" {
		t.Fatalf("Error() = %q, want %q", got, "x")
	}
}

func TestReconcileWarningJoinsMessages(t *testing.T) {
	w := &ReconcileWarning{Messages: []string{"a", "b"}}
	if got := w.Error(); got != "a; b" {
		t.Fatalf("Error() = %q, want %q", got, "a; b")
	}
}

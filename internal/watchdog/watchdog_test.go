package watchdog

import (
	"context"
	"testing"
	"time"
)

func TestWatchdogInitialization(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.CheckInterval != 30*time.Second {
		t.Fatalf("expected 30s check interval, got %v", cfg.CheckInterval)
	}
	if cfg.ExecutionDeadline != 180*time.Second {
		t.Fatalf("expected 180s deadline, got %v", cfg.ExecutionDeadline)
	}

	w := NewWatchdog(nil, cfg)
	reconciled, err := w.ReconcileStaleApplications(context.Background())
	if err != nil {
		t.Fatalf("ReconcileStaleApplications with nil DB failed: %v", err)
	}
	if reconciled != 0 {
		t.Fatalf("expected 0 reconciled with nil DB, got %d", reconciled)
	}
}

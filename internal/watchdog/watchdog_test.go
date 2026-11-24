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

func TestWatchdogStartStop(t *testing.T) {
	cfg := Config{
		CheckInterval:     10 * time.Millisecond,
		ExecutionDeadline: 50 * time.Millisecond,
	}
	w := NewWatchdog(nil, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)
	time.Sleep(25 * time.Millisecond)
	w.Stop()

	// Double stop should be safe and idempotent
	w.Stop()
}

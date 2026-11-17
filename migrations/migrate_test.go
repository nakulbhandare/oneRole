package migrations

import (
	"context"
	"testing"
)

func TestMigrationCurrentVersion(t *testing.T) {
	if CurrentVersion <= 0 {
		t.Fatalf("expected CurrentVersion > 0, got %d", CurrentVersion)
	}
}

func TestApplyRejectsNilDatabase(t *testing.T) {
	err := Apply(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil database, got nil")
	}
}

package migrations

import (
	"context"
	"testing"
)

func TestMigrationCurrentVersion(t *testing.T) {
	if CurrentVersion != 2 {
		t.Fatalf("expected CurrentVersion == 2, got %d", CurrentVersion)
	}
}

func TestApplyRejectsNilDatabase(t *testing.T) {
	err := Apply(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil database, got nil")
	}
}

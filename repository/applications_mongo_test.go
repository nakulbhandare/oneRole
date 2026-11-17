package repository

import (
	"context"
	"crypto/sha256"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oneRole/db"
	"github.com/oneRole/migrations"
	"github.com/oneRole/service"
)

// This opt-in integration test covers transaction boundaries against real
// MongoDB. Set ONEROLE_TEST_DATABASE_URL only to a dedicated database whose
// name includes "test".
func TestApplicationWorkflowMongo(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("ONEROLE_TEST_DATABASE_URL"))
	if uri == "" {
		t.Skip("set ONEROLE_TEST_DATABASE_URL to a dedicated MongoDB test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	client, database, err := db.Open(ctx, db.Config{URI: uri, MaxPoolSize: 8, MinPoolSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)

	databaseName := database.Name()
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("refusing to modify database %q: its name must include 'test'", databaseName)
	}

	if err := migrations.Apply(ctx, database); err != nil {
		t.Fatal(err)
	}

	scope := service.ActorScope{
		TenantID: "11111111-1111-4111-8111-111111111111",
		UserID:   "22222222-2222-4222-8222-222222222222",
	}

	appStore := NewApplicationWorkflowStore(database)
	if appStore == nil {
		t.Fatal("expected non-nil store")
	}

	// Verify tenant membership rejection when not member
	_, err = appStore.AcceptApplication(ctx, scope, service.CreateApplicationCommand{
		JobID:           "33333333-3333-4333-8333-333333333333",
		ResumeVersionID: "44444444-4444-4444-8444-444444444444",
		ConsentID:       "55555555-5555-4555-8555-555555555555",
		Mode:            service.ApplicationModeReviewBeforeSubmit,
	}, service.IdempotencyIdentity{
		KeyHash:     sha256.Sum256([]byte("key-1")),
		RequestHash: sha256.Sum256([]byte("body-1")),
	}, time.Now().UTC())

	if err == nil {
		t.Fatal("expected error for non-member")
	}
}

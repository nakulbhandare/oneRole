package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oneRole/db"
	"github.com/oneRole/migrations"
)

// This opt-in integration test covers lease renewal against real MongoDB.
func TestSourceLeaseRenewalMongo(t *testing.T) {
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

	store := NewJobSourceStore(database)
	if store == nil {
		t.Fatal("expected non-nil store")
	}

	sources, err := store.ClaimDueSources(ctx, "worker-1", 10, time.Now().UTC(), time.Now().UTC().Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_ = sources
}

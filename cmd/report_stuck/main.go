package main

import (
	"context"
	"fmt"
	"log"
	"time"

	dbconn "github.com/oneRole/db"
	"github.com/oneRole/repository"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mongoClient, mongoDB, err := dbconn.Open(ctx, dbconn.Config{URI: "mongodb://localhost:27017/onerole"})
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(ctx)

	store := repository.NewJobSourceStore(mongoDB)
	stuckSources, err := store.GetStuckSources(ctx, 100)
	if err != nil {
		log.Fatalf("Failed to get stuck sources: %v", err)
	}

	fmt.Printf("Found %d stuck sources:\n", len(stuckSources))
	for _, s := range stuckSources {
		fmt.Printf("- %s (ID: %s, Company: %s, Status: %s)\n", s.BaseURL, s.ID, s.CompanyID, s.Status)
	}
}

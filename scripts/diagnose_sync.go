package main

import (
	"context"
	"fmt"
	"time"

	"github.com/oneRole/repository"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27017/onerole"))
	if err != nil {
		fmt.Println("Connect error:", err)
		return
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	store := repository.NewJobSourceStore(db)

	now := time.Now().UTC()
	sources, err := store.ClaimDueSources(ctx, "diag-runner", 1, now, now.Add(10*time.Minute))
	if err != nil {
		fmt.Println("Claim error:", err)
		return
	}
	if len(sources) == 0 {
		fmt.Println("No sources claimed")
		return
	}

	src := sources[0]
	fmt.Printf("Claimed source %s (%s, %s)\n", src.ID, src.Provider, src.BoardIdentifier)
	run, err := store.StartSourceSync(ctx, src, now)
	if err != nil {
		fmt.Printf("StartSourceSync ERROR: %+v\n", err)
	} else {
		fmt.Printf("StartSourceSync SUCCESS: run ID = %s\n", run.ID)
	}
}

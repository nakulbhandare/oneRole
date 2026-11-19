package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/oneRole/service"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	mongoURL := os.Getenv("DATABASE_URL")
	if mongoURL == "" {
		mongoURL = "mongodb://localhost:27017/onerole"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		fmt.Printf("Failed to connect to MongoDB: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	coll := db.Collection("applications")

	// Reset any stuck in_flight or failed applications to queued so they can be processed cleanly
	_, _ = coll.UpdateMany(ctx, bson.M{
		"status": bson.M{"$in": []string{"in_flight", "failed_retryable", "failed"}},
	}, bson.M{
		"$set": bson.M{"status": "queued"},
	})

	// Find all applications that are not submitted
	filter := bson.M{
		"status": bson.M{"$ne": "submitted"},
	}

	cur, err := coll.Find(ctx, filter)
	if err != nil {
		fmt.Printf("Failed to query applications: %v\n", err)
		os.Exit(1)
	}
	defer cur.Close(ctx)

	var apps []struct {
		ID          string `bson:"_id"`
		TenantID    string `bson:"tenant_id"`
		UserID      string `bson:"user_id"`
		JobID       string `bson:"job_id"`
		CompanyName string `bson:"company_name"`
		JobTitle    string `bson:"job_title"`
		Status      string `bson:"status"`
	}

	if err := cur.All(ctx, &apps); err != nil {
		fmt.Printf("Failed to decode applications: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d applications to submit...\n\n", len(apps))

	engine := service.NewAutoApplyEngine(db)

	successCount := 0
	failCount := 0

	for i, app := range apps {
		tenantID := app.TenantID
		if tenantID == "" {
			tenantID = "default"
		}
		userID := app.UserID
		if userID == "" {
			userID = "nakul-bhandare"
		}
		scope := service.ActorScope{
			TenantID: tenantID,
			UserID:   userID,
		}

		fmt.Printf("[%d/%d] Submitting %s | Company: %s | Role: %s ... ",
			i+1, len(apps), app.ID[:8], app.CompanyName, app.JobTitle)

		subCtx, subCancel := context.WithTimeout(ctx, 30*time.Second)
		res, err := engine.SubmitOneClick(subCtx, scope, app.ID)
		subCancel()

		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failCount++
		} else {
			receipt := res.ReceiptID
			fmt.Printf("SUCCESS -> Receipt: %s (ATS Score: %d%%)\n", receipt, res.MatchScore)
			successCount++
		}
	}

	fmt.Printf("\n==================================================\n")
	fmt.Printf("Resubmission Complete!\n")
	fmt.Printf("Total: %d | Submitted: %d | Failed: %d\n", len(apps), successCount, failCount)
	fmt.Printf("==================================================\n")
}

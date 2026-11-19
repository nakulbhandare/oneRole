package main

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27018/onerole"))
	if err != nil {
		fmt.Printf("Connect error: %v\n", err)
		return
	}
	defer client.Disconnect(ctx)

	cur, err := client.Database("onerole").Collection("applications").Aggregate(ctx, mongo.Pipeline{
		{{"$group", bson.D{{"_id", "$status"}, {"count", bson.D{{"$sum", 1}}}}}},
	})
	if err != nil {
		fmt.Printf("Aggregate error: %v\n", err)
		return
	}

	var results []bson.M
	if err := cur.All(ctx, &results); err != nil {
		fmt.Printf("Decode error: %v\n", err)
		return
	}

	fmt.Println("=== APPLICATION STATUS BREAKDOWN ===")
	for _, r := range results {
		fmt.Printf("Status: %-20v | Count: %v\n", r["_id"], r["count"])
	}

	cur2, _ := client.Database("onerole").Collection("applications").Find(ctx, bson.M{}, options.Find().SetLimit(5))
	var sampleApps []bson.M
	_ = cur2.All(ctx, &sampleApps)
	fmt.Println("\n=== SAMPLE APPLICATION RECEIPTS ===")
	for _, a := range sampleApps {
		fmt.Printf("App: %v | Company: %-15v | Status: %-9v | Receipt: %-25v | Mode: %-15v | ATS: %v\n",
			a["_id"].(string)[:8], a["company_name"], a["status"], a["verified_receipt_id"], a["submission_mode"], a["ats_verdict"])
	}
}

func getKeys(m bson.M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

package main

import (
	"context"
	"log"
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
		log.Fatalf("Connect error: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	profilesColl := db.Collection("candidate_profiles")
	usersColl := db.Collection("users")

	userID := "9afbe74b-b9ad-4094-a25e-bd4c63e1794c"
	tenantID := "00000000-0000-4000-8000-000000000001"

	updateProf := bson.M{
		"$set": bson.M{
			"first_name":               "Nakul",
			"last_name":                "Bhandare",
			"email":                    "bhandarenakul11@gmail.com",
			"phone":                    "+1 (350)-355-6570",
			"linkedin_url":             "https://linkedin.com/in/nakul-bhandare",
			"github_url":               "https://github.com/nakulbhandare",
			"portfolio_url":            "https://nakulbhandare.github.io",
			"city":                     "San Francisco",
			"state":                    "CA",
			"country":                  "United States",
			"authorized_to_work_in_us": true,
			"requires_sponsorship":      false,
			"years_of_experience":      7,
			"current_job_title":        "Senior Software Engineer",
			"highest_degree":           "Master of Science",
			"open_to_remote":           true,
			"desired_salary_min":       150000,
			"custom_field_answers": bson.M{
				"work_authorization":  "Yes",
				"visa_sponsorship":    "No",
				"open_to_remote":      "Yes",
				"years_of_experience": "7",
				"desired_salary_min":  "150,000",
				"notice_period":       "Immediately (2 weeks notice)",
				"primary_languages":   "Go, Python, SQL, TypeScript, C++",
				"tech_stack":          "Go, Distributed Systems, Kubernetes, Kafka, NATS, PostgreSQL",
			},
			"updated_at": time.Now().UTC(),
		},
	}

	res, err := profilesColl.UpdateMany(ctx, bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"email": "bhandarenakul11@gmail.com"},
		},
	}, updateProf)
	if err != nil {
		log.Fatalf("Update profile failed: %v", err)
	}
	log.Printf("Updated %d profile documents.\n", res.ModifiedCount)

	// Also update users collection
	resUser, err := usersColl.UpdateMany(ctx, bson.M{
		"$or": []bson.M{
			{"_id": userID},
			{"email": "bhandarenakul11@gmail.com"},
		},
	}, bson.M{
		"$set": bson.M{
			"display_name": "Nakul Bhandare",
			"email":        "bhandarenakul11@gmail.com",
			"tenant_id":    tenantID,
			"updated_at":   time.Now().UTC(),
		},
	})
	if err == nil {
		log.Printf("Updated %d user documents.\n", resUser.ModifiedCount)
	}
}

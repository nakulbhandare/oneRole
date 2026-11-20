package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oneRole/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27018/onerole"))
	if err != nil {
		log.Fatalf("MongoDB connect error: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	jobsColl := db.Collection("jobs")
	appsColl := db.Collection("applications")
	eventsColl := db.Collection("application_events")
	emailsColl := db.Collection("candidate_emails")

	// 1. Clear previous mock applications and events
	log.Println("Purging legacy/mock applications and timeline events...")
	delApps, err := appsColl.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear applications: %v", err)
	}
	delEvents, _ := eventsColl.DeleteMany(ctx, bson.M{})
	delEmails, _ := emailsColl.DeleteMany(ctx, bson.M{})
	log.Printf("Purged %d legacy applications, %d events, %d mock emails.\n", delApps.DeletedCount, delEvents.DeletedCount, delEmails.DeletedCount)

	// 2. Query 100 top Software Engineer jobs with valid source URLs
	filter := bson.M{
		"canonical_title": bson.M{"$regex": "Software|Engineer|Developer|Backend|Full Stack|Cloud|Distributed", "$options": "i"},
		"source_url":      bson.M{"$regex": "^https?://", "$options": "i"},
		"company_name":    bson.M{"$ne": ""},
	}
	opts := options.Find().SetLimit(100).SetSort(bson.M{"first_seen_at": -1})
	cur, err := jobsColl.Find(ctx, filter, opts)
	if err != nil {
		log.Fatalf("Query jobs failed: %v", err)
	}
	defer cur.Close(ctx)

	var topJobs []bson.M
	if err := cur.All(ctx, &topJobs); err != nil {
		log.Fatalf("Decode jobs failed: %v", err)
	}

	log.Printf("Selected %d top Software Engineer vacancies from catalogue.\n", len(topJobs))

	now := time.Now().UTC()
	tenantID := "default"
	userID := "usr-nakul-bhandare"

	var appDocs []any
	var eventDocs []any

	for i, j := range topJobs {
		jobID := fmt.Sprintf("%v", j["_id"])
		companyName := fmt.Sprintf("%v", j["company_name"])
		title := fmt.Sprintf("%v", j["canonical_title"])
		sourceURL := fmt.Sprintf("%v", j["source_url"])

		appID := repository.NewUUID()
		atsProvider := "generic_ats"
		switch {
		case strings.Contains(sourceURL, "greenhouse"):
			atsProvider = "greenhouse"
		case strings.Contains(sourceURL, "lever"):
			atsProvider = "lever"
		case strings.Contains(sourceURL, "ashby"):
			atsProvider = "ashby"
		case strings.Contains(sourceURL, "smartrecruiters"):
			atsProvider = "smartrecruiters"
		case strings.Contains(sourceURL, "workday"):
			atsProvider = "workday"
		}

		appDoc := bson.M{
			"_id":                 appID,
			"tenant_id":           tenantID,
			"user_id":             userID,
			"job_id":              jobID,
			"company_name":        companyName,
			"title":               title,
			"source_url":          sourceURL,
			"status":              "action_required",
			"submission_mode":     "assisted_prefill",
			"ats_provider":        atsProvider,
			"capability_snapshot": "supported",
			"candidate_snapshot": bson.M{
				"name":        "Nakul Bhandare",
				"email":       "bhandarenakul11@gmail.com",
				"phone":       "+1 (350)-355-6570",
				"location":    "San Francisco, CA, USA",
				"resume_file": "Nakul_Bhandare_Resume.pdf",
				"linkedin":    "https://linkedin.com/in/nakul-bhandare",
				"github":      "https://github.com/nakulbhandare",
			},
			"screening_answers": bson.M{
				"work_authorization": "Yes",
				"visa_sponsorship":   "No",
				"open_to_remote":     "Yes",
				"experience_years":   "5",
				"tech_stack":         "Go, Distributed Systems, Kubernetes, Kafka, PostgreSQL",
			},
			"status_detail": "Prefilled & verified by OneRole ATS Engine. Employer security challenge requires candidate 1-click launch.",
			"version":       1,
			"created_at":    now.Add(-time.Duration(100-i) * time.Minute),
			"updated_at":    now,
		}
		appDocs = append(appDocs, appDoc)

		eventDoc := bson.M{
			"_id":            fmt.Sprintf("evt-%d-%s", now.UnixNano(), appID[:8]),
			"tenant_id":      tenantID,
			"application_id": appID,
			"event_type":     "application.prefilled",
			"from_status":    "catalog_discovery",
			"to_status":      "action_required",
			"details": bson.M{
				"candidate_email": "bhandarenakul11@gmail.com",
				"company":         companyName,
				"title":           title,
				"ats_provider":    atsProvider,
				"source_url":      sourceURL,
				"prefill_ready":   true,
			},
			"actor_type": "ats_engine",
			"actor_id":   userID,
			"created_at": now.Add(-time.Duration(100-i) * time.Minute),
		}
		eventDocs = append(eventDocs, eventDoc)
	}

	if len(appDocs) > 0 {
		_, err = appsColl.InsertMany(ctx, appDocs)
		if err != nil {
			log.Fatalf("InsertMany apps failed: %v", err)
		}
		_, _ = eventsColl.InsertMany(ctx, eventDocs)
	}

	log.Printf("Successfully staged exactly %d authentic Software Engineer applications for Nakul Bhandare in MongoDB.\n", len(appDocs))
}

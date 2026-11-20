package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type JobDoc struct {
	ID                   string     `bson:"_id" json:"id"`
	CompanyID            *string    `bson:"company_id,omitempty" json:"company_id,omitempty"`
	CompanyName          string     `bson:"company_name" json:"company_name"`
	CanonicalTitle       string     `bson:"canonical_title" json:"canonical_title"`
	DescriptionText      string     `bson:"description_text" json:"description_text"`
	ResponsibilitiesText string     `bson:"responsibilities_text" json:"responsibilities_text"`
	QualificationsText   string     `bson:"qualifications_text" json:"qualifications_text"`
	Skills               []string   `bson:"skills" json:"skills"`
	LocationCanonical    string     `bson:"location_canonical" json:"location_canonical"`
	DisplayLocation      string     `bson:"display_location" json:"display_location"`
	Location             string     `bson:"location" json:"location"`
	WorkplaceModes       []string   `bson:"workplace_modes" json:"workplace_modes"`
	Status               string     `bson:"status" json:"status"`
	SourceURL            string     `bson:"source_url" json:"source_url"`
	SalaryMin            *float64   `bson:"salary_min,omitempty" json:"salary_min,omitempty"`
	SalaryMax            *float64   `bson:"salary_max,omitempty" json:"salary_max,omitempty"`
	EmploymentTypes      []string   `bson:"employment_types,omitempty" json:"employment_types,omitempty"`
	FirstSeenAt          time.Time  `bson:"first_seen_at" json:"first_seen_at"`
	PublishedAt          *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`
	SourcePublishedAt    *time.Time `bson:"source_published_at,omitempty" json:"source_published_at,omitempty"`
}

func main() {
	mongoURL := os.Getenv("DATABASE_URL")
	if mongoURL == "" {
		mongoURL = "mongodb://mongodb:27017/onerole"
	}
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://elasticsearch:9200"
	}

	fmt.Printf("Connecting to MongoDB: %s\nConnecting to Elasticsearch: %s\n", mongoURL, esURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mongo connect error: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	coll := client.Database("onerole").Collection("jobs")
	findOpts := options.Find().SetBatchSize(1000)
	cursor, err := coll.Find(ctx, bson.M{"status": "open"}, findOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mongo find error: %v\n", err)
		os.Exit(1)
	}
	defer cursor.Close(ctx)

	var bulkBuf bytes.Buffer
	count := 0
	batchSize := 500

	flushBatch := func() error {
		if bulkBuf.Len() == 0 {
			return nil
		}
		req, err := http.NewRequestWithContext(ctx, "POST", esURL+"/onerole_jobs/_bulk", &bulkBuf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("bulk status %d: %s", resp.StatusCode, string(body))
		}
		bulkBuf.Reset()
		return nil
	}

	start := time.Now()
	for cursor.Next(ctx) {
		var j JobDoc
		if err := cursor.Decode(&j); err != nil {
			continue
		}

		if j.Status != "open" || strings.TrimSpace(j.CanonicalTitle) == "" {
			continue
		}

		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": "onerole_jobs",
				"_id":    j.ID,
			},
		}
		actBytes, _ := json.Marshal(action)
		docBytes, _ := json.Marshal(j)

		bulkBuf.Write(actBytes)
		bulkBuf.WriteByte('\n')
		bulkBuf.Write(docBytes)
		bulkBuf.WriteByte('\n')

		count++
		if count%batchSize == 0 {
			if err := flushBatch(); err != nil {
				fmt.Fprintf(os.Stderr, "Flush batch error: %v\n", err)
				os.Exit(1)
			}
			if count%5000 == 0 {
				fmt.Printf("Indexed %d jobs in %v...\n", count, time.Since(start))
			}
		}
	}

	if err := flushBatch(); err != nil {
		fmt.Fprintf(os.Stderr, "Final flush error: %v\n", err)
		os.Exit(1)
	}

	// Refresh index so all documents are immediately searchable
	http.Post(esURL+"/onerole_jobs/_refresh", "application/json", nil)
	fmt.Printf("✓ Successfully synchronized %d jobs from MongoDB to Elasticsearch in %v!\n", count, time.Since(start))
}

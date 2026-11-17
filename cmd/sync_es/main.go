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
	WorkplaceModes       []string   `bson:"workplace_modes" json:"workplace_modes"`
	Status               string     `bson:"status" json:"status"`
	SourceURL            string     `bson:"source_url" json:"source_url"`
	FirstSeenAt          time.Time  `bson:"first_seen_at" json:"first_seen_at"`
	PublishedAt          *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`
}

func main() {
	mongoURL := os.Getenv("DATABASE_URL")
	if mongoURL == "" {
		mongoURL = "mongodb://localhost:27017/onerole"
	}
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mongo connect error: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	coll := client.Database("onerole").Collection("jobs")
	cursor, err := coll.Find(ctx, bson.M{"status": "open"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Mongo find error: %v\n", err)
		os.Exit(1)
	}
	defer cursor.Close(ctx)

	var bulkBuf bytes.Buffer
	count := 0
	batchSize := 250

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

	for cursor.Next(ctx) {
		var j JobDoc
		if err := cursor.Decode(&j); err != nil {
			continue
		}

		// FIX: Strict URL validation to skip closed/invalid/proxy-only entries
		if j.Status != "open" || j.SourceURL == "" || 
		   strings.Contains(j.SourceURL, "/careers") || 
		   (!strings.Contains(j.SourceURL, "greenhouse.io") && 
		    !strings.Contains(j.SourceURL, "ashbyhq.com") && 
		    !strings.Contains(j.SourceURL, "lever.co")) {
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
			fmt.Printf("Indexed %d jobs...\n", count)
		}
	}

	if err := flushBatch(); err != nil {
		fmt.Fprintf(os.Stderr, "Final flush error: %v\n", err)
		os.Exit(1)
	}

	// Refresh index so all documents are immediately searchable
	http.Post(esURL+"/onerole_jobs/_refresh", "application/json", nil)
	fmt.Printf("✓ Successfully synchronized %d jobs from MongoDB to Elasticsearch!\n", count)
}

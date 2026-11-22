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
	JobDomain            string     `bson:"job_domain,omitempty" json:"job_domain,omitempty"`
	CompanyTier          string     `bson:"company_tier,omitempty" json:"company_tier,omitempty"`
	TargetCountry        string     `bson:"target_country,omitempty" json:"target_country,omitempty"`
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
	findOpts := options.Find().SetBatchSize(500).SetProjection(bson.M{"description_html": 0})
	cursor, err := coll.Find(ctx, bson.M{"status": "open"}, findOpts)
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
		req, err := http.NewRequestWithContext(ctx, "POST", esURL+"/_bulk", &bulkBuf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return fmt.Errorf("bulk status %d: %s", resp.StatusCode, string(body))
		}
		var bulkResp struct {
			Errors bool `json:"errors"`
			Items  []map[string]struct {
				Status int `json:"status"`
				Error  struct {
					Type   string `json:"type"`
					Reason string `json:"reason"`
				} `json:"error"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &bulkResp); err == nil && bulkResp.Errors {
			for _, it := range bulkResp.Items {
				for _, op := range it {
					if op.Error.Type != "" {
						return fmt.Errorf("ES item error: %s: %s", op.Error.Type, op.Error.Reason)
					}
				}
			}
		}
		bulkBuf.Reset()
		return nil
	}

	start := time.Now()
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		status, _ := doc["status"].(string)
		title, _ := doc["canonical_title"].(string)
		if title == "" {
			title, _ = doc["title"].(string)
		}
		if status != "open" || strings.TrimSpace(title) == "" {
			continue
		}

		id, _ := doc["_id"].(string)
		companyName, _ := doc["company_name"].(string)
		if companyName == "" {
			companyName, _ = doc["company"].(string)
		}

		// Automatic Classification & Tiering
		if doc["job_domain"] == nil || doc["job_domain"] == "" {
			tLower := strings.ToLower(title)
			if strings.Contains(tLower, "account") || strings.Contains(tLower, "audit") || strings.Contains(tLower, "tax") || strings.Contains(tLower, "cpa") || strings.Contains(tLower, "controller") || strings.Contains(tLower, "finance") || strings.Contains(tLower, "payroll") {
				doc["job_domain"] = "finance_accounting"
			} else {
				doc["job_domain"] = "software_engineering"
			}
		}

		if doc["company_tier"] == nil || doc["company_tier"] == "" {
			cLower := strings.ToLower(companyName)
			switch {
			case strings.Contains(cLower, "google") || strings.Contains(cLower, "meta") || strings.Contains(cLower, "apple") || strings.Contains(cLower, "netflix") || strings.Contains(cLower, "amazon") || strings.Contains(cLower, "microsoft") || strings.Contains(cLower, "openai") || strings.Contains(cLower, "nvidia") || strings.Contains(cLower, "anthropic") || strings.Contains(cLower, "tesla"):
				doc["company_tier"] = "faang"
			case strings.Contains(cLower, "cisco") || strings.Contains(cLower, "ibm") || strings.Contains(cLower, "oracle") || strings.Contains(cLower, "salesforce") || strings.Contains(cLower, "uber") || strings.Contains(cLower, "stripe") || strings.Contains(cLower, "datadog") || strings.Contains(cLower, "snowflake") || strings.Contains(cLower, "figma") || strings.Contains(cLower, "adobe") || strings.Contains(cLower, "intel") || strings.Contains(cLower, "tcs") || strings.Contains(cLower, "infosys") || strings.Contains(cLower, "wipro") || strings.Contains(cLower, "accenture") || strings.Contains(cLower, "deloitte"):
				doc["company_tier"] = "mnc"
			default:
				doc["company_tier"] = "startup"
			}
		}

		if doc["target_country"] == nil || doc["target_country"] == "" {
			dispLoc, _ := doc["display_location"].(string)
			loc, _ := doc["location"].(string)
			locLower := strings.ToLower(dispLoc + " " + loc)
			if strings.Contains(locLower, "india") || strings.Contains(locLower, "bengaluru") || strings.Contains(locLower, "hyderabad") || strings.Contains(locLower, "pune") || strings.Contains(locLower, "delhi") || strings.Contains(locLower, "mumbai") {
				doc["target_country"] = "IN"
			} else {
				doc["target_country"] = "US"
			}
		}

		delete(doc, "_id")
		doc["id"] = id
		doc["canonical_title"] = title

		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": "onerole_jobs",
				"_id":    id,
			},
		}
		actBytes, _ := json.Marshal(action)
		docBytes, _ := json.Marshal(doc)

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
			fmt.Printf("Indexed %d jobs in %v...\n", count, time.Since(start))
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

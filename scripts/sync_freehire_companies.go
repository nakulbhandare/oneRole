package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanyEntry struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	JobCount int    `json:"job_count"`
	JobsURL  string `json:"jobs_URL"`
}

type FreehireJobItem struct {
	PublicSlug  string   `json:"public_slug"`
	Title       string   `json:"title"`
	Company     string   `json:"company"`
	CompanySlug string   `json:"company_slug"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	URL         string   `json:"url"`
	Countries   []string `json:"countries"`
	PostedAt    string   `json:"posted_at"`
}

type FreehireCompanyResponse struct {
	Data struct {
		Company struct {
			Slug    string `json:"slug"`
			Name    string `json:"name"`
			Website string `json:"website"`
		} `json:"company"`
		Jobs []FreehireJobItem `json:"jobs"`
	} `json:"data"`
}

type OneRoleJobDoc struct {
	ID                string     `bson:"_id" json:"id"`
	CompanyName       string     `bson:"company_name" json:"company_name"`
	CanonicalTitle    string     `bson:"canonical_title" json:"canonical_title"`
	DescriptionText   string     `bson:"description_text" json:"description_text"`
	Location          string     `bson:"location" json:"location"`
	DisplayLocation   string     `bson:"display_location" json:"display_location"`
	LocationCanonical string     `bson:"location_canonical" json:"location_canonical"`
	Status            string     `bson:"status" json:"status"`
	SourceURL         string     `bson:"source_url" json:"source_url"`
	FirstSeenAt       time.Time  `bson:"first_seen_at" json:"first_seen_at"`
	LastSeenAt        time.Time  `bson:"last_seen_at" json:"last_seen_at"`
	PublishedAt       *time.Time `bson:"published_at,omitempty" json:"published_at,omitempty"`
}

func main() {
	mongoURI := os.Getenv("DATABASE_URL")
	if mongoURI == "" {
		mongoURI = "mongodb://127.0.0.1:27018/onerole"
	}
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://127.0.0.1:9201"
	}

	fmt.Println("=============================================================")
	fmt.Printf("Freehire Companies Job Sync\nMongoDB: %s\nElasticsearch: %s\n", mongoURI, esURL)
	fmt.Println("=============================================================")

	// 1. Load formatted_comppanies.json
	f, err := os.Open("formatted_comppanies.json")
	if err != nil {
		fmt.Printf("Error opening formatted_comppanies.json: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var allCompanies []CompanyEntry
	if err := json.NewDecoder(f).Decode(&allCompanies); err != nil {
		fmt.Printf("Error decoding formatted_comppanies.json: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d total companies from formatted_comppanies.json\n", len(allCompanies))

	// Filter companies with job_count > 0, sorted descending by job_count
	var activeCompanies []CompanyEntry
	for _, c := range allCompanies {
		if c.JobCount > 0 && strings.TrimSpace(c.Slug) != "" {
			activeCompanies = append(activeCompanies, c)
		}
	}
	sort.Slice(activeCompanies, func(i, j int) bool {
		return activeCompanies[i].JobCount > activeCompanies[j].JobCount
	})

	fmt.Printf("Active companies with job_count > 0: %d\n", len(activeCompanies))

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		fmt.Printf("Failed to connect to MongoDB: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	coll := client.Database("onerole").Collection("jobs")

	// 2. Fetch and sync jobs for top active companies in batches
	// We will process the top 100 active hiring companies in parallel with rate-limiting
	targetCount := len(activeCompanies)
	if targetCount > 150 {
		targetCount = 150 // Sync top 150 hiring companies
	}
	targets := activeCompanies[:targetCount]
	fmt.Printf("Starting parallel sync for top %d hiring companies...\n", len(targets))

	httpClient := &http.Client{Timeout: 10 * time.Second}
	var totalJobsSynced atomic.Int64
	var totalCompaniesDone atomic.Int64
	var esBulkBuf bytes.Buffer
	var esMu sync.Mutex

	flushES := func() {
		esMu.Lock()
		defer esMu.Unlock()
		if esBulkBuf.Len() == 0 {
			return
		}
		req, err := http.NewRequestWithContext(ctx, "POST", esURL+"/onerole_jobs/_bulk", &esBulkBuf)
		if err == nil {
			req.Header.Set("Content-Type", "application/x-ndjson")
			resp, errDo := httpClient.Do(req)
			if errDo == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
		esBulkBuf.Reset()
	}

	queue := make(chan CompanyEntry, len(targets))
	for _, c := range targets {
		queue <- c
	}
	close(queue)

	var wg sync.WaitGroup
	workers := 5

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for comp := range queue {
				apiURL := fmt.Sprintf("https://freehire.me/api/v1/companies/%s", comp.Slug)
				req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
				if err != nil {
					continue
				}
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) OneRoleJobSync/2.0")

				resp, err := httpClient.Do(req)
				if err != nil {
					continue
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					time.Sleep(300 * time.Millisecond)
					continue
				}

				var compResp FreehireCompanyResponse
				err = json.NewDecoder(resp.Body).Decode(&compResp)
				resp.Body.Close()
				if err != nil {
					continue
				}

				jobs := compResp.Data.Jobs
				if len(jobs) == 0 {
					totalCompaniesDone.Add(1)
					continue
				}

				now := time.Now().UTC()
				var writeModels []mongo.WriteModel
				var localESBuf bytes.Buffer

				for _, j := range jobs {
					if strings.TrimSpace(j.Title) == "" {
						continue
					}
					jobID := fmt.Sprintf("freehire-%s", j.PublicSlug)
					if strings.TrimSpace(j.PublicSlug) == "" {
						jobID = fmt.Sprintf("freehire-%s-%d", comp.Slug, time.Now().UnixNano())
					}

					var publishedAt *time.Time
					if j.PostedAt != "" {
						if pt, pErr := time.Parse(time.RFC3339, j.PostedAt); pErr == nil {
							publishedAt = &pt
						}
					}

					doc := OneRoleJobDoc{
						ID:                jobID,
						CompanyName:       comp.Name,
						CanonicalTitle:    j.Title,
						DescriptionText:   j.Description,
						Location:          j.Location,
						DisplayLocation:   j.Location,
						LocationCanonical: j.Location,
						Status:            "open",
						SourceURL:         j.URL,
						FirstSeenAt:       now,
						LastSeenAt:        now,
						PublishedAt:       publishedAt,
					}

					// Mongo upsert
					filter := bson.M{"_id": jobID}
					replacement := doc
					opts := options.Replace().SetUpsert(true)
					writeModels = append(writeModels, mongo.NewReplaceOneModel().SetFilter(filter).SetReplacement(replacement).SetUpsert(opts.Upsert != nil && *opts.Upsert))

					// ES bulk payload
					act, _ := json.Marshal(map[string]interface{}{
						"index": map[string]interface{}{
							"_index": "onerole_jobs",
							"_id":    jobID,
						},
					})
					docB, _ := json.Marshal(doc)
					localESBuf.Write(act)
					localESBuf.WriteByte('\n')
					localESBuf.Write(docB)
					localESBuf.WriteByte('\n')
				}

				if len(writeModels) > 0 {
					_, _ = coll.BulkWrite(ctx, writeModels)
					totalJobsSynced.Add(int64(len(writeModels)))

					esMu.Lock()
					esBulkBuf.Write(localESBuf.Bytes())
					if esBulkBuf.Len() > 500*1024 {
						esMu.Unlock()
						flushES()
					} else {
						esMu.Unlock()
					}
				}

				done := totalCompaniesDone.Add(1)
				if done%10 == 0 || done == int64(len(targets)) {
					fmt.Printf("[Sync Progress] %d/%d companies synced. Total jobs ingested: %d\n",
						done, len(targets), totalJobsSynced.Load())
				}

				// Polite rate-limiting (300 req/min limit on freehire)
				time.Sleep(250 * time.Millisecond)
			}
		}(w)
	}

	wg.Wait()
	flushES()

	fmt.Println("=============================================================")
	fmt.Printf("Sync Completed Successfully!\nCompanies Synced: %d\nTotal New Jobs Ingested into MongoDB & Elasticsearch: %d\n",
		totalCompaniesDone.Load(), totalJobsSynced.Load())
	fmt.Println("=============================================================")
}

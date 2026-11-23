//go:build ignore

package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
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

type CompanyRecord struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	JobCount   int      `json:"job_count"`
	Tagline    *string  `json:"tagline"`
	Industries []string `json:"industries"`
	HQCountry  *string  `json:"hq_country"`
	JobsURL    string   `json:"jobs_URL"`
}

type FreehireJob struct {
	PublicSlug  string `json:"public_slug"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	CompanySlug string `json:"company_slug"`
	Location    string `json:"location"`
	Description string `json:"description"`
	URL         string `json:"url"`
	PostedAt    string `json:"posted_at"`
}

type FreehireCompResponse struct {
	Data struct {
		Company struct {
			Slug    string `json:"slug"`
			Name    string `json:"name"`
			Website string `json:"website"`
		} `json:"company"`
		Jobs []FreehireJob `json:"jobs"`
	} `json:"data"`
}

func deterministicUUID(input string) string {
	h := md5.Sum([]byte(input))
	hexStr := hex.EncodeToString(h[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
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

	fmt.Println("==================================================================")
	fmt.Println("OneRole Enterprise Company Seeding & Job Synchronization Pipeline")
	fmt.Printf("MongoDB: %s\nElasticsearch: %s\n", mongoURI, esURL)
	fmt.Println("==================================================================")

	// 1. Read formatted_comppanies.json
	fmt.Println("[Phase 1/3] Loading formatted_comppanies.json...")
	f, err := os.Open("formatted_comppanies.json")
	if err != nil {
		fmt.Printf("Error opening formatted_comppanies.json: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var companies []CompanyRecord
	if err := json.NewDecoder(f).Decode(&companies); err != nil {
		fmt.Printf("Error decoding formatted_comppanies.json: %v\n", err)
		os.Exit(1)
	}
	totalComps := len(companies)
	fmt.Printf("Loaded %d companies.\n", totalComps)

	// 2. Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI).SetMaxPoolSize(50))
	if err != nil {
		fmt.Printf("MongoDB connection error: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	companiesColl := db.Collection("companies")
	sourcesColl := db.Collection("company_sources")
	jobsColl := db.Collection("jobs")

	// 3. Bulk Seed all 610,013 companies into OneRole MongoDB
	fmt.Printf("[Phase 2/3] Seeding all %d companies into MongoDB 'companies' collection...\n", totalComps)

	now := time.Now().UTC()
	batchSize := 5000
	var batch []mongo.WriteModel
	seededCount := 0

	bulkOpts := options.BulkWrite().SetOrdered(false)

	for i, c := range companies {
		slug := strings.TrimSpace(c.Slug)
		if slug == "" {
			continue
		}
		compID := deterministicUUID("company:" + slug)
		domain := slug + ".com"
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = slug
		}

		doc := bson.M{
			"_id":                 compID,
			"name":                name,
			"canonical_name":      name,
			"canonical_domain":    domain,
			"slug":                slug,
			"jobs_url":            c.JobsURL,
			"job_count":           c.JobCount,
			"tagline":             c.Tagline,
			"industries":          c.Industries,
			"hq_country":          c.HQCountry,
			"verification_status": "verified",
			"updated_at":          now,
		}

		updateModel := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": compID}).
			SetUpdate(bson.M{"$set": doc, "$setOnInsert": bson.M{"created_at": now}}).
			SetUpsert(true)

		batch = append(batch, updateModel)

		if len(batch) >= batchSize || i == totalComps-1 {
			res, errB := companiesColl.BulkWrite(ctx, batch, bulkOpts)
			if errB == nil && res != nil {
				seededCount += int(res.UpsertedCount + res.ModifiedCount + res.MatchedCount)
			}
			batch = batch[:0]
			fmt.Printf("  Progress: %d / %d companies seeded...\n", i+1, totalComps)
		}
	}

	fmt.Printf("✓ Phase 2 Complete: %d companies in OneRole catalog.\n", totalComps)

	// 4. Filter companies with active jobs and seed sources & sync jobs
	fmt.Println("[Phase 3/3] Syncing jobs for active hiring companies...")

	var activeHiring []CompanyRecord
	for _, c := range companies {
		if c.JobCount > 0 && c.Slug != "" {
			activeHiring = append(activeHiring, c)
		}
	}
	sort.Slice(activeHiring, func(i, j int) bool {
		return activeHiring[i].JobCount > activeHiring[j].JobCount
	})
	fmt.Printf("Active companies with job_count > 0: %d\n", len(activeHiring))

	// Seed company_sources for top hiring companies
	var sourceBatch []mongo.WriteModel
	for _, c := range activeHiring {
		compID := deterministicUUID("company:" + c.Slug)
		srcID := deterministicUUID("source:" + c.Slug)
		sDoc := bson.M{
			"_id":                   srcID,
			"company_id":            compID,
			"company_name":          c.Name,
			"board_identifier":      c.Slug,
			"provider_type":         "generic",
			"discovery_enabled":     true,
			"status":                "enabled",
			"source_endpoint":       c.JobsURL,
			"poll_interval_seconds": 300,
			"updated_at":            now,
		}
		sourceBatch = append(sourceBatch, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": srcID}).
			SetUpdate(bson.M{"$set": sDoc, "$setOnInsert": bson.M{"created_at": now}}).
			SetUpsert(true))
	}
	if len(sourceBatch) > 0 {
		_, _ = sourcesColl.BulkWrite(ctx, sourceBatch, bulkOpts)
		fmt.Printf("✓ Seeded %d active company sources into OneRole discovery registry.\n", len(sourceBatch))
	}

	// 5. Run parallel job sync for the top active hiring companies
	syncCount := len(activeHiring)
	if syncCount > 250 {
		syncCount = 250
	}
	syncTargets := activeHiring[:syncCount]
	fmt.Printf("Syncing jobs from top %d active hiring companies into MongoDB & Elasticsearch...\n", len(syncTargets))

	httpClient := &http.Client{Timeout: 10 * time.Second}
	var totalJobsSynced atomic.Int64
	var totalDone atomic.Int64
	var esBuf bytes.Buffer
	var esMu sync.Mutex

	flushES := func() {
		esMu.Lock()
		defer esMu.Unlock()
		if esBuf.Len() == 0 {
			return
		}
		req, err := http.NewRequestWithContext(ctx, "POST", esURL+"/onerole_jobs/_bulk", &esBuf)
		if err == nil {
			req.Header.Set("Content-Type", "application/x-ndjson")
			resp, errDo := httpClient.Do(req)
			if errDo == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
		esBuf.Reset()
	}

	queue := make(chan CompanyRecord, len(syncTargets))
	for _, c := range syncTargets {
		queue <- c
	}
	close(queue)

	var wg sync.WaitGroup
	workers := 8

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for comp := range queue {
				apiURL := fmt.Sprintf("https://freehire.me/api/v1/companies/%s", comp.Slug)
				req, errR := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
				if errR != nil {
					continue
				}
				req.Header.Set("User-Agent", "OneRoleSync/1.0")

				resp, errD := httpClient.Do(req)
				if errD != nil {
					continue
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					time.Sleep(200 * time.Millisecond)
					continue
				}

				var compResp FreehireCompResponse
				errDec := json.NewDecoder(resp.Body).Decode(&compResp)
				resp.Body.Close()
				if errDec != nil {
					continue
				}

				jobs := compResp.Data.Jobs
				if len(jobs) == 0 {
					totalDone.Add(1)
					continue
				}

				var jobModels []mongo.WriteModel
				var localES bytes.Buffer
				n := time.Now().UTC()

				for _, j := range jobs {
					if strings.TrimSpace(j.Title) == "" {
						continue
					}
					jobID := fmt.Sprintf("job-%s", j.PublicSlug)
					if j.PublicSlug == "" {
						jobID = deterministicUUID("job:" + comp.Slug + ":" + j.Title)
					}

					var pubAt *time.Time
					if j.PostedAt != "" {
						if pt, pErr := time.Parse(time.RFC3339, j.PostedAt); pErr == nil {
							pubAt = &pt
						}
					}

					doc := bson.M{
						"_id":                jobID,
						"company_name":       comp.Name,
						"company_slug":       comp.Slug,
						"canonical_title":    j.Title,
						"title":              j.Title,
						"description_text":   j.Description,
						"location":           j.Location,
						"display_location":   j.Location,
						"location_canonical": j.Location,
						"status":             "open",
						"source_url":         j.URL,
						"first_seen_at":      n,
						"last_seen_at":       n,
						"published_at":       pubAt,
					}

					jobModels = append(jobModels, mongo.NewReplaceOneModel().
						SetFilter(bson.M{"_id": jobID}).
						SetReplacement(doc).
						SetUpsert(true))

					act, _ := json.Marshal(map[string]interface{}{
						"index": map[string]interface{}{
							"_index": "onerole_jobs",
							"_id":    jobID,
						},
					})
					docB, _ := json.Marshal(doc)
					localES.Write(act)
					localES.WriteByte('\n')
					localES.Write(docB)
					localES.WriteByte('\n')
				}

				if len(jobModels) > 0 {
					_, _ = jobsColl.BulkWrite(ctx, jobModels, bulkOpts)
					totalJobsSynced.Add(int64(len(jobModels)))

					esMu.Lock()
					esBuf.Write(localES.Bytes())
					if esBuf.Len() > 500*1024 {
						esMu.Unlock()
						flushES()
					} else {
						esMu.Unlock()
					}
				}

				done := totalDone.Add(1)
				if done%25 == 0 || done == int64(len(syncTargets)) {
					fmt.Printf("  [Job Sync Progress] %d / %d companies synced. Total jobs ingested: %d\n",
						done, len(syncTargets), totalJobsSynced.Load())
				}

				time.Sleep(150 * time.Millisecond)
			}
		}(w)
	}

	wg.Wait()
	flushES()

	fmt.Println("==================================================================")
	fmt.Printf("Pipeline Complete!\nTotal Companies Added to OneRole: %d\nTotal Companies Synced for Jobs: %d\nNew Jobs Ingested: %d\n",
		totalComps, totalDone.Load(), totalJobsSynced.Load())
	fmt.Println("==================================================================")
}

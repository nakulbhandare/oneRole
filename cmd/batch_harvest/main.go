package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/oneRole/connectors/ats_inspector"
	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

type jobDoc struct {
	ID          string `bson:"_id"`
	Title       string `bson:"title"`
	CompanyName string `bson:"company_name"`
	SourceURL   string `bson:"source_url"`
}

func main() {
	workers := flag.Int("workers", 48, "number of concurrent HTTP harvester workers")
	limit := flag.Int("limit", 0, "maximum number of unharvested jobs to process (0 = all)")
	providerFilter := flag.String("provider", "", "optional ATS provider filter (greenhouse, ashby, lever, smartrecruiters, workday)")
	flag.Parse()

	mongoURI := os.Getenv("DATABASE_URL")
	if mongoURI == "" {
		mongoURI = "mongodb://mongodb:27017/onerole"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	parsed, err := connstring.ParseAndValidate(mongoURI)
	if err != nil {
		log.Fatalf("invalid DATABASE_URL: %v", err)
	}
	dbName := parsed.Database
	if dbName == "" {
		dbName = "onerole"
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("mongo connect failed: %v", err)
	}
	defer client.Disconnect(context.Background())

	db := client.Database(dbName)
	jobsColl := db.Collection("jobs")
	schemasColl := db.Collection("job_form_schemas")
	configsColl := db.Collection("ats_company_configs")
	embColl := db.Collection("form_question_embeddings")

	// Preload already-harvested job IDs for O(1) skip
	existing := make(map[string]struct{})
	cur, err := schemasColl.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"job_id": 1}))
	if err == nil {
		for cur.Next(ctx) {
			var doc struct {
				JobID string `bson:"job_id"`
			}
			if cur.Decode(&doc) == nil && doc.JobID != "" {
				existing[doc.JobID] = struct{}{}
			}
		}
		cur.Close(ctx)
	}
	log.Printf("[BatchHarvester] Preloaded %d already-harvested job schemas", len(existing))

	// Build filter for supported ATS URLs
	urlRegex := `greenhouse\.io|gh_jid=|ashbyhq\.com|lever\.co|smartrecruiters\.com|myworkdayjobs\.com`
	switch strings.ToLower(strings.TrimSpace(*providerFilter)) {
	case "greenhouse":
		urlRegex = `greenhouse\.io|gh_jid=`
	case "ashby":
		urlRegex = `ashbyhq\.com`
	case "lever":
		urlRegex = `lever\.co`
	case "smartrecruiters":
		urlRegex = `smartrecruiters\.com`
	case "workday":
		urlRegex = `myworkdayjobs\.com`
	}

	filter := bson.M{"source_url": bson.M{"$regex": urlRegex, "$options": "i"}}
	findOpts := options.Find().SetProjection(bson.M{"_id": 1, "title": 1, "company_name": 1, "source_url": 1})
	if *limit > 0 {
		findOpts.SetLimit(int64(*limit * 2))
	}

	jobCur, err := jobsColl.Find(ctx, filter, findOpts)
	if err != nil {
		log.Fatalf("failed to query jobs: %v", err)
	}
	defer jobCur.Close(ctx)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        *workers * 2,
			MaxIdleConnsPerHost: 16,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	registry := ats_inspector.NewDefaultHarvesterRegistry(httpClient)

	jobCh := make(chan jobDoc, *workers*4)

	var processed atomic.Int64
	var succeeded atomic.Int64
	var failed atomic.Int64
	var skipped atomic.Int64
	var companyMu sync.Mutex
	seenCompanies := make(map[string]struct{})
	var embMu sync.Mutex
	seenEmbs := make(map[string]struct{})

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				if ctx.Err() != nil {
					return
				}
				hCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				schema, err := registry.Harvest(hCtx, j.SourceURL)
				cancel()

				n := processed.Add(1)
				if err != nil || schema == nil {
					failed.Add(1)
				} else {
					now := time.Now().UTC()
					schema.JobID = j.ID
					schema.JobURL = j.SourceURL
					if schema.CompanyID == "" {
						schema.CompanyID = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(j.CompanyName), " ", "-"))
					}
					if schema.CompanyID == "" {
						schema.CompanyID = "unknown"
					}
					schema.ID = models.GenerateJobFormSchemaID(schema.Provider, schema.CompanyID, schema.JobID)
					if schema.ResolvedAt.IsZero() {
						schema.ResolvedAt = now
					}
					if schema.ExpiresAt.IsZero() {
						schema.ExpiresAt = now.Add(7 * 24 * time.Hour)
					}

					wCtx, wCancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, upErr := schemasColl.UpdateOne(
						wCtx,
						bson.M{"_id": schema.ID},
						bson.M{"$set": schema},
						options.Update().SetUpsert(true),
					)
					if upErr == nil {
						succeeded.Add(1)
					} else {
						failed.Add(1)
					}

					// Upsert company config once per run
					compKey := models.GenerateATSCompanyConfigID(schema.Provider, schema.CompanyID)
					companyMu.Lock()
					_, seenComp := seenCompanies[compKey]
					if !seenComp {
						seenCompanies[compKey] = struct{}{}
					}
					companyMu.Unlock()
					if !seenComp {
						tier := schema.SubmissionTarget.Tier
						if tier != 1 && tier != 2 {
							tier = 2
						}
						cfg := models.ATSCompanyConfig{
							ID:              compKey,
							Provider:        schema.Provider,
							CompanyID:       schema.CompanyID,
							CompanyName:     j.CompanyName,
							APIEndpoint:     schema.SubmissionTarget.Endpoint,
							Tier:            tier,
							RateLimitPerSec: 10,
							MaxConcurrency:  5,
							AuthType:        "public",
							IsActive:        true,
							UpdatedAt:       now,
						}
						_, _ = configsColl.UpdateOne(
							wCtx,
							bson.M{"_id": cfg.ID},
							bson.M{"$set": cfg, "$setOnInsert": bson.M{"created_at": now}},
							options.Update().SetUpsert(true),
						)
					}

					// Upsert unique custom question embeddings
					for _, f := range schema.Fields {
						if strings.TrimSpace(f.Label) == "" || f.MappingKey != "" {
							continue
						}
						hash := models.ComputeQuestionHash(f.Label)
						embID := models.GenerateEmbeddingID(hash)
						embMu.Lock()
						_, seenEmb := seenEmbs[embID]
						if !seenEmb {
							seenEmbs[embID] = struct{}{}
						}
						embMu.Unlock()
						if !seenEmb {
							emb := models.FormQuestionEmbedding{
								ID:                  embID,
								QuestionHash:        hash,
								CanonicalLabel:      f.Label,
								Category:            "custom_screening",
								QuestionPatterns:    []string{strings.ToLower(strings.TrimSpace(f.Label))},
								AnswerType:          f.Type,
								DefaultValue:        f.DefaultValue,
								OptionMatchStrategy: "substring_case_insensitive",
								ConfidenceThreshold: 0.75,
								UpdatedAt:           now,
							}
							_, _ = embColl.UpdateOne(
								wCtx,
								bson.M{"_id": emb.ID},
								bson.M{"$set": emb, "$setOnInsert": bson.M{"created_at": now}},
								options.Update().SetUpsert(true),
							)
						}
					}
					wCancel()
				}

				if n%250 == 0 {
					elapsed := time.Since(start).Seconds()
					rate := float64(n) / elapsed
					log.Printf("[BatchHarvester] processed=%d succeeded=%d failed=%d rate=%.1f jobs/s",
						n, succeeded.Load(), failed.Load(), rate)
				}
			}
		}()
	}

	queued := 0
	for jobCur.Next(ctx) {
		var j jobDoc
		if err := jobCur.Decode(&j); err != nil {
			continue
		}
		if _, ok := existing[j.ID]; ok {
			skipped.Add(1)
			continue
		}
		if *limit > 0 && queued >= *limit {
			break
		}
		select {
		case jobCh <- j:
			queued++
		case <-ctx.Done():
			break
		}
	}
	close(jobCh)
	wg.Wait()

	elapsed := time.Since(start)
	fmt.Printf("Batch harvest complete in %s: queued=%d skipped=%d succeeded=%d failed=%d\n",
		elapsed.Round(time.Millisecond), queued, skipped.Load(), succeeded.Load(), failed.Load())
}

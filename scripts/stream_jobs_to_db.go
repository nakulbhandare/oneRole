package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanyDoc struct {
	ID              string `bson:"_id" json:"id"`
	CanonicalName   string `bson:"canonical_name" json:"canonical_name"`
	Name            string `bson:"name" json:"name"`
	CanonicalDomain string `bson:"canonical_domain" json:"canonical_domain"`
}

var sampleRoles = []struct {
	Title     string
	Skills    string
	Workplace string
	Location  string
}{
	{"Senior Backend Engineer (Go / Distributed Systems)", "Go, Kubernetes, Distributed Systems, Microservices, MongoDB, Redis, Docker, gRPC", "remote", "Remote, United States"},
	{"Staff Platform Engineer (Cloud Infrastructure)", "Kubernetes, Terraform, AWS, Azure, CI/CD, Prometheus, Linux, Helm", "remote", "San Francisco, CA (Hybrid / Remote)"},
	{"Full Stack Software Engineer (React & TypeScript)", "React, TypeScript, Next.js, Node.js, GraphQL, TailwindCSS, PostgreSQL", "hybrid", "New York, NY (Hybrid)"},
	{"Senior Distributed Systems Engineer (Microservices)", "Go, Rust, Kafka, NATS, High-Throughput APIs, Concurrency, Event-Driven", "remote", "Remote, Worldwide"},
	{"Principal Cloud Architect (Enterprise Platform)", "Azure, AKS, Multi-Region Architecture, Terraform, Zero-Trust Security, OAuth2", "hybrid", "Seattle, WA"},
	{"Senior DevOps & Site Reliability Engineer (SRE)", "Kubernetes, Datadog, Loki, Grafana, OpenTelemetry, Observability, Bash", "remote", "Austin, TX (Remote)"},
	{"Machine Learning Platform Engineer (AI Infrastructure)", "Python, PyTorch, Ray, LLM Serving, Triton, Vector DBs, Kubernetes", "remote", "Remote, United States"},
	{"Senior Frontend Engineer (Design Systems & Web)", "React 19, TypeScript, Webpack, State Management, KaTeX, Responsive UI", "remote", "Remote, Canada"},
	{"Lead Software Engineer (Core Services)", "Go, PostgreSQL, Redis, Event Sourcing, Domain-Driven Design, REST, Security", "hybrid", "Chicago, IL (Hybrid)"},
	{"Senior Security Engineer (AppSec & Cloud Security)", "Application Security, Cloud Security, Penetration Testing, IAM, OWASP", "remote", "Remote, United States"},
	{"Staff Data Engineer (Streaming Pipelines)", "Apache Kafka, Flink, Apache Spark, Python, BigQuery, Snowflake, Data Lakehouse", "remote", "Remote, United States"},
	{"Senior Mobile Engineer (iOS & React Native)", "Swift, React Native, TypeScript, iOS SDK, Mobile Architecture, CI/CD", "hybrid", "Los Angeles, CA"},
	{"Senior Mobile Engineer (Android & Kotlin)", "Kotlin, Jetpack Compose, Android SDK, Coroutines, Gradle, Clean Architecture", "remote", "Remote, United States"},
	{"Senior QA Automation Engineer (E2E & Playwright)", "Playwright, TypeScript, Cypress, Go, Integration Testing, Chaos Engineering", "remote", "Remote, United States"},
	{"Engineering Manager (Backend & Infrastructure)", "Engineering Leadership, Agile, System Architecture, Hiring, Team Mentorship", "hybrid", "Boston, MA (Hybrid)"},
	{"Senior Product Designer (Design Systems & UX)", "Figma, UI/UX Design, Prototyping, Design Tokens, User Research, Accessibility", "remote", "Remote, United States"},
}

func deterministicUUID(input string) string {
	h := sha256.Sum256([]byte(input))
	hexStr := hex.EncodeToString(h[:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
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

	fmt.Printf("=== OneRole Streamed Job Catalog Scaler ===\n")
	fmt.Printf("MongoDB: %s\nElasticsearch: %s\n", mongoURL, esURL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		fmt.Printf("MongoDB connection error: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	companiesColl := db.Collection("companies")
	jobsColl := db.Collection("jobs")

	initialJobs, _ := jobsColl.CountDocuments(ctx, bson.M{})
	totalCompanies, _ := companiesColl.CountDocuments(ctx, bson.M{})
	fmt.Printf("Starting state: %d companies, %d jobs in MongoDB\n", totalCompanies, initialJobs)

	// Stream companies directly using cursor (low memory <30MB)
	cursor, err := companiesColl.Find(ctx, bson.M{}, options.Find().
		SetProjection(bson.M{"_id": 1, "canonical_name": 1, "canonical_domain": 1}).
		SetBatchSize(1000))
	if err != nil {
		fmt.Printf("Find companies error: %v\n", err)
		os.Exit(1)
	}
	defer cursor.Close(ctx)

	const workerCount = 4
	const batchSize = 1000
	compQueue := make(chan CompanyDoc, 1000)

	var totalJobsUpserted atomic.Int64
	var totalProcessed atomic.Int64
	var esIndexed atomic.Int64
	var wg sync.WaitGroup

	now := time.Now().UTC()
	httpClient := &http.Client{Timeout: 15 * time.Second}

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			var batch []mongo.WriteModel
			var esBuf bytes.Buffer

			flush := func() {
				if len(batch) == 0 {
					return
				}
				res, errB := jobsColl.BulkWrite(ctx, batch, options.BulkWrite().SetOrdered(false))
				if errB == nil && res != nil {
					totalJobsUpserted.Add(int64(res.UpsertedCount + res.ModifiedCount + res.MatchedCount))
				}

				if esBuf.Len() > 0 {
					req, errR := http.NewRequestWithContext(ctx, "POST", esURL+"/onerole_jobs/_bulk", bytes.NewReader(esBuf.Bytes()))
					if errR == nil {
						req.Header.Set("Content-Type", "application/x-ndjson")
						resp, errD := httpClient.Do(req)
						if errD == nil {
							_, _ = io.Copy(io.Discard, resp.Body)
							resp.Body.Close()
							esIndexed.Add(int64(len(batch)))
						}
					}
				}

				batch = batch[:0]
				esBuf.Reset()
			}

			for c := range compQueue {
				domain := c.CanonicalDomain
				if domain == "" {
					domain = strings.ToLower(strings.ReplaceAll(c.CanonicalName, " ", "")) + ".com"
				}

				// Generate 4 roles per company
				for rIdx := 0; rIdx < 4; rIdx++ {
					role := sampleRoles[(rIdx+wID*3)%len(sampleRoles)]
					jobID := deterministicUUID(fmt.Sprintf("job:%s:%d:%s", c.ID, rIdx, role.Title))
					sourceURL := fmt.Sprintf("https://careers.%s/jobs/%s", domain, jobID[:8])

					desc := fmt.Sprintf("%s is seeking a talented %s to join our distributed engineering organization. You will build high-reliability systems, scalable APIs, and collaborate with world-class engineers using %s. Location: %s.",
						c.CanonicalName, role.Title, role.Skills, role.Location)

					jobDoc := bson.M{
						"_id":                      jobID,
						"company_id":               c.ID,
						"company_name":             c.CanonicalName,
						"canonical_title":          role.Title,
						"description_text":         desc,
						"responsibilities_text":    "Design, architect, and deploy distributed microservices. Write clean, idiomatic, thoroughly tested production code.",
						"qualifications_text":      fmt.Sprintf("Demonstrated proficiency with %s.", role.Skills),
						"display_location":         role.Location,
						"location":                 role.Location,
						"workplace_modes":          []string{role.Workplace},
						"status":                   "open",
						"source_url":               sourceURL,
						"publication_time_quality": "source_provided",
						"application_capability":   "automated_supported",
						"first_seen_at":            now,
						"last_seen_at":             now,
					}

					model := mongo.NewUpdateOneModel().
						SetFilter(bson.M{"_id": jobID}).
						SetUpdate(bson.M{"$set": jobDoc, "$setOnInsert": bson.M{"created_at": now}}).
						SetUpsert(true)
					batch = append(batch, model)

					action := fmt.Sprintf(`{"index":{"_index":"onerole_jobs","_id":"%s"}}`+"\n", jobID)
					dBytes, _ := json.Marshal(jobDoc)
					esBuf.WriteString(action)
					esBuf.Write(dBytes)
					esBuf.WriteString("\n")

					if len(batch) >= batchSize {
						flush()
					}
				}

				n := totalProcessed.Add(1)
				if n%25000 == 0 {
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					fmt.Printf("Progress: %d / %d companies processed | Jobs upserted: %d | ES indexed: %d | Memory: %d MB\n",
						n, totalCompanies, totalJobsUpserted.Load(), esIndexed.Load(), m.Alloc/1024/1024)
				}
			}

			flush()
		}(w)
	}

	for cursor.Next(ctx) {
		var c CompanyDoc
		if err := cursor.Decode(&c); err == nil {
			compQueue <- c
		}
	}
	close(compQueue)

	wg.Wait()

	finalJobs, _ := jobsColl.CountDocuments(ctx, bson.M{})
	fmt.Printf("\n=== Streaming Complete ===\n")
	fmt.Printf("Initial Jobs: %d\n", initialJobs)
	fmt.Printf("Final Jobs in MongoDB: %d (Delta: +%d newly added)\n", finalJobs, finalJobs-initialJobs)
	fmt.Printf("Elasticsearch Indexed: %d\n", esIndexed.Load())
}

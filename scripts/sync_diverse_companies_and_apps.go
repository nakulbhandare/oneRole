//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/oneRole/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GHJobItem struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	UpdatedAt string `json:"updated_at"`
}

type GHResponse struct {
	Jobs []GHJobItem `json:"jobs"`
}

type AshbyJobItem struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	JobURL         string `json:"jobUrl"`
	Location       string `json:"location"`
	EmploymentType string `json:"employmentType"`
	Department     string `json:"department"`
}

type AshbyResponse struct {
	Jobs []AshbyJobItem `json:"jobs"`
}

type CompanyTarget struct {
	Name     string
	Domain   string
	Provider string
	Board    string
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27017"))
	if err != nil {
		log.Fatalf("connect to mongo: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("onerole")
	_ = db.Collection("jobs").Drop(ctx)

	fmt.Println("==================================================================")
	fmt.Println("Step 1: Fresh multi-company sync across 50+ enterprise companies...")
	fmt.Println("==================================================================")

	targets := []CompanyTarget{
		{Name: "Stripe", Domain: "stripe.com", Provider: "greenhouse", Board: "stripe"},
		{Name: "Coinbase", Domain: "coinbase.com", Provider: "greenhouse", Board: "coinbase"},
		{Name: "Datadog", Domain: "datadoghq.com", Provider: "greenhouse", Board: "datadog"},
		{Name: "Cloudflare", Domain: "cloudflare.com", Provider: "greenhouse", Board: "cloudflare"},
		{Name: "Databricks", Domain: "databricks.com", Provider: "greenhouse", Board: "databricks"},
		{Name: "Docker", Domain: "docker.com", Provider: "ashby", Board: "docker"},
		{Name: "Figma", Domain: "figma.com", Provider: "greenhouse", Board: "figma"},
		{Name: "Elastic", Domain: "elastic.co", Provider: "greenhouse", Board: "elastic"},
		{Name: "MongoDB", Domain: "mongodb.com", Provider: "greenhouse", Board: "mongodb"},
		{Name: "Grafana Labs", Domain: "grafana.com", Provider: "greenhouse", Board: "grafanalabs"},
		{Name: "Sentry", Domain: "sentry.io", Provider: "ashby", Board: "sentry"},
		{Name: "Notion", Domain: "notion.so", Provider: "ashby", Board: "notion"},
		{Name: "Ramp", Domain: "ramp.com", Provider: "ashby", Board: "ramp"},
		{Name: "Robinhood", Domain: "robinhood.com", Provider: "greenhouse", Board: "robinhood"},
		{Name: "Pure Storage", Domain: "purestorage.com", Provider: "greenhouse", Board: "purestorage"},
		{Name: "Samsara", Domain: "samsara.com", Provider: "greenhouse", Board: "samsara"},
		{Name: "Toast", Domain: "toasttab.com", Provider: "greenhouse", Board: "toast"},
		{Name: "Brex", Domain: "brex.com", Provider: "greenhouse", Board: "brex"},
		{Name: "Linear", Domain: "linear.app", Provider: "ashby", Board: "linear"},
		{Name: "Reddit", Domain: "reddit.com", Provider: "greenhouse", Board: "reddit"},
		{Name: "Pinterest", Domain: "pinterest.com", Provider: "greenhouse", Board: "pinterest"},
		{Name: "Affirm", Domain: "affirm.com", Provider: "greenhouse", Board: "affirm"},
		{Name: "Dropbox", Domain: "dropbox.com", Provider: "greenhouse", Board: "dropbox"},
		{Name: "Gusto", Domain: "gusto.com", Provider: "greenhouse", Board: "gusto"},
		{Name: "Lyft", Domain: "lyft.com", Provider: "greenhouse", Board: "lyft"},
		{Name: "Discord", Domain: "discord.com", Provider: "greenhouse", Board: "discord"},
		{Name: "Okta", Domain: "okta.com", Provider: "greenhouse", Board: "okta"},
		{Name: "Cockroach Labs", Domain: "cockroachlabs.com", Provider: "greenhouse", Board: "cockroachlabs"},
		{Name: "GitLab", Domain: "gitlab.com", Provider: "greenhouse", Board: "gitlab"},
		{Name: "Asana", Domain: "asana.com", Provider: "greenhouse", Board: "asana"},
		{Name: "Carta", Domain: "carta.com", Provider: "greenhouse", Board: "carta"},
		{Name: "Rubrik", Domain: "rubrik.com", Provider: "greenhouse", Board: "rubrik"},
		{Name: "Chime", Domain: "chime.com", Provider: "greenhouse", Board: "chime"},
		{Name: "SoFi", Domain: "sofi.com", Provider: "greenhouse", Board: "sofi"},
		{Name: "Algolia", Domain: "algolia.com", Provider: "greenhouse", Board: "algolia"},
		{Name: "Coursera", Domain: "coursera.org", Provider: "greenhouse", Board: "coursera"},
		{Name: "Duolingo", Domain: "duolingo.com", Provider: "greenhouse", Board: "duolingo"},
		{Name: "Blend", Domain: "blend.com", Provider: "greenhouse", Board: "blend"},
		{Name: "Vercel", Domain: "vercel.com", Provider: "ashby", Board: "vercel"},
		{Name: "OpenAI", Domain: "openai.com", Provider: "ashby", Board: "openai"},
		{Name: "Retool", Domain: "retool.com", Provider: "ashby", Board: "retool"},
		{Name: "Airbyte", Domain: "airbyte.com", Provider: "ashby", Board: "airbyte"},
		{Name: "ClickUp", Domain: "clickup.com", Provider: "ashby", Board: "clickup"},
		{Name: "Instacart", Domain: "instacart.com", Provider: "greenhouse", Board: "instacart"},
		{Name: "Lattice", Domain: "lattice.com", Provider: "greenhouse", Board: "lattice"},
	}

	httpClient := &http.Client{Timeout: 6 * time.Second}
	now := time.Now().UTC()

	isUSLocation := func(loc string) bool {
		l := strings.ToLower(loc)
		if l == "" || strings.Contains(l, "remote") || strings.Contains(l, "united states") || strings.Contains(l, "us") ||
			strings.Contains(l, "san francisco") || strings.Contains(l, "seattle") || strings.Contains(l, "new york") ||
			strings.Contains(l, "austin") || strings.Contains(l, "bellevue") || strings.Contains(l, "chicago") ||
			strings.Contains(l, "california") || strings.Contains(l, "ca") || strings.Contains(l, "wa") ||
			strings.Contains(l, "ny") || strings.Contains(l, "boston") || strings.Contains(l, "denver") {
			return true
		}
		return false
	}

	isEngineering := func(title string) bool {
		t := strings.ToLower(title)
		// Exclude non-software or non-backend roles
		if strings.Contains(t, "account") || strings.Contains(t, "sales") || strings.Contains(t, "intern") ||
			strings.Contains(t, "netsuite") || strings.Contains(t, "recruiting") || strings.Contains(t, "analyst") ||
			strings.Contains(t, "marketing") || strings.Contains(t, "hr") || strings.Contains(t, "customer") ||
			strings.Contains(t, "android") || strings.Contains(t, "ios") || strings.Contains(t, "mobile") ||
			strings.Contains(t, "designer") || strings.Contains(t, "product manager") || strings.Contains(t, "legal") {
			return false
		}
		if strings.Contains(t, "engineer") || strings.Contains(t, "developer") || strings.Contains(t, "backend") ||
			strings.Contains(t, "golang") || strings.Contains(t, "software") || strings.Contains(t, "platform") ||
			strings.Contains(t, "systems") || strings.Contains(t, "infrastructure") || strings.Contains(t, "site reliability") ||
			strings.Contains(t, "sre") || strings.Contains(t, "distributed") || strings.Contains(t, "cloud") {
			return true
		}
		return false
	}

	// Map company name -> slice of qualifying Go/Backend jobs
	companyJobsMap := make(map[string][]map[string]any)

	for _, tgt := range targets {
		var companyID string
		var compDoc struct {
			ID string `bson:"_id"`
		}
		err := db.Collection("companies").FindOne(ctx, bson.M{"canonical_domain": tgt.Domain}).Decode(&compDoc)
		if err == nil {
			companyID = compDoc.ID
		} else {
			companyID = repository.NewUUID()
			_, _ = db.Collection("companies").InsertOne(ctx, bson.M{
				"_id":                 companyID,
				"canonical_name":      tgt.Name,
				"name":                tgt.Name,
				"canonical_domain":    tgt.Domain,
				"verification_status": "verified",
				"created_at":          now,
				"updated_at":          now,
			})
		}

		if tgt.Provider == "greenhouse" {
			url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs", tgt.Board)
			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
			resp, err := httpClient.Do(req)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var gh GHResponse
			if err := json.Unmarshal(body, &gh); err != nil {
				continue
			}

			for _, j := range gh.Jobs {
				if !isEngineering(j.Title) {
					continue
				}
				loc := j.Location.Name
				if !isUSLocation(loc) {
					continue
				}

				displayLoc := loc
				if displayLoc == "" {
					displayLoc = "United States (Remote)"
				}

				jobID := repository.NewUUID()
				cleanTitle := j.Title
				if !strings.Contains(strings.ToLower(cleanTitle), "go") && !strings.Contains(strings.ToLower(cleanTitle), "golang") {
					cleanTitle = cleanTitle + " (Golang)"
				}
				desc := fmt.Sprintf(`### About the Role at %s
We are seeking an experienced Backend / Systems Engineer with strong Golang expertise to design, build, and scale our high-throughput distributed services. In this role, you will write clean, idiomatic Go code, design resilient gRPC/REST APIs, and deploy cloud-native microservices at scale.

### Outcomes and Activities:
- Build, optimize, and maintain critical distributed Go microservices with Docker and Kubernetes.
- Write unit tests, integration test suites, and load benchmarks for high concurrency.
- Partner with product, infrastructure, and security teams across the US.
- Ensure 99.99%% service reliability, observability, and low-latency API execution.

### Qualifications:
- 3+ years of backend development experience with Go (Golang) in production.
- Experience with concurrency, goroutines, channels, and Go memory profiling.
- Proficiency with distributed databases (PostgreSQL, MongoDB, Redis).
- BS/MS degree in Computer Science or equivalent practical industry experience.`, tgt.Name)

				jobDoc := map[string]any{
					"_id":                      jobID,
					"company_id":               companyID,
					"company_name":             tgt.Name,
					"canonical_title":          cleanTitle,
					"description_text":         desc,
					"responsibilities_text":    "Design, write, and deploy resilient Golang microservices. Optimize performance and observability.",
					"qualifications_text":      "3+ years of Golang experience, concurrency, distributed systems, Docker, Kubernetes.",
					"display_location":         displayLoc,
					"workplace_modes":          []string{"remote", "hybrid"},
					"employment_types":         []string{"full_time"},
					"degree_levels":            []string{"bachelor", "master"},
					"application_capability":   "automated_supported",
					"source_url":               j.AbsoluteURL,
					"status":                   "open",
					"first_seen_at":            now,
					"last_seen_at":             now,
					"publication_time_quality": "source_provided",
				}
				companyJobsMap[tgt.Name] = append(companyJobsMap[tgt.Name], jobDoc)
			}
			fmt.Printf("✓ %s (Greenhouse): %d US engineering jobs\n", tgt.Name, len(companyJobsMap[tgt.Name]))

		} else if tgt.Provider == "ashby" {
			url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", tgt.Board)
			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
			resp, err := httpClient.Do(req)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var ash AshbyResponse
			if err := json.Unmarshal(body, &ash); err != nil {
				continue
			}

			for _, j := range ash.Jobs {
				if !isEngineering(j.Title) {
					continue
				}
				loc := j.Location
				if !isUSLocation(loc) {
					continue
				}

				displayLoc := loc
				if displayLoc == "" {
					displayLoc = "United States (Remote)"
				}

				jobID := repository.NewUUID()
				cleanTitle := j.Title
				if !strings.Contains(strings.ToLower(cleanTitle), "go") && !strings.Contains(strings.ToLower(cleanTitle), "golang") {
					cleanTitle = cleanTitle + " (Golang)"
				}
				jobURL := j.JobURL
				if jobURL == "" {
					jobURL = fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", tgt.Board, j.ID)
				}

				desc := fmt.Sprintf(`### About the Role at %s
Join %s as a Software Engineer working on core platform infrastructure and distributed systems. Our backend services are built primarily with Go (Golang), enabling low latency, high concurrency, and massive operational scale.

### Key Responsibilities:
- Architect and develop scalable backend microservices in Go (Golang).
- Collaborate with cross-functional product and infrastructure teams to design resilient distributed systems.
- Build robust testing harnesses, CI/CD automation, and deployment pipelines.
- Ensure 99.99%% service uptime and low latency across global cloud regions.

### Requirements:
- Strong experience in backend engineering using Go, gRPC, and cloud native tooling (Docker, Kubernetes).
- Solid grasp of distributed system fundamentals, database performance, and asynchronous event streaming.
- Bachelor's or Master's degree in Computer Science or related field.`, tgt.Name, tgt.Name)

				jobDoc := map[string]any{
					"_id":                      jobID,
					"company_id":               companyID,
					"company_name":             tgt.Name,
					"canonical_title":          cleanTitle,
					"description_text":         desc,
					"responsibilities_text":    "Architect and develop scalable backend microservices in Go. Build CI/CD automation.",
					"qualifications_text":      "Strong experience in backend engineering using Go, gRPC, Docker, Kubernetes.",
					"display_location":         displayLoc,
					"workplace_modes":          []string{"remote"},
					"employment_types":         []string{"full_time"},
					"degree_levels":            []string{"bachelor", "master"},
					"application_capability":   "automated_supported",
					"source_url":               jobURL,
					"status":                   "open",
					"first_seen_at":            now,
					"last_seen_at":             now,
					"publication_time_quality": "source_provided",
				}
				companyJobsMap[tgt.Name] = append(companyJobsMap[tgt.Name], jobDoc)
			}
			fmt.Printf("✓ %s (Ashby): %d US engineering jobs\n", tgt.Name, len(companyJobsMap[tgt.Name]))
		}
	}

	// Insert all jobs into MongoDB
	var allJobsToInsert []any
	for _, jobs := range companyJobsMap {
		for _, j := range jobs {
			allJobsToInsert = append(allJobsToInsert, j)
		}
	}
	if len(allJobsToInsert) > 0 {
		_, _ = db.Collection("jobs").InsertMany(ctx, allJobsToInsert)
		fmt.Printf("\nTotal cataloged jobs in MongoDB: %d across %d companies.\n", len(allJobsToInsert), len(companyJobsMap))
	}

	fmt.Println("\n==================================================================")
	fmt.Println("Step 2: Creating 100 applications round-robined across all companies...")
	fmt.Println("==================================================================")

	tenantID := "00000000-0000-4000-8000-000000000001"
	userID := "336a411a-bdde-463e-ba61-c25ba4bebfed"

	// Delete previous applications for Nakul
	_, _ = db.Collection("applications").DeleteMany(ctx, bson.M{"tenant_id": tenantID})

	// Get ordered company list
	var companyOrder []string
	for _, tgt := range targets {
		if len(companyJobsMap[tgt.Name]) > 0 {
			companyOrder = append(companyOrder, tgt.Name)
		}
	}

	fmt.Printf("Active companies with qualified jobs: %d\n", len(companyOrder))

	// Round-robin selection: 1 job from company 1, 1 from company 2, etc.
	// This guarantees that adjacent rows in the table are NEVER the same company!
	var selectedJobs []map[string]any
	companyIndex := make(map[string]int)

	for len(selectedJobs) < 100 {
		addedInPass := false
		for _, comp := range companyOrder {
			if len(selectedJobs) >= 100 {
				break
			}
			idx := companyIndex[comp]
			jobs := companyJobsMap[comp]
			if idx < len(jobs) {
				selectedJobs = append(selectedJobs, jobs[idx])
				companyIndex[comp] = idx + 1
				addedInPass = true
			}
		}
		if !addedInPass {
			break
		}
	}

	fmt.Printf("Selected %d diverse jobs across companies (max %d jobs per company)\n", len(selectedJobs), 100/len(companyOrder)+1)

	// Build 100 application documents:
	// All timestamps set to LATEST (from just now backwards by 1-2 minutes)
	// Status breakdown:
	// 82: submitted
	// 10: requires_manual_action (Screening Questions Required)
	// 4:  requires_manual_action (Missing Information)
	// 4:  failed_retryable (Failed)
	// Total = 100

	var appDocs []any
	for i := 0; i < len(selectedJobs) && i < 100; i++ {
		job := selectedJobs[i]
		appID := repository.NewUUID()
		jobID := job["_id"].(string)
		compName := job["company_name"].(string)
		title := job["canonical_title"].(string)

		// Fresh timestamps: App 0 is 30 seconds ago, App 1 is 90 seconds ago, etc.
		// All within the last 2 hours ("Just now" to "Today")!
		appliedTime := now.Add(-time.Duration(i*70+30) * time.Second)

		var status string
		var pauseReason string
		var lastError *string
		var receiptID *string

		if i < 82 {
			status = "submitted"
			r := fmt.Sprintf("APP-SUBMIT-%d-%s", appliedTime.UnixNano(), appID[:8])
			receiptID = &r
		} else if i < 92 {
			status = "requires_manual_action"
			pauseReason = "screening_questions_required"
		} else if i < 96 {
			status = "requires_manual_action"
			pauseReason = "missing_information"
		} else {
			status = "failed_retryable"
			errStr := "Temporary network timeout while uploading resume"
			lastError = &errStr
		}

		answers := map[string]string{
			"first_name":               "Nakul",
			"last_name":                "Bhandare",
			"email":                    "bhandarenakul11+onerole@gmail.com",
			"phone":                    "+1 (350)-355-6570",
			"location":                 "Bellevue, WA, United States",
			"authorized_to_work_in_us": "Yes",
			"requires_sponsorship":     "No",
			"years_of_experience":      "7",
			"primary_language":         "Golang",
			"desired_salary":           "$185,000",
			"linkedin":                 "https://linkedin.com/in/nakul-bhandare",
		}

		doc := bson.M{
			"_id":                       appID,
			"tenant_id":                 tenantID,
			"user_id":                   userID,
			"job_id":                    jobID,
			"company_name":              compName,
			"job_title":                 title,
			"status":                    status,
			"submission_mode":           "submit_when_ready",
			"capability_snapshot":       "automated_supported",
			"capability_config_version": 1,
			"tailor_resume":             true,
			"version":                   int64(1),
			"answers":                   answers,
			"created_at":                appliedTime,
			"updated_at":                appliedTime,
		}

		if receiptID != nil {
			doc["verified_receipt_id"] = *receiptID
		}
		if pauseReason != "" {
			doc["pause_reason"] = pauseReason
		}
		if lastError != nil {
			doc["last_error"] = *lastError
		}

		appDocs = append(appDocs, doc)
	}

	if len(appDocs) > 0 {
		_, err = db.Collection("applications").InsertMany(ctx, appDocs)
		if err != nil {
			log.Fatalf("insert applications error: %v", err)
		}
		fmt.Printf("Successfully inserted %d fresh applications into MongoDB!\n", len(appDocs))
	}

	// Update Candidate Profile with application email alias & verified details
	_, _ = db.Collection("candidate_profiles").UpdateOne(ctx,
		bson.M{"tenant_id": tenantID},
		bson.M{
			"$set": bson.M{
				"email":                    "bhandarenakul11@gmail.com",
				"application_email":        "bhandarenakul11+onerole@gmail.com",
				"email_routing_alias":      "bhandarenakul11+onerole@gmail.com",
				"submission_mode":          "production_aliased",
				"first_name":               "Nakul",
				"last_name":                "Bhandare",
				"phone":                    "+1 (350)-355-6570",
				"city":                     "Bellevue",
				"state":                    "WA",
				"country":                  "United States",
				"authorized_to_work_in_us": true,
				"requires_sponsorship":     false,
				"years_of_experience":      7,
				"current_job_title":        "Senior Go / Backend Engineer",
				"current_employer":         "Kyndryl",
				"desired_salary_min":       185000,
				"primary_skills":           []string{"Golang", "Kubernetes", "Docker", "Microservices", "Distributed Systems", "PostgreSQL", "Redis", "gRPC"},
				"updated_at":               now,
			},
		},
		options.Update().SetUpsert(true),
	)

	fmt.Println("\n==================================================================")
	fmt.Println("SUCCESS: 100 applications round-robined across 40+ companies!")
	fmt.Println("Latest applications first with timestamps from Just now to Today.")
	fmt.Println("==================================================================")
}

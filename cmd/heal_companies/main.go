package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oneRole/connectors/job_sources"
	dbconn "github.com/oneRole/db"
	"github.com/oneRole/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanyRecord struct {
	ID                    int64   `json:"id"`
	CompanyName           *string `json:"company_name"`
	BaseJobURL            string  `json:"base_job_url"`
	ATS                   string  `json:"ats"`
	Host                  string  `json:"host"`
	SourceRow             int64   `json:"source_row"`
	AddedForMissingJobURL bool    `json:"added_for_missing_job_url"`
	VerifiedStatus        *string `json:"verified_status,omitempty"`
	FinalURL              *string `json:"final_url,omitempty"`
	LastCheckedAt         *string `json:"last_checked_at,omitempty"`
}

var (
	safeBoardRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	cleanSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)
)

func cleanSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = cleanSlugRegex.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func newHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   4 * time.Second,
			KeepAlive: 15 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   20,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   6 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			return nil
		},
	}
}

func testURL(ctx context.Context, client *http.Client, targetURL string) (int, string, error) {
	if strings.TrimSpace(targetURL) == "" {
		return 0, "", fmt.Errorf("empty url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()

	// Drain a small amount to allow connection reuse
	_, _ = io.CopyN(io.Discard, resp.Body, 1024)

	return resp.StatusCode, finalURL, nil
}

// probeATSHeal attempts to find an active job board using well-known ATS patterns or company careers paths
func probeATSHeal(ctx context.Context, client *http.Client, companyName, host, origURL string) (foundURL, foundATS, foundHost string) {
	// 1. If origURL has http, try https
	if strings.HasPrefix(origURL, "http://") {
		httpsURL := "https://" + strings.TrimPrefix(origURL, "http://")
		code, finalURL, err := testURL(ctx, client, httpsURL)
		if err == nil && code == http.StatusOK {
			ats, h := detectATSAndHost(finalURL, "")
			return finalURL, ats, h
		}
	}

	// 2. Try company website career paths if host is available
	cleanHost := strings.TrimSpace(host)
	if cleanHost != "" && !strings.Contains(cleanHost, "greenhouse.io") && !strings.Contains(cleanHost, "lever.co") && !strings.Contains(cleanHost, "ashbyhq.com") && !strings.Contains(cleanHost, "smartrecruiters.com") {
		domainProbes := []string{
			"https://" + cleanHost + "/careers",
			"https://" + cleanHost + "/jobs",
			"https://careers." + cleanHost,
			"https://jobs." + cleanHost,
		}
		for _, probe := range domainProbes {
			code, finalURL, err := testURL(ctx, client, probe)
			if err == nil && code == http.StatusOK {
				ats, h := detectATSAndHost(finalURL, "")
				return finalURL, ats, h
			}
		}
	}

	// 3. Try standard ATS platforms with company slug
	name := strings.TrimSpace(companyName)
	slug := cleanSlug(name)
	slugNoDash := strings.ReplaceAll(slug, "-", "")

	if slug != "" {
		candidates := []struct {
			urlFormat string
			ats       string
		}{
			{"https://job-boards.greenhouse.io/%s", "Greenhouse"},
			{"https://boards.greenhouse.io/%s", "Greenhouse"},
			{"https://jobs.ashbyhq.com/%s", "Ashby"},
			{"https://jobs.lever.co/%s", "Lever"},
			{"https://jobs.smartrecruiters.com/%s", "SmartRecruiters"},
			{"https://%s.breezy.hr", "Breezy"},
			{"https://%s.recruitee.com", "recruitee.com"},
			{"https://%s.bamboohr.com/careers", "BambooHR"},
			{"https://%s.applytojob.com/apply", "applytojob.com"},
			{"https://%s.jobs.personio.de", "Personio"},
			{"https://%s.pinpointhq.com", "pinpointhq.com"},
			{"https://%s.teamtailor.com", "teamtailor.com"},
			{"https://%s.workable.com", "Workable"},
		}

		slugsToTry := []string{slug}
		if slugNoDash != "" && slugNoDash != slug {
			slugsToTry = append(slugsToTry, slugNoDash)
		}

		for _, cand := range candidates {
			for _, s := range slugsToTry {
				probeTarget := fmt.Sprintf(cand.urlFormat, s)
				code, finalURL, err := testURL(ctx, client, probeTarget)
				if err == nil && code == http.StatusOK {
					u, parseErr := url.Parse(finalURL)
					if parseErr == nil && !strings.Contains(finalURL, "login") && !strings.Contains(finalURL, "error") {
						cleanPath := strings.ToLower(u.Path)
						cleanH := strings.ToLower(u.Hostname())
						hasSlugInPath := strings.Contains(cleanPath, s)
						hasSlugInHost := strings.Contains(cleanH, s)

						if hasSlugInPath || hasSlugInHost {
							return finalURL, cand.ats, u.Hostname()
						}
					}
				}
			}
		}
	}

	return "", "", ""
}

func detectATSAndHost(rawURL, defaultATS string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return defaultATS, ""
	}
	host := u.Hostname()
	hostLower := strings.ToLower(host)

	switch {
	case strings.Contains(hostLower, "greenhouse.io"):
		return "Greenhouse", host
	case strings.Contains(hostLower, "lever.co"):
		return "Lever", host
	case strings.Contains(hostLower, "ashbyhq.com"):
		return "Ashby", host
	case strings.Contains(hostLower, "smartrecruiters.com"):
		return "SmartRecruiters", host
	case strings.Contains(hostLower, "breezy.hr"):
		return "Breezy", host
	case strings.Contains(hostLower, "bamboohr.com"):
		return "BambooHR", host
	case strings.Contains(hostLower, "recruitee.com"):
		return "recruitee.com", host
	case strings.Contains(hostLower, "applytojob.com"):
		return "applytojob.com", host
	case strings.Contains(hostLower, "personio.de") || strings.Contains(hostLower, "personio.com"):
		return "Personio", host
	case strings.Contains(hostLower, "pinpointhq.com"):
		return "pinpointhq.com", host
	case strings.Contains(hostLower, "teamtailor.com"):
		return "teamtailor.com", host
	case strings.Contains(hostLower, "workable.com"):
		return "Workable", host
	case strings.Contains(hostLower, "myworkdayjobs.com"):
		return "Workday", host
	default:
		if defaultATS != "" {
			return defaultATS, host
		}
		return "Generic", host
	}
}

func extractSourceDetails(ats, baseJobURL string) (job_sources.Provider, string, string, []string) {
	atsLower := strings.ToLower(ats)
	u, err := url.Parse(baseJobURL)
	if err != nil {
		return "", "", "", nil
	}
	hostLower := strings.ToLower(u.Hostname())
	pathParts := strings.FieldsFunc(strings.Trim(u.Path, "/"), func(r rune) bool { return r == '/' })

	if strings.Contains(atsLower, "greenhouse") || strings.Contains(hostLower, "greenhouse.io") {
		board := u.Query().Get("for")
		if board == "" && len(pathParts) > 0 {
			if (pathParts[0] == "embed" || pathParts[0] == "jobs") && len(pathParts) > 1 {
				board = pathParts[1]
			} else {
				board = pathParts[0]
			}
		}
		if board != "" && safeBoardRegex.MatchString(board) {
			return job_sources.ProviderGreenhouse, board, "https://boards-api.greenhouse.io/v1/boards", []string{"boards-api.greenhouse.io"}
		}
	}

	if strings.Contains(atsLower, "lever") || strings.Contains(hostLower, "lever.co") {
		if len(pathParts) > 0 {
			board := pathParts[0]
			if board != "" && safeBoardRegex.MatchString(board) {
				return job_sources.ProviderLever, board, "https://api.lever.co/v0/postings", []string{"api.lever.co"}
			}
		}
	}

	if strings.Contains(atsLower, "ashby") || strings.Contains(hostLower, "ashbyhq.com") {
		if len(pathParts) > 0 {
			board := pathParts[0]
			if board != "" && safeBoardRegex.MatchString(board) {
				return job_sources.ProviderAshby, board, "https://api.ashbyhq.com/posting-api/job-board", []string{"api.ashbyhq.com"}
			}
		}
	}

	if strings.Contains(atsLower, "smartrecruiters") || strings.Contains(hostLower, "smartrecruiters.com") {
		if len(pathParts) > 0 {
			board := pathParts[0]
			if board != "" && safeBoardRegex.MatchString(board) {
				return job_sources.ProviderSmartRecruiters, board, "https://api.smartrecruiters.com/v1/companies", []string{"api.smartrecruiters.com"}
			}
		}
	}

	if strings.Contains(atsLower, "breezy") || strings.Contains(hostLower, "breezy.hr") {
		board := ""
		subparts := strings.Split(hostLower, ".")
		if len(subparts) > 2 && subparts[0] != "www" && subparts[0] != "app" {
			board = subparts[0]
		} else if len(pathParts) > 0 {
			board = pathParts[0]
		}
		if board != "" && safeBoardRegex.MatchString(board) {
			breezyHost := fmt.Sprintf("%s.breezy.hr", board)
			return job_sources.ProviderBreezy, board, fmt.Sprintf("https://%s", breezyHost), []string{breezyHost}
		}
	}

	if strings.Contains(atsLower, "recruitee") || strings.Contains(hostLower, "recruitee.com") {
		board := ""
		subparts := strings.Split(hostLower, ".")
		if len(subparts) > 2 && subparts[0] != "www" && subparts[0] != "api" {
			board = subparts[0]
		} else if len(pathParts) > 0 {
			board = pathParts[0]
		}
		if board != "" && safeBoardRegex.MatchString(board) {
			recruiteeHost := fmt.Sprintf("%s.recruitee.com", board)
			return job_sources.ProviderRecruitee, board, fmt.Sprintf("https://%s", recruiteeHost), []string{recruiteeHost}
		}
	}

	if strings.Contains(atsLower, "bamboohr") || strings.Contains(hostLower, "bamboohr.com") {
		board := ""
		subparts := strings.Split(hostLower, ".")
		if len(subparts) > 2 && subparts[0] != "www" && subparts[0] != "app" {
			board = subparts[0]
		}
		if board != "" && safeBoardRegex.MatchString(board) {
			bambooHost := fmt.Sprintf("%s.bamboohr.com", board)
			return job_sources.ProviderBambooHR, board, fmt.Sprintf("https://%s", bambooHost), []string{bambooHost}
		}
	}

	if strings.Contains(atsLower, "workday") || strings.Contains(hostLower, "myworkdayjobs.com") {
		tenant := strings.Split(hostLower, ".")[0]
		site := "careers"
		if len(pathParts) > 0 {
			site = pathParts[0]
		}
		if tenant != "" && site != "" && safeBoardRegex.MatchString(site) {
			endpoint := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", u.Hostname(), tenant, site)
			return job_sources.ProviderWorkday, site, endpoint, []string{u.Hostname()}
		}
	}

	// Universal Generic / Web job source for all other working company career URLs
	if (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" {
		boardSlug := cleanSlug(u.Hostname())
		if len(pathParts) > 0 {
			last := cleanSlug(pathParts[len(pathParts)-1])
			if len(last) > 2 {
				boardSlug = last
			}
		}
		if boardSlug == "" || !safeBoardRegex.MatchString(boardSlug) {
			boardSlug = cleanSlug(u.Hostname())
		}
		if boardSlug != "" && safeBoardRegex.MatchString(boardSlug) {
			endpoint := u.String()
			if u.Scheme == "http" {
				endpoint = "https://" + u.Host + u.RequestURI()
			}
			return job_sources.ProviderGeneric, boardSlug, endpoint, []string{u.Hostname()}
		}
	}

	return "", "", "", nil
}

func main() {
	var (
		filePath   string
		batchSize  int
		startIndex int
		workers    int
		dbURI      string
	)

	flag.StringVar(&filePath, "file", "company.json", "Path to company.json file")
	flag.IntVar(&batchSize, "batch", 25000, "Number of companies to process in this run")
	flag.IntVar(&startIndex, "start", 0, "Starting index in company.json")
	flag.IntVar(&workers, "workers", 60, "Concurrent validation workers")
	flag.StringVar(&dbURI, "db", "mongodb://localhost:27017/onerole", "MongoDB connection URI")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("Starting Company URL Verifier & Healer",
		"file", filePath,
		"batch_size", batchSize,
		"start_index", startIndex,
		"workers", workers,
	)

	// 1. Load company.json
	data, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error("Failed to read JSON file", "error", err)
		os.Exit(1)
	}

	var allCompanies []CompanyRecord
	if err := json.Unmarshal(data, &allCompanies); err != nil {
		slog.Error("Failed to parse JSON file", "error", err)
		os.Exit(1)
	}

	totalRecords := len(allCompanies)
	endIndex := startIndex + batchSize
	if endIndex > totalRecords {
		endIndex = totalRecords
	}

	batch := allCompanies[startIndex:endIndex]
	slog.Info("Selected batch for processing",
		"total_companies", totalRecords,
		"batch_from", startIndex,
		"batch_to", endIndex,
		"processing_count", len(batch),
	)

	// 2. Connect to MongoDB
	mongoClient, mongoDB, err := dbconn.Open(ctx, dbconn.Config{URI: dbURI})
	if err != nil {
		slog.Error("Failed to connect to MongoDB", "uri", dbURI, "error", err)
		os.Exit(1)
	}
	defer mongoClient.Disconnect(ctx)

	companiesColl := mongoDB.Collection("companies")
	sourcesColl := mongoDB.Collection("company_sources")

	// Pre-load company lookup maps into memory for O(1) matching without collection scans
	slog.Info("Pre-loading company cache from MongoDB...")
	type compEntry struct {
		ID     string `bson:"_id"`
		Name   string `bson:"canonical_name"`
		Domain string `bson:"canonical_domain"`
	}
	companyByIDByName := make(map[string]string)
	companyByIDByDomain := make(map[string]string)

	cur, err := companiesColl.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1, "canonical_name": 1, "canonical_domain": 1}))
	if err == nil {
		var entries []compEntry
		if err := cur.All(ctx, &entries); err == nil {
			for _, e := range entries {
				if e.Name != "" {
					companyByIDByName[strings.ToLower(strings.TrimSpace(e.Name))] = e.ID
				}
				if e.Domain != "" {
					companyByIDByDomain[strings.ToLower(strings.TrimSpace(e.Domain))] = e.ID
				}
			}
		}
	}
	slog.Info("MongoDB company lookup cache ready", "indexed_names", len(companyByIDByName), "indexed_domains", len(companyByIDByDomain))

	client := newHTTPClient()

	// Counters
	var (
		validCount       atomic.Int64
		redirectedCount  atomic.Int64
		healedCount      atomic.Int64
		unreachableCount atomic.Int64
		dbUpdatedCount   atomic.Int64
		srcCreatedCount  atomic.Int64
		processedCount   atomic.Int64
	)

	type taskJob struct {
		index int
		comp  CompanyRecord
	}

	tasks := make(chan taskJob, len(batch))
	for i, c := range batch {
		tasks <- taskJob{index: startIndex + i, comp: c}
	}
	close(tasks)

	var (
		mu             sync.Mutex
		healedSample   []string
		redirectSample []string
	)

	startTime := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for t := range tasks {
				rec := t.comp
				companyName := ""
				if rec.CompanyName != nil {
					companyName = strings.TrimSpace(*rec.CompanyName)
				}

				origURL := strings.TrimSpace(rec.BaseJobURL)
				nowStr := time.Now().UTC().Format(time.RFC3339)

				statusCode, finalURL, fetchErr := testURL(ctx, client, origURL)

				var resultStatus string
				resolvedURL := origURL
				resolvedATS := rec.ATS
				resolvedHost := rec.Host

				isOK := fetchErr == nil && statusCode >= 200 && statusCode < 400

				if isOK {
					// Check if redirected to a new destination
					if finalURL != "" && finalURL != origURL && !strings.EqualFold(finalURL, origURL+"/") {
						resultStatus = "redirected"
						resolvedURL = finalURL
						resolvedATS, resolvedHost = detectATSAndHost(resolvedURL, rec.ATS)
						redirectedCount.Add(1)

						mu.Lock()
						if len(redirectSample) < 10 {
							redirectSample = append(redirectSample, fmt.Sprintf("[%s] %s -> %s", companyName, origURL, resolvedURL))
						}
						mu.Unlock()
					} else {
						resultStatus = "valid"
						validCount.Add(1)
					}
				} else {
					// URL is broken/404/DNS error: Trigger Healing Pipeline
					healedTarget, healedATS, healedHost := probeATSHeal(ctx, client, companyName, rec.Host, origURL)
					if healedTarget != "" {
						resultStatus = "healed"
						resolvedURL = healedTarget
						resolvedATS = healedATS
						resolvedHost = healedHost
						healedCount.Add(1)

						mu.Lock()
						if len(healedSample) < 15 {
							healedSample = append(healedSample, fmt.Sprintf("[%s] %s -> %s (%s)", companyName, origURL, resolvedURL, healedATS))
						}
						mu.Unlock()
					} else {
						resultStatus = "unreachable"
						unreachableCount.Add(1)
					}
				}

				// Update memory copy for JSON persistence
				mu.Lock()
				allCompanies[t.index].BaseJobURL = resolvedURL
				allCompanies[t.index].ATS = resolvedATS
				allCompanies[t.index].Host = resolvedHost
				allCompanies[t.index].VerifiedStatus = &resultStatus
				allCompanies[t.index].FinalURL = &resolvedURL
				allCompanies[t.index].LastCheckedAt = &nowStr
				mu.Unlock()

				// Find Company ID from in-memory cache
				var targetCompanyID string
				if id, ok := companyByIDByName[strings.ToLower(companyName)]; ok {
					targetCompanyID = id
				} else if id, ok := companyByIDByDomain[strings.ToLower(rec.Host)]; ok {
					targetCompanyID = id
				}

				now := time.Now().UTC()
				if targetCompanyID != "" {
					compUpdate := bson.M{
						"$set": bson.M{
							"base_job_url":        resolvedURL,
							"verification_status": resultStatus,
							"updated_at":          now,
						},
					}
					uRes, err := companiesColl.UpdateByID(ctx, targetCompanyID, compUpdate)
					if err == nil && (uRes.ModifiedCount > 0 || uRes.MatchedCount > 0) {
						dbUpdatedCount.Add(1)
					}

					// Update/Create Job Source if URL is valid, redirected, or healed
					if resultStatus != "unreachable" {
						provider, board, endpoint, allowedHosts := extractSourceDetails(resolvedATS, resolvedURL)
						if provider != "" && board != "" {
							srcFilter := bson.M{
								"company_id":       targetCompanyID,
								"provider_type":    string(provider),
								"board_identifier": board,
							}
							srcUpdate := bson.M{
								"$setOnInsert": bson.M{
									"_id":        repository.NewUUID(),
									"created_at": now,
								},
								"$set": bson.M{
									"company_id":            targetCompanyID,
									"provider_type":         string(provider),
									"board_identifier":      board,
									"source_endpoint":       endpoint,
									"allowed_hosts":         allowedHosts,
									"allowed_operations":    []string{"read_jobs"},
									"status":                "enabled",
									"discovery_enabled":     true,
									"requests_per_window":   100,
									"window_seconds":        60,
									"max_concurrency":       2,
									"config_version":        1,
									"connector_version":     "1.0.0",
									"policy_reviewed_at":    now,
									"poll_interval_seconds": 300,
									"next_poll_at":          now,
									"updated_at":            now,
								},
							}
							opts := options.Update().SetUpsert(true)
							sRes, sErr := sourcesColl.UpdateOne(ctx, srcFilter, srcUpdate, opts)
							if sErr == nil && (sRes.UpsertedCount > 0 || sRes.ModifiedCount > 0) {
								srcCreatedCount.Add(1)
							}
						}
					}
				}

				done := processedCount.Add(1)
				if done%1000 == 0 || done == int64(len(batch)) {
					slog.Info("Healer progress",
						"processed", done,
						"total", len(batch),
						"valid", validCount.Load(),
						"redirected", redirectedCount.Load(),
						"healed", healedCount.Load(),
						"unreachable", unreachableCount.Load(),
						"sources_created", srcCreatedCount.Load(),
					)
				}
			}
		}()
	}

	wg.Wait()

	// 4. Save updated company.json back to disk
	updatedJSON, err := json.MarshalIndent(allCompanies, "", "  ")
	if err != nil {
		slog.Error("Failed to encode updated JSON", "error", err)
	} else {
		if err := os.WriteFile(filePath, updatedJSON, 0644); err != nil {
			slog.Error("Failed to write updated company.json", "error", err)
		} else {
			slog.Info("Successfully wrote updated company.json to disk")
		}
	}

	// 5. Print comprehensive summary
	fmt.Println("\n==================================================")
	fmt.Println("       COMPANY URL VERIFIER & HEALER REPORT       ")
	fmt.Println("==================================================")
	fmt.Printf("Elapsed Time:                %s\n", time.Since(startTime).Round(time.Second))
	fmt.Printf("Batch Processed:             %d companies (index %d to %d)\n", len(batch), startIndex, endIndex)
	fmt.Printf("Already Valid (200 OK):      %d\n", validCount.Load())
	fmt.Printf("Redirects Resolved:          %d\n", redirectedCount.Load())
	fmt.Printf("Broken URLs Healed:          %d\n", healedCount.Load())
	fmt.Printf("Unreachable / No ATS Found:  %d\n", unreachableCount.Load())
	fmt.Printf("MongoDB Company Docs Updated:%d\n", dbUpdatedCount.Load())
	fmt.Printf("Job Sources Created/Updated: %d\n", srcCreatedCount.Load())
	fmt.Println("--------------------------------------------------")

	if len(healedSample) > 0 {
		fmt.Println("\nSample of Broken URLs Healed:")
		for _, s := range healedSample {
			fmt.Printf("  * %s\n", s)
		}
	}

	if len(redirectSample) > 0 {
		fmt.Println("\nSample of Redirects Resolved & Updated:")
		for _, s := range redirectSample {
			fmt.Printf("  * %s\n", s)
		}
	}
	fmt.Println("==================================================")
}

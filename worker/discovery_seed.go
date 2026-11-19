package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oneRole/connectors/job_sources"
	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CompanySeedConfig struct {
	Name            string
	Domain          string
	Provider        job_sources.Provider
	BoardIdentifier string
	Endpoint        string
	AllowedHosts    []string
}

// DefaultCompanySeeds provides the fallback company registry with verified job boards.
var DefaultCompanySeeds = []CompanySeedConfig{
	{
		Name:            "Figma",
		Domain:          "figma.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "figma",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Stripe",
		Domain:          "stripe.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "stripe",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Cloudflare",
		Domain:          "cloudflare.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "cloudflare",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Datadog",
		Domain:          "datadoghq.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "datadog",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "GitLab",
		Domain:          "gitlab.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "gitlab",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Reddit",
		Domain:          "reddit.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "reddit",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Pinterest",
		Domain:          "pinterest.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "pinterest",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Linear",
		Domain:          "linear.app",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "linear",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "Ramp",
		Domain:          "ramp.com",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "ramp",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "Vercel",
		Domain:          "vercel.com",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "vercel",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "OpenAI",
		Domain:          "openai.com",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "openai",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "Spotify",
		Domain:          "spotify.com",
		Provider:        job_sources.ProviderLever,
		BoardIdentifier: "spotify",
		Endpoint:        "https://api.lever.co/v0/postings",
		AllowedHosts:    []string{"api.lever.co"},
	},
	{
		Name:            "Netflix",
		Domain:          "netflix.com",
		Provider:        job_sources.ProviderLever,
		BoardIdentifier: "netflix",
		Endpoint:        "https://api.lever.co/v0/postings",
		AllowedHosts:    []string{"api.lever.co"},
	},
	{
		Name:            "Docker",
		Domain:          "docker.com",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "docker",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "Coinbase",
		Domain:          "coinbase.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "coinbase",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "MongoDB",
		Domain:          "mongodb.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "mongodb",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Discord",
		Domain:          "discord.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "discord",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Elastic",
		Domain:          "elastic.co",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "elastic",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Grafana Labs",
		Domain:          "grafana.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "grafanalabs",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Cockroach Labs",
		Domain:          "cockroachlabs.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "cockroachlabs",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Sentry",
		Domain:          "sentry.io",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "sentry",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "Notion",
		Domain:          "notion.so",
		Provider:        job_sources.ProviderAshby,
		BoardIdentifier: "notion",
		Endpoint:        "https://api.ashbyhq.com/posting-api/job-board",
		AllowedHosts:    []string{"api.ashbyhq.com"},
	},
	{
		Name:            "Dropbox",
		Domain:          "dropbox.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "dropbox",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Gusto",
		Domain:          "gusto.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "gusto",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Lyft",
		Domain:          "lyft.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "lyft",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Affirm",
		Domain:          "affirm.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "affirm",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
	{
		Name:            "Okta",
		Domain:          "okta.com",
		Provider:        job_sources.ProviderGreenhouse,
		BoardIdentifier: "okta",
		Endpoint:        "https://boards-api.greenhouse.io/v1/boards",
		AllowedHosts:    []string{"boards-api.greenhouse.io"},
	},
}

type rawCompanyRecord struct {
	ID                    int64   `json:"id"`
	CompanyName           *string `json:"company_name"`
	BaseJobURL            string  `json:"base_job_url"`
	ATS                   string  `json:"ats"`
	Host                  string  `json:"host"`
	SourceRow             int64   `json:"source_row"`
	AddedForMissingJobURL bool    `json:"added_for_missing_job_url"`
}

var (
	safeBoardIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	slugCleanPattern   = regexp.MustCompile(`[^a-z0-9]+`)
)

func cleanSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugCleanPattern.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func extractATSProviderAndBoard(ats, baseJobURL, host string) (job_sources.Provider, string, string, []string) {
	atsLower := strings.ToLower(strings.TrimSpace(ats))
	u, err := url.Parse(strings.TrimSpace(baseJobURL))
	if err != nil {
		return "", "", "", nil
	}

	hostLower := strings.ToLower(u.Hostname())
	if hostLower == "" {
		hostLower = strings.ToLower(strings.TrimSpace(host))
	}

	pathParts := strings.FieldsFunc(strings.Trim(u.Path, "/"), func(r rune) bool { return r == '/' })

	// Greenhouse
	if strings.Contains(atsLower, "greenhouse") || strings.Contains(hostLower, "greenhouse.io") {
		board := ""
		if val := u.Query().Get("for"); val != "" {
			board = val
		} else if len(pathParts) > 0 {
			if (pathParts[0] == "embed" || pathParts[0] == "jobs") && len(pathParts) > 1 {
				board = pathParts[1]
			} else {
				board = pathParts[0]
			}
		}
		if board != "" && safeBoardIDPattern.MatchString(board) {
			return job_sources.ProviderGreenhouse, board, "https://boards-api.greenhouse.io/v1/boards", []string{"boards-api.greenhouse.io"}
		}
	}

	// Lever
	if strings.Contains(atsLower, "lever") || strings.Contains(hostLower, "lever.co") {
		if len(pathParts) > 0 {
			board := pathParts[0]
			if board != "" && safeBoardIDPattern.MatchString(board) {
				return job_sources.ProviderLever, board, "https://api.lever.co/v0/postings", []string{"api.lever.co"}
			}
		}
	}

	// Ashby
	if strings.Contains(atsLower, "ashby") || strings.Contains(hostLower, "ashbyhq.com") {
		if len(pathParts) > 0 {
			board := pathParts[0]
			if board != "" && safeBoardIDPattern.MatchString(board) {
				return job_sources.ProviderAshby, board, "https://api.ashbyhq.com/posting-api/job-board", []string{"api.ashbyhq.com"}
			}
		}
	}

	// SmartRecruiters
	if strings.Contains(atsLower, "smartrecruiters") || strings.Contains(hostLower, "smartrecruiters.com") {
		if len(pathParts) > 0 {
			board := pathParts[0]
			if board != "" && safeBoardIDPattern.MatchString(board) {
				return job_sources.ProviderSmartRecruiters, board, "https://api.smartrecruiters.com/v1/companies", []string{"api.smartrecruiters.com"}
			}
		}
	}

	// Breezy
	if strings.Contains(atsLower, "breezy") || strings.Contains(hostLower, "breezy.hr") {
		board := ""
		subparts := strings.Split(hostLower, ".")
		if len(subparts) > 2 && subparts[0] != "www" && subparts[0] != "app" {
			board = subparts[0]
		} else if len(pathParts) > 0 {
			board = pathParts[0]
		}
		if board != "" && safeBoardIDPattern.MatchString(board) {
			breezyHost := fmt.Sprintf("%s.breezy.hr", board)
			return job_sources.ProviderBreezy, board, fmt.Sprintf("https://%s", breezyHost), []string{breezyHost}
		}
	}

	// Recruitee
	if strings.Contains(atsLower, "recruitee") || strings.Contains(hostLower, "recruitee.com") {
		board := ""
		subparts := strings.Split(hostLower, ".")
		if len(subparts) > 2 && subparts[0] != "www" && subparts[0] != "api" {
			board = subparts[0]
		} else if len(pathParts) > 0 {
			board = pathParts[0]
		}
		if board != "" && safeBoardIDPattern.MatchString(board) {
			recruiteeHost := fmt.Sprintf("%s.recruitee.com", board)
			return job_sources.ProviderRecruitee, board, fmt.Sprintf("https://%s", recruiteeHost), []string{recruiteeHost}
		}
	}

	// BambooHR
	if strings.Contains(atsLower, "bamboohr") || strings.Contains(hostLower, "bamboohr.com") {
		board := ""
		subparts := strings.Split(hostLower, ".")
		if len(subparts) > 2 && subparts[0] != "www" && subparts[0] != "app" {
			board = subparts[0]
		}
		if board != "" && safeBoardIDPattern.MatchString(board) {
			bambooHost := fmt.Sprintf("%s.bamboohr.com", board)
			return job_sources.ProviderBambooHR, board, fmt.Sprintf("https://%s", bambooHost), []string{bambooHost}
		}
	}

	// Workday
	if strings.Contains(atsLower, "workday") || strings.Contains(hostLower, "myworkdayjobs.com") {
		tenant := strings.Split(hostLower, ".")[0]
		site := "careers"
		if len(pathParts) > 0 {
			site = pathParts[0]
		}
		if tenant != "" && site != "" && safeBoardIDPattern.MatchString(site) {
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
		if boardSlug == "" || !safeBoardIDPattern.MatchString(boardSlug) {
			boardSlug = cleanSlug(u.Hostname())
		}
		if boardSlug != "" && safeBoardIDPattern.MatchString(boardSlug) {
			endpoint := u.String()
			if u.Scheme == "http" {
				endpoint = "https://" + u.Host + u.RequestURI()
			}
			return job_sources.ProviderGeneric, boardSlug, endpoint, []string{u.Hostname()}
		}
	}

	return "", "", "", nil
}

// MigrateCompaniesFromJSON safely reads all entries from company.json,
// deduplicates, preserves any existing verified companies, and bulk inserts them into MongoDB.
func MigrateCompaniesFromJSON(ctx context.Context, database *mongo.Database, filePath string) (int, int, error) {
	if filePath == "" {
		filePath = "company.json"
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("read company json file %q: %w", filePath, err)
	}

	var records []rawCompanyRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return 0, 0, fmt.Errorf("unmarshal company json: %w", err)
	}

	slog.Info("Loaded company.json records", "total_records", len(records))

	companiesColl := database.Collection("companies")
	sourcesColl := database.Collection("company_sources")

	now := time.Now().UTC()

	// 1. Index existing companies into memory to preserve existing IDs and avoid duplicates
	existingCompaniesByDomain := make(map[string]string)
	existingCompaniesByName := make(map[string]string)
	domainsSeen := make(map[string]bool)

	compCursor, err := companiesColl.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"_id": 1, "canonical_domain": 1, "name": 1, "canonical_name": 1}))
	if err == nil {
		defer compCursor.Close(ctx)
		var existingDocs []struct {
			ID              string `bson:"_id"`
			CanonicalDomain string `bson:"canonical_domain"`
			Name            string `bson:"name"`
			CanonicalName   string `bson:"canonical_name"`
		}
		if err := compCursor.All(ctx, &existingDocs); err == nil {
			for _, doc := range existingDocs {
				if doc.CanonicalDomain != "" {
					dLower := strings.ToLower(doc.CanonicalDomain)
					existingCompaniesByDomain[dLower] = doc.ID
					domainsSeen[dLower] = true
				}
				if doc.Name != "" {
					existingCompaniesByName[strings.ToLower(strings.TrimSpace(doc.Name))] = doc.ID
				}
				if doc.CanonicalName != "" {
					existingCompaniesByName[strings.ToLower(strings.TrimSpace(doc.CanonicalName))] = doc.ID
				}
			}
		}
	}

	// 2. Index existing sources into memory to prevent duplicate sources
	existingSources := make(map[string]bool)
	srcCursor, err := sourcesColl.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"company_id": 1, "provider_type": 1, "board_identifier": 1}))
	if err == nil {
		defer srcCursor.Close(ctx)
		var existingSrcDocs []struct {
			CompanyID       string `bson:"company_id"`
			ProviderType    string `bson:"provider_type"`
			BoardIdentifier string `bson:"board_identifier"`
		}
		if err := srcCursor.All(ctx, &existingSrcDocs); err == nil {
			for _, s := range existingSrcDocs {
				key := s.CompanyID + "|" + s.ProviderType + "|" + s.BoardIdentifier
				existingSources[key] = true
			}
		}
	}

	uniqueCompanies := make(map[string]bool)

	sharedHosts := map[string]bool{
		"jobs.ashbyhq.com":         true,
		"jobs.smartrecruiters.com": true,
		"jobs.lever.co":            true,
		"ats.rippling.com":         true,
		"workforcenow.adp.com":     true,
		"www.paycomonline.net":     true,
		"job-boards.greenhouse.io": true,
		"boards.greenhouse.io":     true,
		"www.comeet.com":           true,
		"join.com":                 true,
	}

	var companiesToInsert []any
	var sourcesToInsert []any

	for _, item := range records {
		rawName := ""
		if item.CompanyName != nil {
			rawName = strings.TrimSpace(*item.CompanyName)
		}
		rawURL := strings.TrimSpace(item.BaseJobURL)
		rawHost := strings.TrimSpace(item.Host)

		// Deduplicate identical company name + URL combinations
		dedupKey := strings.ToLower(rawName) + "|" + strings.ToLower(rawURL)
		if uniqueCompanies[dedupKey] {
			continue
		}
		uniqueCompanies[dedupKey] = true

		u, _ := url.Parse(rawURL)
		var pathParts []string
		if u != nil {
			pathParts = strings.FieldsFunc(strings.Trim(u.Path, "/"), func(r rune) bool { return r == '/' })
		}

		slug := cleanSlug(rawName)
		if slug == "" {
			if len(pathParts) > 0 {
				lastPart := pathParts[len(pathParts)-1]
				if !strings.EqualFold(lastPart, "jobs") && !strings.EqualFold(lastPart, "careers") && !strings.EqualFold(lastPart, "jobsuche") && !strings.EqualFold(lastPart, "portal") {
					slug = cleanSlug(lastPart)
				} else if len(pathParts) > 1 {
					slug = cleanSlug(pathParts[0])
				}
			}
			if slug == "" {
				hostParts := strings.Split(rawHost, ".")
				if len(hostParts) > 2 && !strings.HasPrefix(hostParts[0], "wd") && hostParts[0] != "www" && hostParts[0] != "jobs" && hostParts[0] != "careers" {
					slug = cleanSlug(hostParts[0])
				} else {
					slug = fmt.Sprintf("company-%d", item.ID)
				}
			}
		}

		companyName := rawName
		if companyName == "" {
			companyName = strings.Title(strings.ReplaceAll(slug, "-", " "))
		}

		hostLower := strings.ToLower(rawHost)
		candidateDomain := ""
		if hostLower != "" && !sharedHosts[hostLower] && !strings.HasPrefix(hostLower, "www.") {
			candidateDomain = hostLower
		} else {
			candidateDomain = slug + ".com"
		}

		finalDomain := candidateDomain
		counter := 1
		for domainsSeen[finalDomain] && existingCompaniesByDomain[finalDomain] == "" {
			finalDomain = fmt.Sprintf("%s-%d.com", slug, counter)
			counter++
		}
		domainsSeen[finalDomain] = true

		// Check if company already exists
		companyID := existingCompaniesByDomain[finalDomain]
		if companyID == "" && companyName != "" {
			companyID = existingCompaniesByName[strings.ToLower(companyName)]
		}

		if companyID == "" {
			companyID = repository.NewUUID()
			compDoc := bson.M{
				"_id":                 companyID,
				"canonical_name":      companyName,
				"name":                companyName,
				"canonical_domain":    finalDomain,
				"verification_status": "verified",
				"created_at":          now,
				"updated_at":          now,
			}
			companiesToInsert = append(companiesToInsert, compDoc)
			existingCompaniesByDomain[finalDomain] = companyID
			if companyName != "" {
				existingCompaniesByName[strings.ToLower(companyName)] = companyID
			}
		}

		// Check if ATS is supported (Greenhouse, Lever, Ashby, SmartRecruiters, BambooHR, Breezy, Recruitee, Workday, Generic)
		provider, board, endpoint, allowedHosts := extractATSProviderAndBoard(item.ATS, rawURL, rawHost)
		if provider != "" && board != "" {
			srcKey := companyID + "|" + string(provider) + "|" + board
			if !existingSources[srcKey] {
				srcDoc := bson.M{
					"_id":                   repository.NewUUID(),
					"company_id":            companyID,
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
					"created_at":            now,
					"updated_at":            now,
				}
				sourcesToInsert = append(sourcesToInsert, srcDoc)
				existingSources[srcKey] = true
			}
		}
	}

	// Bulk insert companies in batches
	const batchSize = 1000
	for i := 0; i < len(companiesToInsert); i += batchSize {
		end := i + batchSize
		if end > len(companiesToInsert) {
			end = len(companiesToInsert)
		}
		if _, err := companiesColl.InsertMany(ctx, companiesToInsert[i:end]); err != nil {
			return 0, 0, fmt.Errorf("bulk insert companies (batch %d): %w", i, err)
		}
	}

	// Bulk insert sources in batches
	for i := 0; i < len(sourcesToInsert); i += batchSize {
		end := i + batchSize
		if end > len(sourcesToInsert) {
			end = len(sourcesToInsert)
		}
		if _, err := sourcesColl.InsertMany(ctx, sourcesToInsert[i:end]); err != nil {
			return len(companiesToInsert), 0, fmt.Errorf("bulk insert sources (batch %d): %w", i, err)
		}
	}

	slog.Info("Successfully merged companies and sources from JSON",
		"companies_newly_inserted", len(companiesToInsert),
		"sources_newly_inserted", len(sourcesToInsert),
	)

	return len(companiesToInsert), len(sourcesToInsert), nil
}

// SeedCompaniesAndSources guarantees all verified DefaultCompanySeeds exist and are active in MongoDB.
func SeedCompaniesAndSources(ctx context.Context, database *mongo.Database) (int, int, error) {
	now := time.Now().UTC()
	companiesColl := database.Collection("companies")
	sourcesColl := database.Collection("company_sources")

	companiesUpserted := 0
	sourcesUpserted := 0

	for _, seed := range DefaultCompanySeeds {
		companyFilter := bson.M{"canonical_domain": seed.Domain}
		companyUpdate := bson.M{
			"$setOnInsert": bson.M{
				"_id":        repository.NewUUID(),
				"created_at": now,
			},
			"$set": bson.M{
				"canonical_name":      seed.Name,
				"name":                seed.Name,
				"canonical_domain":    seed.Domain,
				"verification_status": "verified",
				"updated_at":          now,
			},
		}

		opts := options.Update().SetUpsert(true)
		res, err := companiesColl.UpdateOne(ctx, companyFilter, companyUpdate, opts)
		if err != nil {
			return companiesUpserted, sourcesUpserted, fmt.Errorf("upsert company %s: %w", seed.Name, err)
		}
		if res.UpsertedCount > 0 || res.ModifiedCount > 0 {
			companiesUpserted++
		}

		var compDoc struct {
			ID string `bson:"_id"`
		}
		if err := companiesColl.FindOne(ctx, companyFilter).Decode(&compDoc); err != nil {
			return companiesUpserted, sourcesUpserted, fmt.Errorf("find company %s: %w", seed.Name, err)
		}

		sourceFilter := bson.M{
			"company_id":       compDoc.ID,
			"provider_type":    string(seed.Provider),
			"board_identifier": seed.BoardIdentifier,
		}
		sourceUpdate := bson.M{
			"$setOnInsert": bson.M{
				"_id":        repository.NewUUID(),
				"created_at": now,
			},
			"$set": bson.M{
				"company_id":            compDoc.ID,
				"company_name":          seed.Name,
				"provider_type":         string(seed.Provider),
				"board_identifier":      seed.BoardIdentifier,
				"source_endpoint":       seed.Endpoint,
				"allowed_hosts":         seed.AllowedHosts,
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
			"$unset": bson.M{
				"lease_owner": "",
				"lease_token": "",
				"lease_until": "",
			},
		}

		sRes, err := sourcesColl.UpdateOne(ctx, sourceFilter, sourceUpdate, opts)
		if err != nil {
			return companiesUpserted, sourcesUpserted, fmt.Errorf("upsert source %s/%s: %w", seed.Name, seed.BoardIdentifier, err)
		}
		if sRes.UpsertedCount > 0 || sRes.ModifiedCount > 0 {
			sourcesUpserted++
		}
	}

	totalComps, _ := companiesColl.CountDocuments(ctx, bson.M{})
	totalSrcs, _ := sourcesColl.CountDocuments(ctx, bson.M{})
	return int(totalComps), int(totalSrcs), nil
}

type DiscoveryResult struct {
	TotalCompanies int
	TotalSources   int
	SyncedSources  int
	TotalJobs      int64
	JobsPerCompany map[string]int64
}

// RunDiscoveryForAll triggers discovery sync for configured companies and sources,
// executing the discovery connector pipeline directly against MongoDB.
func RunDiscoveryForAll(ctx context.Context, database *mongo.Database) (*DiscoveryResult, error) {
	compCount, srcCount, err := SeedCompaniesAndSources(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("seed companies: %w", err)
	}
	slog.Info("Companies and sources catalog ready", "companies_total", compCount, "sources_total", srcCount)

	store := repository.NewJobSourceStore(database)
	registry := job_sources.NewRegistry(nil)

	now := time.Now().UTC()
	var claimedSources []job_sources.SourceConfig
	for {
		batch, err := store.ClaimDueSources(ctx, "discovery-runner", 500, now, now.Add(30*time.Minute))
		if err != nil {
			return nil, fmt.Errorf("claim due sources: %w", err)
		}
		if len(batch) == 0 {
			break
		}
		claimedSources = append(claimedSources, batch...)
		if len(claimedSources) >= 2000 {
			break
		}
	}

	slog.Info("Claimed sources for discovery sync", "count", len(claimedSources))

	var (
		synced atomic.Int64
		wg     sync.WaitGroup
	)

	const workerPoolSize = 6
	srcChan := make(chan job_sources.SourceConfig, len(claimedSources))
	for _, s := range claimedSources {
		srcChan <- s
	}
	close(srcChan)

	for w := 0; w < workerPoolSize; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for source := range srcChan {
				run, err := store.StartSourceSync(ctx, source, time.Now().UTC())
				if err != nil {
					slog.Error("Failed to start source sync run", "source_id", source.ID, "error", err)
					continue
				}

				handler := SourceSyncEventHandler{
					Store:         store,
					Connector:     registry,
					MaxPages:      20,
					MaxRecords:    10_000,
					LeaseDuration: 20 * time.Minute,
					RetryDelay:    5 * time.Minute,
					MaxRetryDelay: 24 * time.Hour,
					Now:           time.Now,
					OnError: func(err error) {
						slog.Error("Source sync error", "source_id", source.ID, "error", err)
					},
				}

				event := queue.Event{
					ID:            repository.NewUUID(),
					Type:          queue.EventSourceSyncRequested,
					SchemaVersion: 1,
					AggregateType: "source_sync_run",
					AggregateID:   run.ID,
					CreatedAt:     time.Now().UTC(),
					Payload: map[string]string{
						"source_id":   source.ID,
						"sync_run_id": run.ID,
					},
				}

				if err := handler.HandleEvent(ctx, event); err != nil {
					slog.Error("Source sync event handling failed",
						"source_id", source.ID,
						"board", source.BoardIdentifier,
						"error", err,
					)
					continue
				}

				synced.Add(1)
				slog.Info("Completed discovery sync for source",
					"provider", source.Provider,
					"board", source.BoardIdentifier,
				)
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Summarize results directly from MongoDB
	totalJobs, _ := database.Collection("jobs").CountDocuments(ctx, bson.M{})
	totalCompanies, _ := database.Collection("companies").CountDocuments(ctx, bson.M{})
	totalSources, _ := database.Collection("company_sources").CountDocuments(ctx, bson.M{})

	jobsPerCompany := make(map[string]int64)
	pipeline := mongo.Pipeline{
		{{Key: "$lookup", Value: bson.M{
			"from":         "companies",
			"localField":   "company_id",
			"foreignField": "_id",
			"as":           "company",
		}}},
		{{Key: "$unwind", Value: "$company"}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$company.canonical_name",
			"count": bson.M{"$sum": 1},
		}}},
	}

	cursor, err := database.Collection("jobs").Aggregate(ctx, pipeline)
	if err == nil {
		defer cursor.Close(ctx)
		var results []struct {
			CompanyName string `bson:"_id"`
			Count       int64  `bson:"count"`
		}
		if err := cursor.All(ctx, &results); err == nil {
			for _, r := range results {
				jobsPerCompany[r.CompanyName] = r.Count
			}
		}
	}

	return &DiscoveryResult{
		TotalCompanies: int(totalCompanies),
		TotalSources:   int(totalSources),
		SyncedSources:  int(synced.Load()),
		TotalJobs:      totalJobs,
		JobsPerCompany: jobsPerCompany,
	}, nil
}

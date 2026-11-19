package worker

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/oneRole/connectors/job_sources"
)

func TestExtractATSProviderAndBoard(t *testing.T) {
	tests := []struct {
		name         string
		ats          string
		url          string
		host         string
		wantProvider job_sources.Provider
		wantBoard    string
	}{
		{
			name:         "Greenhouse with query param",
			ats:          "Greenhouse",
			url:          "https://boards.greenhouse.io/embed/job_board?for=stripe",
			host:         "boards.greenhouse.io",
			wantProvider: job_sources.ProviderGreenhouse,
			wantBoard:    "stripe",
		},
		{
			name:         "Greenhouse in path",
			ats:          "Greenhouse",
			url:          "https://job-boards.greenhouse.io/accela",
			host:         "job-boards.greenhouse.io",
			wantProvider: job_sources.ProviderGreenhouse,
			wantBoard:    "accela",
		},
		{
			name:         "Lever standard",
			ats:          "Lever",
			url:          "https://jobs.lever.co/abtaba",
			host:         "jobs.lever.co",
			wantProvider: job_sources.ProviderLever,
			wantBoard:    "abtaba",
		},
		{
			name:         "Ashby standard",
			ats:          "Ashby",
			url:          "https://jobs.ashbyhq.com/abacum",
			host:         "jobs.ashbyhq.com",
			wantProvider: job_sources.ProviderAshby,
			wantBoard:    "abacum",
		},
		{
			name:         "SmartRecruiters standard",
			ats:          "SmartRecruiters",
			url:          "https://jobs.smartrecruiters.com/A2Design",
			host:         "jobs.smartrecruiters.com",
			wantProvider: job_sources.ProviderSmartRecruiters,
			wantBoard:    "A2Design",
		},
		{
			name:         "Breezy standard",
			ats:          "Breezy",
			url:          "https://a-hiringgroup.breezy.hr",
			host:         "a-hiringgroup.breezy.hr",
			wantProvider: job_sources.ProviderBreezy,
			wantBoard:    "a-hiringgroup",
		},
		{
			name:         "Recruitee standard",
			ats:          "recruitee.com",
			url:          "https://agaseurope.recruitee.com",
			host:         "agaseurope.recruitee.com",
			wantProvider: job_sources.ProviderRecruitee,
			wantBoard:    "agaseurope",
		},
		{
			name:         "BambooHR standard",
			ats:          "BambooHR",
			url:          "https://aacb.bamboohr.com/careers",
			host:         "aacb.bamboohr.com",
			wantProvider: job_sources.ProviderBambooHR,
			wantBoard:    "aacb",
		},
		{
			name:         "Generic Careers URL",
			ats:          "Custom",
			url:          "https://careers.admortgage.com/jobs",
			host:         "careers.admortgage.com",
			wantProvider: job_sources.ProviderGeneric,
			wantBoard:    "jobs",
		},
		{
			name:         "Workday standard",
			ats:          "Workday",
			url:          "https://accenture.wd103.myworkdayjobs.com/accenturecareers",
			host:         "accenture.wd103.myworkdayjobs.com",
			wantProvider: job_sources.ProviderWorkday,
			wantBoard:    "accenturecareers",
		},
		{
			name:         "SmartRecruiters Freehire job URL",
			ats:          "smartrecruiters",
			url:          "https://jobs.smartrecruiters.com/Dominos/744000162592564-general-manager-09450-2430-us-hwy-27?utm_source=freehire.me",
			host:         "jobs.smartrecruiters.com",
			wantProvider: job_sources.ProviderSmartRecruiters,
			wantBoard:    "Dominos",
		},
		{
			name:         "Greenhouse API boards URL",
			ats:          "greenhouse",
			url:          "https://boards-api.greenhouse.io/v1/boards/stripe",
			host:         "boards-api.greenhouse.io",
			wantProvider: job_sources.ProviderGreenhouse,
			wantBoard:    "stripe",
		},
		{
			name:         "Lever with job path",
			ats:          "lever",
			url:          "https://jobs.lever.co/netflix/12345?utm_source=freehire.me",
			host:         "jobs.lever.co",
			wantProvider: job_sources.ProviderLever,
			wantBoard:    "netflix",
		},
		{
			name:         "Ashby with job path",
			ats:          "ashby",
			url:          "https://jobs.ashbyhq.com/sentry/abc-123",
			host:         "jobs.ashbyhq.com",
			wantProvider: job_sources.ProviderAshby,
			wantBoard:    "sentry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, board, _, _ := extractATSProviderAndBoard(tt.ats, tt.url, tt.host)
			if provider != tt.wantProvider {
				t.Errorf("extractATSProviderAndBoard() provider = %v, want %v", provider, tt.wantProvider)
			}
			if board != tt.wantBoard {
				t.Errorf("extractATSProviderAndBoard() board = %v, want %v", board, tt.wantBoard)
			}
		})
	}
}

func TestCleanSlug(t *testing.T) {
	if got := cleanSlug("  A Place For Mom  "); got != "a-place-for-mom" {
		t.Errorf("cleanSlug() = %v, want %v", got, "a-place-for-mom")
	}
	if got := cleanSlug("4mAnalytics!#$"); got != "4manalytics" {
		t.Errorf("cleanSlug() = %v, want %v", got, "4manalytics")
	}
}

func TestUnmarshalCompaniesJSONFormats(t *testing.T) {
	// 1. Envelope format {"companies": [...]}
	envelopeJSON := []byte(`{
		"companies": [
			{"name": "Dominos", "provider": "smartrecruiters", "board": "Dominos", "host": "jobs.smartrecruiters.com", "job_board_url": "https://jobs.smartrecruiters.com/Dominos"}
		]
	}`)
	var records []rawCompanyRecord
	if err := json.Unmarshal(envelopeJSON, &records); err != nil {
		var env struct {
			Companies []rawCompanyRecord `json:"companies"`
		}
		if envErr := json.Unmarshal(envelopeJSON, &env); envErr != nil || len(env.Companies) == 0 {
			t.Fatalf("failed to unmarshal envelope: %v", envErr)
		}
		records = env.Companies
	}
	if len(records) != 1 || records[0].Name != "Dominos" || records[0].Provider != "smartrecruiters" {
		t.Errorf("unexpected record parsed: %+v", records)
	}

	// 2. Direct array format [...]
	arrayJSON := []byte(`[
		{"company_name": "Netflix", "base_job_url": "https://jobs.lever.co/netflix", "ats": "lever", "host": "jobs.lever.co"}
	]`)
	var arrRecords []rawCompanyRecord
	if err := json.Unmarshal(arrayJSON, &arrRecords); err != nil {
		t.Fatalf("failed to unmarshal array: %v", err)
	}
	if len(arrRecords) != 1 || *arrRecords[0].CompanyName != "Netflix" {
		t.Errorf("unexpected record parsed: %+v", arrRecords)
	}
}

func TestParseIngestedCompaniesJSON(t *testing.T) {
	// Path relative to worker package
	paths := []string{"../companies.json", "../config/companies.json"}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Skipf("File not found at %s: %v", p, err)
			continue
		}

		var envelope struct {
			Companies []rawCompanyRecord `json:"companies"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("failed to unmarshal %s: %v", p, err)
		}

		if len(envelope.Companies) == 0 {
			t.Fatalf("expected companies in %s, got 0", p)
		}

		validCount := 0
		for _, c := range envelope.Companies {
			rawName := c.Name
			if rawName == "" && c.CompanyName != nil {
				rawName = *c.CompanyName
			}
			urlStr := c.JobBoardURL
			if urlStr == "" {
				urlStr = c.BaseJobURL
			}
			providerStr := c.Provider
			if providerStr == "" {
				providerStr = c.ATS
			}

			provider, board, endpoint, allowedHosts := extractATSProviderAndBoard(providerStr, urlStr, c.Host)
			if provider != "" && board != "" && endpoint != "" && len(allowedHosts) > 0 {
				validCount++
			}
		}

		if validCount < len(envelope.Companies) {
			t.Errorf("expected all %d companies to produce valid provider & board, but only %d did", len(envelope.Companies), validCount)
		}
	}
}

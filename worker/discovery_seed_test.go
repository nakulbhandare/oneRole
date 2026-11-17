package worker

import (
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

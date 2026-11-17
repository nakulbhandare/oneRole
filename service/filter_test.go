package service

import (
	"strings"
	"testing"
	"time"

	"github.com/oneRole/models"
)

func TestNormalizeJobSearchRequestDefaultsAndCanonicalizes(t *testing.T) {
	companyID := "550E8400-E29B-41D4-A716-446655440000"
	radius := 25
	request := models.JobSearchRequest{
		Query:          "  Go Engineer  ",
		LocationIDs:    []string{" us-ca-sf ", "us-ca-sf"},
		CompanyIDs:     []string{companyID},
		WorkplaceModes: []models.WorkplaceMode{"REMOTE", models.WorkplaceRemote},
		Locations: []models.LocationFilter{{
			CountryCode: "us", City: " San Francisco ", RadiusKM: &radius,
		}},
	}

	got, errs := NormalizeJobSearchRequest(request)
	if len(errs) != 0 {
		t.Fatalf("NormalizeJobSearchRequest() errors = %#v", errs)
	}
	if got.Query != "Go Engineer" || got.Limit != 25 || got.Sort != models.SortRelevance ||
		got.Date.Preset != models.DateAny || got.DegreeRequirement != models.DegreeAny || got.VisaSponsorship != models.VisaAny {
		t.Fatalf("defaults or trimmed query not applied: %#v", got)
	}
	if len(got.LocationIDs) != 1 || got.LocationIDs[0] != "us-ca-sf" {
		t.Fatalf("location ids were not deduplicated: %#v", got.LocationIDs)
	}
	if len(got.WorkplaceModes) != 1 || got.WorkplaceModes[0] != models.WorkplaceRemote {
		t.Fatalf("workplace modes were not normalized: %#v", got.WorkplaceModes)
	}
	if got.CompanyIDs[0] != strings.ToLower(companyID) || got.Locations[0].CountryCode != "US" || got.Locations[0].City != "San Francisco" {
		t.Fatalf("values were not canonicalized: %#v", got)
	}
	if request.Locations[0].CountryCode != "us" || request.Locations[0].City != " San Francisco " {
		t.Fatalf("normalization mutated the caller's request: %#v", request.Locations[0])
	}
}

func TestNormalizeJobSearchRequestReportsBoundsEnumsAndDateOrdering(t *testing.T) {
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	tooMuch := 101
	request := models.JobSearchRequest{
		Date:               models.DateFilter{Preset: "tomorrow", PublishedAfter: &start, PublishedBefore: &end},
		MaxExperienceYears: &tooMuch,
		WorkplaceModes:     []models.WorkplaceMode{"office"},
		Sort:               "random",
		Limit:              0, // zero means omitted and receives the documented default
		CompanyIDs:         []string{"not-a-uuid"},
	}

	got, errs := NormalizeJobSearchRequest(request)
	if got.Limit != 25 {
		t.Fatalf("default limit = %d, want 25", got.Limit)
	}
	fields := make(map[string]bool, len(errs))
	for _, err := range errs {
		fields[err.Field] = true
	}
	for _, field := range []string{"date.preset", "date", "max_experience_years", "workplace_modes[0]", "sort", "company_ids[0]"} {
		if !fields[field] {
			t.Errorf("missing validation error for %q; got %#v", field, errs)
		}
	}
}

func TestNormalizeJobSearchRequestRejectsOutOfRangeLimitAndInvalidLocation(t *testing.T) {
	radius := 0
	request := models.JobSearchRequest{
		Limit:     101,
		Locations: []models.LocationFilter{{CountryCode: "USA", RadiusKM: &radius}},
	}
	_, errs := NormalizeJobSearchRequest(request)
	fields := make(map[string]bool, len(errs))
	for _, err := range errs {
		fields[err.Field] = true
	}
	for _, field := range []string{"limit", "locations[0].country_code", "locations[0].radius_km"} {
		if !fields[field] {
			t.Errorf("missing validation error for %q; got %#v", field, errs)
		}
	}
}

func TestMatchesCategoricalFilterKeepsUnknownSeparate(t *testing.T) {
	selected := []models.WorkplaceMode{models.WorkplaceRemote}
	if !MatchesCategoricalFilter(selected, []models.WorkplaceMode{models.WorkplaceRemote}, false) {
		t.Fatal("selected known value should match")
	}
	if MatchesCategoricalFilter(selected, nil, false) {
		t.Fatal("missing value must not be treated as a known selected value")
	}
	if !MatchesCategoricalFilter(selected, nil, true) {
		t.Fatal("include_unknown should include a missing value when a facet is active")
	}
	if !MatchesCategoricalFilter([]models.WorkplaceMode(nil), nil, false) {
		t.Fatal("empty selection should leave the facet unrestricted")
	}
}

func TestMatchesJobDateFilterKeepsPublicationUnknownSeparate(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.FixedZone("PDT", -7*60*60))
	job := models.Job{FirstSeenAt: now.Add(-48 * time.Hour), PublicationTimeQuality: models.PublicationUnknown}
	filter := models.DateFilter{Preset: models.DateLast24Hours}
	if MatchesJobDateFilter(job, filter, now) {
		t.Fatal("unknown publication time must not match an active date window by default")
	}
	filter.IncludeUnknownPublicationTime = true
	if !MatchesJobDateFilter(job, filter, now) {
		t.Fatal("include_unknown_publication_time should include unknown dates")
	}
	firstSeenThreshold := now.Add(-24 * time.Hour)
	filter.FirstSeenAfter = &firstSeenThreshold
	if MatchesJobDateFilter(job, filter, now) {
		t.Fatal("first_seen_after must be applied independently of publication time")
	}
	filter.Preset = models.DateAny
	filter.FirstSeenAfter = nil
	filter.IncludeUnknownPublicationTime = false
	if !MatchesJobDateFilter(job, filter, now) {
		t.Fatal("unknown publication time is unrestricted when no publication window is active")
	}
}

func TestMatchesJobDateFilterUsesPublishedAtAndCustomBounds(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	published := now.Add(-6 * 24 * time.Hour)
	job := models.Job{PublishedAt: &published, PublicationTimeQuality: models.PublicationSourceProvided}
	if !MatchesJobDateFilter(job, models.DateFilter{Preset: models.DateLast7Days}, now) {
		t.Fatal("job published within last seven days should match")
	}
	if MatchesJobDateFilter(job, models.DateFilter{Preset: models.DateLast24Hours}, now) {
		t.Fatal("job published outside last 24 hours should not match")
	}
	recentPublished := now.Add(-3 * time.Hour)
	recentJob := models.Job{PublishedAt: &recentPublished, PublicationTimeQuality: models.PublicationSourceProvided}
	if !MatchesJobDateFilter(recentJob, models.DateFilter{Preset: models.DateLast6Hours}, now) {
		t.Fatal("job published 3 hours ago should match last_6_hours")
	}
	if MatchesJobDateFilter(job, models.DateFilter{Preset: models.DateLast6Hours}, now) {
		t.Fatal("job published 6 days ago should not match last_6_hours")
	}
	after := now.Add(-5 * 24 * time.Hour)
	if MatchesJobDateFilter(job, models.DateFilter{Preset: models.DateCustom, PublishedAfter: &after}, now) {
		t.Fatal("job before the custom published_after bound should not match")
	}
}

func TestNormalizeJobSearchRequestJobIDs(t *testing.T) {
	validUUID1 := "644150ef-1c41-41e5-b049-f4256bab80e7"
	validUUID2 := "f5da1e10-136e-42b9-85cf-e830f45f381b"
	req := models.JobSearchRequest{
		JobID:  strings.ToUpper(validUUID1),
		JobIDs: []string{validUUID2, strings.ToUpper(validUUID1)},
	}
	normalized, errs := NormalizeJobSearchRequest(req)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %#v", errs)
	}
	if len(normalized.JobIDs) != 2 {
		t.Fatalf("expected 2 unique job IDs, got %d: %#v", len(normalized.JobIDs), normalized.JobIDs)
	}
	if normalized.JobIDs[0] != validUUID2 || normalized.JobIDs[1] != validUUID1 {
		t.Fatalf("expected normalized lowercase IDs [%s, %s], got %#v", validUUID2, validUUID1, normalized.JobIDs)
	}
	if normalized.JobID != "" {
		t.Fatalf("expected JobID to be cleared into JobIDs, got %q", normalized.JobID)
	}

	invalidReq := models.JobSearchRequest{
		JobIDs: []string{"invalid-uuid"},
	}
	_, invalidErrs := NormalizeJobSearchRequest(invalidReq)
	if len(invalidErrs) != 1 || invalidErrs[0].Field != "job_ids[0]" {
		t.Fatalf("expected invalid UUID error on job_ids[0], got %#v", invalidErrs)
	}
}

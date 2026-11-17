package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

type catalogAPIFake struct {
	request         models.JobSearchRequest
	searchCalls     int
	companyQuery    string
	companyLimit    int
	locationQuery   string
	locationCountry string
	locationLimit   int
	jobID           string
	searchErr       error
}

func (c *catalogAPIFake) SearchJobs(_ context.Context, request models.JobSearchRequest) (models.JobSearchResponse, error) {
	c.searchCalls++
	c.request = request
	return models.JobSearchResponse{Items: []models.Job{}, AppliedFilters: request, Facets: models.FacetSet{}}, c.searchErr
}
func (c *catalogAPIFake) SearchJobPage(_ context.Context, request models.JobSearchRequest) (models.JobPage, error) {
	c.request = request
	return models.JobPage{Items: []models.Job{}}, c.searchErr
}
func (c *catalogAPIFake) SearchFacets(_ context.Context, request models.JobSearchRequest) (models.FacetSet, error) {
	c.request = request
	return models.FacetSet{}, c.searchErr
}
func (c *catalogAPIFake) GetFilterMetadata(context.Context) (service.FilterMetadata, error) {
	return service.FilterMetadata{Facets: models.FacetSet{}}, nil
}
func (c *catalogAPIFake) GetJob(_ context.Context, id string) (models.Job, error) {
	c.jobID = id
	return models.Job{ID: id}, nil
}
func (c *catalogAPIFake) SearchCompanies(_ context.Context, query string, _ *string, limit int) (service.CompanySearchResponse, error) {
	c.companyQuery, c.companyLimit = query, limit
	return service.CompanySearchResponse{Items: []service.CompanySuggestion{}}, nil
}
func (c *catalogAPIFake) SuggestLocations(_ context.Context, query, country string, limit int) (service.LocationSuggestions, error) {
	c.locationQuery, c.locationCountry, c.locationLimit = query, country, limit
	return service.LocationSuggestions{Items: []service.LocationSuggestion{}}, nil
}
func (c *catalogAPIFake) RecentJobs(_ context.Context, _ int) (models.JobPage, error) {
	return models.JobPage{Items: []models.Job{}}, nil
}
func (c *catalogAPIFake) ListSources(_ context.Context, _ int) ([]models.SourceSummary, error) {
	return []models.SourceSummary{}, nil
}
func (c *catalogAPIFake) GetSourceHealth(_ context.Context, id string) (*models.SourceHealth, error) {
	return &models.SourceHealth{SourceID: id, Status: "active", Capability: "greenhouse"}, nil
}

func TestParseJobSearchQueryMapsAllFilters(t *testing.T) {
	values, err := url.ParseQuery("q=senior+golang&exclude=intern&exclude=junior&date_preset=last_60_days&published_after=2026-10-01T00%3A00%3A00Z&include_unknown_publication_time=true&location=loc-1&radius_km=25&workplace=remote&workplace=hybrid&company_id=11111111-1111-4111-8111-111111111111&exclude_company_id=22222222-2222-4222-8222-222222222222&degree_level=master&degree_requirement=preferred&max_experience_years=8&include_unknown_experience=true&visa_sponsorship=yes&role_id=backend&job_type=senior&employment_type=full_time&include_unknown_location=true&include_unknown_company=true&include_unknown_degree=true&include_unknown_workplace=true&include_unknown_role=true&include_unknown_job_type=true&include_unknown_employment_type=true&cursor=next&limit=50&sort=published_desc")
	if err != nil {
		t.Fatal(err)
	}
	request, err := parseJobSearchQuery(values)
	if err != nil {
		t.Fatalf("parse filters: %v", err)
	}
	normalized, issues := service.NormalizeJobSearchRequest(request)
	if len(issues) > 0 {
		t.Fatalf("parsed filters did not satisfy domain rules: %#v", issues)
	}
	if normalized.Query != "senior golang" || len(normalized.ExcludeTerms) != 2 || normalized.Date.Preset != models.DateLast60Days ||
		!normalized.Date.IncludeUnknownPublicationTime || normalized.WorkplaceModes[0] != models.WorkplaceRemote ||
		len(normalized.Locations) != 1 || normalized.Locations[0].RadiusKM == nil || *normalized.Locations[0].RadiusKM != 25 ||
		normalized.MaxExperienceYears == nil || *normalized.MaxExperienceYears != 8 || normalized.VisaSponsorship != models.VisaYes ||
		normalized.Sort != models.SortPublishedDesc || normalized.Cursor == nil || *normalized.Cursor != "next" || normalized.Limit != 50 {
		t.Fatalf("filters were not mapped: %#v", normalized)
	}
}

func TestParseJobSearchQueryRejectsRepeatedScalarAndRadiusWithoutLocation(t *testing.T) {
	values := url.Values{"date_preset": {"last_7_days", "last_30_days"}}
	if _, err := parseJobSearchQuery(values); err == nil {
		t.Fatal("repeated scalar parameter should be rejected")
	}
	values = url.Values{"radius_km": {"20"}}
	if _, err := parseJobSearchQuery(values); err == nil {
		t.Fatal("radius without selected location should be rejected")
	}
}

func TestParseJobSearchQueryMapsJobIDFilters(t *testing.T) {
	uuid1 := "644150ef-1c41-41e5-b049-f4256bab80e7"
	uuid2 := "f5da1e10-136e-42b9-85cf-e830f45f381b"
	values, err := url.ParseQuery("job_id=" + uuid1 + "&id=" + uuid2)
	if err != nil {
		t.Fatal(err)
	}
	req, err := parseJobSearchQuery(values)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	normalized, issues := service.NormalizeJobSearchRequest(req)
	if len(issues) > 0 {
		t.Fatalf("validation issues: %#v", issues)
	}
	if len(normalized.JobIDs) != 2 || normalized.JobIDs[0] != uuid1 || normalized.JobIDs[1] != uuid2 {
		t.Fatalf("expected job IDs [%s, %s], got %#v", uuid1, uuid2, normalized.JobIDs)
	}
}

func TestCatalogHandlerReturnsDetailedFilterErrorsAndStrictPostBody(t *testing.T) {
	api := &catalogAPIFake{searchErr: service.JobSearchValidationError{Issues: []service.ValidationError{{
		Field: "max_experience_years", Code: "out_of_range", Message: "must be between 0 and 50",
	}}}}
	handler, err := NewCatalogHandler(api)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/search?max_experience_years=100", nil)
	response := httptest.NewRecorder()
	handler.SearchJobs(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "max_experience_years") || !strings.Contains(response.Body.String(), "out_of_range") {
		t.Fatalf("expected detailed filter error, status=%d body=%s", response.Code, response.Body.String())
	}

	api.searchErr = nil
	searchCallsBefore := api.searchCalls
	request = httptest.NewRequest(http.MethodPost, "/api/v1/jobs/search", strings.NewReader(`{"query":"golang","tenant_id":"`+testTenantID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.SearchJobs(response, request)
	if response.Code != http.StatusBadRequest || api.searchCalls != searchCallsBefore {
		t.Fatalf("unknown request properties must be rejected before search: status=%d", response.Code)
	}
}

func TestCatalogHandlerCompanyAndLocationSuggestionsUseLiveAPI(t *testing.T) {
	api, err := NewCatalogHandler(&catalogAPIFake{})
	if err != nil {
		t.Fatal(err)
	}
	fake := api.api.(*catalogAPIFake)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/companies/search?q=Acme&limit=10", nil)
	response := httptest.NewRecorder()
	api.Companies(response, request)
	if response.Code != http.StatusOK || fake.companyQuery != "Acme" || fake.companyLimit != 10 {
		t.Fatalf("company filter suggestions not handled: %d %#v", response.Code, fake)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/locations/suggest?q=San+Francisco&country_code=us&limit=5", nil)
	response = httptest.NewRecorder()
	api.Locations(response, request)
	if response.Code != http.StatusOK || fake.locationQuery != "San Francisco" || fake.locationCountry != "us" || fake.locationLimit != 5 {
		t.Fatalf("location suggestions not handled: %d %#v", response.Code, fake)
	}
}

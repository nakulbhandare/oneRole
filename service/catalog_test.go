package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oneRole/models"
)

type catalogStoreFake struct {
	request         models.JobSearchRequest
	companyQuery    string
	companyLimit    int
	locationQuery   string
	locationCountry string
	locationLimit   int
	jobID           string
	searchCalls     int
}

func (s *catalogStoreFake) SearchJobs(_ context.Context, request models.JobSearchRequest) (models.JobSearchResponse, error) {
	s.request = request
	s.searchCalls++
	return models.JobSearchResponse{Items: []models.Job{}, AppliedFilters: request, Facets: models.FacetSet{}}, nil
}
func (s *catalogStoreFake) GetJob(_ context.Context, id string) (models.Job, error) {
	s.jobID = id
	return models.Job{ID: id}, nil
}
func (s *catalogStoreFake) SearchCompanies(_ context.Context, query string, _ *string, limit int) (CompanySearchResponse, error) {
	s.companyQuery, s.companyLimit = query, limit
	return CompanySearchResponse{Items: []CompanySuggestion{}}, nil
}
func (s *catalogStoreFake) SuggestLocations(_ context.Context, query, country string, limit int) (LocationSuggestions, error) {
	s.locationQuery, s.locationCountry, s.locationLimit = query, country, limit
	return LocationSuggestions{Items: []LocationSuggestion{}}, nil
}
func (s *catalogStoreFake) GetFilterMetadata(context.Context) (FilterMetadata, error) {
	return FilterMetadata{Facets: models.FacetSet{}}, nil
}

func TestCatalogServiceNormalizesBeforeDatabaseSearch(t *testing.T) {
	store := &catalogStoreFake{}
	catalog, err := NewCatalogService(store, store, store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	response, err := catalog.SearchJobs(context.Background(), models.JobSearchRequest{Query: "  senior engineer  "})
	if err != nil {
		t.Fatal(err)
	}
	if store.searchCalls != 1 || store.request.Query != "senior engineer" || store.request.Limit != 25 || store.request.Date.Preset != models.DateAny || response.AppliedFilters.Query != "senior engineer" {
		t.Fatalf("search should receive normalized filters: %#v", store.request)
	}
}

func TestCatalogServiceRejectsInvalidSearchBeforeStoreCall(t *testing.T) {
	store := &catalogStoreFake{}
	catalog, err := NewCatalogService(store, store, store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	_, err = catalog.SearchJobs(context.Background(), models.JobSearchRequest{Limit: 101})
	var validation JobSearchValidationError
	if !errors.As(err, &validation) || !errors.Is(err, ErrJobSearchInvalid) {
		t.Fatalf("expected field validation error, got %v", err)
	}
	if store.searchCalls != 0 {
		t.Fatal("invalid filters must be rejected before reaching the database")
	}
}

func TestCatalogServiceValidatesGlobalSuggestionQueries(t *testing.T) {
	store := &catalogStoreFake{}
	catalog, err := NewCatalogService(store, store, store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.SearchCompanies(context.Background(), "A", nil, 25); !errors.Is(err, ErrJobSearchInvalid) {
		t.Fatalf("one-character company query should be rejected: %v", err)
	}
	if _, err := catalog.SuggestLocations(context.Background(), "US", "USA", 25); !errors.Is(err, ErrJobSearchInvalid) {
		t.Fatalf("invalid country code should be rejected: %v", err)
	}
	if _, err := catalog.GetJob(context.Background(), "invalid"); !errors.Is(err, ErrCatalogItemNotFound) {
		t.Fatalf("invalid job ID should be rejected: %v", err)
	}
	if store.jobID != "" {
		t.Fatal("invalid ID must not reach the repository")
	}
}

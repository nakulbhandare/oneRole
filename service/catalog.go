package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/oneRole/models"
)

var (
	ErrJobSearchInvalid    = errors.New("job search request is invalid")
	ErrCatalogItemNotFound = errors.New("catalog item not found")
	ErrCatalogDependency   = errors.New("catalog dependency unavailable")
)

// CompanySuggestion and CompanySearchResponse back the Companies filter.
type CompanySuggestion struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	LogoURL *string `json:"logo_url"`
}

type CompanySearchResponse struct {
	Items      []CompanySuggestion `json:"items"`
	NextCursor *string             `json:"next_cursor"`
}

// LocationSuggestion is a normalized location and its current matching count.
type LocationSuggestion struct {
	ID               string  `json:"id"`
	Label            string  `json:"label"`
	City             *string `json:"city"`
	Region           *string `json:"region"`
	CountryCode      string  `json:"country_code"`
	MatchingJobCount int     `json:"matching_job_count"`
}

type LocationSuggestions struct {
	Items      []LocationSuggestion `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

type FilterMetadata struct {
	Facets models.FacetSet `json:"facets"`
}

// Catalog repository ports return real normalized records. SearchJobs owns query
// filter semantics and contextual facets; no service layer fabricates data.
type JobSearchStore interface {
	SearchJobs(ctx context.Context, request models.JobSearchRequest) (models.JobSearchResponse, error)
}

type JobReader interface {
	GetJob(ctx context.Context, jobID string) (models.Job, error)
}

type CompanySearchStore interface {
	SearchCompanies(ctx context.Context, query string, cursor *string, limit int) (CompanySearchResponse, error)
}

type LocationSuggestionStore interface {
	SuggestLocations(ctx context.Context, query, countryCode string, limit int) (LocationSuggestions, error)
}

type FilterStore interface {
	GetFilterMetadata(ctx context.Context) (FilterMetadata, error)
}

type SourceStore interface {
	ListSources(ctx context.Context, limit int) ([]models.SourceSummary, error)
	GetSourceHealth(ctx context.Context, sourceID string) (*models.SourceHealth, error)
}

// CatalogService normalizes and validates filter input before calling the real
// database-backed search and suggestion stores.
type CatalogService struct {
	jobs      JobSearchStore
	jobReader JobReader
	companies CompanySearchStore
	locations LocationSuggestionStore
	filters   FilterStore
	sources   SourceStore
}

func NewCatalogService(jobs JobSearchStore, jobReader JobReader, companies CompanySearchStore, locations LocationSuggestionStore, filters FilterStore) (*CatalogService, error) {
	if jobs == nil || jobReader == nil || companies == nil || locations == nil || filters == nil {
		return nil, errors.New("all catalog stores are required")
	}
	return &CatalogService{jobs: jobs, jobReader: jobReader, companies: companies, locations: locations, filters: filters}, nil
}

func (s *CatalogService) WithSources(sources SourceStore) *CatalogService {
	s.sources = sources
	return s
}

func (s *CatalogService) SearchJobs(ctx context.Context, request models.JobSearchRequest) (models.JobSearchResponse, error) {
	normalized, issues := NormalizeJobSearchRequest(request)
	if len(issues) > 0 {
		return models.JobSearchResponse{}, JobSearchValidationError{Issues: issues}
	}
	response, err := s.jobs.SearchJobs(ctx, normalized)
	if err != nil {
		return models.JobSearchResponse{}, err
	}
	return response, nil
}

func (s *CatalogService) SearchJobPage(ctx context.Context, request models.JobSearchRequest) (models.JobPage, error) {
	response, err := s.SearchJobs(ctx, request)
	if err != nil {
		return models.JobPage{}, err
	}
	return models.JobPage{Items: response.Items, NextCursor: response.NextCursor}, nil
}

func (s *CatalogService) SearchFacets(ctx context.Context, request models.JobSearchRequest) (models.FacetSet, error) {
	request.IncludeFacets = true
	response, err := s.SearchJobs(ctx, request)
	if err != nil {
		return nil, err
	}
	return response.Facets, nil
}

func (s *CatalogService) GetJob(ctx context.Context, jobID string) (models.Job, error) {
	if !isUUID(jobID) {
		return models.Job{}, ErrCatalogItemNotFound
	}
	return s.jobReader.GetJob(ctx, strings.ToLower(jobID))
}

func (s *CatalogService) SearchCompanies(ctx context.Context, query string, cursor *string, limit int) (CompanySearchResponse, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 || len([]rune(query)) > 200 || limit < 1 || limit > 100 {
		return CompanySearchResponse{}, ErrJobSearchInvalid
	}
	return s.companies.SearchCompanies(ctx, query, cursor, limit)
}

func (s *CatalogService) SuggestLocations(ctx context.Context, query, countryCode string, limit int) (LocationSuggestions, error) {
	query = strings.TrimSpace(query)
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	if len([]rune(query)) < 2 || len([]rune(query)) > 160 || limit < 1 || limit > 100 ||
		(countryCode != "" && len(countryCode) != 2) {
		return LocationSuggestions{}, ErrJobSearchInvalid
	}
	return s.locations.SuggestLocations(ctx, query, countryCode, limit)
}

var (
	cachedFilterMetadata FilterMetadata
	cachedFilterMetaExp  time.Time
	cachedFilterMetaMu   sync.RWMutex
)

func (s *CatalogService) GetFilterMetadata(ctx context.Context) (FilterMetadata, error) {
	cachedFilterMetaMu.RLock()
	if time.Now().Before(cachedFilterMetaExp) && len(cachedFilterMetadata.Facets) > 0 {
		meta := cachedFilterMetadata
		cachedFilterMetaMu.RUnlock()
		return meta, nil
	}
	cachedFilterMetaMu.RUnlock()

	meta, err := s.filters.GetFilterMetadata(ctx)
	if err != nil {
		return FilterMetadata{}, err
	}

	cachedFilterMetaMu.Lock()
	cachedFilterMetadata = meta
	cachedFilterMetaExp = time.Now().Add(60 * time.Second)
	cachedFilterMetaMu.Unlock()

	return meta, nil
}

func (s *CatalogService) RecentJobs(ctx context.Context, limit int) (models.JobPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return s.SearchJobPage(ctx, models.JobSearchRequest{
		Sort:  models.SortFirstSeenDesc,
		Limit: limit,
	})
}

func (s *CatalogService) ListSources(ctx context.Context, limit int) ([]models.SourceSummary, error) {
	if s.sources == nil {
		return []models.SourceSummary{}, nil
	}
	return s.sources.ListSources(ctx, limit)
}

func (s *CatalogService) GetSourceHealth(ctx context.Context, sourceID string) (*models.SourceHealth, error) {
	if s.sources == nil {
		return nil, ErrCatalogItemNotFound
	}
	return s.sources.GetSourceHealth(ctx, sourceID)
}

// JobSearchValidationError preserves the stable field issues generated by the
// domain normalizer while exposing a generic error string to logs.
type JobSearchValidationError struct {
	Issues []ValidationError
}

func (e JobSearchValidationError) Error() string { return ErrJobSearchInvalid.Error() }
func (e JobSearchValidationError) Unwrap() error { return ErrJobSearchInvalid }

package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

// CatalogAPI provides global, database-backed discovery and facet results.
// service.CatalogService implements it after validating filters through the
// domain normalizer.
type CatalogAPI interface {
	SearchJobs(context.Context, models.JobSearchRequest) (models.JobSearchResponse, error)
	SearchJobPage(context.Context, models.JobSearchRequest) (models.JobPage, error)
	SearchFacets(context.Context, models.JobSearchRequest) (models.FacetSet, error)
	GetFilterMetadata(context.Context) (service.FilterMetadata, error)
	GetJob(context.Context, string) (models.Job, error)
	SearchCompanies(context.Context, string, *string, int) (service.CompanySearchResponse, error)
	SuggestLocations(context.Context, string, string, int) (service.LocationSuggestions, error)
	RecentJobs(context.Context, int) (models.JobPage, error)
	ListSources(context.Context, int) ([]models.SourceSummary, error)
	GetSourceHealth(context.Context, string) (*models.SourceHealth, error)
}

type CatalogHandler struct {
	api CatalogAPI
}

func NewCatalogHandler(api CatalogAPI) (*CatalogHandler, error) {
	if api == nil {
		return nil, errors.New("catalog API is required")
	}
	return &CatalogHandler{api: api}, nil
}

func (h *CatalogHandler) Jobs(w http.ResponseWriter, r *http.Request) {
	request, err := parseJobSearchQuery(r.URL.Query())
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	h.resolveCompanyNames(r.Context(), &request)
	page, err := h.api.SearchJobPage(r.Context(), request)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *CatalogHandler) SearchJobs(w http.ResponseWriter, r *http.Request) {
	var request models.JobSearchRequest
	if r.Method == http.MethodPost {
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_request", "The job search request is invalid.")
			return
		}
	} else {
		var err error
		request, err = parseJobSearchQuery(r.URL.Query())
		if err != nil {
			h.writeCatalogError(w, r, err)
			return
		}
	}
	h.resolveCompanyNames(r.Context(), &request)
	response, err := h.api.SearchJobs(r.Context(), request)
	if err != nil {
		fmt.Printf("ERROR SearchJobs failed: %v\n", err)
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *CatalogHandler) resolveCompanyNames(ctx context.Context, request *models.JobSearchRequest) {
	if len(request.CompanyIDs) == 0 {
		return
	}
	var resolved []string
	for _, cid := range request.CompanyIDs {
		trimmed := strings.TrimSpace(cid)
		if isUUID(trimmed) {
			resolved = append(resolved, strings.ToLower(trimmed))
		} else if trimmed != "" {
			compRes, err := h.api.SearchCompanies(ctx, trimmed, nil, 1)
			if err == nil && len(compRes.Items) > 0 {
				resolved = append(resolved, compRes.Items[0].ID)
			}
		}
	}
	request.CompanyIDs = resolved
}

func (h *CatalogHandler) Filters(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		metadata, err := h.api.GetFilterMetadata(r.Context())
		if err != nil {
			h.writeCatalogError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=1800")
		writeJSON(w, http.StatusOK, metadata)
		return
	}
	var request models.JobSearchRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The filter request is invalid.")
		return
	}
	facets, err := h.api.SearchFacets(r.Context(), request)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (h *CatalogHandler) Job(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	if !isUUID(jobID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The job ID is invalid.")
		return
	}
	job, err := h.api.GetJob(r.Context(), jobID)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h *CatalogHandler) Companies(w http.ResponseWriter, r *http.Request) {
	query, err := scalarQuery(r.URL.Query(), "q")
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	cursor, err := optionalCursor(r.URL.Query())
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	limit, err := queryLimit(r.URL.Query())
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	response, err := h.api.SearchCompanies(r.Context(), query, cursor, limit)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *CatalogHandler) Locations(w http.ResponseWriter, r *http.Request) {
	query, err := scalarQuery(r.URL.Query(), "q")
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	country, err := scalarQuery(r.URL.Query(), "country_code")
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	limit, err := queryLimit(r.URL.Query())
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	response, err := h.api.SuggestLocations(r.Context(), query, country, limit)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *CatalogHandler) RecentJobs(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	page, err := h.api.RecentJobs(r.Context(), limit)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *CatalogHandler) Sources(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	sources, err := h.api.ListSources(r.Context(), limit)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": sources,
		"total": len(sources),
	})
}

func (h *CatalogHandler) SourceHealth(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("sourceID")
	if sourceID == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The source ID is invalid.")
		return
	}
	health, err := h.api.GetSourceHealth(r.Context(), sourceID)
	if err != nil {
		h.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (h *CatalogHandler) writeCatalogError(w http.ResponseWriter, r *http.Request, err error) {
	var validation service.JobSearchValidationError
	if errors.As(err, &validation) {
		details := make([]map[string]any, 0, len(validation.Issues))
		for _, issue := range validation.Issues {
			details = append(details, map[string]any{"field": issue.Field, "code": issue.Code, "message": issue.Message})
		}
		writeErrorDetails(w, r, http.StatusBadRequest, "invalid_filter", "One or more job filters are invalid.", details)
		return
	}
	var queryValidation queryValidationError
	if errors.As(err, &queryValidation) {
		details := []map[string]any{{"field": queryValidation.field, "code": queryValidation.code, "message": queryValidation.message}}
		writeErrorDetails(w, r, http.StatusBadRequest, "invalid_filter", "One or more job filters are invalid.", details)
		return
	}
	switch {
	case errors.Is(err, service.ErrJobSearchInvalid):
		writeError(w, r, http.StatusBadRequest, "invalid_filter", "One or more search parameters are invalid.")
	case errors.Is(err, service.ErrCatalogItemNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "Job discovery could not be completed.")
	}
}

type queryValidationError struct {
	field, code, message string
}

func (e queryValidationError) Error() string { return "invalid query parameter" }

func parseJobSearchQuery(query url.Values) (models.JobSearchRequest, error) {
	request := models.JobSearchRequest{}
	var err error
	var jobIDs []string
	for _, key := range []string{"job_id", "job_ids", "id"} {
		if ids := cleanSlice(query[key]); len(ids) > 0 {
			jobIDs = append(jobIDs, ids...)
		}
	}
	if len(jobIDs) > 0 {
		request.JobIDs = jobIDs
	}
	request.Query, err = scalarQuery(query, "q")
	if err != nil {
		return request, err
	}
	if request.Query == "" {
		request.Query, err = scalarQuery(query, "query")
		if err != nil {
			return request, err
		}
	}
	request.ExcludeTerms = cleanSlice(query["exclude"])
	datePreset, err := scalarQuery(query, "date_preset")
	if err != nil {
		return request, err
	}
	if datePreset != "" {
		request.Date.Preset = models.DatePreset(datePreset)
	}
	request.WorkplaceModes = castValues[models.WorkplaceMode](query["workplace"])
	if len(request.WorkplaceModes) == 0 {
		request.WorkplaceModes = castValues[models.WorkplaceMode](query["workplace_mode"])
	}
	request.CompanyIDs = cleanSlice(query["company_id"])
	if len(request.CompanyIDs) == 0 {
		request.CompanyIDs = cleanSlice(query["company"])
	}
	if len(request.CompanyIDs) == 0 {
		request.CompanyIDs = cleanSlice(query["companies"])
	}
	request.ExcludedCompanyIDs = cleanSlice(query["exclude_company_id"])
	request.DegreeLevels = castValues[models.DegreeLevel](query["degree_level"])
	visaSponsorship, err := scalarQuery(query, "visa_sponsorship")
	if err != nil {
		return request, err
	}
	if visaSponsorship != "" {
		request.VisaSponsorship = models.VisaSponsorship(visaSponsorship)
	}
	request.RoleIDs = cleanSlice(query["role_id"])
	request.JobTypes = castValues[models.JobType](query["job_type"])
	request.EmploymentTypes = castValues[models.EmploymentType](query["employment_type"])

	if request.Date.PublishedAfter, err = queryTime(query, "published_after"); err != nil {
		return request, err
	}
	if request.Date.PublishedBefore, err = queryTime(query, "published_before"); err != nil {
		return request, err
	}
	if request.Date.FirstSeenAfter, err = queryTime(query, "first_seen_after"); err != nil {
		return request, err
	}
	if request.Date.IncludeUnknownPublicationTime, err = queryBool(query, "include_unknown_publication_time"); err != nil {
		return request, err
	}
	if request.IncludeUnknownLocation, err = queryBool(query, "include_unknown_location"); err != nil {
		return request, err
	}
	if request.IncludeUnknownCompany, err = queryBool(query, "include_unknown_company"); err != nil {
		return request, err
	}
	if request.IncludeUnknownDegree, err = queryBool(query, "include_unknown_degree"); err != nil {
		return request, err
	}
	if request.IncludeUnknownWorkplace, err = queryBool(query, "include_unknown_workplace"); err != nil {
		return request, err
	}
	if request.IncludeUnknownRole, err = queryBool(query, "include_unknown_role"); err != nil {
		return request, err
	}
	if request.IncludeUnknownJobType, err = queryBool(query, "include_unknown_job_type"); err != nil {
		return request, err
	}
	if request.IncludeUnknownEmploymentType, err = queryBool(query, "include_unknown_employment_type"); err != nil {
		return request, err
	}
	if request.IncludeUnknownExperience, err = queryBool(query, "include_unknown_experience"); err != nil {
		return request, err
	}
	if request.DegreeRequirement, err = queryEnum[models.DegreeRequirement](query, "degree_requirement"); err != nil {
		return request, err
	}
	if request.MaxExperienceYears, err = queryOptionalInt(query, "max_experience_years"); err != nil {
		return request, err
	}
	if request.Cursor, err = optionalCursor(query); err != nil {
		return request, err
	}
	if request.Limit, err = queryLimit(query); err != nil {
		return request, err
	}
	if request.Sort, err = queryEnum[models.JobSort](query, "sort"); err != nil {
		return request, err
	}
	if request.IncludeFacets, err = queryBool(query, "include_facets"); err != nil {
		return request, err
	}
	locations := cleanSlice(query["location"])
	if len(locations) > 10 {
		return request, queryValidationError{field: "location", code: "max_items", message: "must contain at most 10 items"}
	}
	radius, radiusErr := queryOptionalInt(query, "radius_km")
	if radiusErr != nil {
		return request, radiusErr
	}
	if len(locations) > 0 {
		if radius != nil {
			request.Locations = make([]models.LocationFilter, 0, len(locations))
			for _, locationID := range locations {
				radiusCopy := *radius
				request.Locations = append(request.Locations, models.LocationFilter{LocationID: locationID, RadiusKM: &radiusCopy})
			}
		} else {
			request.LocationIDs = append([]string(nil), locations...)
		}
	} else if radius != nil {
		return request, queryValidationError{field: "radius_km", code: "location_required", message: "requires at least one location filter"}
	}
	return request, nil
}

func scalarQuery(query url.Values, key string) (string, error) {
	values := query[key]
	if len(values) > 1 {
		return "", queryValidationError{field: key, code: "repeated_scalar", message: "must appear at most once"}
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

func queryTime(query url.Values, key string) (*time.Time, error) {
	value, err := scalarQuery(query, key)
	if err != nil || value == "" {
		return nil, err
	}
	parsed, parseErr := time.Parse(time.RFC3339, value)
	if parseErr != nil {
		return nil, queryValidationError{field: key, code: "invalid_date_time", message: "must be an RFC3339 date-time"}
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func queryBool(query url.Values, key string) (bool, error) {
	value, err := scalarQuery(query, key)
	if err != nil || value == "" {
		return false, err
	}
	parsed, parseErr := strconv.ParseBool(value)
	if parseErr != nil {
		return false, queryValidationError{field: key, code: "invalid_boolean", message: "must be true or false"}
	}
	return parsed, nil
}

func queryOptionalInt(query url.Values, key string) (*int, error) {
	value, err := scalarQuery(query, key)
	if err != nil || value == "" {
		return nil, err
	}
	parsed, parseErr := strconv.Atoi(value)
	if parseErr != nil {
		return nil, queryValidationError{field: key, code: "invalid_integer", message: "must be an integer"}
	}
	return &parsed, nil
}

func queryLimit(query url.Values) (int, error) {
	value, err := scalarQuery(query, "limit")
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 25, nil
	}
	parsed, parseErr := strconv.Atoi(value)
	if parseErr != nil {
		return 0, queryValidationError{field: "limit", code: "invalid_integer", message: "must be an integer"}
	}
	return parsed, nil
}

func optionalCursor(query url.Values) (*string, error) {
	value, err := scalarQuery(query, "cursor")
	if err != nil || value == "" {
		return nil, err
	}
	return &value, nil
}

func queryEnum[T ~string](query url.Values, key string) (T, error) {
	value, err := scalarQuery(query, key)
	if err != nil {
		var zero T
		return zero, err
	}
	return T(value), nil
}

func cleanSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func castValues[T ~string](values []string) []T {
	if len(values) == 0 {
		return nil
	}
	result := make([]T, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, T(trimmed))
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

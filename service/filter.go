package service

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/oneRole/models"
)

// ValidationError describes one invalid request field using a stable field
// path and machine-readable code suitable for the API error envelope.
type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// NormalizeJobSearchRequest applies defaults and deterministic normalization,
// then reports all contract violations. It never maps absent/unknown values to
// a guessed category. Repositories can implement the actual AND/OR query using
// this validated request.
func NormalizeJobSearchRequest(req models.JobSearchRequest) (models.JobSearchRequest, []ValidationError) {
	var errs []ValidationError
	add := func(field, code, message string) {
		errs = append(errs, ValidationError{Field: field, Code: code, Message: message})
	}

	if req.JobID != "" {
		req.JobIDs = append(req.JobIDs, req.JobID)
		req.JobID = ""
	}
	if req.ID != "" {
		req.JobIDs = append(req.JobIDs, req.ID)
		req.ID = ""
	}

	req.Query = strings.TrimSpace(req.Query)
	if len([]rune(req.Query)) > 200 {
		add("query", "max_length", "must be at most 200 characters")
	}

	if len(req.ExcludeTerms) > 30 {
		add("exclude_terms", "max_items", "must contain at most 30 items")
	}
	req.ExcludeTerms = normalizeStrings(req.ExcludeTerms)
	for i, term := range req.ExcludeTerms {
		if term == "" || len([]rune(term)) > 100 {
			add(fmt.Sprintf("exclude_terms[%d]", i), "invalid_length", "must contain 1 to 100 characters")
		}
	}

	if req.Date.Preset == "" {
		req.Date.Preset = models.DateAny
	}
	if !oneOf(string(req.Date.Preset), string(models.DateLast6Hours), string(models.DateLast24Hours), string(models.DateLast7Days), string(models.DateLast30Days), string(models.DateLast60Days), string(models.DateCustom), string(models.DateAny)) {
		add("date.preset", "invalid_enum", "is not a supported date preset")
	}
	normalizeTime := func(field string, value *time.Time) {
		if value == nil {
			return
		}
		if value.IsZero() {
			add(field, "invalid_time", "must be a non-zero date-time")
			return
		}
		utc := value.UTC()
		valueCopy := utc
		if field == "date.published_after" {
			req.Date.PublishedAfter = &valueCopy
		} else if field == "date.published_before" {
			req.Date.PublishedBefore = &valueCopy
		} else {
			req.Date.FirstSeenAfter = &valueCopy
		}
	}
	normalizeTime("date.published_after", req.Date.PublishedAfter)
	normalizeTime("date.published_before", req.Date.PublishedBefore)
	normalizeTime("date.first_seen_after", req.Date.FirstSeenAfter)
	if req.Date.PublishedAfter != nil && req.Date.PublishedBefore != nil &&
		!req.Date.PublishedAfter.IsZero() && !req.Date.PublishedBefore.IsZero() && req.Date.PublishedAfter.After(*req.Date.PublishedBefore) {
		add("date", "invalid_time_order", "published_after must be earlier than or equal to published_before")
	}

	if len(req.LocationIDs) > 10 {
		add("location_ids", "max_items", "must contain at most 10 items")
	}
	req.LocationIDs = normalizeStrings(req.LocationIDs)
	for i, id := range req.LocationIDs {
		if len([]rune(id)) > 120 {
			add(fmt.Sprintf("location_ids[%d]", i), "max_length", "must be at most 120 characters")
		}
	}
	if len(req.Locations) > 10 {
		add("locations", "max_items", "must contain at most 10 items")
	}
	if req.Locations != nil {
		req.Locations = append([]models.LocationFilter(nil), req.Locations...)
	}
	for i := range req.Locations {
		loc := &req.Locations[i]
		loc.LocationID = strings.TrimSpace(loc.LocationID)
		loc.CountryCode = strings.ToUpper(strings.TrimSpace(loc.CountryCode))
		loc.Region = strings.TrimSpace(loc.Region)
		loc.City = strings.TrimSpace(loc.City)
		prefix := fmt.Sprintf("locations[%d]", i)
		if len([]rune(loc.LocationID)) > 120 {
			add(prefix+".location_id", "max_length", "must be at most 120 characters")
		}
		if loc.CountryCode != "" && len(loc.CountryCode) != 2 {
			add(prefix+".country_code", "invalid_length", "must be a two-letter country code")
		}
		if len([]rune(loc.Region)) > 100 {
			add(prefix+".region", "max_length", "must be at most 100 characters")
		}
		if len([]rune(loc.City)) > 100 {
			add(prefix+".city", "max_length", "must be at most 100 characters")
		}
		if loc.RadiusKM != nil && (*loc.RadiusKM < 1 || *loc.RadiusKM > 500) {
			add(prefix+".radius_km", "out_of_range", "must be between 1 and 500 kilometers")
		}
	}

	req.WorkplaceModes = normalizeEnums(req.WorkplaceModes)
	validateEnums("workplace_modes", req.WorkplaceModes,
		[]models.WorkplaceMode{models.WorkplaceRemote, models.WorkplaceHybrid, models.WorkplaceOnSite, models.WorkplaceUnknown}, add)

	for _, group := range []struct {
		field string
		items *[]string
		max   int
		uuid  bool
	}{
		{"job_ids", &req.JobIDs, 100, true},
		{"company_ids", &req.CompanyIDs, 100, true},
		{"excluded_company_ids", &req.ExcludedCompanyIDs, 100, true},
		{"role_ids", &req.RoleIDs, 30, false},
	} {
		if len(*group.items) > group.max {
			add(group.field, "max_items", fmt.Sprintf("must contain at most %d items", group.max))
		}
		*group.items = normalizeStrings(*group.items)
		for i, value := range *group.items {
			path := fmt.Sprintf("%s[%d]", group.field, i)
			if group.uuid {
				if !isUUID(value) {
					add(path, "invalid_uuid", "must be a UUID")
				} else {
					(*group.items)[i] = strings.ToLower(value)
				}
			} else if len([]rune(value)) > 80 {
				add(path, "max_length", "must be at most 80 characters")
			}
		}
	}

	req.DegreeLevels = normalizeEnums(req.DegreeLevels)
	validateEnums("degree_levels", req.DegreeLevels,
		[]models.DegreeLevel{models.DegreeHighSchool, models.DegreeAssociate, models.DegreeBachelor, models.DegreeMaster, models.DegreeDoctorate, models.DegreeNoDegreeStated, models.DegreeUnknown}, add)
	if req.DegreeRequirement == "" {
		req.DegreeRequirement = models.DegreeAny
	}
	if !oneOf(string(req.DegreeRequirement), string(models.DegreeRequired), string(models.DegreePreferred), string(models.DegreeAny)) {
		add("degree_requirement", "invalid_enum", "must be required, preferred, or any")
	}
	if req.MaxExperienceYears != nil && (*req.MaxExperienceYears < 0 || *req.MaxExperienceYears > 50) {
		add("max_experience_years", "out_of_range", "must be between 0 and 50")
	}
	if req.VisaSponsorship == "" {
		req.VisaSponsorship = models.VisaAny
	}
	if !oneOf(string(req.VisaSponsorship), string(models.VisaYes), string(models.VisaNo), string(models.VisaUnknown), string(models.VisaAny)) {
		add("visa_sponsorship", "invalid_enum", "must be yes, no, unknown, or any")
	}

	req.JobTypes = normalizeEnums(req.JobTypes)
	validateEnums("job_types", req.JobTypes,
		[]models.JobType{models.JobInternship, models.JobEntryLevel, models.JobAssociate, models.JobMidLevel, models.JobSenior, models.JobLead, models.JobManager, models.JobDirector, models.JobExecutive}, add)
	req.EmploymentTypes = normalizeEnums(req.EmploymentTypes)
	validateEnums("employment_types", req.EmploymentTypes,
		[]models.EmploymentType{models.EmploymentFullTime, models.EmploymentPartTime, models.EmploymentContract, models.EmploymentTemporary, models.EmploymentSeasonal, models.EmploymentFreelance, models.EmploymentInternship, models.EmploymentApprenticeship, models.EmploymentOther}, add)

	if req.Sort == "" {
		req.Sort = models.SortRelevance
	}
	if !oneOf(string(req.Sort), string(models.SortRelevance), string(models.SortPublishedDesc), string(models.SortFirstSeenDesc), string(models.SortSalaryDesc)) {
		add("sort", "invalid_enum", "is not a supported sort order")
	}
	if req.Cursor != nil {
		trimmed := strings.TrimSpace(*req.Cursor)
		if trimmed == "" {
			req.Cursor = nil
		} else {
			req.Cursor = &trimmed
		}
	}
	if req.Limit == 0 {
		req.Limit = 25
	} else if req.Limit < 1 || req.Limit > 100 {
		add("limit", "out_of_range", "must be between 1 and 100")
	}

	return req, errs
}

// MatchesCategoricalFilter applies OR semantics within one selected facet.
// An empty selection leaves the facet unrestricted. If a selection is active,
// records without a known value match only when includeUnknown is true.
func MatchesCategoricalFilter[T comparable](selected, actual []T, includeUnknown bool) bool {
	if len(selected) == 0 {
		return true
	}
	if len(actual) == 0 {
		return includeUnknown
	}
	for _, want := range selected {
		for _, got := range actual {
			if want == got {
				return true
			}
		}
	}
	return false
}

// MatchesJobDateFilter applies publication recency and first-seen criteria.
// Unknown employer publication dates remain a separate bucket; the
// include_unknown_publication_time flag matters only when a publication window
// is active. FirstSeenAfter is always evaluated independently.
func MatchesJobDateFilter(job models.Job, filter models.DateFilter, now time.Time) bool {
	now = now.UTC()
	var publishedAfter, publishedBefore *time.Time
	if filter.Preset == "" || filter.Preset == models.DateAny {
		// No preset window; explicit custom bounds below still apply.
	} else {
		var window time.Duration
		switch filter.Preset {
		case models.DateLast6Hours:
			window = 6 * time.Hour
		case models.DateLast24Hours:
			window = 24 * time.Hour
		case models.DateLast7Days:
			window = 7 * 24 * time.Hour
		case models.DateLast30Days:
			window = 30 * 24 * time.Hour
		case models.DateLast60Days:
			window = 60 * 24 * time.Hour
		case models.DateCustom:
		default:
			return false
		}
		if window > 0 {
			lower := now.Add(-window)
			publishedAfter = &lower
			publishedBefore = &now
		}
	}
	if filter.PublishedAfter != nil {
		if publishedAfter == nil || filter.PublishedAfter.After(*publishedAfter) {
			publishedAfter = filter.PublishedAfter
		}
	}
	if filter.PublishedBefore != nil {
		if publishedBefore == nil || filter.PublishedBefore.Before(*publishedBefore) {
			publishedBefore = filter.PublishedBefore
		}
	}

	publicationWindowActive := publishedAfter != nil || publishedBefore != nil
	publicationUnknown := job.PublishedAt == nil || job.PublicationTimeQuality == models.PublicationUnknown
	if publicationUnknown {
		if publicationWindowActive && !filter.IncludeUnknownPublicationTime {
			return false
		}
	} else {
		publishedAt := job.PublishedAt.UTC()
		if publishedAfter != nil && publishedAt.Before(*publishedAfter) {
			return false
		}
		if publishedBefore != nil && publishedAt.After(*publishedBefore) {
			return false
		}
	}
	if filter.FirstSeenAfter != nil && job.FirstSeenAt.Before(filter.FirstSeenAfter.UTC()) {
		return false
	}
	return true
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeEnums[T ~string](values []T) []T {
	out := make([]T, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(string(value)))
		if trimmed == "" {
			continue
		}
		normalized := T(trimmed)
		if _, exists := seen[string(normalized)]; exists {
			continue
		}
		seen[string(normalized)] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func validateEnums[T ~string](field string, values, allowed []T, add func(string, string, string)) {
	for i, value := range values {
		valid := false
		for _, option := range allowed {
			if value == option {
				valid = true
				break
			}
		}
		if !valid {
			add(fmt.Sprintf("%s[%d]", field, i), "invalid_enum", "is not a supported value")
		}
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, option := range allowed {
		if value == option {
			return true
		}
	}
	return false
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUID(value string) bool { return uuidPattern.MatchString(value) }

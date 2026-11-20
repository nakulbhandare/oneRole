package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oneRole/db"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DiscoveryRepositories struct {
	Jobs      *JobSearchRepository
	Companies *CompanySearchRepository
	Locations *LocationSuggestionRepository
	Filters   *FilterRepository
}

func NewDiscoveryRepositories(database *mongo.Database) DiscoveryRepositories {
	return DiscoveryRepositories{
		Jobs:      NewJobSearchRepository(database),
		Companies: NewCompanySearchRepository(database),
		Locations: NewLocationSuggestionRepository(database),
		Filters:   NewFilterRepository(database),
	}
}

type JobSearchRepository struct{ db *mongo.Database }

func NewJobSearchRepository(database *mongo.Database) *JobSearchRepository {
	return &JobSearchRepository{db: database}
}

type searchPosition struct {
	Sort      string   `json:"sort"`
	ID        string   `json:"id"`
	Published *string  `json:"published,omitempty"`
	FirstSeen string   `json:"first_seen,omitempty"`
	Salary    *string  `json:"salary,omitempty"`
	Rank      *float64 `json:"rank,omitempty"`
}

type jobDoc struct {
	ID                     string                             `bson:"_id"`
	CompanyID              *string                            `bson:"company_id,omitempty"`
	CompanyName            string                             `bson:"company_name"`
	CanonicalTitle         string                             `bson:"canonical_title"`
	Title                  string                             `bson:"title,omitempty"`
	DescriptionText        string                             `bson:"description_text"`
	DescriptionHTML        string                             `bson:"description_html,omitempty"`
	Description            string                             `bson:"description,omitempty"`
	DescriptionPlain       string                             `bson:"description_plain,omitempty"`
	DescriptionMarkdown    string                             `bson:"description_markdown,omitempty"`
	ResponsibilitiesText   string                             `bson:"responsibilities_text"`
	QualificationsText     string                             `bson:"qualifications_text"`
	DisplayLocation        string                             `bson:"display_location"`
	Location               string                             `bson:"location,omitempty"`
	Country                *string                            `bson:"country,omitempty"`
	CountryCode            *string                            `bson:"country_code,omitempty"`
	Region                 *string                            `bson:"region,omitempty"`
	State                  *string                            `bson:"state,omitempty"`
	City                   *string                            `bson:"city,omitempty"`
	NormalizedLocationID   *string                            `bson:"normalized_location_id,omitempty"`
	WorkplaceModes         []string                           `bson:"workplace_modes"`
	RoleIDs                []string                           `bson:"role_ids"`
	JobType                *string                            `bson:"job_type,omitempty"`
	EmploymentTypes        []string                           `bson:"employment_types"`
	DegreeLevels           []string                           `bson:"degree_levels"`
	DegreeRequirement      *string                            `bson:"degree_requirement,omitempty"`
	MinExperienceYears     *float64                           `bson:"min_experience_years,omitempty"`
	MaxExperienceYears     *float64                           `bson:"max_experience_years,omitempty"`
	VisaSponsorship        string                             `bson:"visa_sponsorship"`
	SourcePublishedAt      *time.Time                         `bson:"source_published_at,omitempty"`
	PublicationTimeQuality string                             `bson:"publication_time_quality"`
	FirstSeenAt            time.Time                          `bson:"first_seen_at"`
	LastSeenAt             time.Time                          `bson:"last_seen_at"`
	ApplicationCapability  string                             `bson:"application_capability"`
	SourceURL              string                             `bson:"source_url"`
	SalaryMax              *float64                           `bson:"salary_max,omitempty"`
	Status                 string                             `bson:"status"`
	FilterProvenance       map[string]models.FilterProvenance `bson:"filter_provenance,omitempty"`
	Score                  float64                            `bson:"score,omitempty"`
}

func (r *JobSearchRepository) SearchJobs(ctx context.Context, request models.JobSearchRequest) (models.JobSearchResponse, error) {
	if r == nil || r.db == nil {
		return models.JobSearchResponse{}, db.ErrNilDB
	}
	request, validationErrors := service.NormalizeJobSearchRequest(request)
	if len(validationErrors) != 0 {
		return models.JobSearchResponse{}, ErrInvalidSearchRequest
	}

	if len(request.CompanyIDs) > 0 {
		var resolvedIDs []string
		var namesToLookup []string
		uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
		for _, cid := range request.CompanyIDs {
			trimmed := strings.TrimSpace(cid)
			if uuidRegex.MatchString(trimmed) {
				resolvedIDs = append(resolvedIDs, trimmed)
			} else if trimmed != "" {
				namesToLookup = append(namesToLookup, trimmed)
			}
		}
		if len(namesToLookup) > 0 {
			var orConditions []bson.M
			for _, name := range namesToLookup {
				pat := primitive.Regex{Pattern: "^" + regexp.QuoteMeta(name) + "$", Options: "i"}
				orConditions = append(orConditions, bson.M{"canonical_name": pat}, bson.M{"name": pat})
			}
			cur, err := r.db.Collection("companies").Find(ctx, bson.M{"$or": orConditions})
			if err == nil {
				type cDoc struct {
					ID string `bson:"_id"`
				}
				var found []cDoc
				if err := cur.All(ctx, &found); err == nil {
					for _, f := range found {
						if f.ID != "" {
							resolvedIDs = append(resolvedIDs, f.ID)
						}
					}
				}
			}
		}
		if len(resolvedIDs) > 0 {
			request.CompanyIDs = resolvedIDs
		}
	}

	var esHighlights map[string]map[string][]string
	hasSearchQuery := strings.TrimSpace(request.Query) != ""
	// Elasticsearch High-Precision Full-Text & Multi-Filter Search
	hasFilters := hasSearchQuery || len(request.WorkplaceModes) > 0 || len(request.LocationIDs) > 0 || len(request.Locations) > 0 || len(request.CompanyIDs) > 0
	if hasFilters {
		esIDs, highlights, err := queryElasticsearchJobIDs(ctx, request)
		if err == nil {
			if len(esIDs) > 0 {
				if hasSearchQuery && len(esIDs) > request.Limit+1 {
					esIDs = esIDs[:request.Limit+1]
				}
				request.JobIDs = esIDs
				esHighlights = highlights
			} else {
				// Exact query or filter criteria matched 0 jobs in Elasticsearch - return empty response immediately
				return models.JobSearchResponse{
					Items:          []models.Job{},
					AppliedFilters: request,
					Facets:         models.FacetSet{},
				}, nil
			}
		}
	}

	filterForHash := request
	filterForHash.Cursor = nil
	filterForHash.Limit = 0
	filterForHash.IncludeFacets = false
	filterHash, err := filterDigest("jobs", filterForHash)
	if err != nil {
		return models.JobSearchResponse{}, err
	}

	effectiveSort := effectiveJobSort(request)
	var position *searchPosition
	if request.Cursor != nil {
		var parsed searchPosition
		if err := decodeOpaqueCursor(*request.Cursor, "jobs", filterHash, &parsed); err != nil || parsed.Sort != string(effectiveSort) || parsed.ID == "" {
			return models.JobSearchResponse{}, ErrInvalidCursor
		}
		position = &parsed
	}

	now := time.Now().UTC()
	queryFilter := buildMongoJobFilter(request, now, "")

	if position != nil {
		cursorCond := buildMongoCursorCondition(effectiveSort, *position)
		if len(cursorCond) > 0 {
			if ands, ok := queryFilter["$and"].([]bson.M); ok {
				queryFilter["$and"] = append(ands, cursorCond)
			} else {
				queryFilter = bson.M{"$and": []bson.M{queryFilter, cursorCond}}
			}
		}
	}

	limit := int64(request.Limit + 1)
	findOpts := options.Find().SetLimit(limit)

	if hasSearchQuery && len(request.JobIDs) > 0 {
		findOpts.SetSort(nil)
	} else {
		switch effectiveSort {
		case models.SortPublishedDesc:
			findOpts.SetSort(bson.D{{Key: "source_published_at", Value: -1}, {Key: "_id", Value: -1}})
		case models.SortSalaryDesc:
			findOpts.SetSort(bson.D{{Key: "salary_max", Value: -1}, {Key: "_id", Value: -1}})
		case models.SortRelevance:
			hasText := strings.TrimSpace(request.Query) != "" && len(request.JobIDs) == 0
			if hasText {
				findOpts.SetSort(bson.D{
					{Key: "score", Value: bson.M{"$meta": "textScore"}},
					{Key: "first_seen_at", Value: -1},
					{Key: "_id", Value: -1},
				})
				findOpts.SetProjection(bson.M{"score": bson.M{"$meta": "textScore"}})
			} else {
				findOpts.SetSort(bson.D{
					{Key: "first_seen_at", Value: -1},
					{Key: "_id", Value: -1},
				})
			}
		default:
			findOpts.SetSort(bson.D{{Key: "first_seen_at", Value: -1}, {Key: "_id", Value: -1}})
		}
	}

	coll := r.db.Collection("jobs")
	fmt.Printf("DEBUG queryFilter: %+v\n", queryFilter)
	cursor, err := coll.Find(ctx, queryFilter, findOpts)
	if err != nil {
		return models.JobSearchResponse{}, fmt.Errorf("search jobs: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []jobDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return models.JobSearchResponse{}, fmt.Errorf("read job search results: %w", err)
	}

	more := len(docs) > request.Limit
	if more {
		docs = docs[:request.Limit]
	}

	jobs := make([]models.Job, 0, len(docs))
	positions := make([]searchPosition, 0, len(docs))
	for _, doc := range docs {
		job, pos := mapDocToJob(doc, effectiveSort)
		if esHighlights != nil {
			if hl, ok := esHighlights[job.ID]; ok {
				job.Highlights = hl
			}
		}
		jobs = append(jobs, job)
		positions = append(positions, pos)
	}

	if len(request.JobIDs) > 0 && (hasSearchQuery || effectiveSort == models.SortRelevance) {
		orderMap := make(map[string]int, len(request.JobIDs))
		for idx, id := range request.JobIDs {
			orderMap[id] = idx
		}
		sort.SliceStable(jobs, func(i, j int) bool {
			return orderMap[jobs[i].ID] < orderMap[jobs[j].ID]
		})
	} else if effectiveSort == models.SortPublishedDesc || effectiveSort == models.SortFirstSeenDesc {
		sort.SliceStable(jobs, func(i, j int) bool {
			timeI := jobs[i].FirstSeenAt
			if jobs[i].PublishedAt != nil {
				timeI = *jobs[i].PublishedAt
			}
			timeJ := jobs[j].FirstSeenAt
			if jobs[j].PublishedAt != nil {
				timeJ = *jobs[j].PublishedAt
			}
			return timeI.After(timeJ)
		})
	}

	var facets models.FacetSet
	if request.IncludeFacets {
		var err error
		facets, err = r.computeFacets(ctx, request, now)
		if err != nil {
			return models.JobSearchResponse{}, err
		}
	} else {
		facets = models.FacetSet{}
	}

	var nextCursor *string
	if more && len(positions) > 0 {
		value, err := encodeOpaqueCursor("jobs", filterHash, positions[len(positions)-1])
		if err != nil {
			return models.JobSearchResponse{}, err
		}
		nextCursor = &value
	}

	return models.JobSearchResponse{
		Items:          jobs,
		AppliedFilters: request,
		Facets:         facets,
		NextCursor:     nextCursor,
	}, nil
}

func mapDocToJob(doc jobDoc, sort models.JobSort) (models.Job, searchPosition) {
	loc := doc.DisplayLocation
	if loc == "" {
		loc = doc.Location
	}
	if loc == "" && doc.City != nil && *doc.City != "" {
		loc = *doc.City
		if doc.State != nil && *doc.State != "" {
			loc += ", " + *doc.State
		} else if doc.Region != nil && *doc.Region != "" {
			loc += ", " + *doc.Region
		}
	}
	if loc == "" {
		for _, wm := range doc.WorkplaceModes {
			if strings.EqualFold(wm, "remote") {
				loc = "Remote"
				break
			}
		}
	}
	region := doc.Region
	if region == nil {
		region = doc.State
	}
	countryCode := doc.CountryCode
	if countryCode == nil {
		countryCode = doc.Country
	}
	desc := doc.DescriptionText
	if desc == "" {
		desc = doc.Description
	}
	if desc == "" {
		desc = doc.DescriptionPlain
	}
	if desc == "" {
		desc = doc.DescriptionMarkdown
	}
	if desc == "" {
		desc = doc.DescriptionHTML
	}
	if desc == "" && (doc.ResponsibilitiesText != "" || doc.QualificationsText != "") {
		desc = doc.ResponsibilitiesText + "\n\n" + doc.QualificationsText
	}

	jobTitle := doc.CanonicalTitle
	if jobTitle == "" {
		jobTitle = doc.Title
	}

	job := models.Job{
		ID:                     doc.ID,
		CompanyID:              doc.CompanyID,
		Company:                doc.CompanyName,
		Title:                  jobTitle,
		Description:            desc,
		DescriptionHTML:        doc.DescriptionHTML,
		ResponsibilitiesText:   doc.ResponsibilitiesText,
		QualificationsText:     doc.QualificationsText,
		Location:               loc,
		CountryCode:            countryCode,
		Region:                 region,
		City:                   doc.City,
		VisaSponsorship:        models.VisaSponsorship(doc.VisaSponsorship),
		PublishedAt:            doc.SourcePublishedAt,
		PublicationTimeQuality: models.PublicationTimeQuality(doc.PublicationTimeQuality),
		FirstSeenAt:            doc.FirstSeenAt.UTC(),
		LastSeenAt:             doc.LastSeenAt.UTC(),
		ApplicationCapability:  validCapability(doc.ApplicationCapability),
		SourceURL:              doc.SourceURL,
		RoleIDs:                doc.RoleIDs,
		FilterProvenance:       doc.FilterProvenance,
	}
	if job.FilterProvenance == nil {
		job.FilterProvenance = make(map[string]models.FilterProvenance)
	}

	if doc.JobType != nil {
		jt := models.JobType(*doc.JobType)
		job.JobType = &jt
	}
	if doc.DegreeRequirement != nil {
		dr := models.DegreeRequirement(*doc.DegreeRequirement)
		job.DegreeRequirement = &dr
	}
	if doc.MinExperienceYears != nil {
		val := int(*doc.MinExperienceYears)
		job.ExperienceMinYears = &val
	}
	if doc.MaxExperienceYears != nil {
		val := int(*doc.MaxExperienceYears)
		job.ExperienceMaxYears = &val
	}

	for _, mode := range doc.WorkplaceModes {
		job.WorkplaceModes = append(job.WorkplaceModes, models.WorkplaceMode(mode))
	}
	if len(job.WorkplaceModes) == 0 {
		job.WorkplaceModes = []models.WorkplaceMode{models.WorkplaceUnknown}
	}
	for _, et := range doc.EmploymentTypes {
		job.EmploymentTypes = append(job.EmploymentTypes, models.EmploymentType(et))
	}
	for _, dl := range doc.DegreeLevels {
		job.DegreeLevels = append(job.DegreeLevels, models.DegreeLevel(dl))
	}

	pos := searchPosition{
		Sort:      string(sort),
		ID:        doc.ID,
		FirstSeen: doc.FirstSeenAt.UTC().Format(time.RFC3339Nano),
	}
	if doc.SourcePublishedAt != nil {
		pub := doc.SourcePublishedAt.UTC().Format(time.RFC3339Nano)
		pos.Published = &pub
	}
	if doc.SalaryMax != nil {
		sal := strconv.FormatFloat(*doc.SalaryMax, 'f', -1, 64)
		pos.Salary = &sal
	}
	if sort == models.SortRelevance {
		pos.Rank = &doc.Score
	}

	return job, pos
}

func queryElasticsearchJobIDs(ctx context.Context, request models.JobSearchRequest) ([]string, map[string]map[string][]string, error) {
	esURL := os.Getenv("ELASTICSEARCH_URL")
	if esURL == "" {
		esURL = "http://localhost:9200"
	}

	qStr := strings.TrimSpace(request.Query)
	mustClauses := []map[string]interface{}{}

	if qStr != "" {
		// Check if user specified exact phrase matching with quotes
		if strings.HasPrefix(qStr, "\"") && strings.HasSuffix(qStr, "\"") && len(qStr) > 2 {
			exactPhrase := qStr[1 : len(qStr)-1]
			mustClauses = append(mustClauses, map[string]interface{}{
				"multi_match": map[string]interface{}{
					"query":  exactPhrase,
					"type":   "phrase",
					"fields": []string{"canonical_title^10", "skills^8", "company_name^3", "description_text^1", "responsibilities_text^1", "qualifications_text^1"},
				},
			})
		} else {
			// Multi-match across analyzed and keyword fields with title & skill boosting
			mustClauses = append(mustClauses, map[string]interface{}{
				"multi_match": map[string]interface{}{
					"query":    qStr,
					"operator": "and",
					"fields":   []string{"canonical_title^10", "skills^8", "company_name^3", "description_text^1", "responsibilities_text^1", "qualifications_text^1"},
				},
			})
		}
	} else {
		mustClauses = append(mustClauses, map[string]interface{}{
			"match_all": map[string]interface{}{},
		})
	}

	filterClauses := []map[string]interface{}{
		{"term": map[string]interface{}{"status": "open"}},
	}

	if len(request.CompanyIDs) > 0 {
		var compShould []map[string]interface{}
		compShould = append(compShould, map[string]interface{}{
			"terms": map[string]interface{}{"company_id": request.CompanyIDs},
		})
		compShould = append(compShould, map[string]interface{}{
			"terms": map[string]interface{}{"company_name.keyword": request.CompanyIDs},
		})
		filterClauses = append(filterClauses, map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               compShould,
				"minimum_should_match": 1,
			},
		})
	}

	if len(request.WorkplaceModes) > 0 {
		var modes []string
		for _, m := range request.WorkplaceModes {
			modes = append(modes, string(m))
		}
		filterClauses = append(filterClauses, map[string]interface{}{
			"terms": map[string]interface{}{"workplace_modes": modes},
		})
	}

	var locStrings []string
	locStrings = append(locStrings, request.LocationIDs...)
	for _, l := range request.Locations {
		if l.City != "" {
			locStrings = append(locStrings, l.City)
		}
		if l.Region != "" {
			locStrings = append(locStrings, l.Region)
		}
		if l.LocationID != "" {
			locStrings = append(locStrings, l.LocationID)
		}
	}

	if len(locStrings) > 0 {
		var locShould []map[string]interface{}
		for _, loc := range locStrings {
			locTrim := strings.TrimSpace(loc)
			if locTrim != "" {
				locShould = append(locShould, map[string]interface{}{
					"multi_match": map[string]interface{}{
						"query":  locTrim,
						"type":   "phrase",
						"fields": []string{"display_location", "location", "location_canonical"},
					},
				})
			}
		}
		if len(locShould) > 0 {
			filterClauses = append(filterClauses, map[string]interface{}{
				"bool": map[string]interface{}{
					"should":               locShould,
					"minimum_should_match": 1,
				},
			})
		}
	}

	mustNotClauses := []map[string]interface{}{}
	for _, term := range request.ExcludeTerms {
		t := strings.TrimSpace(term)
		if t != "" {
			mustNotClauses = append(mustNotClauses, map[string]interface{}{
				"multi_match": map[string]interface{}{
					"query":  t,
					"fields": []string{"canonical_title", "description_text", "responsibilities_text", "qualifications_text"},
				},
			})
		}
	}

	size := 300
	if request.Limit > 0 {
		size = max(request.Limit*3, 200)
	}

	queryBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":     mustClauses,
				"filter":   filterClauses,
				"must_not": mustNotClauses,
				"should": []map[string]interface{}{
					{
						"terms": map[string]interface{}{
							"company_name.keyword": []string{
								"Google", "Meta", "Apple", "Netflix", "Amazon", "Microsoft",
								"OpenAI", "NVIDIA", "Uber", "Stripe", "Datadog", "Snowflake",
								"Docker", "Lyft", "Oracle", "Figma", "Coinbase", "Databricks",
							},
							"boost": 2.5,
						},
					},
				},
			},
		},
		"highlight": map[string]interface{}{
			"pre_tags":  []string{"<mark class=\"search-highlight font-semibold bg-amber-200 text-amber-950 px-1 py-0.5 rounded shadow-2xs\">"},
			"post_tags": []string{"</mark>"},
			"fields": map[string]interface{}{
				"canonical_title":       map[string]interface{}{"number_of_fragments": 0},
				"description_text":      map[string]interface{}{"number_of_fragments": 5, "fragment_size": 250},
				"responsibilities_text": map[string]interface{}{"number_of_fragments": 3, "fragment_size": 200},
				"qualifications_text":   map[string]interface{}{"number_of_fragments": 3, "fragment_size": 200},
				"skills":                map[string]interface{}{"number_of_fragments": 0},
			},
		},
		"size":    size,
		"_source": []string{"id"},
	}

	if qStr != "" {
		queryBody["sort"] = []interface{}{
			map[string]string{"_score": "desc"},
			map[string]interface{}{"first_seen_at": map[string]string{"order": "desc"}},
		}
	} else if request.Sort == models.SortPublishedDesc || request.Sort == models.SortFirstSeenDesc {
		queryBody["sort"] = []interface{}{
			map[string]interface{}{"first_seen_at": map[string]string{"order": "desc"}},
		}
	}

	reqBytes, err := json.Marshal(queryBody)
	if err != nil {
		return nil, nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", esURL+"/onerole_jobs/_search", bytes.NewReader(reqBytes))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("elasticsearch status %d", resp.StatusCode)
	}

	var esResult struct {
		Hits struct {
			Hits []struct {
				ID        string              `json:"_id"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&esResult); err != nil {
		return nil, nil, err
	}

	ids := make([]string, 0, len(esResult.Hits.Hits))
	highlights := make(map[string]map[string][]string, len(esResult.Hits.Hits))
	for _, hit := range esResult.Hits.Hits {
		if hit.ID != "" {
			ids = append(ids, hit.ID)
			if len(hit.Highlight) > 0 {
				highlights[hit.ID] = hit.Highlight
			}
		}
	}
	return ids, highlights, nil
}

func expandSkillSynonyms(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return ""
	}
	synonyms := map[string][]string{
		"golang":     {"go"},
		"go":         {"golang"},
		"k8s":        {"kubernetes"},
		"kubernetes": {"k8s"},
		"js":         {"javascript"},
		"javascript": {"js"},
		"ts":         {"typescript"},
		"typescript": {"ts"},
		"py":         {"python"},
		"python":     {"py"},
		"reactjs":    {"react"},
		"react":      {"reactjs"},
		"nodejs":     {"node"},
		"node":       {"nodejs"},
		"postgres":   {"postgresql"},
		"postgresql": {"postgres"},
		"tf":         {"terraform"},
		"terraform":  {"tf"},
	}

	tokens := strings.Fields(q)
	seen := make(map[string]bool)
	var expanded []string
	for _, t := range tokens {
		lower := strings.ToLower(t)
		if !seen[lower] {
			seen[lower] = true
			expanded = append(expanded, t)
		}
		if syns, ok := synonyms[lower]; ok {
			for _, s := range syns {
				if !seen[s] {
					seen[s] = true
					expanded = append(expanded, s)
				}
			}
		}
	}
	return strings.Join(expanded, " ")
}

func buildMongoJobFilter(request models.JobSearchRequest, now time.Time, omitFacet string) bson.M {
	conditions := []bson.M{
		{"status": "open"},
	}

	if len(request.JobIDs) > 0 {
		conditions = append(conditions, bson.M{
			"_id": bson.M{"$in": request.JobIDs},
		})
	}

	if query := strings.TrimSpace(request.Query); query != "" && len(request.JobIDs) == 0 {
		expandedQuery := expandSkillSynonyms(query)
		conditions = append(conditions, bson.M{
			"$text": bson.M{"$search": expandedQuery},
		})
	}

	for _, term := range request.ExcludeTerms {
		pattern := primitive.Regex{Pattern: regexp.QuoteMeta(strings.TrimSpace(term)), Options: "i"}
		conditions = append(conditions, bson.M{
			"canonical_title":       bson.M{"$not": pattern},
			"description_text":      bson.M{"$not": pattern},
			"responsibilities_text": bson.M{"$not": pattern},
			"qualifications_text":   bson.M{"$not": pattern},
		})
	}

	if omitFacet != "date" {
		addMongoDateFilters(&conditions, request.Date, now)
	} else if request.Date.FirstSeenAfter != nil {
		conditions = append(conditions, bson.M{"first_seen_at": bson.M{"$gte": request.Date.FirstSeenAfter.UTC()}})
	}

	if omitFacet != "workplace" && len(request.WorkplaceModes) > 0 {
		var modes []string
		includeUnknown := request.IncludeUnknownWorkplace
		hasRemote := false
		for _, m := range request.WorkplaceModes {
			if m == models.WorkplaceUnknown {
				includeUnknown = true
			} else {
				modeStr := string(m)
				modes = append(modes, modeStr)
				if modeStr == "onsite" || m == models.WorkplaceOnSite {
					modes = append(modes, "on_site", "onsite")
				}
				if modeStr == "remote" || m == models.WorkplaceRemote {
					hasRemote = true
				}
			}
		}
		var orConds []bson.M
		if len(modes) > 0 {
			orConds = append(orConds, bson.M{"workplace_modes": bson.M{"$in": modes}})
		}
		if hasRemote {
			orConds = append(orConds,
				bson.M{"is_remote": true},
				bson.M{"display_location": primitive.Regex{Pattern: "(?i)remote", Options: ""}},
				bson.M{"location": primitive.Regex{Pattern: "(?i)remote", Options: ""}},
			)
		}
		if includeUnknown {
			orConds = append(orConds, bson.M{"$or": []bson.M{
				{"workplace_modes": bson.M{"$size": 0}},
				{"workplace_modes": "unknown"},
				{"workplace_modes": nil},
			}})
		}
		if len(orConds) > 0 {
			conditions = append(conditions, bson.M{"$or": orConds})
		}
	}

	if omitFacet != "company" {
		if len(request.CompanyIDs) > 0 {
			var ids []string
			var names []string
			uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
			for _, item := range request.CompanyIDs {
				trimmed := strings.TrimSpace(item)
				if uuidRegex.MatchString(trimmed) {
					ids = append(ids, trimmed)
				} else if trimmed != "" {
					names = append(names, trimmed)
				}
			}
			var orConds []bson.M
			if len(ids) > 0 {
				orConds = append(orConds, bson.M{"company_id": bson.M{"$in": ids}})
			}
			if len(names) > 0 {
				var nameConds []bson.M
				for _, n := range names {
					nameConds = append(nameConds, bson.M{
						"company_name": primitive.Regex{Pattern: "^" + regexp.QuoteMeta(n) + "$", Options: "i"},
					})
				}
				if len(nameConds) == 1 {
					orConds = append(orConds, nameConds[0])
				} else {
					orConds = append(orConds, bson.M{"$or": nameConds})
				}
			}

			var cond bson.M
			if len(orConds) == 1 {
				cond = orConds[0]
			} else if len(orConds) > 1 {
				cond = bson.M{"$or": orConds}
			} else {
				cond = bson.M{"company_id": bson.M{"$in": request.CompanyIDs}}
			}

			if request.IncludeUnknownCompany {
				conditions = append(conditions, bson.M{"$or": []bson.M{cond, {"company_id": nil}}})
			} else {
				conditions = append(conditions, cond)
			}
		}
		if len(request.ExcludedCompanyIDs) > 0 {
			conditions = append(conditions, bson.M{
				"$or": []bson.M{
					{"company_id": nil},
					{"company_id": bson.M{"$nin": request.ExcludedCompanyIDs}},
				},
			})
		}
	}

	if omitFacet != "degree" {
		if len(request.DegreeLevels) > 0 {
			var levels []string
			var orConds []bson.M
			includeUnknown := request.IncludeUnknownDegree
			for _, l := range request.DegreeLevels {
				if l == models.DegreeUnknown {
					includeUnknown = true
				} else {
					strLevel := string(l)
					levels = append(levels, strLevel)
					var degPat string
					switch strLevel {
					case "bachelor", "bachelors":
						degPat = "(?i)(bachelor|b\\.s\\.|b\\.a\\.|undergraduate)"
					case "master", "masters":
						degPat = "(?i)(master|m\\.s\\.|m\\.a\\.|graduate)"
					case "doctorate":
						degPat = "(?i)(ph\\.?d|doctorate)"
					case "associate":
						degPat = "(?i)associate"
					}
					if degPat != "" {
						orConds = append(orConds,
							bson.M{"description_text": primitive.Regex{Pattern: degPat, Options: ""}},
							bson.M{"qualifications_text": primitive.Regex{Pattern: degPat, Options: ""}},
						)
					}
				}
			}
			if len(levels) > 0 {
				orConds = append(orConds, bson.M{"degree_levels": bson.M{"$in": levels}})
			}
			if includeUnknown {
				orConds = append(orConds, bson.M{"$or": []bson.M{
					{"degree_levels": bson.M{"$size": 0}},
					{"degree_levels": nil},
				}})
			}
			if len(orConds) > 0 {
				conditions = append(conditions, bson.M{"$or": orConds})
			}
		}
		if request.DegreeRequirement != "" && request.DegreeRequirement != models.DegreeAny {
			conditions = append(conditions, bson.M{"degree_requirement": string(request.DegreeRequirement)})
		}
	}

	if omitFacet != "experience" && request.MaxExperienceYears != nil {
		cond := bson.M{
			"max_experience_years": bson.M{"$lte": *request.MaxExperienceYears},
		}
		if request.IncludeUnknownExperience {
			conditions = append(conditions, bson.M{"$or": []bson.M{cond, {"max_experience_years": nil}}})
		} else {
			conditions = append(conditions, cond)
		}
	}

	if omitFacet != "visa" && request.VisaSponsorship != "" && request.VisaSponsorship != models.VisaAny {
		if request.VisaSponsorship == models.VisaYes {
			conditions = append(conditions, bson.M{
				"$or": []bson.M{
					{"visa_sponsorship": "yes"},
					{"description_text": primitive.Regex{Pattern: "(?i)(visa|sponsorship|h-?1b)", Options: ""}},
				},
			})
		} else {
			conditions = append(conditions, bson.M{"visa_sponsorship": string(request.VisaSponsorship)})
		}
	}

	if omitFacet != "role" && len(request.RoleIDs) > 0 {
		var orConds []bson.M
		orConds = append(orConds, bson.M{"role_ids": bson.M{"$in": request.RoleIDs}})
		for _, rid := range request.RoleIDs {
			cleanRole := strings.ReplaceAll(strings.ToLower(rid), "-", " ")
			cleanRole = strings.ReplaceAll(cleanRole, "_", " ")
			orConds = append(orConds,
				bson.M{"canonical_title": primitive.Regex{Pattern: "(?i)" + regexp.QuoteMeta(cleanRole), Options: ""}},
				bson.M{"title": primitive.Regex{Pattern: "(?i)" + regexp.QuoteMeta(cleanRole), Options: ""}},
			)
		}
		if request.IncludeUnknownRole {
			orConds = append(orConds, bson.M{"role_ids": bson.M{"$size": 0}}, bson.M{"role_ids": nil})
		}
		conditions = append(conditions, bson.M{"$or": orConds})
	}

	if omitFacet != "location" {
		var locConds []bson.M
		for _, loc := range request.LocationIDs {
			trimmed := strings.TrimSpace(loc)
			if trimmed == "" {
				continue
			}
			isRemote := strings.EqualFold(trimmed, "Remote") || strings.Contains(strings.ToLower(trimmed), "remote")
			var pattern string
			if strings.EqualFold(trimmed, "US") || strings.EqualFold(trimmed, "United States") {
				pattern = "(?i)(united states|\\busa?\\b|san francisco|new york|seattle|austin|chicago|california|texas|washington)"
			} else if isRemote {
				pattern = "(?i)remote"
			} else {
				pattern = "(?i)" + regexp.QuoteMeta(trimmed)
			}
			subConds := []bson.M{
				{"display_location": bson.M{"$regex": pattern}},
				{"location": bson.M{"$regex": pattern}},
				{"city": bson.M{"$regex": pattern}},
				{"region": bson.M{"$regex": pattern}},
				{"state": bson.M{"$regex": pattern}},
				{"country": bson.M{"$regex": pattern}},
				{"country_code": bson.M{"$regex": pattern}},
			}
			if isRemote {
				subConds = append(subConds, bson.M{"workplace_modes": "remote"})
			}
			locConds = append(locConds, bson.M{"$or": subConds})
		}
		for _, loc := range request.Locations {
			term := loc.LocationID
			if term == "" {
				term = loc.City
			}
			if term == "" {
				term = loc.CountryCode
			}
			trimmed := strings.TrimSpace(term)
			if trimmed == "" {
				continue
			}
			isRemote := strings.EqualFold(trimmed, "Remote") || strings.Contains(strings.ToLower(trimmed), "remote")
			pattern := "(?i)" + regexp.QuoteMeta(trimmed)
			subConds := []bson.M{
				{"display_location": bson.M{"$regex": pattern}},
				{"location": bson.M{"$regex": pattern}},
				{"city": bson.M{"$regex": pattern}},
				{"region": bson.M{"$regex": pattern}},
				{"state": bson.M{"$regex": pattern}},
				{"country": bson.M{"$regex": pattern}},
				{"country_code": bson.M{"$regex": pattern}},
			}
			if isRemote {
				subConds = append(subConds, bson.M{"workplace_modes": "remote"})
			}
			locConds = append(locConds, bson.M{"$or": subConds})
		}
		if len(locConds) > 0 {
			if request.IncludeUnknownLocation {
				locConds = append(locConds, bson.M{
					"$or": []bson.M{
						{"display_location": ""},
						{"display_location": nil},
						{"location": ""},
						{"location": nil},
					},
				})
			}
			conditions = append(conditions, bson.M{"$or": locConds})
		}
	}

	if omitFacet != "job_type" && len(request.JobTypes) > 0 {
		conditions = append(conditions, bson.M{"job_type": bson.M{"$in": enumStrings(request.JobTypes)}})
	}

	if omitFacet != "employment_type" && len(request.EmploymentTypes) > 0 {
		conditions = append(conditions, bson.M{"employment_types": bson.M{"$in": enumStrings(request.EmploymentTypes)}})
	}

	if len(conditions) == 1 {
		return conditions[0]
	}
	return bson.M{"$and": conditions}
}

func addMongoDateFilters(conditions *[]bson.M, date models.DateFilter, now time.Time) {
	var after, before *time.Time
	switch date.Preset {
	case models.DateLast6Hours:
		v := now.Add(-6 * time.Hour)
		after, before = &v, &now
	case models.DateLast24Hours:
		v := now.Add(-24 * time.Hour)
		after, before = &v, &now
	case models.DateLast7Days:
		v := now.Add(-7 * 24 * time.Hour)
		after, before = &v, &now
	case models.DateLast30Days:
		v := now.Add(-30 * 24 * time.Hour)
		after, before = &v, &now
	case models.DateLast60Days:
		v := now.Add(-60 * 24 * time.Hour)
		after, before = &v, &now
	}

	if date.PublishedAfter != nil && (after == nil || date.PublishedAfter.After(*after)) {
		v := date.PublishedAfter.UTC()
		after = &v
	}
	if date.PublishedBefore != nil && (before == nil || date.PublishedBefore.Before(*before)) {
		v := date.PublishedBefore.UTC()
		before = &v
	}

	if after != nil || before != nil {
		dateRange := bson.M{}
		if after != nil {
			dateRange["$gte"] = after.UTC()
		}
		if before != nil {
			dateRange["$lte"] = before.UTC()
		}
		dateCond := bson.M{
			"$or": []bson.M{
				{"source_published_at": dateRange},
				{"published_at": dateRange},
				{"first_seen_at": dateRange},
			},
		}
		if date.IncludeUnknownPublicationTime {
			*conditions = append(*conditions, bson.M{
				"$or": []bson.M{
					dateCond,
					{"source_published_at": nil},
					{"first_seen_at": nil},
					{"publication_time_quality": "unknown"},
				},
			})
		} else {
			*conditions = append(*conditions, dateCond)
		}
	}

	if date.FirstSeenAfter != nil {
		*conditions = append(*conditions, bson.M{"first_seen_at": bson.M{"$gte": date.FirstSeenAfter.UTC()}})
	}
}

func buildMongoCursorCondition(sort models.JobSort, pos searchPosition) bson.M {
	switch sort {
	case models.SortPublishedDesc:
		if pos.Published == nil {
			return bson.M{"source_published_at": nil, "_id": bson.M{"$lt": pos.ID}}
		}
		t, err := time.Parse(time.RFC3339Nano, *pos.Published)
		if err != nil {
			return bson.M{"_id": nil}
		}
		tUTC := t.UTC()
		return bson.M{
			"$or": []bson.M{
				{"source_published_at": bson.M{"$lt": tUTC}},
				{"source_published_at": nil},
				{"source_published_at": tUTC, "_id": bson.M{"$lt": pos.ID}},
			},
		}
	case models.SortSalaryDesc:
		if pos.Salary == nil {
			return bson.M{"salary_max": nil, "_id": bson.M{"$lt": pos.ID}}
		}
		sal, err := strconv.ParseFloat(*pos.Salary, 64)
		if err != nil {
			return bson.M{"_id": nil}
		}
		return bson.M{
			"$or": []bson.M{
				{"salary_max": bson.M{"$lt": sal}},
				{"salary_max": nil},
				{"salary_max": sal, "_id": bson.M{"$lt": pos.ID}},
			},
		}
	default:
		seen, err := time.Parse(time.RFC3339Nano, pos.FirstSeen)
		if err != nil {
			return bson.M{"_id": nil}
		}
		seenUTC := seen.UTC()
		return bson.M{
			"$or": []bson.M{
				{"first_seen_at": bson.M{"$lt": seenUTC}},
				{"first_seen_at": seenUTC, "_id": bson.M{"$lt": pos.ID}},
			},
		}
	}
}

func (r *JobSearchRepository) computeFacets(ctx context.Context, request models.JobSearchRequest, now time.Time) (models.FacetSet, error) {
	specs := discoveryFacetSpecs()
	facets := models.FacetSet{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, spec := range specs {
		wg.Add(1)
		go func(s facetSpec) {
			defer wg.Done()
			f, err := r.fetchFacet(ctxTimeout, request, s, now)
			if err == nil {
				mu.Lock()
				facets[s.key] = f
				mu.Unlock()
			}
		}(spec)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		d, err := r.fetchDateFacet(ctxTimeout, request, now)
		if err == nil {
			mu.Lock()
			facets["date"] = d
			mu.Unlock()
		}
	}()

	wg.Wait()
	return facets, nil
}

type facetSpec struct {
	key, omit, field, taxonomy string
}

func discoveryFacetSpecs() []facetSpec {
	return []facetSpec{
		{key: "company", omit: "company", field: "company_name"},
		{key: "location", omit: "location", field: "display_location"},
		{key: "workplace", omit: "workplace", field: "workplace_modes"},
		{key: "degree_level", omit: "degree", field: "degree_levels", taxonomy: "onerole-degree-v1"},
		{key: "degree_requirement", omit: "degree", field: "degree_requirement", taxonomy: "onerole-degree-v1"},
		{key: "experience", omit: "experience", field: "max_experience_years"},
		{key: "visa_sponsorship", omit: "visa", field: "visa_sponsorship"},
		{key: "role", omit: "role", field: "role_ids", taxonomy: "onerole-role-v1"},
		{key: "job_type", omit: "job_type", field: "job_type", taxonomy: "onerole-job-type-v1"},
		{key: "employment_type", omit: "employment_type", field: "employment_types", taxonomy: "onerole-employment-v1"},
	}
}

func (r *JobSearchRepository) fetchFacet(ctx context.Context, request models.JobSearchRequest, spec facetSpec, now time.Time) (models.Facet, error) {
	filter := buildMongoJobFilter(request, now, spec.omit)
	coll := r.db.Collection("jobs")

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
	}
	if spec.field == "workplace_modes" || spec.field == "degree_levels" || spec.field == "role_ids" || spec.field == "employment_types" {
		pipeline = append(pipeline, bson.D{{Key: "$unwind", Value: "$" + spec.field}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$group", Value: bson.M{
			"_id":   "$" + spec.field,
			"count": bson.M{"$sum": 1},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		bson.D{{Key: "$limit", Value: 100}},
	)

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return models.Facet{}, fmt.Errorf("aggregate %s job facet: %w", spec.key, err)
	}
	defer cursor.Close(ctx)

	var options []models.FacetOption
	var unknownCount int
	for cursor.Next(ctx) {
		var doc struct {
			ID    any `bson:"_id"`
			Count int `bson:"count"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return models.Facet{}, err
		}
		if doc.ID == nil || doc.ID == "" || doc.ID == "unknown" {
			unknownCount += doc.Count
			continue
		}
		label := fmt.Sprint(doc.ID)
		options = append(options, models.FacetOption{
			Value: label,
			Label: label,
			Count: doc.Count,
		})
	}

	var taxonomy *string
	if spec.taxonomy != "" {
		taxonomy = &spec.taxonomy
	}
	return models.Facet{Options: options, UnknownCount: unknownCount, TaxonomyVersion: taxonomy}, nil
}

func (r *JobSearchRepository) fetchDateFacet(ctx context.Context, request models.JobSearchRequest, now time.Time) (models.Facet, error) {
	baseFilter := buildMongoJobFilter(request, now, "date")
	coll := r.db.Collection("jobs")

	options := make([]models.FacetOption, 0, 5)
	for _, period := range []struct {
		key, label string
		duration   time.Duration
	}{
		{"last_6_hours", "Last 6 hours", 6 * time.Hour},
		{"last_24_hours", "Last 24 hours", 24 * time.Hour},
		{"last_7_days", "Last 7 days", 7 * 24 * time.Hour},
		{"last_30_days", "Last 30 days", 30 * 24 * time.Hour},
		{"last_60_days", "Last 60 days", 60 * 24 * time.Hour},
	} {
		dateFilter := bson.M{
			"$and": []bson.M{
				baseFilter,
				{
					"source_published_at": bson.M{
						"$gte": now.Add(-period.duration),
						"$lte": now,
					},
					"publication_time_quality": "source_provided",
				},
			},
		}
		count, err := coll.CountDocuments(ctx, dateFilter)
		if err != nil {
			return models.Facet{}, fmt.Errorf("count %s job facet: %w", period.key, err)
		}
		options = append(options, models.FacetOption{Value: period.key, Label: period.label, Count: int(count)})
	}

	unknownFilter := bson.M{
		"$and": []bson.M{
			baseFilter,
			{
				"$or": []bson.M{
					{"source_published_at": nil},
					{"publication_time_quality": "unknown"},
				},
			},
		},
	}
	unknownCount, err := coll.CountDocuments(ctx, unknownFilter)
	if err != nil {
		return models.Facet{}, fmt.Errorf("count unknown publication jobs: %w", err)
	}

	return models.Facet{Options: options, UnknownCount: int(unknownCount)}, nil
}

func (r *JobSearchRepository) GetJob(ctx context.Context, jobID string) (models.Job, error) {
	if r == nil || r.db == nil {
		return models.Job{}, db.ErrNilDB
	}
	if !dbUUID(jobID) {
		return models.Job{}, ErrInvalidSearchRequest
	}

	var doc jobDoc
	err := r.db.Collection("jobs").FindOne(ctx, bson.M{
		"_id":    jobID,
		"status": bson.M{"$in": []string{"open", "stale"}},
	}).Decode(&doc)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.Job{}, ErrJobNotFound
	}
	if err != nil {
		return models.Job{}, fmt.Errorf("load job: %w", err)
	}

	job, _ := mapDocToJob(doc, models.SortFirstSeenDesc)
	return job, nil
}

type CompanySearchRepository struct{ db *mongo.Database }

func NewCompanySearchRepository(database *mongo.Database) *CompanySearchRepository {
	return &CompanySearchRepository{db: database}
}

type companySearchPosition struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type companyDoc struct {
	ID                 string  `bson:"_id"`
	CanonicalName      string  `bson:"canonical_name"`
	LogoURL            *string `bson:"logo_url,omitempty"`
	VerificationStatus string  `bson:"verification_status"`
}

func (r *CompanySearchRepository) SearchCompanies(ctx context.Context, query string, cursor *string, limit int) (service.CompanySearchResponse, error) {
	if r == nil || r.db == nil {
		return service.CompanySearchResponse{}, db.ErrNilDB
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 200 || limit < 1 || limit > 100 {
		return service.CompanySearchResponse{}, ErrInvalidSearchRequest
	}

	filters := struct {
		Query string `json:"query"`
	}{Query: query}
	digest, err := filterDigest("companies", filters)
	if err != nil {
		return service.CompanySearchResponse{}, err
	}

	var position *companySearchPosition
	if cursor != nil {
		var parsed companySearchPosition
		if err := decodeOpaqueCursor(*cursor, "companies", digest, &parsed); err != nil || parsed.ID == "" {
			return service.CompanySearchResponse{}, ErrInvalidCursor
		}
		position = &parsed
	}

	coll := r.db.Collection("companies")
	filter := bson.M{
		"verification_status": bson.M{"$nin": []string{"merged", "inactive"}},
	}
	if query != "" {
		pattern := primitive.Regex{Pattern: regexp.QuoteMeta(query), Options: "i"}
		filter["canonical_name"] = pattern
	}
	if position != nil {
		filter["canonical_name"] = bson.M{"$gt": position.Name}
	}

	findLimit := int64(limit + 1)
	if query != "" && findLimit < 100 {
		findLimit = 100
	}
	findOpts := options.Find().
		SetSort(bson.D{{Key: "canonical_name", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(findLimit)

	cur, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return service.CompanySearchResponse{}, fmt.Errorf("search companies: %w", err)
	}
	defer cur.Close(ctx)

	var docs []companyDoc
	if err := cur.All(ctx, &docs); err != nil {
		return service.CompanySearchResponse{}, fmt.Errorf("read company suggestions: %w", err)
	}

	if query != "" {
		lowerQ := strings.ToLower(query)
		sort.SliceStable(docs, func(i, j int) bool {
			iName := strings.ToLower(docs[i].CanonicalName)
			jName := strings.ToLower(docs[j].CanonicalName)
			iExact := iName == lowerQ
			jExact := jName == lowerQ
			if iExact != jExact {
				return iExact
			}
			iPrefix := strings.HasPrefix(iName, lowerQ)
			jPrefix := strings.HasPrefix(jName, lowerQ)
			if iPrefix != jPrefix {
				return iPrefix
			}
			return iName < jName
		})
	}

	more := len(docs) > limit
	if more {
		docs = docs[:limit]
	}

	response := service.CompanySearchResponse{Items: make([]service.CompanySuggestion, 0, len(docs))}
	var positions []companySearchPosition
	for _, doc := range docs {
		response.Items = append(response.Items, service.CompanySuggestion{
			ID:      doc.ID,
			Name:    doc.CanonicalName,
			LogoURL: doc.LogoURL,
		})
		positions = append(positions, companySearchPosition{Name: doc.CanonicalName, ID: doc.ID})
	}

	if more && len(positions) > 0 {
		encoded, err := encodeOpaqueCursor("companies", digest, positions[len(positions)-1])
		if err != nil {
			return service.CompanySearchResponse{}, err
		}
		response.NextCursor = &encoded
	}

	return response, nil
}

type LocationSuggestionRepository struct{ db *mongo.Database }

func NewLocationSuggestionRepository(database *mongo.Database) *LocationSuggestionRepository {
	return &LocationSuggestionRepository{db: database}
}

func (r *LocationSuggestionRepository) SuggestLocations(ctx context.Context, query, countryCode string, limit int) (service.LocationSuggestions, error) {
	if r == nil || r.db == nil {
		return service.LocationSuggestions{}, db.ErrNilDB
	}
	query, countryCode = strings.TrimSpace(query), strings.ToUpper(strings.TrimSpace(countryCode))
	if len([]rune(query)) > 200 || len(countryCode) > 2 || limit < 1 || limit > 100 {
		return service.LocationSuggestions{}, ErrInvalidSearchRequest
	}

	coll := r.db.Collection("jobs")
	match := bson.M{
		"status": bson.M{"$in": []string{"open", "stale"}},
	}
	if query != "" {
		match["display_location"] = primitive.Regex{Pattern: regexp.QuoteMeta(query), Options: "i"}
	}
	if countryCode != "" {
		match["country_code"] = countryCode
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":          "$display_location",
			"city":         bson.M{"$first": "$city"},
			"region":       bson.M{"$first": "$region"},
			"country_code": bson.M{"$first": "$country_code"},
			"count":        bson.M{"$sum": 1},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		bson.D{{Key: "$limit", Value: limit}},
	}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return service.LocationSuggestions{}, fmt.Errorf("suggest locations: %w", err)
	}
	defer cursor.Close(ctx)

	var items []service.LocationSuggestion
	for cursor.Next(ctx) {
		var doc struct {
			ID          string  `bson:"_id"`
			City        *string `bson:"city"`
			Region      *string `bson:"region"`
			CountryCode *string `bson:"country_code"`
			Count       int     `bson:"count"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return service.LocationSuggestions{}, err
		}
		cc := ""
		if doc.CountryCode != nil {
			cc = *doc.CountryCode
		}
		items = append(items, service.LocationSuggestion{
			ID:               doc.ID,
			Label:            doc.ID,
			City:             doc.City,
			Region:           doc.Region,
			CountryCode:      cc,
			MatchingJobCount: doc.Count,
		})
	}

	return service.LocationSuggestions{Items: items}, nil
}

type FilterRepository struct {
	db       *mongo.Database
	mu       sync.RWMutex
	cached   service.FilterMetadata
	cachedAt time.Time
}

func defaultFilterMetadata() service.FilterMetadata {
	return service.FilterMetadata{
		Facets: models.FacetSet{
			"workplace": models.Facet{
				Options: []models.FacetOption{
					{Value: "remote", Label: "Remote", Count: 62450},
					{Value: "hybrid", Label: "Hybrid", Count: 51200},
					{Value: "onsite", Label: "Onsite", Count: 73342},
				},
			},
			"employment_type": models.Facet{
				Options: []models.FacetOption{
					{Value: "full_time", Label: "Full Time", Count: 145000},
					{Value: "contract", Label: "Contract", Count: 28000},
					{Value: "internship", Label: "Internship", Count: 14000},
				},
			},
			"degree_level": models.Facet{
				Options: []models.FacetOption{
					{Value: "bachelors", Label: "Bachelor's Degree", Count: 110000},
					{Value: "masters", Label: "Master's Degree", Count: 42000},
					{Value: "doctorate", Label: "Doctorate / Ph.D.", Count: 12000},
				},
			},
			"visa_sponsorship": models.Facet{
				Options: []models.FacetOption{
					{Value: "yes", Label: "Visa Sponsorship Offered", Count: 35000},
					{Value: "no", Label: "No Sponsorship", Count: 120000},
				},
			},
		},
	}
}

func NewFilterRepository(database *mongo.Database) *FilterRepository {
	repo := &FilterRepository{
		db:       database,
		cached:   defaultFilterMetadata(),
		cachedAt: time.Now(),
	}
	go repo.refreshFacetsBackground()
	return repo
}

func (r *FilterRepository) refreshFacetsBackground() {
	if r.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	search := NewJobSearchRepository(r.db)
	request, _ := service.NormalizeJobSearchRequest(models.JobSearchRequest{Limit: 1})
	facets, err := search.computeFacets(ctx, request, time.Now().UTC())
	if err == nil && len(facets) > 0 {
		r.mu.Lock()
		r.cached = service.FilterMetadata{Facets: facets}
		r.cachedAt = time.Now()
		r.mu.Unlock()
	}
}

func (r *FilterRepository) GetFilterMetadata(ctx context.Context) (service.FilterMetadata, error) {
	if r == nil || r.db == nil {
		return service.FilterMetadata{}, db.ErrNilDB
	}

	r.mu.RLock()
	hasCached := len(r.cached.Facets) > 0
	cached := r.cached
	cachedAt := r.cachedAt
	r.mu.RUnlock()

	if hasCached {
		if time.Since(cachedAt) > 15*time.Minute {
			go r.refreshFacetsBackground()
		}
		return cached, nil
	}

	return defaultFilterMetadata(), nil
}

func effectiveJobSort(request models.JobSearchRequest) models.JobSort {
	if request.Sort == models.SortRelevance && strings.TrimSpace(request.Query) == "" {
		return models.SortFirstSeenDesc
	}
	return request.Sort
}

func enumStrings[T ~string](values []T) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = string(value)
	}
	return result
}

func validCapability(value string) models.ApplicationCapability {
	capability := models.ApplicationCapability(value)
	switch capability {
	case models.CapabilityAutomatedSupported, models.CapabilityAssistedSupported,
		models.CapabilityManualRequired, models.CapabilityRequiresAuthentication,
		models.CapabilityRequiresInformation, models.CapabilityUnsupported:
		return capability
	default:
		return models.CapabilityUnsupported
	}
}

var repositoryUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func dbUUID(value string) bool { return repositoryUUIDPattern.MatchString(value) }

var (
	ErrInvalidSearchRequest = errors.New("invalid job search request")
	ErrJobNotFound          = errors.New("job not found")
)

var _ service.JobSearchStore = (*JobSearchRepository)(nil)
var _ service.JobReader = (*JobSearchRepository)(nil)
var _ service.CompanySearchStore = (*CompanySearchRepository)(nil)
var _ service.LocationSuggestionStore = (*LocationSuggestionRepository)(nil)
var _ service.FilterStore = (*FilterRepository)(nil)

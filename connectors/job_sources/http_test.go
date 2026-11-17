package job_sources

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testLimiter struct{}

func (testLimiter) Acquire(context.Context, SourceConfig) (func(), error) { return func() {}, nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    &http.Request{},
	}
}

func enabledSource(provider Provider, endpointHost string) SourceConfig {
	reviewedAt := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	endpoints := map[Provider]string{
		ProviderGreenhouse: "https://boards-api.greenhouse.io/v1/boards",
		ProviderLever:      "https://api.lever.co/v0/postings",
		ProviderAshby:      "https://api.ashbyhq.com/posting-api/job-board",
	}
	return SourceConfig{
		ID: "source-1", CompanyID: "company-1", Provider: provider, BoardIdentifier: "acme",
		Endpoint: endpoints[provider], AllowedHosts: []string{endpointHost}, Status: "enabled", DiscoveryEnabled: true,
		AllowedOperations: []string{"read_jobs"}, RequestsPerWindow: 10, Window: time.Minute,
		MaxConcurrency: 1, ConfigVersion: 1, ConnectorVersion: "test-v1", PolicyReviewedAt: &reviewedAt,
	}
}

func TestGreenhouseFetchParsesRealListingAndUsesReadOnlyURL(t *testing.T) {
	var requestMethod, requestPath, requestQuery string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestMethod, requestPath, requestQuery = r.Method, r.URL.Path, r.URL.RawQuery
		body, err := json.Marshal(map[string]any{"jobs": []any{map[string]any{
			"id": 123, "title": "Go Engineer", "updated_at": "2026-10-29T10:00:00Z",
			"location":     map[string]string{"name": "Remote, United States"},
			"absolute_url": "https://boards.greenhouse.io/acme/jobs/123",
			"content":      "<p>Build services</p><script>ignored()</script>",
		}}})
		if err != nil {
			return nil, err
		}
		return response(http.StatusOK, string(body)), nil
	})}
	connector := &httpConnector{provider: ProviderGreenhouse, apiBase: "https://boards-api.greenhouse.io/v1/boards", client: client, limiter: testLimiter{}}
	source := enabledSource(ProviderGreenhouse, "boards-api.greenhouse.io")
	page, err := connector.Fetch(context.Background(), source, "")
	if err != nil {
		t.Fatal(err)
	}
	if requestMethod != http.MethodGet || requestPath != "/v1/boards/acme/jobs" || requestQuery != "content=true" {
		t.Fatalf("unexpected request: %s %s?%s", requestMethod, requestPath, requestQuery)
	}
	if len(page.Items) != 1 || !page.Complete || !page.FullSnapshot || page.Items[0].ExternalID != "123" || page.Items[0].Title != "Go Engineer" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if page.Items[0].Description != "Build services" {
		t.Fatalf("HTML content was not reduced to text: %q", page.Items[0].Description)
	}
	if page.Items[0].PublishedAt != nil {
		t.Fatalf("Greenhouse update time was misrepresented as publish time: %v", page.Items[0].PublishedAt)
	}
	if page.Items[0].UpdatedAt == nil || page.Items[0].UpdatedAt.Location() != time.UTC {
		t.Fatalf("source update timestamp was not retained in UTC: %v", page.Items[0].UpdatedAt)
	}
}

func TestLeverFetchUsesOffsetCursorAndDoesNotInventPublicationTime(t *testing.T) {
	var gotSkip, gotLimit string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotSkip, gotLimit = r.URL.Query().Get("skip"), r.URL.Query().Get("limit")
		items := make([]map[string]any, 50)
		for i := range items {
			items[i] = map[string]any{
				"id": strconv.Itoa(i), "text": "Platform Engineer",
				"hostedUrl":        "https://jobs.lever.co/acme/" + strconv.Itoa(i),
				"applyUrl":         "https://jobs.lever.co/acme/" + strconv.Itoa(i) + "/apply",
				"categories":       map[string]any{"commitment": "Full-time", "allLocations": []string{"Austin", "Remote"}},
				"descriptionPlain": "Build a platform",
			}
		}
		body, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		return response(http.StatusOK, string(body)), nil
	})}
	connector := &httpConnector{provider: ProviderLever, apiBase: "https://api.lever.co/v0/postings", client: client, limiter: testLimiter{}}
	source := enabledSource(ProviderLever, "api.lever.co")
	source.MaxPageSize = 50
	page, err := connector.Fetch(context.Background(), source, "50")
	if err != nil {
		t.Fatal(err)
	}
	if gotSkip != "50" || gotLimit != "50" {
		t.Fatalf("offset cursor was not sent: skip=%q limit=%q", gotSkip, gotLimit)
	}
	if page.Complete || page.NextCursor != "100" || page.PageNumber != 2 {
		t.Fatalf("expected next offset page: %#v", page)
	}
	if page.Items[0].PublishedAt != nil {
		t.Fatalf("undocumented creation fields must not become publication time: %v", page.Items[0].PublishedAt)
	}
	if page.Items[0].Employment != "Full-time" {
		t.Fatalf("explicit employment category not retained: %q", page.Items[0].Employment)
	}
	if len(page.Items[0].Locations) != 2 {
		t.Fatalf("documented allLocations values were not retained: %#v", page.Items[0].Locations)
	}
}

func TestAshbyHonorsPublicListingFlagAndDocumentedWorkplaceType(t *testing.T) {
	body := `{"apiVersion":"1","jobs":[
		{"title":"Hybrid Product Engineer","location":"New York, NY","isListed":true,"isRemote":true,"workplaceType":"Hybrid","publishedAt":"2026-10-29T10:00:00Z","employmentType":"FullTime","jobUrl":"https://jobs.ashbyhq.com/acme/hybrid-role","applyUrl":"https://jobs.ashbyhq.com/acme/hybrid-role/application","secondaryLocations":[{"location":"Boston","address":{"addressLocality":"Boston","addressRegion":"Massachusetts","addressCountry":"United States"}}]},
		{"title":"Unlisted Role","isListed":false,"jobUrl":"https://jobs.ashbyhq.com/acme/unlisted"},
		{"title":"Unknown Listing Flag","jobUrl":"https://jobs.ashbyhq.com/acme/unknown"}
	]}`
	page, err := decodePage(ProviderAshby, []byte(body), 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("only explicitly listed jobs should be discovered: %#v", page.Items)
	}
	job := page.Items[0]
	if job.ExternalID != job.CanonicalURL {
		t.Fatalf("missing provider ID should fall back to its verified canonical URL, got %q", job.ExternalID)
	}
	if job.WorkplaceMode != "hybrid" {
		t.Fatalf("isRemote must not misclassify hybrid; workplaceType was %q", job.WorkplaceMode)
	}
	if job.PublishedAt == nil || job.PublishedAt.Location() != time.UTC {
		t.Fatalf("documented publishedAt should be retained in UTC: %v", job.PublishedAt)
	}
	if len(job.Locations) != 3 {
		t.Fatalf("primary and secondary location evidence missing: %#v", job.Locations)
	}
}

func TestDisabledAndUnconfiguredSourcesFailClosed(t *testing.T) {
	registry := NewRegistry(testLimiter{})
	source := enabledSource(ProviderGreenhouse, "boards-api.greenhouse.io")
	source.Status = "paused"
	if _, err := registry.Fetch(context.Background(), source, ""); err != ErrSourceNotEnabled {
		t.Fatalf("paused source error = %v, want %v", err, ErrSourceNotEnabled)
	}
	source.Status = "enabled"
	source.AllowedOperations = nil
	if _, err := registry.Fetch(context.Background(), source, ""); err != ErrSourceNotEnabled {
		t.Fatalf("unconfigured operation error = %v, want %v", err, ErrSourceNotEnabled)
	}
	if _, ok := registry.Connector(Provider("other_authorized")); ok {
		t.Fatal("unsupported provider unexpectedly received a generic connector")
	}
}

func TestSourceResponsesRequireHTTPSJobLinks(t *testing.T) {
	_, err := normalizePosting(Posting{ExternalID: "1", Title: "Role", CanonicalURL: "http://example.com/job"})
	if err == nil {
		t.Fatal("insecure canonical URL unexpectedly accepted")
	}
}

package job_sources

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Registry struct {
	connectors map[Provider]Connector
}

func NewRegistry(limiter RateLimiter) *Registry {
	if limiter == nil {
		limiter = NewProcessRateLimiter()
	}
	client := newSecureHTTPClient()
	return &Registry{connectors: map[Provider]Connector{
		ProviderGreenhouse:      &httpConnector{provider: ProviderGreenhouse, apiBase: "https://boards-api.greenhouse.io/v1/boards", limiter: limiter, client: client},
		ProviderLever:           &httpConnector{provider: ProviderLever, apiBase: "https://api.lever.co/v0/postings", limiter: limiter, client: client},
		ProviderAshby:           &httpConnector{provider: ProviderAshby, apiBase: "https://api.ashbyhq.com/posting-api/job-board", limiter: limiter, client: client},
		ProviderSmartRecruiters: &httpConnector{provider: ProviderSmartRecruiters, apiBase: "https://api.smartrecruiters.com/v1/companies", limiter: limiter, client: client},
		ProviderBreezy:          &httpConnector{provider: ProviderBreezy, apiBase: "", limiter: limiter, client: client},
		ProviderRecruitee:       &httpConnector{provider: ProviderRecruitee, apiBase: "", limiter: limiter, client: client},
		ProviderBambooHR:        &httpConnector{provider: ProviderBambooHR, apiBase: "", limiter: limiter, client: client},
		ProviderWorkday:         &httpConnector{provider: ProviderWorkday, apiBase: "", limiter: limiter, client: client},
		ProviderGeneric:         &httpConnector{provider: ProviderGeneric, apiBase: "", limiter: limiter, client: client},
	}}
}

// Connector returns the reviewed read-only connector for a supported provider.
// Unsupported configured source types remain disabled rather than falling back
// to a generic URL fetcher.
func (r *Registry) Connector(provider Provider) (Connector, bool) {
	if r == nil {
		return nil, false
	}
	connector, ok := r.connectors[provider]
	return connector, ok
}

func (r *Registry) Fetch(ctx context.Context, source SourceConfig, cursor string) (Page, error) {
	if r == nil {
		return Page{}, ErrUnsupported
	}
	connector, ok := r.Connector(source.Provider)
	if !ok {
		return Page{}, fmt.Errorf("%w: %s", ErrUnsupported, source.Provider)
	}
	return connector.Fetch(ctx, source, cursor)
}

func validateConfig(source SourceConfig, expectedBase string) (*url.URL, error) {
	if source.ID == "" || source.CompanyID == "" || !safeBoardID.MatchString(source.BoardIdentifier) {
		return nil, fmt.Errorf("%w: source ID, company ID, and verified board identifier are required", ErrInvalidConfig)
	}
	if (source.Status != "enabled" && source.Status != "degraded") || !source.DiscoveryEnabled || source.PolicyReviewedAt == nil || source.PolicyReviewedAt.IsZero() || !contains(source.AllowedOperations, "read_jobs") {
		return nil, ErrSourceNotEnabled
	}
	if source.RequestsPerWindow <= 0 || source.Window <= 0 || source.MaxConcurrency <= 0 || source.ConfigVersion <= 0 || source.ConnectorVersion == "" {
		return nil, fmt.Errorf("%w: positive rate, concurrency, config, and connector versions are required", ErrInvalidConfig)
	}
	if expectedBase == "" {
		expectedBase = source.Endpoint
	}
	base, err := url.Parse(expectedBase)
	if err != nil {
		return nil, fmt.Errorf("invalid built-in provider endpoint: %w", err)
	}
	if source.Endpoint == "" {
		return nil, fmt.Errorf("%w: an explicit documented endpoint is required", ErrInvalidConfig)
	}
	configured, err := url.Parse(source.Endpoint)
	if err != nil || configured.Scheme != "https" || configured.Host == "" {
		return nil, fmt.Errorf("%w: endpoint must be a valid HTTPS URL for %s", ErrInvalidConfig, source.Provider)
	}
	if expectedBase != source.Endpoint && !sameBase(configured, base) {
		return nil, fmt.Errorf("%w: endpoint must match the documented endpoint for %s", ErrInvalidConfig, source.Provider)
	}
	if !containsFold(source.AllowedHosts, configured.Hostname()) {
		return nil, fmt.Errorf("%w: provider host %q must be explicitly allowlisted", ErrInvalidConfig, configured.Hostname())
	}
	return mustParseURL(configured.String()), nil
}

func sameBase(candidate, expected *url.URL) bool {
	return candidate != nil && candidate.Scheme == "https" && candidate.Host == expected.Host &&
		candidate.Path == expected.Path && candidate.RawQuery == "" && candidate.Fragment == "" && candidate.User == nil
}

func mustParseURL(value string) *url.URL {
	u, _ := url.Parse(value)
	return u
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsFold(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSuffix(target, "."))
	for _, value := range values {
		if strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), ".")) == target {
			return true
		}
	}
	return false
}

func newSecureHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           safeDialContext,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   4,
		MaxConnsPerHost:       8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" {
				return fmt.Errorf("redirect target is not an allowed HTTPS URL")
			}
			previous := via[len(via)-1].URL
			if !strings.EqualFold(req.URL.Host, previous.Host) {
				return fmt.Errorf("redirect to a different host is not allowed")
			}
			return nil
		},
	}
}

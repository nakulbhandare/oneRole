package job_sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

const defaultMaxResponseBytes int64 = 8 << 20

var (
	safeBoardID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

type httpConnector struct {
	provider Provider
	apiBase  string
	client   *http.Client
	limiter  RateLimiter
}

type HTTPError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string { return fmt.Sprintf("job source returned HTTP %d", e.StatusCode) }

func (c *httpConnector) Fetch(ctx context.Context, source SourceConfig, cursor string) (Page, error) {
	if c == nil || c.client == nil || c.limiter == nil {
		return Page{}, ErrUnsupported
	}
	base, err := validateConfig(source, c.apiBase)
	if err != nil {
		return Page{}, err
	}
	if source.Provider != c.provider {
		return Page{}, fmt.Errorf("%w: provider connector mismatch", ErrInvalidConfig)
	}
	if source.RequestTimeout <= 0 {
		source.RequestTimeout = 10 * time.Second
	}
	if source.MaxResponseBytes <= 0 {
		source.MaxResponseBytes = defaultMaxResponseBytes
	}
	if source.MaxPageSize <= 0 || source.MaxPageSize > 100 {
		source.MaxPageSize = 100
	}

	release, err := c.limiter.Acquire(ctx, source)
	if err != nil {
		return Page{}, err
	}
	defer release()

	requestURL, pageNumber, err := requestURL(base, source, cursor)
	if err != nil {
		return Page{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, source.RequestTimeout)
	defer cancel()

	method := http.MethodGet
	var reqBody io.Reader
	if source.Provider == ProviderWorkday {
		method = http.MethodPost
		offset := 0
		if cursor != "" {
			if parsed, err := strconv.Atoi(cursor); err == nil && parsed >= 0 {
				offset = parsed
			}
		}
		limit := 20
		payload := fmt.Sprintf(`{"appliedFacets":{},"limit":%d,"offset":%d,"searchText":""}`, limit, offset)
		reqBody = strings.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(requestCtx, method, requestURL.String(), reqBody)
	if err != nil {
		return Page{}, fmt.Errorf("build source request: %w", err)
	}
	if source.Provider == ProviderWorkday {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	} else if source.Provider == ProviderGeneric {
		request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,application/json,*/*;q=0.8")
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	} else {
		request.Header.Set("Accept", "application/json")
		request.Header.Set("User-Agent", "OneRole-JobDiscovery/1.0")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return Page{}, fmt.Errorf("fetch source %s: %w", source.ID, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Page{}, &HTTPError{StatusCode: response.StatusCode, RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now())}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, source.MaxResponseBytes+1))
	if err != nil {
		return Page{}, fmt.Errorf("read source response: %w", err)
	}
	if int64(len(body)) > source.MaxResponseBytes {
		return Page{}, fmt.Errorf("source response exceeded %d bytes", source.MaxResponseBytes)
	}

	page, err := decodePage(c.provider, body, pageNumber, source.MaxPageSize, source)
	if err != nil {
		return Page{}, fmt.Errorf("decode %s job response: %w", c.provider, err)
	}
	return page, nil
}

func requestURL(base *url.URL, source SourceConfig, cursor string) (*url.URL, int, error) {
	if !safeBoardID.MatchString(source.BoardIdentifier) {
		return nil, 0, fmt.Errorf("%w: invalid board identifier", ErrInvalidConfig)
	}
	u := *base
	u.Path = path.Join(u.Path, source.BoardIdentifier)
	pageNumber := 1
	query := url.Values{}
	switch source.Provider {
	case ProviderGreenhouse:
		if cursor != "" {
			return nil, 0, fmt.Errorf("%w: Greenhouse endpoint is a full snapshot and has no cursor", ErrInvalidConfig)
		}
		u.Path += "/jobs"
		query.Set("content", "true")
	case ProviderLever:
		offset := 0
		if cursor != "" {
			parsed, err := strconv.Atoi(cursor)
			if err != nil || parsed < 0 || parsed > 10_000_000 {
				return nil, 0, fmt.Errorf("%w: invalid Lever offset cursor", ErrInvalidConfig)
			}
			offset = parsed
			pageNumber = offset/source.MaxPageSize + 1
		}
		query.Set("mode", "json")
		query.Set("skip", strconv.Itoa(offset))
		query.Set("limit", strconv.Itoa(source.MaxPageSize))
	case ProviderAshby:
		if cursor != "" {
			return nil, 0, fmt.Errorf("%w: Ashby endpoint is a full snapshot and has no cursor", ErrInvalidConfig)
		}
		query.Set("includeCompensation", "true")
	case ProviderSmartRecruiters:
		offset := 0
		if cursor != "" {
			parsed, err := strconv.Atoi(cursor)
			if err != nil || parsed < 0 || parsed > 10_000_000 {
				return nil, 0, fmt.Errorf("%w: invalid SmartRecruiters offset cursor", ErrInvalidConfig)
			}
			offset = parsed
			pageNumber = offset/source.MaxPageSize + 1
		}
		u.Path += "/postings"
		query.Set("limit", strconv.Itoa(source.MaxPageSize))
		query.Set("offset", strconv.Itoa(offset))
	case ProviderBreezy:
		u.Path = "/json"
	case ProviderRecruitee:
		u.Path = "/api/offers"
	case ProviderBambooHR:
		u.Path = "/careers/list"
	case ProviderWorkday:
		u.Path = base.Path
		query = u.Query()
	case ProviderGeneric:
		// For generic job pages, use base endpoint path directly
		u.Path = base.Path
		query = u.Query()
	default:
		return nil, 0, fmt.Errorf("%w: %s", ErrUnsupported, source.Provider)
	}
	u.RawQuery = query.Encode()
	return &u, pageNumber, nil
}

type greenhouseResponse struct {
	Jobs []struct {
		ID        int64  `json:"id"`
		Title     string `json:"title"`
		UpdatedAt string `json:"updated_at"`
		Location  struct {
			Name string `json:"name"`
		} `json:"location"`
		AbsoluteURL string `json:"absolute_url"`
		Content     string `json:"content"`
		Offices     []struct {
			Name     string `json:"name"`
			Location string `json:"location"`
		} `json:"offices"`
	} `json:"jobs"`
}

type leverResponse []struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Categories struct {
		Location     string   `json:"location"`
		AllLocations []string `json:"allLocations"`
		Team         string   `json:"team"`
		Department   string   `json:"department"`
		Commitment   string   `json:"commitment"`
		Level        string   `json:"level"`
	} `json:"categories"`
	HostedURL        string `json:"hostedUrl"`
	ApplyURL         string `json:"applyUrl"`
	DescriptionPlain string `json:"descriptionPlain"`
	Description      string `json:"description"`
	WorkplaceType    string `json:"workplaceType"`
}

type ashbyResponse struct {
	Jobs []struct {
		ID                 string `json:"id"`
		Title              string `json:"title"`
		Location           string `json:"location"`
		SecondaryLocations []struct {
			Location string `json:"location"`
			Address  struct {
				Locality string `json:"addressLocality"`
				Region   string `json:"addressRegion"`
				Country  string `json:"addressCountry"`
			} `json:"address"`
		} `json:"secondaryLocations"`
		Department       string `json:"department"`
		Team             string `json:"team"`
		IsRemote         bool   `json:"isRemote"`
		IsListed         *bool  `json:"isListed"`
		WorkplaceType    string `json:"workplaceType"`
		EmploymentType   string `json:"employmentType"`
		DescriptionPlain string `json:"descriptionPlain"`
		DescriptionHTML  string `json:"descriptionHtml"`
		ApplyURL         string `json:"applyUrl"`
		JobURL           string `json:"jobUrl"`
		PublishedAt      string `json:"publishedAt"`
	} `json:"jobs"`
}

type smartRecruitersResponse struct {
	TotalFound int `json:"totalFound"`
	Offset     int `json:"offset"`
	Limit      int `json:"limit"`
	Content    []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		ReleasedDate string `json:"releasedDate"`
		Location     struct {
			City    string `json:"city"`
			Region  string `json:"region"`
			Country string `json:"country"`
			Remote  bool   `json:"remote"`
		} `json:"location"`
		Department struct {
			Label string `json:"label"`
		} `json:"department"`
		TypeOfEmployment struct {
			Label string `json:"label"`
		} `json:"typeOfEmployment"`
		ExperienceLevel struct {
			Label string `json:"label"`
		} `json:"experienceLevel"`
		Ref string `json:"ref"`
	} `json:"content"`
}

type breezyResponse []struct {
	ID          string `json:"_id"`
	Name        string `json:"name"`
	FriendlyURL string `json:"friendly_url"`
	URL         string `json:"url"`
	Type        struct {
		Name string `json:"name"`
	} `json:"type"`
	Location struct {
		Name     string `json:"name"`
		City     string `json:"city"`
		State    string `json:"state"`
		Country  string `json:"country"`
		IsRemote bool   `json:"is_remote"`
	} `json:"location"`
	Department string `json:"department"`
	UpdatedAt  string `json:"updated_at"`
}

type recruiteeResponse struct {
	Offers []struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		CareersURL  string `json:"careers_url"`
		Department  string `json:"department"`
		Location    string `json:"location"`
		Remote      bool   `json:"remote"`
		PublishedAt string `json:"published_at"`
		Description string `json:"description"`
	} `json:"offers"`
}

type bambooHRResponse struct {
	Result []struct {
		ID             string `json:"id"`
		JobOpeningName string `json:"jobOpeningName"`
		Department     string `json:"department"`
		Location       struct {
			City  string `json:"city"`
			State string `json:"state"`
		} `json:"location"`
		EmploymentType string `json:"employmentType"`
	} `json:"result"`
}

type workdayResponse struct {
	Total       int `json:"total"`
	Limit       int `json:"limit"`
	Offset      int `json:"offset"`
	JobPostings []struct {
		Title         string   `json:"title"`
		ExternalPath  string   `json:"externalPath"`
		PostedOn      string   `json:"postedOn"`
		BulletFields  []string `json:"bulletFields"`
		LocationsText string   `json:"locationsText"`
	} `json:"jobPostings"`
}

func decodePage(provider Provider, body []byte, pageNumber, pageSize int, source ...SourceConfig) (Page, error) {
	page := Page{Complete: true, FullSnapshot: true, PageNumber: pageNumber}
	switch provider {
	case ProviderGreenhouse:
		var result greenhouseResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		for _, item := range result.Jobs {
			id := strconv.FormatInt(item.ID, 10)
			locations := []string{}
			if strings.TrimSpace(item.Location.Name) != "" {
				locations = append(locations, strings.TrimSpace(item.Location.Name))
			}
			for _, office := range item.Offices {
				for _, value := range []string{office.Location, office.Name} {
					if value = strings.TrimSpace(value); value != "" && !containsFold(locations, value) {
						locations = append(locations, value)
					}
				}
			}
			updatedAt, err := parseTimestamp(item.UpdatedAt)
			if err != nil {
				return Page{}, fmt.Errorf("job %s has invalid updated_at: %w", id, err)
			}
			posting, err := normalizePosting(Posting{ExternalID: id, Title: item.Title, Description: plainText(item.Content), CanonicalURL: item.AbsoluteURL, ApplyURL: item.AbsoluteURL, UpdatedAt: updatedAt, SourceStatus: "open", Locations: locations})
			if err != nil {
				return Page{}, err
			}
			page.Items = append(page.Items, posting)
		}
	case ProviderLever:
		// Lever's offset pagination is not a stable snapshot cursor. A full
		// page walk is useful for discovery, but it is not proof that listings
		// missing from this walk have been removed.
		page.FullSnapshot = false
		var result leverResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		for _, item := range result {
			description := item.DescriptionPlain
			if description == "" {
				description = plainText(item.Description)
			}
			locations := []string{}
			if value := strings.TrimSpace(item.Categories.Location); value != "" {
				locations = append(locations, value)
			}
			for _, value := range item.Categories.AllLocations {
				value = strings.TrimSpace(value)
				if value != "" && !containsFold(locations, value) {
					locations = append(locations, value)
				}
			}
			posting, err := normalizePosting(Posting{
				ExternalID: item.ID, Title: item.Text, Description: description, CanonicalURL: item.HostedURL,
				ApplyURL: item.ApplyURL, SourceStatus: "open", Locations: locations,
				WorkplaceMode: normalizeWorkplace(item.WorkplaceType), Department: item.Categories.Department,
				Team: item.Categories.Team, Level: item.Categories.Level, Employment: item.Categories.Commitment,
			})
			if err != nil {
				return Page{}, err
			}
			page.Items = append(page.Items, posting)
		}
		if len(result) >= pageSize {
			next := strconv.Itoa((pageNumber) * pageSize)
			page.NextCursor = next
			page.Complete = false
		}
	case ProviderAshby:
		var result ashbyResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		for _, item := range result.Jobs {
			// Only expose postings that Ashby says should appear on the public
			// job board. Missing listing metadata fails closed as well.
			if item.IsListed == nil || !*item.IsListed {
				continue
			}
			var publishedAt *time.Time
			if strings.TrimSpace(item.PublishedAt) != "" {
				var err error
				publishedAt, err = parseTimestamp(item.PublishedAt)
				if err != nil {
					return Page{}, fmt.Errorf("job %s has invalid publishedAt: %w", item.ID, err)
				}
			}
			locations := []string{}
			if value := strings.TrimSpace(item.Location); value != "" {
				locations = append(locations, value)
			}
			for _, secondary := range item.SecondaryLocations {
				for _, value := range []string{secondary.Location, strings.TrimSpace(strings.Join([]string{secondary.Address.Locality, secondary.Address.Region, secondary.Address.Country}, ", "))} {
					value = strings.Trim(value, ", ")
					if value != "" && !containsFold(locations, value) {
						locations = append(locations, value)
					}
				}
			}
			workplace := normalizeWorkplace(item.WorkplaceType)
			description := item.DescriptionPlain
			if description == "" {
				description = plainText(item.DescriptionHTML)
			}
			externalID := strings.TrimSpace(item.ID)
			if externalID == "" {
				// Ashby's documented public feed omits an immutable ID. The
				// canonical posting URL is the provider's stable key available
				// in that response; do not synthesize a numeric identifier.
				externalID = strings.TrimSpace(item.JobURL)
			}
			posting, err := normalizePosting(Posting{
				ExternalID: externalID, Title: item.Title, Description: description, CanonicalURL: item.JobURL,
				ApplyURL: item.ApplyURL, PublishedAt: publishedAt, SourceStatus: "open", Locations: locations,
				WorkplaceMode: workplace, Department: item.Department, Team: item.Team, Employment: item.EmploymentType,
			})
			if err != nil {
				return Page{}, err
			}
			page.Items = append(page.Items, posting)
		}
	case ProviderSmartRecruiters:
		page.FullSnapshot = false
		var result smartRecruitersResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		for _, item := range result.Content {
			locations := []string{}
			var locParts []string
			if item.Location.City != "" {
				locParts = append(locParts, item.Location.City)
			}
			if item.Location.Region != "" {
				locParts = append(locParts, item.Location.Region)
			}
			if item.Location.Country != "" {
				locParts = append(locParts, item.Location.Country)
			}
			if len(locParts) > 0 {
				locations = append(locations, strings.Join(locParts, ", "))
			}

			workplace := ""
			if item.Location.Remote {
				workplace = "remote"
			}

			var publishedAt *time.Time
			if item.ReleasedDate != "" {
				publishedAt, _ = parseTimestamp(item.ReleasedDate)
			}

			boardID := ""
			if len(source) > 0 {
				boardID = source[0].BoardIdentifier
			}
			canonicalURL := fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", boardID, item.ID)

			posting, err := normalizePosting(Posting{
				ExternalID:    item.ID,
				Title:         item.Name,
				Description:   item.Name,
				CanonicalURL:  canonicalURL,
				ApplyURL:      canonicalURL,
				PublishedAt:   publishedAt,
				SourceStatus:  "open",
				Locations:     locations,
				WorkplaceMode: workplace,
				Department:    item.Department.Label,
				Level:         item.ExperienceLevel.Label,
				Employment:    item.TypeOfEmployment.Label,
			})
			if err == nil {
				page.Items = append(page.Items, posting)
			}
		}
		if result.Offset+len(result.Content) < result.TotalFound && len(result.Content) >= pageSize {
			next := strconv.Itoa(result.Offset + pageSize)
			page.NextCursor = next
			page.Complete = false
		}
	case ProviderBreezy:
		var result breezyResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		for _, item := range result {
			locations := []string{}
			if item.Location.Name != "" {
				locations = append(locations, item.Location.Name)
			}
			workplace := ""
			if item.Location.IsRemote {
				workplace = "remote"
			}
			jobURL := item.URL
			if jobURL == "" {
				jobURL = item.FriendlyURL
			}
			posting, err := normalizePosting(Posting{
				ExternalID:    item.ID,
				Title:         item.Name,
				Description:   item.Name,
				CanonicalURL:  jobURL,
				ApplyURL:      jobURL,
				SourceStatus:  "open",
				Locations:     locations,
				WorkplaceMode: workplace,
				Department:    item.Department,
				Employment:    item.Type.Name,
			})
			if err == nil {
				page.Items = append(page.Items, posting)
			}
		}
	case ProviderRecruitee:
		var result recruiteeResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		for _, item := range result.Offers {
			locations := []string{}
			if item.Location != "" {
				locations = append(locations, item.Location)
			}
			workplace := ""
			if item.Remote {
				workplace = "remote"
			}
			var publishedAt *time.Time
			if item.PublishedAt != "" {
				publishedAt, _ = parseTimestamp(item.PublishedAt)
			}
			posting, err := normalizePosting(Posting{
				ExternalID:    strconv.FormatInt(item.ID, 10),
				Title:         item.Title,
				Description:   plainText(item.Description),
				CanonicalURL:  item.CareersURL,
				ApplyURL:      item.CareersURL,
				PublishedAt:   publishedAt,
				SourceStatus:  "open",
				Locations:     locations,
				WorkplaceMode: workplace,
				Department:    item.Department,
			})
			if err == nil {
				page.Items = append(page.Items, posting)
			}
		}
	case ProviderBambooHR:
		var result bambooHRResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		boardID := ""
		if len(source) > 0 {
			boardID = source[0].BoardIdentifier
		}
		for _, item := range result.Result {
			locations := []string{}
			var locParts []string
			if item.Location.City != "" {
				locParts = append(locParts, item.Location.City)
			}
			if item.Location.State != "" {
				locParts = append(locParts, item.Location.State)
			}
			if len(locParts) > 0 {
				locations = append(locations, strings.Join(locParts, ", "))
			}
			jobURL := fmt.Sprintf("https://%s.bamboohr.com/careers/%s", boardID, item.ID)
			posting, err := normalizePosting(Posting{
				ExternalID:   item.ID,
				Title:        item.JobOpeningName,
				Description:  item.JobOpeningName,
				CanonicalURL: jobURL,
				ApplyURL:     jobURL,
				SourceStatus: "open",
				Locations:    locations,
				Department:   item.Department,
				Employment:   item.EmploymentType,
			})
			if err == nil {
				page.Items = append(page.Items, posting)
			}
		}
	case ProviderWorkday:
		page.FullSnapshot = false
		var result workdayResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return Page{}, err
		}
		baseHost := ""
		if len(source) > 0 {
			if u, err := url.Parse(source[0].Endpoint); err == nil {
				baseHost = u.Host
			}
		}
		for _, item := range result.JobPostings {
			title := strings.TrimSpace(item.Title)
			if title == "" {
				continue
			}
			jobURL := item.ExternalPath
			if !strings.HasPrefix(jobURL, "http") {
				jobURL = "https://" + baseHost + item.ExternalPath
			}
			extID := ""
			for _, b := range item.BulletFields {
				b = strings.TrimSpace(b)
				if (strings.HasPrefix(b, "R") || strings.HasPrefix(b, "JR")) && len(b) > 4 && !strings.Contains(b, " ") {
					extID = b
					break
				}
			}
			if extID == "" {
				parts := strings.Split(item.ExternalPath, "_")
				if len(parts) > 1 {
					extID = parts[len(parts)-1]
				} else {
					extID = item.ExternalPath
				}
			}

			locations := []string{}
			if item.LocationsText != "" {
				locations = append(locations, item.LocationsText)
			}

			posting, err := normalizePosting(Posting{
				ExternalID:   extID,
				Title:        title,
				Description:  title,
				CanonicalURL: jobURL,
				ApplyURL:     jobURL,
				SourceStatus: "open",
				Locations:    locations,
			})
			if err == nil {
				page.Items = append(page.Items, posting)
			}
		}
		if len(source) > 0 && result.Offset+len(result.JobPostings) < result.Total && len(result.JobPostings) >= 20 {
			next := strconv.Itoa(result.Offset + len(result.JobPostings))
			page.NextCursor = next
			page.Complete = false
		}
	case ProviderGeneric:
		baseURL := ""
		if len(source) > 0 {
			baseURL = source[0].Endpoint
		}
		postings, err := decodeGenericHTML(body, baseURL)
		if err != nil {
			return Page{}, err
		}
		page.Items = postings
	default:
		return Page{}, fmt.Errorf("%w: %s", ErrUnsupported, provider)
	}
	return page, nil
}

func normalizePosting(posting Posting) (Posting, error) {
	posting.ExternalID = strings.TrimSpace(posting.ExternalID)
	posting.Title = strings.TrimSpace(posting.Title)
	posting.Description = strings.TrimSpace(posting.Description)
	posting.CanonicalURL = strings.TrimSpace(posting.CanonicalURL)
	posting.ApplyURL = strings.TrimSpace(posting.ApplyURL)
	if posting.ExternalID == "" || posting.Title == "" {
		return Posting{}, errors.New("job listing has no source ID or title")
	}
	if _, err := validHTTPSURL(posting.CanonicalURL); err != nil {
		return Posting{}, fmt.Errorf("job %s has invalid canonical URL: %w", posting.ExternalID, err)
	}
	if posting.ApplyURL != "" {
		if _, err := validHTTPSURL(posting.ApplyURL); err != nil {
			return Posting{}, fmt.Errorf("job %s has invalid apply URL: %w", posting.ExternalID, err)
		}
	}
	for i := range posting.Locations {
		posting.Locations[i] = strings.TrimSpace(posting.Locations[i])
	}
	return posting, nil
}

func validHTTPSURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("expected an absolute HTTPS URL without user information")
	}
	return u, nil
}

func parseTimestamp(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func plainText(value string) string {
	value = xhtml.UnescapeString(value)
	// Greenhouse content may encode the HTML markup itself as entities. Decode
	// only that extra layer before parsing so nested tags never leak as text.
	if strings.Contains(value, "&lt;") || strings.Contains(value, "&gt;") {
		value = xhtml.UnescapeString(value)
	}
	document, err := xhtml.Parse(strings.NewReader(value))
	if err != nil {
		return strings.Join(strings.Fields(value), " ")
	}
	var text strings.Builder
	var visit func(*xhtml.Node, bool)
	visit = func(node *xhtml.Node, hidden bool) {
		if node.Type == xhtml.ElementNode && (node.Data == "script" || node.Data == "style") {
			hidden = true
		}
		if !hidden && node.Type == xhtml.TextNode {
			text.WriteString(node.Data)
			text.WriteByte(' ')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, hidden)
		}
	}
	visit(document, false)
	return strings.Join(strings.Fields(text.String()), " ")
}

func normalizeWorkplace(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "remote", "fully remote", "distributed":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "onsite", "on-site", "on site", "in office":
		return "on_site"
	default:
		return ""
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("refusing connection to non-public address")
		}
		return (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(host, port))
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve source host: %w", err)
	}
	var lastErr error
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			continue
		}
		conn, err := (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("refusing connection to non-public address")
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() && !ip.IsUnspecified()
}

func decodeGenericHTML(body []byte, baseURL string) ([]Posting, error) {
	doc, err := xhtml.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	parsedBase, _ := url.Parse(baseURL)
	var postings []Posting
	seenURLs := make(map[string]bool)

	// 1. First search for Schema.org JSON-LD JobPosting
	var extractJSONLD func(*xhtml.Node)
	extractJSONLD = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if a.Key == "type" && strings.EqualFold(strings.TrimSpace(a.Val), "application/ld+json") {
					if n.FirstChild != nil && n.FirstChild.Data != "" {
						parseJobPostingJSONLD(n.FirstChild.Data, parsedBase, &postings, seenURLs)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractJSONLD(c)
		}
	}
	extractJSONLD(doc)

	if len(postings) > 0 {
		return postings, nil
	}

	// 2. If no JSON-LD found, find structured anchor tags matching career links
	var extractLinks func(*xhtml.Node)
	extractLinks = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "a" {
			href := ""
			for _, a := range n.Attr {
				if a.Key == "href" {
					href = strings.TrimSpace(a.Val)
					break
				}
			}
			hrefLower := strings.ToLower(href)
			if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") && !strings.HasPrefix(href, "mailto:") {
				isJobLink := strings.Contains(hrefLower, "/job/") || strings.Contains(hrefLower, "/jobs/") ||
					strings.Contains(hrefLower, "/career/") || strings.Contains(hrefLower, "/careers/") ||
					strings.Contains(hrefLower, "/position/") || strings.Contains(hrefLower, "/posting/") ||
					strings.Contains(hrefLower, "/opening/") || strings.Contains(hrefLower, "gh_jid=")

				if isJobLink {
					var textBuilder strings.Builder
					var getText func(*xhtml.Node)
					getText = func(node *xhtml.Node) {
						if node.Type == xhtml.TextNode {
							textBuilder.WriteString(node.Data)
							textBuilder.WriteByte(' ')
						}
						for ch := node.FirstChild; ch != nil; ch = ch.NextSibling {
							getText(ch)
						}
					}
					getText(n)
					title := strings.TrimSpace(strings.Join(strings.Fields(textBuilder.String()), " "))
					if len(title) > 3 && len(title) < 120 && !strings.EqualFold(title, "apply") && !strings.EqualFold(title, "view job") && !strings.EqualFold(title, "learn more") {
						resolvedURL := href
						if parsedBase != nil {
							rel, pErr := url.Parse(href)
							if pErr == nil {
								resolvedURL = parsedBase.ResolveReference(rel).String()
							}
						}
						if !seenURLs[resolvedURL] && strings.HasPrefix(resolvedURL, "https://") {
							seenURLs[resolvedURL] = true
							extID := resolvedURL
							p, pErr := normalizePosting(Posting{
								ExternalID:   extID,
								Title:        title,
								Description:  title,
								CanonicalURL: resolvedURL,
								ApplyURL:     resolvedURL,
								SourceStatus: "open",
							})
							if pErr == nil {
								postings = append(postings, p)
							}
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractLinks(c)
		}
	}
	extractLinks(doc)

	return postings, nil
}

func parseJobPostingJSONLD(raw string, base *url.URL, postings *[]Posting, seenURLs map[string]bool) {
	var rawData any
	if err := json.Unmarshal([]byte(raw), &rawData); err != nil {
		return
	}

	var visitItem func(item any)
	visitItem = func(item any) {
		m, ok := item.(map[string]any)
		if !ok {
			return
		}

		if graph, ok := m["@graph"].([]any); ok {
			for _, g := range graph {
				visitItem(g)
			}
			return
		}

		t, _ := m["@type"].(string)
		if !strings.EqualFold(t, "JobPosting") {
			return
		}

		title, _ := m["title"].(string)
		title = strings.TrimSpace(title)
		if title == "" {
			return
		}

		desc, _ := m["description"].(string)
		desc = plainText(desc)

		jobURL, _ := m["url"].(string)
		if jobURL == "" {
			if direct, ok := m["directApply"].(string); ok {
				jobURL = direct
			}
		}
		if jobURL == "" && base != nil {
			jobURL = base.String()
		}

		if base != nil && jobURL != "" {
			if rel, err := url.Parse(jobURL); err == nil {
				jobURL = base.ResolveReference(rel).String()
			}
		}

		if jobURL == "" || seenURLs[jobURL] {
			return
		}
		seenURLs[jobURL] = true

		var publishedAt *time.Time
		if dp, ok := m["datePosted"].(string); ok && dp != "" {
			publishedAt, _ = parseTimestamp(dp)
		}

		empType, _ := m["employmentType"].(string)

		p, err := normalizePosting(Posting{
			ExternalID:   jobURL,
			Title:        title,
			Description:  desc,
			CanonicalURL: jobURL,
			ApplyURL:     jobURL,
			PublishedAt:  publishedAt,
			Employment:   empType,
			SourceStatus: "open",
		})
		if err == nil {
			*postings = append(*postings, p)
		}
	}

	if list, ok := rawData.([]any); ok {
		for _, it := range list {
			visitItem(it)
		}
	} else {
		visitItem(rawData)
	}
}

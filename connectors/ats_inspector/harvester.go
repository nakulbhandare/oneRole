package ats_inspector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/oneRole/models"
)

// SchemaHarvester abstracts offline ingestion of ATS application form definitions into normalized schemas.
type SchemaHarvester interface {
	Provider() string
	CanHarvest(jobURL string) bool
	Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error)
}

// HarvesterRegistry coordinates provider-specific schema harvesters.
type HarvesterRegistry struct {
	harvesters []SchemaHarvester
}

// NewDefaultHarvesterRegistry constructs a registry containing harvesters for all 5 target ATS platforms.
func NewDefaultHarvesterRegistry(client *http.Client) *HarvesterRegistry {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	normalizer := NewASTSchemaNormalizer(nil)
	return &HarvesterRegistry{
		harvesters: []SchemaHarvester{
			NewGreenhouseHarvester(client, normalizer),
			NewAshbyHarvester(client, normalizer),
			NewLeverHarvester(client, normalizer),
			NewSmartRecruitersHarvester(client, normalizer),
			NewWorkdayHarvester(client, normalizer),
		},
	}
}

// Register adds a custom harvester to the registry.
func (r *HarvesterRegistry) Register(h SchemaHarvester) {
	r.harvesters = append(r.harvesters, h)
}

// GetHarvester finds the matching harvester for a given job URL.
func (r *HarvesterRegistry) GetHarvester(jobURL string) (SchemaHarvester, error) {
	for _, h := range r.harvesters {
		if h.CanHarvest(jobURL) {
			return h, nil
		}
	}
	return nil, fmt.Errorf("no harvester registered for job URL: %s", jobURL)
}

// Harvest routes harvesting to the appropriate provider harvester.
func (r *HarvesterRegistry) Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error) {
	h, err := r.GetHarvester(jobURL)
	if err != nil {
		return nil, err
	}
	return h.Harvest(ctx, jobURL)
}

// --- Greenhouse Harvester ---

type GreenhouseHarvester struct {
	client     *http.Client
	normalizer *ASTSchemaNormalizer
}

func NewGreenhouseHarvester(client *http.Client, normalizer *ASTSchemaNormalizer) *GreenhouseHarvester {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if normalizer == nil {
		normalizer = NewASTSchemaNormalizer(nil)
	}
	return &GreenhouseHarvester{client: client, normalizer: normalizer}
}

func (h *GreenhouseHarvester) Provider() string {
	return "greenhouse"
}

func (h *GreenhouseHarvester) CanHarvest(jobURL string) bool {
	u, err := url.Parse(jobURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "greenhouse.io")
}

func (h *GreenhouseHarvester) Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("parse greenhouse job URL: %w", err)
	}

	board, jobID, err := h.parseBoardAndJobID(u)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s?questions=true", board, jobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create greenhouse request: %w", err)
	}
	req.Header.Set("User-Agent", "OneRole-Engine/3.0")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute greenhouse request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("greenhouse api returned status %d for %s", resp.StatusCode, apiURL)
	}

	var ghResp struct {
		ID        any    `json:"id"`
		Title     string `json:"title"`
		Questions []struct {
			Name     string `json:"name"`
			Label    string `json:"label"`
			Required bool   `json:"required"`
			Type     string `json:"type"`
			Fields   []struct {
				Name   string `json:"name"`
				Type   string `json:"type"`
				Values []struct {
					Label string `json:"label"`
					Value any    `json:"value"`
				} `json:"values"`
			} `json:"fields"`
		} `json:"questions"`
		Compliance []struct {
			Type      string `json:"type"`
			Questions []struct {
				Label    string `json:"label"`
				Required bool   `json:"required"`
				Fields   []struct {
					Name   string `json:"name"`
					Type   string `json:"type"`
					Values []struct {
						Label string `json:"label"`
						Value any    `json:"value"`
					} `json:"values"`
				} `json:"fields"`
			} `json:"questions"`
		} `json:"compliance"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghResp); err != nil {
		return nil, fmt.Errorf("decode greenhouse response: %w", err)
	}

	var rawFields []RawATSField
	for _, q := range ghResp.Questions {
		fID := strings.TrimSpace(q.Name)
		fType := q.Type
		var opts []string
		optVals := make(map[string]any)

		if len(q.Fields) > 0 {
			// Sub-field handling: if question root name is empty, extract from fields[0].name
			if fID == "" {
				for _, sub := range q.Fields {
					if strings.TrimSpace(sub.Name) != "" {
						fID = strings.TrimSpace(sub.Name)
						break
					}
				}
			}
			// Special handling for resume file input vs textarea
			for _, sub := range q.Fields {
				if sub.Type == "input_file" {
					fID = sub.Name
					fType = "input_file"
					break
				}
			}
			if fType == "" && q.Fields[0].Type != "" {
				fType = q.Fields[0].Type
			}
			for _, v := range q.Fields[0].Values {
				if v.Label != "" {
					opts = append(opts, v.Label)
					if v.Value != nil {
						optVals[v.Label] = v.Value
					}
				}
			}
		}

		if fID == "" {
			fID = fmt.Sprintf("gh_q_%s", h.normalizer.computeFieldHash(q.Label))
		}

		rawFields = append(rawFields, RawATSField{
			ID:           fID,
			Label:        q.Label,
			Type:         fType,
			Required:     q.Required,
			Options:      opts,
			OptionValues: optVals,
		})
	}

	// Ingest compliance questions (EEOC)
	for _, comp := range ghResp.Compliance {
		for _, q := range comp.Questions {
			fID := ""
			fType := "select"
			var opts []string
			optVals := make(map[string]any)
			if len(q.Fields) > 0 {
				fID = q.Fields[0].Name
				if q.Fields[0].Type != "" {
					fType = q.Fields[0].Type
				}
				for _, v := range q.Fields[0].Values {
					if v.Label != "" {
						opts = append(opts, v.Label)
						if v.Value != nil {
							optVals[v.Label] = v.Value
						}
					}
				}
			}
			if fID == "" {
				fID = fmt.Sprintf("gh_comp_%s", h.normalizer.computeFieldHash(q.Label))
			}
			rawFields = append(rawFields, RawATSField{
				ID:           fID,
				Label:        q.Label,
				Type:         fType,
				Required:     q.Required,
				Options:      opts,
				OptionValues: optVals,
			})
		}
	}

	target := models.SubmissionTarget{
		Tier:     1,
		Endpoint: fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s", board, jobID),
		Method:   http.MethodPost,
		Encoding: "multipart/form-data",
	}

	return h.normalizer.Normalize("greenhouse", board, jobID, jobURL, target, rawFields, nil), nil
}

func (h *GreenhouseHarvester) parseBoardAndJobID(u *url.URL) (string, string, error) {
	// Parse /board/jobs/jobId
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 3 && parts[1] == "jobs" {
		return parts[0], parts[2], nil
	}
	// Parse /embed/job_app?for=board&token=jobId
	if strings.Contains(u.Path, "embed/job_app") {
		q := u.Query()
		board := q.Get("for")
		token := q.Get("token")
		if board != "" && token != "" {
			return board, token, nil
		}
	}
	// Parse /jobs/jobId?gh_jid=jobId
	if len(parts) >= 2 && parts[0] == "jobs" {
		if jid := u.Query().Get("gh_jid"); jid != "" {
			return "default", jid, nil
		}
		return "default", parts[1], nil
	}
	return "", "", fmt.Errorf("unable to extract board and jobId from greenhouse url: %s", u.String())
}

// --- Ashby Harvester ---

type AshbyHarvester struct {
	client     *http.Client
	normalizer *ASTSchemaNormalizer
}

func NewAshbyHarvester(client *http.Client, normalizer *ASTSchemaNormalizer) *AshbyHarvester {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if normalizer == nil {
		normalizer = NewASTSchemaNormalizer(nil)
	}
	return &AshbyHarvester{client: client, normalizer: normalizer}
}

func (h *AshbyHarvester) Provider() string {
	return "ashby"
}

func (h *AshbyHarvester) CanHarvest(jobURL string) bool {
	u, err := url.Parse(jobURL)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Host), "ashbyhq.com")
}

func (h *AshbyHarvester) Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("parse ashby job URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid ashby job URL structure: %s", jobURL)
	}
	org := parts[0]
	jobID := parts[1]

	// 1. Try GraphQL query endpoint
	rawFields, err := h.harvestViaGraphQL(ctx, org, jobID)
	if err != nil || len(rawFields) == 0 {
		// 2. Fallback to HTML Next.js inspection
		rawFields, err = h.harvestViaHTML(ctx, jobURL)
		if err != nil {
			return nil, fmt.Errorf("harvest ashby form: %w", err)
		}
	}

	target := models.SubmissionTarget{
		Tier:     1,
		Endpoint: "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiSubmitSingleApplicationFormAction",
		Method:   http.MethodPost,
		Encoding: "application/json",
	}

	return h.normalizer.Normalize("ashby", org, jobID, jobURL, target, rawFields, nil), nil
}

func (h *AshbyHarvester) harvestViaGraphQL(ctx context.Context, org, jobID string) ([]RawATSField, error) {
	gqlQuery := `query ApiJobPosting($organizationHostedJobsPageName: String!, $jobPostingId: String!) {
  jobPosting(organizationHostedJobsPageName: $organizationHostedJobsPageName, jobPostingId: $jobPostingId) {
    id
    title
    applicationForm {
      id
      sections {
        title
        fieldEntries {
          id
          isRequired
          field
        }
      }
    }
  }
}`

	payload := map[string]any{
		"operationName": "ApiJobPosting",
		"variables": map[string]any{
			"organizationHostedJobsPageName": org,
			"jobPostingId":                   jobID,
		},
		"query": gqlQuery,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://jobs.ashbyhq.com/api/non-user-graphql?op=ApiJobPosting", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ashby graphql error: status %d", resp.StatusCode)
	}

	var res struct {
		Data struct {
			JobPosting struct {
				ApplicationForm struct {
					Sections []struct {
						FieldEntries []struct {
							ID         string `json:"id"`
							IsRequired bool   `json:"isRequired"`
							Field      struct {
								ID               string `json:"id"`
								Path             string `json:"path"`
								Title            string `json:"title"`
								Type             string `json:"type"`
								SelectableValues []struct {
									Label string `json:"label"`
									Value string `json:"value"`
								} `json:"selectableValues"`
							} `json:"field"`
						} `json:"fieldEntries"`
					} `json:"sections"`
				} `json:"applicationForm"`
			} `json:"jobPosting"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var rawFields []RawATSField
	for _, sec := range res.Data.JobPosting.ApplicationForm.Sections {
		for _, fe := range sec.FieldEntries {
			var opts []string
			optVals := make(map[string]any)
			for _, sv := range fe.Field.SelectableValues {
				lbl := sv.Label
				if lbl == "" {
					lbl = sv.Value
				}
				if lbl != "" {
					opts = append(opts, lbl)
					optVals[lbl] = sv.Value
				}
			}

			fieldID := fe.ID
			if fieldID == "" {
				fieldID = fe.Field.Path
			}
			if fieldID == "" {
				fieldID = fe.Field.ID
			}

			rawFields = append(rawFields, RawATSField{
				ID:           fieldID,
				Label:        fe.Field.Title,
				Type:         fe.Field.Type,
				Required:     fe.IsRequired,
				Options:      opts,
				OptionValues: optVals,
				Metadata: map[string]string{
					"field_path": fe.Field.Path,
				},
			})
		}
	}

	return rawFields, nil
}

func (h *AshbyHarvester) harvestViaHTML(ctx context.Context, jobURL string) ([]RawATSField, error) {
	appURL := strings.TrimRight(jobURL, "/")
	if !strings.HasSuffix(appURL, "/application") {
		appURL = appURL + "/application"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	body := string(bodyBytes)

	reNext := regexp.MustCompile(`<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)
	match := reNext.FindStringSubmatch(body)
	if len(match) < 2 {
		return nil, fmt.Errorf("no __NEXT_DATA__ script found in ashby html")
	}

	var nextData struct {
		Props struct {
			PageProps struct {
				JobPosting struct {
					ApplicationForm struct {
						Sections []struct {
							Fields []struct {
								ID               string `json:"id"`
								Title            string `json:"title"`
								Type             string `json:"type"`
								IsRequired       bool   `json:"isRequired"`
								SelectableValues []struct {
									Label string `json:"label"`
									Value string `json:"value"`
								} `json:"selectableValues"`
							} `json:"fields"`
						} `json:"sections"`
					} `json:"applicationForm"`
				} `json:"jobPosting"`
			} `json:"pageProps"`
		} `json:"props"`
	}

	if err := json.Unmarshal([]byte(match[1]), &nextData); err != nil {
		return nil, err
	}

	var rawFields []RawATSField
	for _, sec := range nextData.Props.PageProps.JobPosting.ApplicationForm.Sections {
		for _, f := range sec.Fields {
			var opts []string
			optVals := make(map[string]any)
			for _, sv := range f.SelectableValues {
				lbl := sv.Label
				if lbl == "" {
					lbl = sv.Value
				}
				if lbl != "" {
					opts = append(opts, lbl)
					optVals[lbl] = sv.Value
				}
			}
			rawFields = append(rawFields, RawATSField{
				ID:           f.ID,
				Label:        f.Title,
				Type:         f.Type,
				Required:     f.IsRequired,
				Options:      opts,
				OptionValues: optVals,
			})
		}
	}

	return rawFields, nil
}

// --- Lever Harvester ---

type LeverHarvester struct {
	client     *http.Client
	normalizer *ASTSchemaNormalizer
}

func NewLeverHarvester(client *http.Client, normalizer *ASTSchemaNormalizer) *LeverHarvester {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if normalizer == nil {
		normalizer = NewASTSchemaNormalizer(nil)
	}
	return &LeverHarvester{client: client, normalizer: normalizer}
}

func (h *LeverHarvester) Provider() string {
	return "lever"
}

func (h *LeverHarvester) CanHarvest(jobURL string) bool {
	u, err := url.Parse(jobURL)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Host), "lever.co")
}

func (h *LeverHarvester) Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("parse lever job URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid lever job URL structure: %s", jobURL)
	}
	company := parts[0]
	jobID := parts[1]

	var rawFields []RawATSField

	// Standard Lever fields present on all apply forms
	rawFields = append(rawFields,
		RawATSField{ID: "name", Label: "Full Name", Type: "text", Required: true},
		RawATSField{ID: "email", Label: "Email", Type: "email", Required: true},
		RawATSField{ID: "phone", Label: "Phone", Type: "tel", Required: false},
		RawATSField{ID: "org", Label: "Current Company", Type: "text", Required: false},
		RawATSField{ID: "resume", Label: "Resume/CV", Type: "file", Required: true},
		RawATSField{ID: "urls[LinkedIn]", Label: "LinkedIn URL", Type: "text", Required: false},
		RawATSField{ID: "urls[GitHub]", Label: "GitHub URL", Type: "text", Required: false},
		RawATSField{ID: "urls[Portfolio]", Label: "Portfolio URL", Type: "text", Required: false},
		RawATSField{ID: "comments", Label: "Additional Information", Type: "textarea", Required: false},
	)

	// Fetch apply page HTML to harvest custom card inputs
	applyURL := fmt.Sprintf("https://jobs.lever.co/%s/%s/apply", company, jobID)
	applyReq, err := http.NewRequestWithContext(ctx, http.MethodGet, applyURL, nil)
	if err == nil {
		applyReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		if resp, err := h.client.Do(applyReq); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				cardFields := h.parseLeverApplyDOM(string(bodyBytes))
				rawFields = append(rawFields, cardFields...)
			}
		}
	}

	target := models.SubmissionTarget{
		Tier:     1,
		Endpoint: applyURL,
		Method:   http.MethodPost,
		Encoding: "multipart/form-data",
	}

	return h.normalizer.Normalize("lever", company, jobID, jobURL, target, rawFields, nil), nil
}

func (h *LeverHarvester) parseLeverApplyDOM(html string) []RawATSField {
	var fields []RawATSField
	// Search for card inputs: cards[{cardId}][field{idx}]
	reCard := regexp.MustCompile(`name=["'](cards\[[^\]]+\]\[[^\]]+\])["'][^>]*>`)
	matches := reCard.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool)

	for _, m := range matches {
		nameAttr := m[1]
		if seen[nameAttr] {
			continue
		}
		seen[nameAttr] = true

		fType := "text"
		if strings.Contains(nameAttr, "field0") {
			fType = "checkbox"
		}

		fields = append(fields, RawATSField{
			ID:       nameAttr,
			Label:    nameAttr,
			Type:     fType,
			Required: strings.Contains(html, nameAttr) && strings.Contains(html, "required"),
		})
	}

	return fields
}

// --- SmartRecruiters Harvester ---

type SmartRecruitersHarvester struct {
	client     *http.Client
	normalizer *ASTSchemaNormalizer
}

func NewSmartRecruitersHarvester(client *http.Client, normalizer *ASTSchemaNormalizer) *SmartRecruitersHarvester {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if normalizer == nil {
		normalizer = NewASTSchemaNormalizer(nil)
	}
	return &SmartRecruitersHarvester{client: client, normalizer: normalizer}
}

func (h *SmartRecruitersHarvester) Provider() string {
	return "smartrecruiters"
}

func (h *SmartRecruitersHarvester) CanHarvest(jobURL string) bool {
	u, err := url.Parse(jobURL)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Host), "smartrecruiters.com")
}

func (h *SmartRecruitersHarvester) Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("parse smartrecruiters job URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid smartrecruiters job URL: %s", jobURL)
	}
	company := parts[0]
	// Handle {jobId}-{slug}
	rawJobID := parts[1]
	jobIDParts := strings.Split(rawJobID, "-")
	jobID := jobIDParts[0]

	// Query posting API for structured custom fields
	apiURL := fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings/%s", company, jobID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "OneRole-Engine/3.0")

	var rawFields []RawATSField

	// Standard SmartRecruiters DOM candidate fields
	rawFields = append(rawFields,
		RawATSField{ID: "firstName", Selector: "input#first-name-input, input[name*='firstName' i]", Label: "First Name", Type: "text", Required: true},
		RawATSField{ID: "lastName", Selector: "input#last-name-input, input[name*='lastName' i]", Label: "Last Name", Type: "text", Required: true},
		RawATSField{ID: "email", Selector: "input#email-input, input[name*='email' i]", Label: "Email", Type: "email", Required: true},
		RawATSField{ID: "phoneNumber", Selector: "input#phone-number-input, input[name*='phone' i]", Label: "Phone", Type: "tel", Required: false},
		RawATSField{ID: "city", Selector: "input#location-input, input[name*='location' i]", Label: "City", Type: "text", Required: false},
		RawATSField{ID: "resume", Selector: "input[type='file']", Label: "Resume", Type: "file", Required: true},
		RawATSField{ID: "web.linkedin", Selector: "input#linkedin-input, input[name*='linkedin' i]", Label: "LinkedIn", Type: "text", Required: false},
		RawATSField{ID: "consent_privacy", Selector: "input[type='checkbox'][id*='privacy' i], input[type='checkbox']", Label: "Privacy Consent", Type: "checkbox", Required: true},
	)

	// Fetch posting API for employer custom fields
	if resp, err := h.client.Do(req); err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var srPosting struct {
				CustomField []struct {
					FieldID    string `json:"fieldId"`
					FieldLabel string `json:"fieldLabel"`
					ValueLabel string `json:"valueLabel"`
				} `json:"customField"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&srPosting); err == nil {
				for _, cf := range srPosting.CustomField {
					rawFields = append(rawFields, RawATSField{
						ID:           cf.FieldID,
						Label:        cf.FieldLabel,
						Type:         "text",
						Required:     false,
						DefaultValue: cf.ValueLabel,
					})
				}
			}
		}
	}

	applyURL := fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s/apply", company, jobID)
	target := models.SubmissionTarget{
		Tier:     2,
		Endpoint: applyURL,
		Method:   http.MethodPost,
		Encoding: "window.__oneRoleApply",
	}

	return h.normalizer.Normalize("smartrecruiters", company, jobID, jobURL, target, rawFields, nil), nil
}

// --- Workday Harvester ---

type WorkdayHarvester struct {
	client     *http.Client
	normalizer *ASTSchemaNormalizer
}

func NewWorkdayHarvester(client *http.Client, normalizer *ASTSchemaNormalizer) *WorkdayHarvester {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if normalizer == nil {
		normalizer = NewASTSchemaNormalizer(nil)
	}
	return &WorkdayHarvester{client: client, normalizer: normalizer}
}

func (h *WorkdayHarvester) Provider() string {
	return "workday"
}

func (h *WorkdayHarvester) CanHarvest(jobURL string) bool {
	u, err := url.Parse(jobURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return strings.Contains(host, "myworkdayjobs.com") || strings.Contains(host, "workday.com")
}

func (h *WorkdayHarvester) Harvest(ctx context.Context, jobURL string) (*models.JobFormSchema, error) {
	if !h.CanHarvest(jobURL) {
		return nil, fmt.Errorf("invalid workday job URL: %s", jobURL)
	}
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("parse workday job URL: %w", err)
	}

	tenant, site, jobID, externalPath := h.parseWorkdayURL(u)
	if tenant == "" || site == "" || jobID == "" {
		return nil, fmt.Errorf("invalid workday job URL: %s", jobURL)
	}

	// Standard Workday 5-Section Hierarchy Fields mapped to data-automation-ids
	var rawFields []RawATSField
	rawFields = append(rawFields,
		// 1. Contact Information
		RawATSField{ID: "legalNameSection_firstName", Selector: "[data-automation-id='legalNameSection_firstName']", Label: "First Name", Type: "text", Required: true},
		RawATSField{ID: "legalNameSection_lastName", Selector: "[data-automation-id='legalNameSection_lastName']", Label: "Last Name", Type: "text", Required: true},
		RawATSField{ID: "addressSection_addressLine1", Selector: "[data-automation-id='addressSection_addressLine1']", Label: "Address", Type: "text", Required: false},
		RawATSField{ID: "addressSection_city", Selector: "[data-automation-id='addressSection_city']", Label: "City", Type: "text", Required: false},
		RawATSField{ID: "email", Selector: "[data-automation-id='email']", Label: "Email", Type: "email", Required: true},
		RawATSField{ID: "phone-number", Selector: "[data-automation-id='phone-number']", Label: "Phone", Type: "tel", Required: false},
		// 2. Experience & Resume
		RawATSField{ID: "file-upload-input", Selector: "input[type='file'][data-automation-id='file-upload-input'], [data-automation-id='file-upload-input']", Label: "Resume", Type: "file", Required: true},
		RawATSField{ID: "jobTitle", Selector: "[data-automation-id='jobTitle']", Label: "Current Job Title", Type: "text", Required: false},
		RawATSField{ID: "company", Selector: "[data-automation-id='company']", Label: "Current Company", Type: "text", Required: false},
		// 3. Questionnaires & Screening
		RawATSField{ID: "workAuthQuestion", Selector: "div[data-automation-id*='questionnaire'] input[type='radio']", Label: "Are you authorized to work in the country?", Type: "radio", Required: true, Options: []string{"Yes", "No"}},
		RawATSField{ID: "sponsorshipQuestion", Selector: "div[data-automation-id*='questionnaire'] input[type='radio']", Label: "Will you require visa sponsorship?", Type: "radio", Required: true, Options: []string{"Yes", "No"}},
		// 4. EEO & Voluntary Disclosures
		RawATSField{ID: "gender", Selector: "button[data-automation-id='gender']", Label: "Gender", Type: "select", Required: false, Options: []string{"Decline to Self-Identify", "Male", "Female"}},
		RawATSField{ID: "ethnicity", Selector: "button[data-automation-id='ethnicity']", Label: "Race / Ethnicity", Type: "select", Required: false, Options: []string{"Decline to Self-Identify", "White", "Asian", "Hispanic / Latino"}},
		RawATSField{ID: "veteranStatus", Selector: "button[data-automation-id='veteranStatus']", Label: "Veteran Status", Type: "select", Required: false, Options: []string{"I am not a protected veteran", "Decline to Self-Identify"}},
		RawATSField{ID: "disabilityStatus", Selector: "button[data-automation-id='disabilityStatus']", Label: "Disability Status", Type: "select", Required: false, Options: []string{"No, I do not have a disability", "Decline to Self-Identify"}},
		// 5. Review & Submit
		RawATSField{ID: "agreement", Selector: "input[type='checkbox'][data-automation-id*='agreement']", Label: "Agreement Consent", Type: "checkbox", Required: true},
	)

	// If external path is available, query CXS Job Details API
	if externalPath != "" && site != "" && tenant != "" {
		cxsURL := fmt.Sprintf("https://%s/wday/cxs/%s/%s%s", u.Host, tenant, site, externalPath)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cxsURL, nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
		if resp, err := h.client.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var cxsDetails struct {
					JobPostingInfo struct {
						ID                       string `json:"id"`
						JobReqID                 string `json:"jobReqId"`
						QuestionnaireID          string `json:"questionnaireId"`
						SecondaryQuestionnaireID string `json:"secondaryQuestionnaireId"`
						CanQuickApply            bool   `json:"canQuickApply"`
					} `json:"jobPostingInfo"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&cxsDetails); err == nil {
					if cxsDetails.JobPostingInfo.JobReqID != "" {
						jobID = cxsDetails.JobPostingInfo.JobReqID
					}
				}
			}
		}
	}

	target := models.SubmissionTarget{
		Tier:     2,
		Endpoint: fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs/%s/apply", u.Host, tenant, site, jobID),
		Method:   http.MethodPost,
		Encoding: "window.__oneRoleApply",
	}

	return h.normalizer.Normalize("workday", tenant, jobID, jobURL, target, rawFields, nil), nil
}

var rxWorkdayLocale = regexp.MustCompile(`(?i)^[a-z]{2,3}((-[a-z]{4})?([-_]([a-z]{2}|\d{3}))?|_[a-z]{4}_([a-z]{2}|\d{3}))$`)

func (h *WorkdayHarvester) parseWorkdayURL(u *url.URL) (tenant, site, jobID, externalPath string) {
	// Domain host often has tenant prefix: tenant.wd12.myworkdayjobs.com
	hostParts := strings.Split(u.Host, ".")
	if len(hostParts) > 0 {
		tenant = hostParts[0]
	}

	rawParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	pathParts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		if p != "" {
			pathParts = append(pathParts, p)
		}
	}

	if len(pathParts) == 1 && rxWorkdayLocale.MatchString(pathParts[0]) {
		return tenant, "", "", ""
	}
	if len(pathParts) > 1 && rxWorkdayLocale.MatchString(pathParts[0]) {
		if pathParts[1] != "job" && pathParts[1] != "details" {
			pathParts = pathParts[1:]
		}
	}

	for len(pathParts) > 0 {
		last := strings.ToLower(pathParts[len(pathParts)-1])
		if last == "apply" || last == "details" {
			pathParts = pathParts[:len(pathParts)-1]
		} else {
			break
		}
	}

	if len(pathParts) >= 1 {
		site = pathParts[0]
	}

	if len(pathParts) >= 3 && (pathParts[1] == "job" || pathParts[1] == "details") {
		externalPath = "/" + strings.Join(pathParts[1:], "/")
		jobID = pathParts[len(pathParts)-1]
		// If jobID ends with _R-xxxx or similar, isolate the ID
		if idx := strings.LastIndex(jobID, "_"); idx != -1 && idx < len(jobID)-1 {
			jobID = jobID[idx+1:]
		}
	} else if len(pathParts) >= 2 && pathParts[1] != "job" && pathParts[1] != "details" {
		jobID = pathParts[len(pathParts)-1]
		if idx := strings.LastIndex(jobID, "_"); idx != -1 && idx < len(jobID)-1 {
			jobID = jobID[idx+1:]
		}
	}

	return tenant, site, jobID, externalPath
}

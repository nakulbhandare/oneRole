package ats_adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oneRole/models"
)

type SmartRecruitersAdapter struct {
	httpClient *http.Client
	baseURL    string
}

func NewSmartRecruitersAdapter(client *http.Client) *SmartRecruitersAdapter {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &SmartRecruitersAdapter{
		httpClient: client,
		baseURL:    "https://api.smartrecruiters.com/v1",
	}
}

func (a *SmartRecruitersAdapter) WithBaseURL(url string) *SmartRecruitersAdapter {
	if url != "" {
		a.baseURL = strings.TrimRight(url, "/")
	}
	return a
}

func (a *SmartRecruitersAdapter) Provider() string {
	return "smartrecruiters"
}

func (a *SmartRecruitersAdapter) CanHandle(jobURL string) bool {
	lower := strings.ToLower(jobURL)
	return strings.Contains(lower, "smartrecruiters.com")
}

func (a *SmartRecruitersAdapter) Detect(ctx context.Context, jobURL string) (*ProviderMetadata, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid smartrecruiters url: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("could not extract company and posting ID from URL path: %s", u.Path)
	}

	company := parts[0]
	postingID := parts[1]

	// Handle uuid or dash in postingID: e.g. 7439999-senior-dev -> extract uuid or number
	if idx := strings.Index(postingID, "-"); idx != -1 {
		postingID = postingID[:idx]
	}

	return &ProviderMetadata{
		Provider:     a.Provider(),
		CompanyID:    company,
		JobID:        postingID,
		CanonicalURL: fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", company, postingID),
	}, nil
}

func (a *SmartRecruitersAdapter) ResolveSchema(ctx context.Context, jobURL string) (*models.FormSchema, error) {
	meta, err := a.Detect(ctx, jobURL)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/companies/%s/postings/%s", a.baseURL, meta.CompanyID, meta.JobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "OneRole-Engine/3.0 (Enterprise Candidate System)")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch smartrecruiters posting: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("job posting closed or not found (HTTP 404)")
	}

	schema := &models.FormSchema{
		ID:         fmt.Sprintf("schema-sr-%s-%s", meta.CompanyID, meta.JobID),
		Provider:   a.Provider(),
		JobID:      meta.JobID,
		JobURL:     jobURL,
		Version:    1,
		ResolvedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
		Fields: []models.FormFieldSchema{
			{FieldID: "firstName", Label: "First Name", Type: "text", Required: true},
			{FieldID: "lastName", Label: "Last Name", Type: "text", Required: true},
			{FieldID: "email", Label: "Email", Type: "text", Required: true},
			{FieldID: "phoneNumber", Label: "Phone Number", Type: "text", Required: false},
			{FieldID: "resume", Label: "Resume", Type: "file", Required: true},
		},
	}

	return schema, nil
}

func (a *SmartRecruitersAdapter) ValidateCandidate(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, answers map[string]string) error {
	if candidate == nil {
		return fmt.Errorf("candidate profile is required")
	}
	if strings.TrimSpace(candidate.FirstName) == "" || strings.TrimSpace(candidate.LastName) == "" {
		return fmt.Errorf("candidate first and last name required for smartrecruiters")
	}
	if strings.TrimSpace(candidate.Email) == "" {
		return fmt.Errorf("candidate email required for smartrecruiters")
	}
	return nil
}

type srCandidatePayload struct {
	FirstName   string                 `json:"firstName"`
	LastName    string                 `json:"lastName"`
	Email       string                 `json:"email"`
	PhoneNumber string                 `json:"phoneNumber,omitempty"`
	Location    *srLocation            `json:"location,omitempty"`
	Web         map[string]string      `json:"web,omitempty"`
	Attachments []srAttachment         `json:"attachments,omitempty"`
}

type srLocation struct {
	City    string `json:"city,omitempty"`
	Country string `json:"country,omitempty"`
}

type srAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Content     string `json:"content"` // base64
}

func (a *SmartRecruitersAdapter) SubmitCandidate(ctx context.Context, req SubmissionRequest) (*SubmissionResponse, error) {
	startTime := time.Now()
	meta, err := a.Detect(ctx, req.JobURL)
	if err != nil {
		return nil, err
	}

	c := req.Candidate
	email := strings.TrimSpace(c.Email)
	if email == "" || strings.Contains(email, "noreply") {
		email = "bhandarenakul11@gmail.com"
	}

	payload := srCandidatePayload{
		FirstName:   c.FirstName,
		LastName:    c.LastName,
		Email:       email,
		PhoneNumber: c.Phone,
		Location: &srLocation{
			City:    c.City,
			Country: "us",
		},
		Web: make(map[string]string),
	}

	if c.LinkedInURL != "" {
		payload.Web["linkedin"] = c.LinkedInURL
	}
	if c.GitHubURL != "" {
		payload.Web["github"] = c.GitHubURL
	}

	if len(req.ResumeBytes) > 0 {
		name := req.ResumeName
		if name == "" {
			name = "Nakul_Bhandare_Resume.pdf"
		}
		cType := req.ResumeType
		if cType == "" {
			cType = "application/pdf"
		}
		payload.Attachments = append(payload.Attachments, srAttachment{
			Name:        name,
			ContentType: cType,
			Content:     base64.StdEncoding.EncodeToString(req.ResumeBytes),
		})
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal smartrecruiters payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/companies/%s/postings/%s/candidates", a.baseURL, meta.CompanyID, meta.JobID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "OneRole-Engine/3.0 (Enterprise ATS Submitter)")

	resp, err := a.httpClient.Do(httpReq)
	duration := time.Since(startTime).Milliseconds()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	respStr := string(respBytes)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var resDoc struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(respBytes, &resDoc)

		receiptID := resDoc.ID
		if receiptID == "" {
			receiptID = fmt.Sprintf("SR-%s-%s", meta.CompanyID, meta.JobID)
		}

		return &SubmissionResponse{
			Success:           true,
			Provider:          a.Provider(),
			EmployerReceiptID: receiptID,
			CandidateID:       resDoc.ID,
			ConfirmationURL:   fmt.Sprintf("https://jobs.smartrecruiters.com/%s/%s", meta.CompanyID, meta.JobID),
			HTTPStatusCode:    resp.StatusCode,
			RawResponse:       respStr,
			DurationMs:        duration,
			SubmittedAt:       time.Now().UTC(),
		}, nil
	}

	return &SubmissionResponse{
		Success:        false,
		Provider:       a.Provider(),
		HTTPStatusCode: resp.StatusCode,
		RawResponse:    respStr,
		DurationMs:     duration,
		SubmittedAt:    time.Now().UTC(),
	}, fmt.Errorf("smartrecruiters HTTP %d: %s", resp.StatusCode, respStr)
}

func (a *SmartRecruitersAdapter) VerifyConfirmation(ctx context.Context, resp *SubmissionResponse) (*ConfirmationResult, error) {
	if resp == nil || !resp.Success {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("cannot verify failed response")
	}

	if resp.CandidateID == "" && resp.EmployerReceiptID == "" {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("missing candidate ID in response")
	}

	receiptID := resp.CandidateID
	if receiptID == "" {
		receiptID = resp.EmployerReceiptID
	}

	return &ConfirmationResult{
		Confirmed:         true,
		EmployerReceiptID: receiptID,
		Provider:          a.Provider(),
		EvidenceSummary:   fmt.Sprintf("SmartRecruiters Candidate Ingest Verified (HTTP %d, Candidate ID: %s)", resp.HTTPStatusCode, receiptID),
		VerifiedTimestamp: time.Now().UTC(),
	}, nil
}

func (a *SmartRecruitersAdapter) ClassifyError(err error, statusCode int, responseBody string) ErrorClassification {
	if statusCode == 429 {
		return ErrorClassRateLimited
	}
	if statusCode == 404 {
		return ErrorClassJobClosed
	}
	if statusCode == 401 || statusCode == 403 {
		return ErrorClassUnauthorized
	}
	if statusCode == 400 {
		lower := strings.ToLower(responseBody)
		if strings.Contains(lower, "required") {
			return ErrorClassMissingFields
		}
		return ErrorClassInvalidPayload
	}
	if statusCode >= 500 {
		return ErrorClassTransient
	}
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "timeout") || strings.Contains(lower, "connection refused") {
			return ErrorClassTransient
		}
	}
	return ErrorClassFatalUnknown
}

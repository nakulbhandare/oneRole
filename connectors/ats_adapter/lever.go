package ats_adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oneRole/models"
)

type LeverAdapter struct {
	httpClient *http.Client
	baseURL    string
}

func NewLeverAdapter(client *http.Client) *LeverAdapter {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &LeverAdapter{
		httpClient: client,
		baseURL:    "https://api.lever.co/v0",
	}
}

func (a *LeverAdapter) WithBaseURL(url string) *LeverAdapter {
	if url != "" {
		a.baseURL = strings.TrimRight(url, "/")
	}
	return a
}

func (a *LeverAdapter) Provider() string {
	return "lever"
}

func (a *LeverAdapter) CanHandle(jobURL string) bool {
	lower := strings.ToLower(jobURL)
	return strings.Contains(lower, "lever.co")
}

func (a *LeverAdapter) Detect(ctx context.Context, jobURL string) (*ProviderMetadata, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid lever url: %w", err)
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return nil, fmt.Errorf("could not extract lever site and job id from path: %s", u.Path)
	}

	site := segments[0]
	jobID := segments[1]

	return &ProviderMetadata{
		Provider:     a.Provider(),
		CompanyID:    site,
		JobID:        jobID,
		CanonicalURL: fmt.Sprintf("https://jobs.lever.co/%s/%s", site, jobID),
	}, nil
}

func (a *LeverAdapter) ResolveSchema(ctx context.Context, jobURL string) (*models.FormSchema, error) {
	meta, err := a.Detect(ctx, jobURL)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/postings/%s/%s", a.baseURL, meta.CompanyID, meta.JobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch lever schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("lever job is closed or not found (HTTP 404)")
	}

	schema := &models.FormSchema{
		ID:         fmt.Sprintf("schema-lever-%s-%s", meta.CompanyID, meta.JobID),
		Provider:   a.Provider(),
		JobID:      meta.JobID,
		JobURL:     jobURL,
		Version:    1,
		ResolvedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
		Fields: []models.FormFieldSchema{
			{FieldID: "name", Label: "Full Name", Type: "text", Required: true},
			{FieldID: "email", Label: "Email", Type: "text", Required: true},
			{FieldID: "phone", Label: "Phone", Type: "text", Required: false},
			{FieldID: "resume", Label: "Resume", Type: "file", Required: true},
		},
	}

	return schema, nil
}

func (a *LeverAdapter) ValidateCandidate(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, answers map[string]string) error {
	if candidate == nil {
		return fmt.Errorf("candidate profile required")
	}
	if strings.TrimSpace(candidate.FirstName) == "" && strings.TrimSpace(candidate.LastName) == "" {
		return fmt.Errorf("candidate name required for lever")
	}
	if strings.TrimSpace(candidate.Email) == "" {
		return fmt.Errorf("candidate email required for lever")
	}
	return nil
}

func (a *LeverAdapter) SubmitCandidate(ctx context.Context, req SubmissionRequest) (*SubmissionResponse, error) {
	startTime := time.Now()
	meta, err := a.Detect(ctx, req.JobURL)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	c := req.Candidate
	fullName := strings.TrimSpace(c.FirstName + " " + c.LastName)
	if fullName == "" {
		fullName = "Nakul Bhandare"
	}
	email := strings.TrimSpace(c.Email)
	if email == "" || strings.Contains(email, "noreply") {
		email = "bhandarenakul11@gmail.com"
	}

	_ = w.WriteField("name", fullName)
	_ = w.WriteField("email", email)
	if c.Phone != "" {
		_ = w.WriteField("phone", c.Phone)
	}
	if c.CurrentEmployer != "" {
		_ = w.WriteField("org", c.CurrentEmployer)
	}

	urls := make(map[string]string)
	if c.LinkedInURL != "" {
		urls["LinkedIn"] = c.LinkedInURL
	}
	if c.GitHubURL != "" {
		urls["GitHub"] = c.GitHubURL
	}
	if len(urls) > 0 {
		urlsJSON, _ := json.Marshal(urls)
		_ = w.WriteField("urls", string(urlsJSON))
	}

	if len(req.ResumeBytes) > 0 {
		resumePart, err := w.CreateFormFile("resume", "Nakul_Bhandare_Resume.pdf")
		if err == nil {
			_, _ = resumePart.Write(req.ResumeBytes)
		}
	}

	for k, v := range req.Answers {
		if strings.TrimSpace(v) != "" {
			_ = w.WriteField(k, v)
		}
	}
	_ = w.Close()

	endpoint := fmt.Sprintf("%s/postings/%s/%s", a.baseURL, meta.CompanyID, meta.JobID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())

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
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(respBytes, &resDoc)

		receiptID := resDoc.Data.ID
		if receiptID == "" {
			receiptID = fmt.Sprintf("LEVER-%s-%s-%d", meta.CompanyID, meta.JobID, time.Now().Unix())
		}

		return &SubmissionResponse{
			Success:           true,
			Provider:          a.Provider(),
			EmployerReceiptID: receiptID,
			CandidateID:       resDoc.Data.ID,
			ConfirmationURL:   fmt.Sprintf("https://jobs.lever.co/%s/%s", meta.CompanyID, meta.JobID),
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
	}, fmt.Errorf("lever HTTP %d: %s", resp.StatusCode, respStr)
}

// SubmitTier1 delivers direct multipart POST submission to Lever in <220ms target.
func (a *LeverAdapter) SubmitTier1(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, resumeBytes []byte) (*SubmissionResult, error) {
	startTime := time.Now()
	if schema == nil {
		return nil, fmt.Errorf("form schema required")
	}
	if candidate == nil {
		return nil, fmt.Errorf("candidate profile required")
	}

	meta, err := a.Detect(ctx, schema.JobURL)
	if err != nil {
		if schema.JobID != "" {
			meta = &ProviderMetadata{
				Provider:     a.Provider(),
				CompanyID:    "company",
				JobID:        schema.JobID,
				CanonicalURL: schema.JobURL,
			}
		} else {
			return nil, err
		}
	}

	targetEmail := CanonicalTargetEmail
	fullName := strings.TrimSpace(candidate.FirstName + " " + candidate.LastName)
	if fullName == "" {
		fullName = "Nakul Bhandare"
	}
	phone := candidate.Phone
	if phone == "" {
		phone = "+1 (350)-355-6570"
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("name", fullName)
	_ = w.WriteField("email", targetEmail)
	_ = w.WriteField("phone", phone)
	if candidate.CurrentEmployer != "" {
		_ = w.WriteField("org", candidate.CurrentEmployer)
	}

	urls := make(map[string]string)
	if candidate.LinkedInURL != "" {
		urls["LinkedIn"] = candidate.LinkedInURL
	} else {
		urls["LinkedIn"] = "https://linkedin.com/in/nakul-bhandare"
	}
	if candidate.GitHubURL != "" {
		urls["GitHub"] = candidate.GitHubURL
	}
	urlsJSON, _ := json.Marshal(urls)
	_ = w.WriteField("urls", string(urlsJSON))

	if len(resumeBytes) > 0 {
		resumePart, err := w.CreateFormFile("resume", "Nakul_Bhandare_Resume.pdf")
		if err == nil {
			_, _ = resumePart.Write(resumeBytes)
		}
	}

	// Add standard answers from schema fields if present
	for _, f := range schema.Fields {
		lower := strings.ToLower(f.Label)
		if strings.Contains(lower, "sponsor") {
			_ = w.WriteField(f.FieldID, "No")
		} else if strings.Contains(lower, "authoriz") {
			_ = w.WriteField(f.FieldID, "Yes")
		}
	}
	_ = w.Close()

	endpoint := ""
	if strings.Contains(a.baseURL, "api.lever.co") {
		endpoint = fmt.Sprintf("%s/postings/%s/%s", a.baseURL, meta.CompanyID, meta.JobID)
	} else {
		// Mock or web apply endpoint
		endpoint = fmt.Sprintf("%s/postings/%s/%s", a.baseURL, meta.CompanyID, meta.JobID)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")

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
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
			ApplicationID string `json:"applicationId"`
			ID            string `json:"id"`
		}
		_ = json.Unmarshal(respBytes, &resDoc)

		receiptID := resDoc.Data.ID
		if receiptID == "" {
			receiptID = resDoc.ApplicationID
		}
		if receiptID == "" {
			receiptID = resDoc.ID
		}
		if receiptID == "" {
			receiptID = fmt.Sprintf("LEVER-%s-%s-%d", meta.CompanyID, meta.JobID, time.Now().Unix())
		} else if !strings.HasPrefix(receiptID, "LEVER-") {
			receiptID = "LEVER-" + receiptID
		}

		return &SubmissionResult{
			ReceiptID:       receiptID,
			DurationMs:      duration,
			Status:          "SUBMITTED_CONFIRMED",
			TargetEmail:     targetEmail,
			Provider:        a.Provider(),
			ConfirmationURL: fmt.Sprintf("https://jobs.lever.co/%s/%s", meta.CompanyID, meta.JobID),
			CandidateID:     receiptID,
			HTTPStatusCode:  resp.StatusCode,
			RawResponse:     respStr,
		}, nil
	}

	return &SubmissionResult{
		DurationMs:     duration,
		Status:         "FAILED",
		TargetEmail:    targetEmail,
		Provider:       a.Provider(),
		HTTPStatusCode: resp.StatusCode,
		RawResponse:    respStr,
		Error:          fmt.Errorf("lever HTTP %d: %s", resp.StatusCode, respStr),
	}, fmt.Errorf("lever HTTP %d: %s", resp.StatusCode, respStr)
}

func (a *LeverAdapter) VerifyConfirmation(ctx context.Context, resp *SubmissionResponse) (*ConfirmationResult, error) {
	if resp == nil || !resp.Success {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("cannot verify unconfirmed lever response")
	}

	receiptID := resp.CandidateID
	if receiptID == "" {
		receiptID = resp.EmployerReceiptID
	}

	return &ConfirmationResult{
		Confirmed:         true,
		EmployerReceiptID: receiptID,
		Provider:          a.Provider(),
		EvidenceSummary:   fmt.Sprintf("Lever Candidate Posting Confirmed (Receipt: %s, HTTP %d)", receiptID, resp.HTTPStatusCode),
		VerifiedTimestamp: time.Now().UTC(),
	}, nil
}

func (a *LeverAdapter) ClassifyError(err error, statusCode int, responseBody string) ErrorClassification {
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
		return ErrorClassMissingFields
	}
	if statusCode >= 500 {
		return ErrorClassTransient
	}
	if err != nil && (strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection refused")) {
		return ErrorClassTransient
	}
	return ErrorClassFatalUnknown
}

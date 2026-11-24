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
	"strconv"
	"strings"
	"time"

	"github.com/oneRole/models"
)

const CanonicalTargetEmail = "bhandarenakul11@gmail.com"

type GreenhouseAdapter struct {
	httpClient *http.Client
	baseURL    string
}

func NewGreenhouseAdapter(client *http.Client) *GreenhouseAdapter {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &GreenhouseAdapter{
		httpClient: client,
		baseURL:    "https://boards-api.greenhouse.io/v1",
	}
}

func (a *GreenhouseAdapter) WithBaseURL(url string) *GreenhouseAdapter {
	if url != "" {
		a.baseURL = strings.TrimRight(url, "/")
	}
	return a
}

func (a *GreenhouseAdapter) Provider() string {
	return "greenhouse"
}

func (a *GreenhouseAdapter) CanHandle(jobURL string) bool {
	lower := strings.ToLower(jobURL)
	return strings.Contains(lower, "greenhouse.io") || strings.Contains(lower, "gh_jid=")
}

func (a *GreenhouseAdapter) Detect(ctx context.Context, jobURL string) (*ProviderMetadata, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid greenhouse url: %w", err)
	}

	search := queryValues(u.Query())
	token := search.getFallback("token", "for")
	ghJid := u.Query().Get("gh_jid")

	if token != "" && ghJid != "" {
		return &ProviderMetadata{
			Provider:     a.Provider(),
			CompanyID:    strings.TrimSpace(token),
			JobID:        strings.TrimSpace(ghJid),
			CanonicalURL: fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", token, ghJid),
		}, nil
	}

	if ghJid != "" {
		host := strings.TrimPrefix(u.Hostname(), "www.")
		host = strings.TrimPrefix(host, "jobs.")
		host = strings.TrimPrefix(host, "careers.")
		board := strings.Split(host, ".")[0]
		return &ProviderMetadata{
			Provider:     a.Provider(),
			CompanyID:    board,
			JobID:        ghJid,
			CanonicalURL: fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", board, ghJid),
		}, nil
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) >= 3 && segments[1] == "jobs" {
		return &ProviderMetadata{
			Provider:     a.Provider(),
			CompanyID:    segments[0],
			JobID:        segments[2],
			CanonicalURL: fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", segments[0], segments[2]),
		}, nil
	}
	if len(segments) >= 4 && segments[0] == "job-board" && segments[2] == "job" {
		return &ProviderMetadata{
			Provider:     a.Provider(),
			CompanyID:    segments[1],
			JobID:        segments[3],
			CanonicalURL: fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", segments[1], segments[3]),
		}, nil
	}
	if len(segments) >= 2 && (segments[0] == "job-board" || segments[0] == "job-boards" || segments[0] == "embed") {
		return &ProviderMetadata{
			Provider:     a.Provider(),
			CompanyID:    segments[1],
			JobID:        segments[len(segments)-1],
			CanonicalURL: fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", segments[1], segments[len(segments)-1]),
		}, nil
	}

	return nil, fmt.Errorf("could not extract greenhouse board and job id from %s", jobURL)
}

type queryValues url.Values

func (q queryValues) getFallback(keys ...string) string {
	for _, k := range keys {
		if val := url.Values(q).Get(k); val != "" {
			return val
		}
	}
	return ""
}

func (a *GreenhouseAdapter) ResolveSchema(ctx context.Context, jobURL string) (*models.FormSchema, error) {
	meta, err := a.Detect(ctx, jobURL)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/boards/%s/jobs/%s?questions=true", a.baseURL, meta.CompanyID, meta.JobID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "OneRole-Engine/3.0 (Enterprise Candidate System)")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch greenhouse schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("greenhouse job is closed or does not exist (HTTP 404)")
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("greenhouse board requires authentication (HTTP %d)", resp.StatusCode)
	}

	respBytes, _ := io.ReadAll(resp.Body)
	var ghJob struct {
		ID        int    `json:"id"`
		Title     string `json:"title"`
		Questions []struct {
			Name     string `json:"name"`
			Label    string `json:"label"`
			Required bool   `json:"required"`
			Type     string `json:"type"`
		} `json:"questions"`
	}
	_ = json.Unmarshal(respBytes, &ghJob)

	schema := &models.FormSchema{
		ID:         fmt.Sprintf("schema-gh-%s-%s", meta.CompanyID, meta.JobID),
		Provider:   a.Provider(),
		JobID:      meta.JobID,
		JobURL:     jobURL,
		Version:    1,
		ResolvedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
		Fields: []models.FormFieldSchema{
			{FieldID: "first_name", Label: "First Name", Type: "text", Required: true},
			{FieldID: "last_name", Label: "Last Name", Type: "text", Required: true},
			{FieldID: "email", Label: "Email", Type: "text", Required: true},
			{FieldID: "phone", Label: "Phone", Type: "text", Required: false},
			{FieldID: "resume", Label: "Resume", Type: "file", Required: true},
		},
	}

	for _, q := range ghJob.Questions {
		schema.Fields = append(schema.Fields, models.FormFieldSchema{
			FieldID:  q.Name,
			Label:    q.Label,
			Type:     q.Type,
			Required: q.Required,
		})
	}

	return schema, nil
}

func (a *GreenhouseAdapter) ValidateCandidate(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, answers map[string]string) error {
	if candidate == nil {
		return fmt.Errorf("candidate profile required")
	}
	if strings.TrimSpace(candidate.FirstName) == "" || strings.TrimSpace(candidate.LastName) == "" {
		return fmt.Errorf("first and last name required for greenhouse")
	}
	if strings.TrimSpace(candidate.Email) == "" {
		return fmt.Errorf("email required for greenhouse")
	}
	return nil
}

func (a *GreenhouseAdapter) SubmitCandidate(ctx context.Context, req SubmissionRequest) (*SubmissionResponse, error) {
	startTime := time.Now()
	meta, err := a.Detect(ctx, req.JobURL)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	c := req.Candidate
	email := strings.TrimSpace(c.Email)
	if email == "" || strings.Contains(email, "noreply") {
		email = "bhandarenakul11@gmail.com"
	}

	_ = w.WriteField("first_name", c.FirstName)
	_ = w.WriteField("last_name", c.LastName)
	_ = w.WriteField("email", email)
	if c.Phone != "" {
		_ = w.WriteField("phone", c.Phone)
	}
	if c.City != "" {
		_ = w.WriteField("location", c.City)
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

	endpoint := fmt.Sprintf("%s/boards/%s/jobs/%s", a.baseURL, meta.CompanyID, meta.JobID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

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
			ID            int    `json:"id"`
			ApplicationID int    `json:"application_id"`
		}
		_ = json.Unmarshal(respBytes, &resDoc)

		appIDNum := resDoc.ID
		if appIDNum == 0 {
			appIDNum = resDoc.ApplicationID
		}

		receiptID := ""
		if appIDNum > 0 {
			receiptID = fmt.Sprintf("GH-%d", appIDNum)
		} else {
			receiptID = fmt.Sprintf("GH-%s-%s-%d", meta.CompanyID, meta.JobID, time.Now().Unix())
		}

		return &SubmissionResponse{
			Success:           true,
			Provider:          a.Provider(),
			EmployerReceiptID: receiptID,
			CandidateID:       strconv.Itoa(appIDNum),
			ConfirmationURL:   fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", meta.CompanyID, meta.JobID),
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
	}, fmt.Errorf("greenhouse HTTP %d: %s", resp.StatusCode, respStr)
}

// SubmitTier1 delivers direct headless multipart POST submission to Greenhouse in <250ms target.
func (a *GreenhouseAdapter) SubmitTier1(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, resumeBytes []byte) (*SubmissionResult, error) {
	startTime := time.Now()
	if schema == nil {
		return nil, fmt.Errorf("form schema is required")
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
	firstName := strings.TrimSpace(candidate.FirstName)
	if firstName == "" {
		firstName = "Nakul"
	}
	lastName := strings.TrimSpace(candidate.LastName)
	if lastName == "" {
		lastName = "Bhandare"
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("first_name", firstName)
	_ = w.WriteField("last_name", lastName)
	_ = w.WriteField("email", targetEmail)
	if candidate.Phone != "" {
		_ = w.WriteField("phone", candidate.Phone)
	} else {
		_ = w.WriteField("phone", "+1 (350)-355-6570")
	}
	if candidate.City != "" {
		_ = w.WriteField("location", candidate.City)
	} else {
		_ = w.WriteField("location", "San Francisco, CA")
	}
	if candidate.LinkedInURL != "" {
		_ = w.WriteField("urls[LinkedIn]", candidate.LinkedInURL)
	}
	if candidate.GitHubURL != "" {
		_ = w.WriteField("urls[GitHub]", candidate.GitHubURL)
	}

	if len(resumeBytes) > 0 {
		resumePart, err := w.CreateFormFile("resume", "Nakul_Bhandare_Resume.pdf")
		if err == nil {
			_, _ = resumePart.Write(resumeBytes)
		}
	}

	for _, field := range schema.Fields {
		if field.FieldID != "first_name" && field.FieldID != "last_name" && field.FieldID != "email" && field.FieldID != "phone" && field.FieldID != "resume" {
			val := ""
			lowerLabel := strings.ToLower(field.Label)
			if strings.Contains(lowerLabel, "authorized") || strings.Contains(lowerLabel, "authorization") {
				val = "Yes"
			} else if strings.Contains(lowerLabel, "sponsor") {
				val = "No"
			}
			if val != "" {
				_ = w.WriteField(field.FieldID, val)
			}
		}
	}
	_ = w.Close()

	endpoint := fmt.Sprintf("%s/boards/%s/jobs/%s", a.baseURL, meta.CompanyID, meta.JobID)
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
			ID            int `json:"id"`
			ApplicationID int `json:"application_id"`
		}
		_ = json.Unmarshal(respBytes, &resDoc)

		appIDNum := resDoc.ID
		if appIDNum == 0 {
			appIDNum = resDoc.ApplicationID
		}

		receiptID := ""
		if appIDNum > 0 {
			receiptID = fmt.Sprintf("GH-%d", appIDNum)
		} else {
			receiptID = fmt.Sprintf("GH-%s-%s-%d", meta.CompanyID, meta.JobID, time.Now().Unix())
		}

		return &SubmissionResult{
			ReceiptID:       receiptID,
			DurationMs:      duration,
			Status:          "SUBMITTED_CONFIRMED",
			TargetEmail:     targetEmail,
			Provider:        a.Provider(),
			ConfirmationURL: fmt.Sprintf("https://boards.greenhouse.io/%s/jobs/%s", meta.CompanyID, meta.JobID),
			CandidateID:     strconv.Itoa(appIDNum),
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
		Error:          fmt.Errorf("greenhouse HTTP %d: %s", resp.StatusCode, respStr),
	}, fmt.Errorf("greenhouse HTTP %d: %s", resp.StatusCode, respStr)
}

func (a *GreenhouseAdapter) VerifyConfirmation(ctx context.Context, resp *SubmissionResponse) (*ConfirmationResult, error) {
	if resp == nil || !resp.Success {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("cannot verify unconfirmed greenhouse response")
	}
	if resp.EmployerReceiptID == "" {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("missing greenhouse employer receipt")
	}

	return &ConfirmationResult{
		Confirmed:         true,
		EmployerReceiptID: resp.EmployerReceiptID,
		Provider:          a.Provider(),
		EvidenceSummary:   fmt.Sprintf("Greenhouse API Submission Confirmed (Receipt: %s, HTTP %d)", resp.EmployerReceiptID, resp.HTTPStatusCode),
		VerifiedTimestamp: time.Now().UTC(),
	}, nil
}

func (a *GreenhouseAdapter) ClassifyError(err error, statusCode int, responseBody string) ErrorClassification {
	if statusCode == 429 {
		return ErrorClassRateLimited
	}
	if statusCode == 404 || strings.Contains(strings.ToLower(responseBody), "no longer active") {
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

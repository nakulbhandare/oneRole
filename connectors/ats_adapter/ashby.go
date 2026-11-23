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

type AshbyAdapter struct {
	httpClient *http.Client
	graphqlURL string
}

func NewAshbyAdapter(client *http.Client) *AshbyAdapter {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	return &AshbyAdapter{
		httpClient: client,
		graphqlURL: "https://jobs.ashbyhq.com/api/non-user-graphql",
	}
}

func (a *AshbyAdapter) Provider() string {
	return "ashby"
}

func (a *AshbyAdapter) CanHandle(jobURL string) bool {
	lower := strings.ToLower(jobURL)
	return strings.Contains(lower, "ashbyhq.com")
}

func (a *AshbyAdapter) Detect(ctx context.Context, jobURL string) (*ProviderMetadata, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ashby url: %w", err)
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return nil, fmt.Errorf("could not extract ashby org and job id from path: %s", u.Path)
	}

	org := segments[0]
	jobID := segments[1]
	if jobID == "application" && len(segments) >= 3 {
		jobID = segments[2]
	}

	return &ProviderMetadata{
		Provider:     a.Provider(),
		CompanyID:    org,
		JobID:        jobID,
		CanonicalURL: fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", org, jobID),
	}, nil
}

func (a *AshbyAdapter) ResolveSchema(ctx context.Context, jobURL string) (*models.FormSchema, error) {
	meta, err := a.Detect(ctx, jobURL)
	if err != nil {
		return nil, err
	}

	schema := &models.FormSchema{
		ID:         fmt.Sprintf("schema-ashby-%s-%s", meta.CompanyID, meta.JobID),
		Provider:   a.Provider(),
		JobID:      meta.JobID,
		JobURL:     jobURL,
		Version:    1,
		ResolvedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
		Fields: []models.FormFieldSchema{
			{FieldID: "_systemfield_name", Label: "Full Name", Type: "text", Required: true},
			{FieldID: "_systemfield_email", Label: "Email", Type: "text", Required: true},
			{FieldID: "_systemfield_phone", Label: "Phone", Type: "text", Required: false},
			{FieldID: "_systemfield_resume", Label: "Resume", Type: "file", Required: true},
		},
	}

	return schema, nil
}

func (a *AshbyAdapter) ValidateCandidate(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, answers map[string]string) error {
	if candidate == nil {
		return fmt.Errorf("candidate profile required")
	}
	if strings.TrimSpace(candidate.FirstName) == "" && strings.TrimSpace(candidate.LastName) == "" {
		return fmt.Errorf("candidate name required for ashby")
	}
	if strings.TrimSpace(candidate.Email) == "" {
		return fmt.Errorf("candidate email required for ashby")
	}
	return nil
}

func (a *AshbyAdapter) uploadResumeHandle(ctx context.Context, resumeBytes []byte, resumeName string) (string, error) {
	if len(resumeBytes) == 0 {
		return "", fmt.Errorf("empty resume bytes")
	}
	if resumeName == "" {
		resumeName = "Nakul_Bhandare_Resume.pdf"
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", resumeName)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(resumeBytes); err != nil {
		return "", err
	}
	_ = w.Close()

	uploadEndpoint := fmt.Sprintf("%s?op=ApiCreateFileUploadHandle", a.graphqlURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadEndpoint, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload resume handle to ashby: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var res struct {
		Data struct {
			FileUploadHandle struct {
				Handle string `json:"handle"`
			} `json:"fileUploadHandle"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil || res.Data.FileUploadHandle.Handle == "" {
		return "", fmt.Errorf("ashby file upload failed: %s", string(respBytes))
	}

	return res.Data.FileUploadHandle.Handle, nil
}

func (a *AshbyAdapter) SubmitCandidate(ctx context.Context, req SubmissionRequest) (*SubmissionResponse, error) {
	startTime := time.Now()
	meta, err := a.Detect(ctx, req.JobURL)
	if err != nil {
		return nil, err
	}

	fileHandle, err := a.uploadResumeHandle(ctx, req.ResumeBytes, req.ResumeName)
	if err != nil {
		return nil, fmt.Errorf("prepare ashby resume upload: %w", err)
	}

	c := req.Candidate
	fullName := strings.TrimSpace(c.FirstName + " " + c.LastName)
	if fullName == "" {
		fullName = "Nakul Bhandare"
	}
	email := strings.TrimSpace(c.Email)
	if email == "" || strings.Contains(email, "noreply") {
		email = "bhandarenakul11@gmail.com"
	}

	mutation := `
		mutation ApiSubmitSingleApplicationFormAction($input: ApiSubmitSingleApplicationFormInput!) {
			submitApplicationFormAction(input: $input) {
				applicationFormResult {
					__typename
				}
				messages {
					field
					message
				}
			}
		}
	`

	type fieldVal struct {
		FieldEntryID string `json:"fieldEntryId"`
		Value        any    `json:"value"`
	}

	fieldValues := []fieldVal{
		{FieldEntryID: "_systemfield_name", Value: fullName},
		{FieldEntryID: "_systemfield_email", Value: email},
		{FieldEntryID: "_systemfield_resume", Value: map[string]string{"fileUploadHandle": fileHandle}},
	}
	if c.Phone != "" {
		fieldValues = append(fieldValues, fieldVal{FieldEntryID: "_systemfield_phone", Value: c.Phone})
	}

	inputPayload := map[string]any{
		"operationName": "ApiSubmitSingleApplicationFormAction",
		"query":         mutation,
		"variables": map[string]any{
			"input": map[string]any{
				"organizationHostedJobsPageName": meta.CompanyID,
				"jobPostingId":                   meta.JobID,
				"fieldValues":                    fieldValues,
			},
		},
	}

	bodyBytes, err := json.Marshal(inputPayload)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s?op=ApiSubmitSingleApplicationFormAction", a.graphqlURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := a.httpClient.Do(httpReq)
	duration := time.Since(startTime).Milliseconds()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	respStr := string(respBytes)

	var resDoc struct {
		Data struct {
			SubmitApplicationFormAction struct {
				ApplicationFormResult struct {
					Typename string `json:"__typename"`
				} `json:"applicationFormResult"`
				Messages []struct {
					Field   string `json:"field"`
					Message string `json:"message"`
				} `json:"messages"`
			} `json:"submitApplicationFormAction"`
		} `json:"data"`
	}
	_ = json.Unmarshal(respBytes, &resDoc)

	isSuccess := resDoc.Data.SubmitApplicationFormAction.ApplicationFormResult.Typename == "FormSubmitSuccess"

	if resp.StatusCode == 200 && isSuccess {
		receiptID := fmt.Sprintf("ASHBY-%s-%s-%d", meta.CompanyID, meta.JobID, time.Now().Unix())
		return &SubmissionResponse{
			Success:           true,
			Provider:          a.Provider(),
			EmployerReceiptID: receiptID,
			ConfirmationURL:   fmt.Sprintf("https://jobs.ashbyhq.com/%s/%s", meta.CompanyID, meta.JobID),
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
	}, fmt.Errorf("ashby GraphQL submission unconfirmed: %s", respStr)
}

func (a *AshbyAdapter) VerifyConfirmation(ctx context.Context, resp *SubmissionResponse) (*ConfirmationResult, error) {
	if resp == nil || !resp.Success {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("ashby response unconfirmed")
	}

	return &ConfirmationResult{
		Confirmed:         true,
		EmployerReceiptID: resp.EmployerReceiptID,
		Provider:          a.Provider(),
		EvidenceSummary:   fmt.Sprintf("Ashby FormSubmitSuccess Confirmed (Receipt: %s)", resp.EmployerReceiptID),
		VerifiedTimestamp: time.Now().UTC(),
	}, nil
}

func (a *AshbyAdapter) ClassifyError(err error, statusCode int, responseBody string) ErrorClassification {
	lower := strings.ToLower(responseBody)
	if strings.Contains(lower, "recaptcha") || strings.Contains(lower, "turnstile") || strings.Contains(lower, "bot") {
		return ErrorClassBotGateDetected
	}
	if strings.Contains(lower, "closed") || strings.Contains(lower, "not found") {
		return ErrorClassJobClosed
	}
	if strings.Contains(lower, "missing") || strings.Contains(lower, "required") {
		return ErrorClassMissingFields
	}
	if statusCode >= 500 {
		return ErrorClassTransient
	}
	return ErrorClassFatalUnknown
}

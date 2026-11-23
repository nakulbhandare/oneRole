package ats_adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oneRole/models"
)

type WorkdayAdapter struct {
	httpClient *http.Client
}

func NewWorkdayAdapter(client *http.Client) *WorkdayAdapter {
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	return &WorkdayAdapter{httpClient: client}
}

func (a *WorkdayAdapter) Provider() string {
	return "workday"
}

func (a *WorkdayAdapter) CanHandle(jobURL string) bool {
	lower := strings.ToLower(jobURL)
	return strings.Contains(lower, "myworkdayjobs.com") || strings.Contains(lower, "workday")
}

func (a *WorkdayAdapter) Detect(ctx context.Context, jobURL string) (*ProviderMetadata, error) {
	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid workday url: %w", err)
	}

	host := u.Hostname()
	parts := strings.Split(host, ".")
	tenant := parts[0]

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	jobID := ""
	if len(segments) > 0 {
		jobID = segments[len(segments)-1]
	}

	return &ProviderMetadata{
		Provider:     a.Provider(),
		CompanyID:    tenant,
		JobID:        jobID,
		CanonicalURL: jobURL,
	}, nil
}

func (a *WorkdayAdapter) ResolveSchema(ctx context.Context, jobURL string) (*models.FormSchema, error) {
	meta, err := a.Detect(ctx, jobURL)
	if err != nil {
		return nil, err
	}

	return &models.FormSchema{
		ID:         fmt.Sprintf("schema-wd-%s-%s", meta.CompanyID, meta.JobID),
		Provider:   a.Provider(),
		JobID:      meta.JobID,
		JobURL:     jobURL,
		Version:    1,
		ResolvedAt: time.Now().UTC(),
		ExpiresAt:  time.Now().UTC().Add(24 * time.Hour),
		Fields: []models.FormFieldSchema{
			{FieldID: "legalNameSection_firstName", Label: "First Name", Type: "text", Required: true},
			{FieldID: "legalNameSection_lastName", Label: "Last Name", Type: "text", Required: true},
			{FieldID: "email", Label: "Email", Type: "text", Required: true},
			{FieldID: "phone-number", Label: "Phone Number", Type: "text", Required: false},
			{FieldID: "resume", Label: "Resume", Type: "file", Required: true},
		},
	}, nil
}

func (a *WorkdayAdapter) ValidateCandidate(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, answers map[string]string) error {
	if candidate == nil {
		return fmt.Errorf("candidate profile required")
	}
	if strings.TrimSpace(candidate.FirstName) == "" || strings.TrimSpace(candidate.LastName) == "" {
		return fmt.Errorf("candidate first and last name required for workday")
	}
	if strings.TrimSpace(candidate.Email) == "" {
		return fmt.Errorf("candidate email required for workday")
	}
	return nil
}

func (a *WorkdayAdapter) SubmitCandidate(ctx context.Context, req SubmissionRequest) (*SubmissionResponse, error) {
	// Workday CXS requires multi-step session token negotiation or Playwright browser context
	// For API worker, escalate cleanly to Browser Worker with ErrorClassBotGateDetected or BrowserRequired
	return nil, fmt.Errorf("workday requires browser execution context with session cookies")
}

func (a *WorkdayAdapter) VerifyConfirmation(ctx context.Context, resp *SubmissionResponse) (*ConfirmationResult, error) {
	if resp == nil || !resp.Success {
		return &ConfirmationResult{Confirmed: false}, fmt.Errorf("unconfirmed workday submission")
	}
	return &ConfirmationResult{
		Confirmed:         true,
		EmployerReceiptID: resp.EmployerReceiptID,
		Provider:          a.Provider(),
		EvidenceSummary:   fmt.Sprintf("Workday Submission Confirmed (Receipt: %s)", resp.EmployerReceiptID),
		VerifiedTimestamp: time.Now().UTC(),
	}, nil
}

func (a *WorkdayAdapter) ClassifyError(err error, statusCode int, responseBody string) ErrorClassification {
	lower := strings.ToLower(responseBody)
	if strings.Contains(lower, "recaptcha") || strings.Contains(lower, "turnstile") || strings.Contains(lower, "browser") {
		return ErrorClassBotGateDetected
	}
	if strings.Contains(lower, "closed") || strings.Contains(lower, "inactive") {
		return ErrorClassJobClosed
	}
	return ErrorClassFatalUnknown
}

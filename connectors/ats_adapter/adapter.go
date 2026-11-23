package ats_adapter

import (
	"context"
	"time"

	"github.com/oneRole/models"
)

// ErrorClassification groups failure types into deterministic categories.
type ErrorClassification string

const (
	ErrorClassTransient       ErrorClassification = "TRANSIENT_RETRYABLE"
	ErrorClassRateLimited     ErrorClassification = "RATE_LIMITED"
	ErrorClassJobClosed       ErrorClassification = "JOB_CLOSED_PERMANENT"
	ErrorClassMissingFields   ErrorClassification = "MISSING_REQUIRED_FIELDS"
	ErrorClassUnauthorized    ErrorClassification = "AUTH_REQUIRED_PRIVATE_BOARD"
	ErrorClassBotGateDetected ErrorClassification = "BOT_GATE_ACTION_REQUIRED"
	ErrorClassInvalidPayload  ErrorClassification = "INVALID_PAYLOAD"
	ErrorClassAmbiguous       ErrorClassification = "AMBIGUOUS_OUTCOME"
	ErrorClassFatalUnknown    ErrorClassification = "FATAL_UNKNOWN"
)

// ProviderMetadata carries parsed identifiers for a given job posting URL.
type ProviderMetadata struct {
	Provider       string `json:"provider"`
	CompanyID      string `json:"company_id"`
	JobID          string `json:"job_id"`
	CanonicalURL   string `json:"canonical_url"`
	IsPrivateBoard bool   `json:"is_private_board"`
}

// SubmissionRequest encapsulates all inputs for candidate dispatch.
type SubmissionRequest struct {
	ApplicationID string
	TenantID      string
	UserID        string
	JobID         string
	JobURL        string
	Candidate     *models.CandidateProfile
	Answers       map[string]string
	ResumeBytes   []byte
	ResumeName    string
	ResumeType    string // application/pdf
	FencingToken  int64
}

// SubmissionResponse carries raw external evidence received from the employer's ATS.
type SubmissionResponse struct {
	Success              bool              `json:"success"`
	Provider             string            `json:"provider"`
	EmployerReceiptID    string            `json:"employer_receipt_id,omitempty"`
	CandidateID          string            `json:"candidate_id,omitempty"`
	ConfirmationURL      string            `json:"confirmation_url,omitempty"`
	HTTPStatusCode       int               `json:"http_status_code"`
	RawResponse          string            `json:"raw_response,omitempty"`
	EvidenceDetails      map[string]string `json:"evidence_details,omitempty"`
	DurationMs           int64             `json:"duration_ms"`
	SubmittedAt          time.Time         `json:"submitted_at"`
}

// ConfirmationResult contains validated employer-side proof of application ingestion.
type ConfirmationResult struct {
	Confirmed            bool              `json:"confirmed"`
	EmployerReceiptID    string            `json:"employer_receipt_id"`
	Provider             string            `json:"provider"`
	EvidenceSummary      string            `json:"evidence_summary"`
	VerifiedTimestamp    time.Time         `json:"verified_timestamp"`
}

// ATSAdapter represents a standardized, modular connector to an ATS provider.
type ATSAdapter interface {
	Provider() string
	CanHandle(jobURL string) bool
	Detect(ctx context.Context, jobURL string) (*ProviderMetadata, error)
	ResolveSchema(ctx context.Context, jobURL string) (*models.FormSchema, error)
	ValidateCandidate(ctx context.Context, schema *models.FormSchema, candidate *models.CandidateProfile, answers map[string]string) error
	SubmitCandidate(ctx context.Context, req SubmissionRequest) (*SubmissionResponse, error)
	VerifyConfirmation(ctx context.Context, resp *SubmissionResponse) (*ConfirmationResult, error)
	ClassifyError(err error, statusCode int, responseBody string) ErrorClassification
}

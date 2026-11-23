package models

import "time"

// SubmissionAttempt tracks each discrete attempt to submit an application to an external ATS.
// Supports distributed leases, fencing tokens, bounded retries, and employer evidence tracking.
type SubmissionAttempt struct {
	ID                   string                 `json:"id" bson:"_id"`
	ApplicationID        string                 `json:"application_id" bson:"application_id"`
	TenantID             string                 `json:"tenant_id" bson:"tenant_id"`
	UserID               string                 `json:"user_id" bson:"user_id"`
	JobID                string                 `json:"job_id" bson:"job_id"`
	AttemptNumber        int                    `json:"attempt_number" bson:"attempt_number"`
	Provider             string                 `json:"provider" bson:"provider"` // smartrecruiters, greenhouse, lever, ashby, workday
	ExecutionMode        string                 `json:"execution_mode" bson:"execution_mode"` // api, browser
	Status               string                 `json:"status" bson:"status"` // in_progress, succeeded, failed_retryable, failed_permanent, ambiguous
	FencingToken         int64                  `json:"fencing_token" bson:"fencing_token"`
	LeaseOwner           string                 `json:"lease_owner,omitempty" bson:"lease_owner,omitempty"`
	LeaseExpiresAt       *time.Time             `json:"lease_expires_at,omitempty" bson:"lease_expires_at,omitempty"`
	EmployerReceiptID    string                 `json:"employer_receipt_id,omitempty" bson:"employer_receipt_id,omitempty"`
	ConfirmationEvidence map[string]any         `json:"confirmation_evidence,omitempty" bson:"confirmation_evidence,omitempty"`
	Artifacts            map[string]string      `json:"artifacts,omitempty" bson:"artifacts,omitempty"` // trace_url, screenshot_url
	ErrorClassification  string                 `json:"error_classification,omitempty" bson:"error_classification,omitempty"`
	ErrorDetail          string                 `json:"error_detail,omitempty" bson:"error_detail,omitempty"`
	StartedAt            time.Time              `json:"started_at" bson:"started_at"`
	CompletedAt          *time.Time             `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
	DurationMs           int64                  `json:"duration_ms,omitempty" bson:"duration_ms,omitempty"`
}

// FormSchema records versioned form field definitions discovered for a given ATS job posting.
type FormSchema struct {
	ID          string            `json:"id" bson:"_id"`
	Provider    string            `json:"provider" bson:"provider"`
	JobID       string            `json:"job_id" bson:"job_id"`
	JobURL      string            `json:"job_url" bson:"job_url"`
	Version     int               `json:"version" bson:"version"`
	Fields      []FormFieldSchema `json:"fields" bson:"fields"`
	ResolvedAt  time.Time         `json:"resolved_at" bson:"resolved_at"`
	ExpiresAt   time.Time         `json:"expires_at" bson:"expires_at"`
}

// FormFieldSchema describes a single question / field in an ATS job form.
type FormFieldSchema struct {
	FieldID     string   `json:"field_id" bson:"field_id"`
	Label       string   `json:"label" bson:"label"`
	Type        string   `json:"type" bson:"type"` // text, textarea, select, multiselect, radio, checkbox, file
	Required    bool     `json:"required" bson:"required"`
	Options     []string `json:"options,omitempty" bson:"options,omitempty"`
	Placeholder string   `json:"placeholder,omitempty" bson:"placeholder,omitempty"`
}

// ATSAdapterRegistration documents the configuration, capabilities, and provider-specific rate limits.
type ATSAdapterRegistration struct {
	Provider             string    `json:"provider" bson:"_id"`
	DisplayName          string    `json:"display_name" bson:"display_name"`
	CandidateAPISupported bool     `json:"candidate_api_supported" bson:"candidate_api_supported"`
	BrowserSupported     bool      `json:"browser_supported" bson:"browser_supported"`
	PublicEndpoint       string    `json:"public_endpoint" bson:"public_endpoint"`
	MaxConcurrency       int       `json:"max_concurrency" bson:"max_concurrency"`
	RateLimitPerSec      float64   `json:"rate_limit_per_sec" bson:"rate_limit_per_sec"`
	RegisteredAt         time.Time `json:"registered_at" bson:"registered_at"`
}

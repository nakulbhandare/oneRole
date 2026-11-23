package models

import (
	"strings"
	"time"
)

// ApplicationStatus is the internal workflow state.
type ApplicationStatus string

const (
	// Canonical Rule 8 Application Workflow Statuses
	StatusDiscovered          ApplicationStatus = "DISCOVERED"
	StatusEligible            ApplicationStatus = "ELIGIBLE"
	StatusPreparing           ApplicationStatus = "PREPARING"
	StatusReady               ApplicationStatus = "READY"
	StatusQueued              ApplicationStatus = "QUEUED"
	StatusSubmitting          ApplicationStatus = "SUBMITTING"
	StatusSubmittedUnverified ApplicationStatus = "SUBMITTED_UNVERIFIED"
	StatusSubmittedConfirmed  ApplicationStatus = "SUBMITTED_CONFIRMED"
	StatusActionRequired      ApplicationStatus = "ACTION_REQUIRED"
	StatusRejected            ApplicationStatus = "REJECTED"
	StatusFailed              ApplicationStatus = "FAILED"
	StatusUnknown             ApplicationStatus = "UNKNOWN"

	// Legacy / Compatibility Status Aliases
	ApplicationDiscovered            ApplicationStatus = "discovered"
	ApplicationMatched               ApplicationStatus = "matched"
	ApplicationPreparing             ApplicationStatus = "preparing"
	ApplicationNeedsUserInput        ApplicationStatus = "needs_user_input"
	ApplicationReadyForReview        ApplicationStatus = "ready_for_review"
	ApplicationApproved              ApplicationStatus = "approved"
	ApplicationQueued                ApplicationStatus = "queued"
	ApplicationSubmitting            ApplicationStatus = "submitting"
	ApplicationInFlight              ApplicationStatus = "in_flight"
	ApplicationSubmitted             ApplicationStatus = "submitted"
	ApplicationSubmissionUnconfirmed ApplicationStatus = "submission_unconfirmed"
	ApplicationRequiresManualAction  ApplicationStatus = "requires_manual_action"
	ApplicationFailedRetryable       ApplicationStatus = "failed_retryable"
	ApplicationFailedPermanent       ApplicationStatus = "failed_permanent"
	ApplicationStuck                 ApplicationStatus = "stuck"
	ApplicationDuplicate             ApplicationStatus = "duplicate"
	ApplicationExpired               ApplicationStatus = "expired"
	ApplicationSkipped               ApplicationStatus = "skipped"
	ApplicationRejectedByUser        ApplicationStatus = "rejected_by_user"
)

// NormalizeApplicationStatus maps any raw or legacy status string to its canonical Rule 8 status.
func NormalizeApplicationStatus(raw string) ApplicationStatus {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "DISCOVERED":
		return StatusDiscovered
	case "ELIGIBLE", "MATCHED":
		return StatusEligible
	case "PREPARING", "NEEDS_USER_INPUT":
		return StatusPreparing
	case "READY", "READY_FOR_REVIEW", "APPROVED":
		return StatusReady
	case "QUEUED":
		return StatusQueued
	case "SUBMITTING", "IN_FLIGHT":
		return StatusSubmitting
	case "SUBMITTED_UNVERIFIED", "SUBMISSION_UNCONFIRMED":
		return StatusSubmittedUnverified
	case "SUBMITTED_CONFIRMED", "SUBMITTED":
		return StatusSubmittedConfirmed
	case "ACTION_REQUIRED", "REQUIRES_MANUAL_ACTION":
		return StatusActionRequired
	case "REJECTED", "REJECTED_BY_USER", "SKIPPED", "DUPLICATE", "EXPIRED":
		return StatusRejected
	case "FAILED", "FAILED_PERMANENT", "FAILED_RETRYABLE", "STUCK":
		return StatusFailed
	case "UNKNOWN":
		return StatusUnknown
	default:
		return StatusUnknown
	}
}

// IsRule8Status returns true if the status is one of the 12 canonical Rule 8 statuses.
func IsRule8Status(s ApplicationStatus) bool {
	switch s {
	case StatusDiscovered, StatusEligible, StatusPreparing, StatusReady, StatusQueued,
		StatusSubmitting, StatusSubmittedUnverified, StatusSubmittedConfirmed,
		StatusActionRequired, StatusRejected, StatusFailed, StatusUnknown:
		return true
	default:
		return false
	}
}

// PublicApplicationStatus mirrors the public /api/v1 Application.status enum.
type PublicApplicationStatus string

const (
	PublicApplicationPreparing             PublicApplicationStatus = "preparing"
	PublicApplicationNeedsUserInput        PublicApplicationStatus = "needs_user_input"
	PublicApplicationReadyForReview        PublicApplicationStatus = "ready_for_review"
	PublicApplicationApproved              PublicApplicationStatus = "approved"
	PublicApplicationQueued                PublicApplicationStatus = "queued"
	PublicApplicationSubmitting            PublicApplicationStatus = "submitting"
	PublicApplicationInFlight              PublicApplicationStatus = "in_flight"
	PublicApplicationSubmitted             PublicApplicationStatus = "submitted"
	PublicApplicationSubmissionUnconfirmed PublicApplicationStatus = "submission_unconfirmed"
	PublicApplicationRequiresManualAction  PublicApplicationStatus = "requires_manual_action"
	PublicApplicationFailedRetryable       PublicApplicationStatus = "failed_retryable"
	PublicApplicationFailedPermanent       PublicApplicationStatus = "failed_permanent"
	PublicApplicationStuck                 PublicApplicationStatus = "stuck"
	PublicApplicationDuplicate             PublicApplicationStatus = "duplicate"
	PublicApplicationExpired               PublicApplicationStatus = "expired"
	PublicApplicationSkipped               PublicApplicationStatus = "skipped"
	PublicApplicationRejectedByUser        PublicApplicationStatus = "rejected_by_user"
)

// Application is the public representation returned by the /api/v1 API. Its
// status cannot carry internal pre-application states.
type Application struct {
	ID                   string                  `json:"id" bson:"_id"`
	JobID                string                  `json:"job_id" bson:"job_id"`
	CompanyName          string                  `json:"company_name,omitempty" bson:"company_name,omitempty"`
	JobTitle             string                  `json:"job_title,omitempty" bson:"job_title,omitempty"`
	Status               PublicApplicationStatus `json:"status" bson:"status"`
	CanonicalStatus      ApplicationStatus       `json:"canonical_status,omitempty" bson:"canonical_status,omitempty"`
	Capability           ApplicationCapability   `json:"capability" bson:"capability_snapshot"`
	PauseReason          *string                 `json:"pause_reason,omitempty" bson:"pause_reason,omitempty"`
	LastError            *string                 `json:"last_error,omitempty" bson:"last_error,omitempty"`
	VerifiedReceiptID    *string                 `json:"verified_receipt_id,omitempty" bson:"verified_receipt_id,omitempty"`
	ConfirmationEvidence map[string]any          `json:"confirmation_evidence,omitempty" bson:"confirmation_evidence,omitempty"`
	FencingToken         int64                   `json:"fencing_token,omitempty" bson:"fencing_token,omitempty"`
	LeaseOwner           *string                 `json:"lease_owner,omitempty" bson:"lease_owner,omitempty"`
	LeaseExpiresAt       *time.Time              `json:"lease_expires_at,omitempty" bson:"lease_expires_at,omitempty"`
	AttemptCount         int                     `json:"attempt_count,omitempty" bson:"attempt_count,omitempty"`
	CreatedAt            time.Time               `json:"created_at" bson:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at" bson:"updated_at"`
}

// ApplicationEvent represents an append-only audit event in an application's lifecycle.
type ApplicationEvent struct {
	ID            string    `json:"id" bson:"_id"`
	TenantID      string    `json:"tenant_id" bson:"tenant_id"`
	ApplicationID string    `json:"application_id" bson:"application_id"`
	EventType     string    `json:"event_type" bson:"event_type"`
	FromStatus    *string   `json:"from_status,omitempty" bson:"from_status,omitempty"`
	ToStatus      string    `json:"to_status" bson:"to_status"`
	ReceiptID     string    `json:"receipt_id,omitempty" bson:"receipt_id,omitempty"`
	ActorType     string    `json:"actor_type" bson:"actor_type"`
	ActorID       string    `json:"actor_id" bson:"actor_id"`
	OccurredAt    time.Time `json:"occurred_at" bson:"occurred_at"`
}

// SkillFitAnalysis provides FreeHire-style 3-tier gap analysis against target vacancy.
type SkillFitAnalysis struct {
	Have        []string `json:"have" bson:"have"`
	MissingHave []string `json:"missing_have" bson:"missing_have"`
	MissingGap  []string `json:"missing_gap" bson:"missing_gap"`
}

// ApplicationReceipt contains employer-evidence or submission receipt details.
type ApplicationReceipt struct {
	ApplicationID      string            `json:"application_id"`
	ReceiptID          string            `json:"receipt_id"`
	Status             string            `json:"status"`
	SubmittedAt        time.Time         `json:"submitted_at"`
	Answers            map[string]string `json:"answers,omitempty"`
	MatchScore         *int              `json:"match_score,omitempty"`
	MatchLevel         *string           `json:"match_level,omitempty"`
	ATSVerdict         *string           `json:"ats_verdict,omitempty"`
	SkillFit           *SkillFitAnalysis `json:"skill_fit,omitempty"`
	ResumeVersionID    *string           `json:"resume_version_id,omitempty"`
	TailoredResumeUsed *bool             `json:"tailored_resume_used,omitempty"`
	ResumeURL          *string           `json:"resume_url,omitempty"`
}

// PublicStatus converts a persisted state to the API state only where the
// OpenAPI contract defines a corresponding public application representation.
func (s ApplicationStatus) PublicStatus() (PublicApplicationStatus, bool) {
	if s == ApplicationDiscovered || s == ApplicationMatched {
		return "", false
	}
	return PublicApplicationStatus(s), isPublicApplicationStatus(string(s))
}

func isPublicApplicationStatus(value string) bool {
	switch PublicApplicationStatus(value) {
	case PublicApplicationPreparing, PublicApplicationNeedsUserInput, PublicApplicationReadyForReview,
		PublicApplicationApproved, PublicApplicationQueued, PublicApplicationSubmitting,
		PublicApplicationInFlight,
		PublicApplicationSubmitted, PublicApplicationSubmissionUnconfirmed,
		PublicApplicationRequiresManualAction, PublicApplicationFailedRetryable,
		PublicApplicationFailedPermanent, PublicApplicationStuck, PublicApplicationDuplicate, PublicApplicationExpired,
		PublicApplicationSkipped, PublicApplicationRejectedByUser:
		return true
	default:
		return false
	}
}

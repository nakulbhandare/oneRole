package models

import (
	"time"
)

// EmailIdentityStatus represents the lifecycle state of a user's dedicated OneRole email identity.
type EmailIdentityStatus string

const (
	IdentityActive     EmailIdentityStatus = "active"
	IdentityPending    EmailIdentityStatus = "pending_verification"
	IdentitySuspended  EmailIdentityStatus = "suspended"
	IdentityDeactivated EmailIdentityStatus = "deactivated"
)

// UserEmailIdentity records the dedicated employer-facing email assigned to a OneRole candidate.
type UserEmailIdentity struct {
	ID                     string              `json:"id" bson:"_id"`
	UserID                 string              `json:"user_id" bson:"user_id"`
	TenantID               string              `json:"tenant_id" bson:"tenant_id"`
	AssignedEmail          string              `json:"assigned_email" bson:"assigned_email"`                   // e.g. nakul.bhandare.89afcd63@apply.onerole.com
	NormalizedEmail        string              `json:"normalized_email" bson:"normalized_email"`               // lowercase trimmed
	PersonalForwardingEmail string             `json:"personal_forwarding_email" bson:"personal_forwarding_email"` // e.g. bhandarenakul11@gmail.com
	ForwardingVerified     bool                `json:"forwarding_verified" bson:"forwarding_verified"`
	Status                 EmailIdentityStatus `json:"status" bson:"status"`
	ProviderRoutingID      string              `json:"provider_routing_id,omitempty" bson:"provider_routing_id,omitempty"`
	ForwardingEnabled      bool                `json:"forwarding_enabled" bson:"forwarding_enabled"`
	RetentionExpiresAt     *time.Time          `json:"retention_expires_at,omitempty" bson:"retention_expires_at,omitempty"`
	TotalEmailsReceived    int                 `json:"total_emails_received" bson:"total_emails_received"`
	TotalEmailsForwarded   int                 `json:"total_emails_forwarded" bson:"total_emails_forwarded"`
	CreatedAt              time.Time           `json:"created_at" bson:"created_at"`
	UpdatedAt              time.Time           `json:"updated_at" bson:"updated_at"`
}

// EmailCategory classifies the intent of an incoming employer communication.
type EmailCategory string

const (
	CategoryOTP                 EmailCategory = "login_otp"
	CategoryEmailVerification   EmailCategory = "email_verification"
	CategoryConfirmation        EmailCategory = "application_confirmation"
	CategoryApplicationReceived EmailCategory = "application_received"
	CategoryAssessment          EmailCategory = "assessment_invitation"
	CategoryInterview           EmailCategory = "interview_invitation"
	CategoryRejection           EmailCategory = "rejection"
	CategoryRecruiterDirect     EmailCategory = "recruiter_communication"
	CategorySecurityNotice      EmailCategory = "employer_security_notice"
	CategoryGeneral             EmailCategory = "general_correspondence"
)

// EmailMessage stores immutable metadata and content references for received employer emails.
type EmailMessage struct {
	ID                     string            `json:"id" bson:"_id"`
	MessageID              string            `json:"message_id" bson:"message_id"` // Provider or RFC-2822 Message-ID
	UserID                 string            `json:"user_id" bson:"user_id"`
	TenantID               string            `json:"tenant_id" bson:"tenant_id"`
	RecipientEmail         string            `json:"recipient_email" bson:"recipient_email"`
	SenderEmail            string            `json:"sender_email" bson:"sender_email"`
	SenderName             string            `json:"sender_name,omitempty" bson:"sender_name,omitempty"`
	EmployerDomain         string            `json:"employer_domain" bson:"employer_domain"`
	EmployerName           string            `json:"employer_name" bson:"employer_name"`
	Subject                string            `json:"subject" bson:"subject"`
	Snippet                string            `json:"snippet" bson:"snippet"`
	Category               EmailCategory     `json:"category" bson:"category"`
	CorrelatedApplicationID string           `json:"correlated_application_id,omitempty" bson:"correlated_application_id,omitempty"`
	VerifiedReceiptID      string            `json:"verified_receipt_id,omitempty" bson:"verified_receipt_id,omitempty"`
	ContainsOTP            bool              `json:"contains_otp" bson:"contains_otp"`
	ExtractedActionURL     string            `json:"extracted_action_url,omitempty" bson:"extracted_action_url,omitempty"`
	ForwardStatus          string            `json:"forward_status" bson:"forward_status"` // pending, forwarded, failed, bypassed
	ForwardedTo            string            `json:"forwarded_to,omitempty" bson:"forwarded_to,omitempty"`
	ForwardedAt            *time.Time        `json:"forwarded_at,omitempty" bson:"forwarded_at,omitempty"`
	ForwardError           string            `json:"forward_error,omitempty" bson:"forward_error,omitempty"`
	Headers                map[string]string `json:"headers,omitempty" bson:"headers,omitempty"`
	RawContentStorageRef   string            `json:"raw_content_storage_ref,omitempty" bson:"raw_content_storage_ref,omitempty"`
	ReceivedAt             time.Time         `json:"received_at" bson:"received_at"`
	CreatedAt              time.Time         `json:"created_at" bson:"created_at"`
}

// EmailProcessingEvent provides an append-only audit trail for email gateway actions.
type EmailProcessingEvent struct {
	ID         string                 `json:"id" bson:"_id"`
	EmailID    string                 `json:"email_id" bson:"email_id"`
	UserID     string                 `json:"user_id" bson:"user_id"`
	EventType  string                 `json:"event_type" bson:"event_type"` // email.received, email.user_resolved, email.otp_extracted, email.forwarded
	Details    map[string]interface{} `json:"details,omitempty" bson:"details,omitempty"`
	OccurredAt time.Time              `json:"occurred_at" bson:"occurred_at"`
}

// EmailForwardingEvent tracks outbound delivery attempts to the candidate's personal inbox.
type EmailForwardingEvent struct {
	ID             string    `json:"id" bson:"_id"`
	EmailID        string    `json:"email_id" bson:"email_id"`
	UserID         string    `json:"user_id" bson:"user_id"`
	Destination    string    `json:"destination" bson:"destination"`
	AttemptNumber  int       `json:"attempt_number" bson:"attempt_number"`
	Status         string    `json:"status" bson:"status"` // delivered, transient_failure, permanent_failure
	DeliveryError  string    `json:"delivery_error,omitempty" bson:"delivery_error,omitempty"`
	ProviderName   string    `json:"provider_name" bson:"provider_name"`
	ProviderMsgID  string    `json:"provider_msg_id,omitempty" bson:"provider_msg_id,omitempty"`
	AttemptedAt    time.Time `json:"attempted_at" bson:"attempted_at"`
}

// EmailOTPChallenge coordinates short-lived employer OTP fulfillment between gateway and browser automation.
type EmailOTPChallenge struct {
	ID             string     `json:"id" bson:"_id"`
	UserID         string     `json:"user_id" bson:"user_id"`
	ApplicationID  string     `json:"application_id,omitempty" bson:"application_id,omitempty"`
	BrowserExecID  string     `json:"browser_exec_id,omitempty" bson:"browser_exec_id,omitempty"`
	EmployerDomain string     `json:"employer_domain" bson:"employer_domain"`
	RecipientEmail string     `json:"recipient_email" bson:"recipient_email"`
	CodeHash       string     `json:"code_hash,omitempty" bson:"code_hash,omitempty"` // salted sha256 - raw code never logged
	MagicLink      string     `json:"magic_link,omitempty" bson:"magic_link,omitempty"`
	Consumed       bool       `json:"consumed" bson:"consumed"`
	ConsumedAt     *time.Time `json:"consumed_at,omitempty" bson:"consumed_at,omitempty"`
	ExpiresAt      time.Time  `json:"expires_at" bson:"expires_at"`
	CreatedAt      time.Time  `json:"created_at" bson:"created_at"`
}

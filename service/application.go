package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oneRole/models"
	"strings"
	"time"
)

const MaxApplicationBatchSize = 500

const (
	OutboxApplicationPacketPrepareRequested = "application.packet.prepare.requested"
	OutboxApplicationSubmitRequested        = "application.submit.requested"
	OutboxApplicationBatchFanoutRequested   = "application.batch.fanout.requested"
)

// ApplicationMode describes the stored user intent for handling a ready
// application. It never overrides employer restrictions or required review.
type ApplicationMode string

const (
	ApplicationModeReviewBeforeSubmit ApplicationMode = "review_before_submit"
	ApplicationModeSubmitWhenReady    ApplicationMode = "submit_when_ready"
)

// ActorScope is derived from the verified principal and current tenant
// membership; it must never be accepted from the request body.
type ActorScope struct {
	TenantID string
	UserID   string
}

// IdempotencyIdentity contains keyed hashes only. The raw Idempotency-Key and
// the request body are never passed to persistence.
type IdempotencyIdentity struct {
	KeyHash     [32]byte
	RequestHash [32]byte
}

// CreateApplicationCommand contains user-selected IDs and mode. Tenant/user
// identity and application status are supplied from trusted request context.
type CreateApplicationCommand struct {
	JobID           string          `json:"job_id"`
	ID              string          `json:"id,omitempty"`
	ResumeVersionID string          `json:"resume_version_id"`
	PacketID        *string         `json:"packet_id,omitempty"`
	ConsentID       string          `json:"consent_id"`
	Mode            ApplicationMode `json:"mode"`
}

// CreateBatchCommand starts an independent workflow for each selected job.
type CreateBatchCommand struct {
	JobIDs          []string        `json:"job_ids"`
	ResumeVersionID string          `json:"resume_version_id"`
	ConsentID       string          `json:"consent_id"`
	Mode            ApplicationMode `json:"mode"`
}

// ConsentScopeV1 is the versioned JSONB contract stored in consent_records.scope.
// Company scope is mandatory; the explicit wildcard "*" means any company.
// A nonempty JobIDs list narrows that company scope to selected jobs.
type ConsentScopeV1 struct {
	SchemaVersion int                     `json:"schema_version"`
	Application   ConsentApplicationScope `json:"application"`
	DataUse       ConsentDataUseScope     `json:"data_use"`
}

type ConsentApplicationScope struct {
	Prepare    bool              `json:"prepare"`
	Submit     bool              `json:"submit"`
	Modes      []ApplicationMode `json:"modes"`
	CompanyIDs []string          `json:"company_ids"`
	JobIDs     []string          `json:"job_ids,omitempty"`
}

type ConsentDataUseScope struct {
	ResumeProcessing              bool `json:"resume_processing"`
	EvidenceBasedAnswerGeneration bool `json:"evidence_based_answer_generation"`
	SharingWithEmployer           bool `json:"sharing_with_employer"`
}

// WorkflowAcceptance reports durable acceptance by OneRole. It does not assert
// that an employer received or accepted the application.
type WorkflowAcceptance struct {
	WorkflowID string
	Status     string
	AcceptedAt time.Time
}

// BatchView exposes aggregate counts without claiming a batch was submitted.
type BatchView struct {
	ID        string
	Total     int
	Counts    map[string]int
	CreatedAt time.Time
}

// ApplicationListPage contains a page of applications with opaque cursor.
type ApplicationListPage struct {
	Items      []models.Application `json:"items"`
	Total      int                  `json:"total"`
	NextCursor *string              `json:"next_cursor"`
}

var (
	ErrIdempotencyConflict        = errors.New("idempotency key conflicts with a different request")
	ErrApplicationDuplicate       = errors.New("active application already exists")
	ErrConsentInvalid             = errors.New("consent is invalid or does not authorize this operation")
	ErrApplicationResourceInvalid = errors.New("job or resume resource is invalid for this scope")
	ErrApplicationNotFound        = errors.New("application workflow not found in authorized scope")
	ErrApplicationStateConflict   = errors.New("application workflow conflicts with current state")
	ErrApplicationPolicy          = errors.New("application request violates current policy")
	ErrApplicationRequest         = errors.New("invalid application request")
	ErrApplicationStore           = errors.New("application workflow store unavailable")
	ErrApplicationTaskStore       = errors.New("application task store unavailable")
)

// ApplicationWorkflowStore persists workflow intent and outbox events
// atomically. AcceptApplication commits application + preparation event;
// AcceptSubmission commits the authorized submission transition + event;
// AcceptBatch commits the batch + items + fanout event. Each method returns
// only after the durable transaction commits. The store enforces tenant/user
// ownership, consent, resource readiness, duplicate rules, and state guards.
type ApplicationWorkflowStore interface {
	AcceptApplication(ctx context.Context, scope ActorScope, command CreateApplicationCommand, idempotency IdempotencyIdentity, now time.Time) (WorkflowAcceptance, error)
	AcceptSubmission(ctx context.Context, scope ActorScope, applicationID string, idempotency IdempotencyIdentity, now time.Time) (WorkflowAcceptance, error)
	AcceptBatch(ctx context.Context, scope ActorScope, command CreateBatchCommand, idempotency IdempotencyIdentity, now time.Time) (WorkflowAcceptance, error)
	GetApplication(ctx context.Context, scope ActorScope, applicationID string) (models.Application, error)
	GetBatch(ctx context.Context, scope ActorScope, batchID string) (BatchView, error)
	ListApplications(ctx context.Context, scope ActorScope, status *string, cursor string, limit int) ([]models.Application, *string, error)
	GetApplicationTimeline(ctx context.Context, scope ActorScope, applicationID string) ([]models.ApplicationEvent, error)
	GetApplicationReceipt(ctx context.Context, scope ActorScope, applicationID string) (*models.ApplicationReceipt, error)
}

// TaskOutcome determines whether an application worker should acknowledge a
// durable business result or report a retryable infrastructure error.
type TaskOutcome string

const (
	TaskOutcomeCompleted            TaskOutcome = "completed"
	TaskOutcomePaused               TaskOutcome = "paused"
	TaskOutcomeReconciliationQueued TaskOutcome = "reconciliation_queued"
)

// TaskResult is returned after the task's state/event/outbox changes commit.
// Paused and reconciliation outcomes are durable business results, not queue
// errors; workers acknowledge them after commit.
type TaskResult struct {
	Outcome     TaskOutcome
	PauseReason string
}

// TaskFailure provides an explicit retry decision to queue workers. The
// underlying cause is kept for error classification, but Error intentionally
// omits database/provider details so generic worker logs stay sanitized.
type TaskFailure struct {
	Class       string
	SafeToRetry bool
	RetryAfter  time.Duration
	Cause       error
}

func (failure TaskFailure) Error() string {
	if failure.SafeToRetry {
		return "retryable application task failure"
	}
	return "permanent application task failure"
}

func (failure TaskFailure) Unwrap() error { return failure.Cause }

// RetryableTaskFailure marks an infrastructure failure safe to retry. Callers
// must not use it after an external submit may have taken effect.
func RetryableTaskFailure(class string, retryAfter time.Duration, cause error) error {
	return TaskFailure{Class: class, SafeToRetry: true, RetryAfter: retryAfter, Cause: cause}
}

// PermanentTaskFailure marks an invalid or irrecoverable task for dead-letter
// handling without retry.
func PermanentTaskFailure(class string, cause error) error {
	return TaskFailure{Class: class, SafeToRetry: false, Cause: cause}
}

// IsTaskRetryable returns true only for an explicitly typed safe failure.
func IsTaskRetryable(err error) bool {
	var failure TaskFailure
	return errors.As(err, &failure) && failure.SafeToRetry
}

// ApplicationTaskService exposes durable application work to queue consumers.
// The implementation rechecks tenant ownership, current consent, job state,
// connector policy, evidence, and submission safety before each action.
type ApplicationTaskService interface {
	PrepareApplication(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (TaskResult, error)
	FanoutBatch(ctx context.Context, scope ActorScope, batchID string, now time.Time) (TaskResult, error)
	SubmitApplication(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (TaskResult, error)
	ReconcileSubmission(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (TaskResult, error)
}

// ApplicationService validates public workflow input, hashes idempotency data,
// and delegates transactionally guarded operations to ApplicationWorkflowStore.
type ApplicationService struct {
	store              ApplicationWorkflowStore
	idempotencyHMACKey []byte
	now                func() time.Time
}

// NewApplicationService requires a dedicated idempotency HMAC key of at least
// 32 bytes; callers should load it from a secret manager or local env file.
func NewApplicationService(store ApplicationWorkflowStore, idempotencyHMACKey []byte) (*ApplicationService, error) {
	if store == nil || len(idempotencyHMACKey) < 32 {
		return nil, errors.New("invalid application service configuration")
	}
	return &ApplicationService{
		store: store, idempotencyHMACKey: append([]byte(nil), idempotencyHMACKey...), now: time.Now,
	}, nil
}

// CreateApplication queues preparation after the store transaction commits.
func (s *ApplicationService) CreateApplication(ctx context.Context, scope ActorScope, command CreateApplicationCommand, rawIdempotencyKey string) (WorkflowAcceptance, error) {
	scope = normalizeActorScope(scope)
	command = command.Normalize()
	if err := validateActorScope(scope); err != nil || command.Validate() != nil {
		return WorkflowAcceptance{}, ErrApplicationRequest
	}
	identity, err := s.idempotencyIdentity("create_application", rawIdempotencyKey, command)
	if err != nil {
		return WorkflowAcceptance{}, err
	}
	accepted, err := s.store.AcceptApplication(ctx, scope, command, identity, s.now().UTC())
	if err != nil {
		return WorkflowAcceptance{}, err
	}
	if err := accepted.Validate(); err != nil {
		return WorkflowAcceptance{}, ErrApplicationStore
	}
	return accepted, nil
}

// SubmitApplication authorizes a ready application for a durable submission
// workflow. Employer confirmation is produced later by the connector worker.
func (s *ApplicationService) SubmitApplication(ctx context.Context, scope ActorScope, applicationID, rawIdempotencyKey string) (WorkflowAcceptance, error) {
	scope = normalizeActorScope(scope)
	applicationID = strings.ToLower(applicationID)
	if err := validateActorScope(scope); err != nil || !isUUID(applicationID) {
		return WorkflowAcceptance{}, ErrApplicationRequest
	}
	identity, err := s.idempotencyIdentity("submit_application", rawIdempotencyKey, struct {
		ApplicationID string `json:"application_id"`
	}{applicationID})
	if err != nil {
		return WorkflowAcceptance{}, err
	}
	accepted, err := s.store.AcceptSubmission(ctx, scope, applicationID, identity, s.now().UTC())
	if err != nil {
		return WorkflowAcceptance{}, err
	}
	if err := accepted.Validate(); err != nil {
		return WorkflowAcceptance{}, ErrApplicationStore
	}
	return accepted, nil
}

// CreateBatch accepts 1..500 distinct jobs and creates independent item states.
func (s *ApplicationService) CreateBatch(ctx context.Context, scope ActorScope, command CreateBatchCommand, rawIdempotencyKey string) (WorkflowAcceptance, error) {
	scope = normalizeActorScope(scope)
	command = command.Normalize()
	if err := validateActorScope(scope); err != nil || command.Validate() != nil {
		return WorkflowAcceptance{}, ErrApplicationRequest
	}
	identity, err := s.idempotencyIdentity("create_application_batch", rawIdempotencyKey, command)
	if err != nil {
		return WorkflowAcceptance{}, err
	}
	accepted, err := s.store.AcceptBatch(ctx, scope, command, identity, s.now().UTC())
	if err != nil {
		return WorkflowAcceptance{}, err
	}
	if err := accepted.Validate(); err != nil {
		return WorkflowAcceptance{}, ErrApplicationStore
	}
	return accepted, nil
}

// GetApplication returns only an application visible in the verified tenant
// and actor scope. The store must use tenant and user predicates in its query.
func (s *ApplicationService) GetApplication(ctx context.Context, scope ActorScope, applicationID string) (models.Application, error) {
	scope = normalizeActorScope(scope)
	applicationID = strings.ToLower(applicationID)
	if err := validateActorScope(scope); err != nil || !isUUID(applicationID) {
		return models.Application{}, ErrApplicationRequest
	}
	return s.store.GetApplication(ctx, scope, applicationID)
}

// GetBatch returns only a batch visible in the verified tenant and actor scope.
func (s *ApplicationService) GetBatch(ctx context.Context, scope ActorScope, batchID string) (BatchView, error) {
	scope = normalizeActorScope(scope)
	batchID = strings.ToLower(batchID)
	if err := validateActorScope(scope); err != nil || !isUUID(batchID) {
		return BatchView{}, ErrApplicationRequest
	}
	batch, err := s.store.GetBatch(ctx, scope, batchID)
	if err != nil {
		return BatchView{}, err
	}
	if batch.Validate() != nil {
		return BatchView{}, ErrApplicationStore
	}
	return batch, nil
}

// ListApplications retrieves a paginated list of applications for the tenant/user scope.
func (s *ApplicationService) ListApplications(ctx context.Context, scope ActorScope, status *string, cursor string, limit int) (ApplicationListPage, error) {
	scope = normalizeActorScope(scope)
	if err := validateActorScope(scope); err != nil {
		return ApplicationListPage{}, ErrApplicationRequest
	}
	items, nextCursor, err := s.store.ListApplications(ctx, scope, status, cursor, limit)
	if err != nil {
		return ApplicationListPage{}, err
	}
	return ApplicationListPage{
		Items:      items,
		Total:      len(items),
		NextCursor: nextCursor,
	}, nil
}

// GetApplicationTimeline retrieves the append-only event stream for an application.
func (s *ApplicationService) GetApplicationTimeline(ctx context.Context, scope ActorScope, applicationID string) ([]models.ApplicationEvent, error) {
	scope = normalizeActorScope(scope)
	applicationID = strings.ToLower(applicationID)
	if err := validateActorScope(scope); err != nil || !isUUID(applicationID) {
		return nil, ErrApplicationRequest
	}
	return s.store.GetApplicationTimeline(ctx, scope, applicationID)
}

// GetApplicationReceipt retrieves receipt and proof-of-submission details for an application.
func (s *ApplicationService) GetApplicationReceipt(ctx context.Context, scope ActorScope, applicationID string) (*models.ApplicationReceipt, error) {
	scope = normalizeActorScope(scope)
	applicationID = strings.ToLower(applicationID)
	if err := validateActorScope(scope); err != nil || !isUUID(applicationID) {
		return nil, ErrApplicationRequest
	}
	return s.store.GetApplicationReceipt(ctx, scope, applicationID)
}

func (s *ApplicationService) idempotencyIdentity(operation, rawKey string, request any) (IdempotencyIdentity, error) {
	if len(rawKey) < 16 || len(rawKey) > 200 || strings.TrimSpace(rawKey) != rawKey || strings.ContainsAny(rawKey, "\r\n") {
		return IdempotencyIdentity{}, ErrApplicationRequest
	}
	canonical, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Request   any    `json:"request"`
	}{Operation: operation, Request: request})
	if err != nil {
		return IdempotencyIdentity{}, fmt.Errorf("canonicalize idempotent request: %w", ErrApplicationRequest)
	}
	keyMAC := hmac.New(sha256.New, s.idempotencyHMACKey)
	_, _ = keyMAC.Write([]byte(rawKey))
	requestHash := sha256.Sum256(canonical)
	var keyHash [32]byte
	copy(keyHash[:], keyMAC.Sum(nil))
	return IdempotencyIdentity{KeyHash: keyHash, RequestHash: requestHash}, nil
}

func (c CreateApplicationCommand) Validate() error {
	if !isUUID(c.JobID) || !isUUID(c.ResumeVersionID) ||
		!isUUID(c.ConsentID) || !validApplicationMode(c.Mode) {
		return ErrApplicationRequest
	}
	if c.PacketID != nil && !isUUID(*c.PacketID) {
		return ErrApplicationRequest
	}
	return nil
}

func (c CreateBatchCommand) Validate() error {
	if len(c.JobIDs) == 0 || len(c.JobIDs) > MaxApplicationBatchSize ||
		!isUUID(c.ResumeVersionID) || !isUUID(c.ConsentID) ||
		!validApplicationMode(c.Mode) {
		return ErrApplicationRequest
	}
	seen := make(map[string]struct{}, len(c.JobIDs))
	for _, jobID := range c.JobIDs {
		if !isUUID(jobID) {
			return ErrApplicationRequest
		}
		if _, ok := seen[jobID]; ok {
			return ErrApplicationRequest
		}
		seen[jobID] = struct{}{}
	}
	return nil
}

func (a WorkflowAcceptance) Validate() error {
	if !isUUID(a.WorkflowID) ||
		(a.Status != "accepted" && a.Status != "preparing" && a.Status != "queued") ||
		a.AcceptedAt.IsZero() || a.AcceptedAt.Location() != time.UTC {
		return ErrApplicationStore
	}
	return nil
}

func (b BatchView) Validate() error {
	if !isUUID(b.ID) || b.Total < 1 || b.Total > MaxApplicationBatchSize ||
		b.CreatedAt.IsZero() || b.CreatedAt.Location() != time.UTC || len(b.Counts) == 0 {
		return ErrApplicationStore
	}
	total := 0
	for status, count := range b.Counts {
		if strings.TrimSpace(status) == "" || count < 0 {
			return ErrApplicationStore
		}
		total += count
	}
	if total != b.Total {
		return ErrApplicationStore
	}
	return nil
}

// Validate ensures the JSONB value is complete and explicit. The store must
// separately match its IDs and mode to every operation and current job.
func (scope ConsentScopeV1) Validate() error {
	if scope.SchemaVersion != 1 || !scope.Application.Prepare || !scope.Application.Submit ||
		!scope.DataUse.ResumeProcessing || !scope.DataUse.EvidenceBasedAnswerGeneration ||
		!scope.DataUse.SharingWithEmployer || len(scope.Application.Modes) == 0 ||
		len(scope.Application.CompanyIDs) == 0 {
		return ErrConsentInvalid
	}
	modeSet := make(map[ApplicationMode]struct{}, len(scope.Application.Modes))
	for _, mode := range scope.Application.Modes {
		if !validApplicationMode(mode) {
			return ErrConsentInvalid
		}
		if _, duplicate := modeSet[mode]; duplicate {
			return ErrConsentInvalid
		}
		modeSet[mode] = struct{}{}
	}
	companySet := make(map[string]struct{}, len(scope.Application.CompanyIDs))
	for _, companyID := range scope.Application.CompanyIDs {
		if companyID != "*" && !isUUID(companyID) {
			return ErrConsentInvalid
		}
		if _, duplicate := companySet[companyID]; duplicate {
			return ErrConsentInvalid
		}
		companySet[companyID] = struct{}{}
	}
	if _, wildcard := companySet["*"]; wildcard && len(companySet) != 1 {
		return ErrConsentInvalid
	}
	if scope.Application.JobIDs != nil {
		if len(scope.Application.JobIDs) == 0 {
			return ErrConsentInvalid
		}
		jobSet := make(map[string]struct{}, len(scope.Application.JobIDs))
		for _, jobID := range scope.Application.JobIDs {
			if !isUUID(jobID) {
				return ErrConsentInvalid
			}
			if _, duplicate := jobSet[jobID]; duplicate {
				return ErrConsentInvalid
			}
			jobSet[jobID] = struct{}{}
		}
	}
	return nil
}

func validateActorScope(scope ActorScope) error {
	if !isUUID(scope.TenantID) || !isUUID(scope.UserID) {
		return ErrApplicationRequest
	}
	return nil
}

func normalizeActorScope(scope ActorScope) ActorScope {
	scope.TenantID = strings.ToLower(scope.TenantID)
	scope.UserID = strings.ToLower(scope.UserID)
	return scope
}

func (c CreateApplicationCommand) Normalize() CreateApplicationCommand {
	if c.JobID == "" && c.ID != "" {
		c.JobID = c.ID
	}
	c.JobID = strings.ToLower(c.JobID)
	c.ResumeVersionID = strings.ToLower(c.ResumeVersionID)
	c.ConsentID = strings.ToLower(c.ConsentID)
	if c.PacketID != nil {
		packetID := strings.ToLower(*c.PacketID)
		c.PacketID = &packetID
	}
	return c
}

func (c CreateBatchCommand) Normalize() CreateBatchCommand {
	c.ResumeVersionID = strings.ToLower(c.ResumeVersionID)
	c.ConsentID = strings.ToLower(c.ConsentID)
	c.JobIDs = append([]string(nil), c.JobIDs...)
	for index, jobID := range c.JobIDs {
		c.JobIDs[index] = strings.ToLower(jobID)
	}
	return c
}

func validApplicationMode(mode ApplicationMode) bool {
	return mode == ApplicationModeReviewBeforeSubmit || mode == ApplicationModeSubmitWhenReady
}

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/oneRole/models"
)

const (
	ApplicationTaskEventPaused          = "application.task.paused"
	TaskPausePacketEngineUnavailable    = "packet_preparer_unavailable"
	TaskPauseEmployerAdapterUnavailable = "employer_adapter_unavailable"
	TaskPauseReconciliationUnavailable  = "reconciliation_adapter_unavailable"
)

// ApplicationTaskSnapshot is the authoritative, tenant-scoped state needed to
// make an idempotent worker decision. Persistence must populate it from the
// current database row after checking membership and actor ownership.
type ApplicationTaskSnapshot struct {
	ID          string
	TenantID    string
	UserID      string
	JobID       string
	CompanyID   string
	ConsentID   string
	Status      models.ApplicationStatus
	Mode        ApplicationMode
	Version     int64
	PauseReason string
}

func (snapshot ApplicationTaskSnapshot) Validate(scope ActorScope) error {
	if !isUUID(snapshot.ID) || snapshot.TenantID != scope.TenantID ||
		snapshot.UserID != scope.UserID || !isUUID(snapshot.JobID) ||
		!isUUID(snapshot.CompanyID) || !isUUID(snapshot.ConsentID) ||
		!validApplicationMode(snapshot.Mode) || snapshot.Version < 1 ||
		!isApplicationStatus(snapshot.Status) {
		return ErrApplicationTaskStore
	}
	return nil
}

// ApplicationTaskTransition is a compare-and-set command. The repository
// verifies the current tenant membership and application owner again, compares
// both expected fields, writes status and pause reason, and appends an immutable
// timeline event in one transaction. Pause transitions have no external side
// effect and therefore do not create another queue message.
type ApplicationTaskTransition struct {
	ApplicationID   string
	ExpectedStatus  models.ApplicationStatus
	ExpectedVersion int64
	ToStatus        models.ApplicationStatus
	PauseReason     string
	EventType       string
	Now             time.Time
}

// ApplicationTaskStore is the persistence boundary for safe worker decisions.
// Fanout must create per-job application states and identifier-only preparation
// outbox messages atomically, while rechecking tenant, membership, job, and
// consent scope for every item.
type ApplicationTaskStore interface {
	LoadApplicationTask(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (ApplicationTaskSnapshot, error)
	RecordApplicationTaskTransition(ctx context.Context, scope ActorScope, command ApplicationTaskTransition) error
	FanoutApplicationBatch(ctx context.Context, scope ActorScope, batchID string, now time.Time) (TaskResult, error)
}

// DurableApplicationTaskService is the conservative task runner available
// until a real evidence-backed packet preparer and verified employer adapters
// are installed. It turns unsupported work into persisted manual-action states;
// it never reports a submission or performs an external employer request.
type DurableApplicationTaskService struct {
	store ApplicationTaskStore
}

// NewDurableApplicationTaskService requires the transactionally guarded store
// used by API and worker processes.
func NewDurableApplicationTaskService(store ApplicationTaskStore) (*DurableApplicationTaskService, error) {
	if store == nil {
		return nil, errors.New("application task store is required")
	}
	return &DurableApplicationTaskService{store: store}, nil
}

// PrepareApplication records a durable manual-action pause until a packet
// preparer can safely read verified evidence and answer supported questions.
// It intentionally does not fabricate a packet or answer.
func (s *DurableApplicationTaskService) PrepareApplication(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (TaskResult, error) {
	snapshot, err := s.load(ctx, scope, applicationID, now)
	if err != nil {
		return TaskResult{}, err
	}
	switch snapshot.Status {
	case models.ApplicationPreparing:
		return s.pause(ctx, scope, snapshot, models.ApplicationRequiresManualAction, TaskPausePacketEngineUnavailable, now)
	case models.ApplicationNeedsUserInput, models.ApplicationRequiresManualAction:
		return persistedPause(snapshot, "application is already paused"), nil
	case models.ApplicationReadyForReview, models.ApplicationApproved, models.ApplicationQueued,
		models.ApplicationSubmitting, models.ApplicationSubmitted, models.ApplicationSubmissionUnconfirmed:
		return TaskResult{Outcome: TaskOutcomeCompleted}, nil
	default:
		return TaskResult{}, PermanentTaskFailure("invalid_application_state", ErrApplicationStateConflict)
	}
}

// FanoutBatch delegates fanout to the store so batch items, per-application
// states, timeline entries, and preparation outbox records commit together.
func (s *DurableApplicationTaskService) FanoutBatch(ctx context.Context, scope ActorScope, batchID string, now time.Time) (TaskResult, error) {
	scope = normalizeActorScope(scope)
	batchID = strings.ToLower(strings.TrimSpace(batchID))
	if err := validateActorScope(scope); err != nil || !isUUID(batchID) || !isUTCTimestamp(now) {
		return TaskResult{}, PermanentTaskFailure("invalid_task", ErrApplicationRequest)
	}
	result, err := s.store.FanoutApplicationBatch(ctx, scope, batchID, now.UTC())
	if err != nil {
		return TaskResult{}, classifyApplicationTaskStoreError(err)
	}
	if result.Outcome != TaskOutcomeCompleted && result.Outcome != TaskOutcomePaused {
		return TaskResult{}, PermanentTaskFailure("invalid_store_result", ErrApplicationTaskStore)
	}
	if result.Outcome == TaskOutcomePaused && strings.TrimSpace(result.PauseReason) == "" {
		return TaskResult{}, PermanentTaskFailure("invalid_store_result", ErrApplicationTaskStore)
	}
	return result, nil
}

// SubmitApplication deliberately leaves queued work in a durable manual state
// while no verified, employer-authorized connector is configured. It cannot
// mark an application submitted or issue a speculative external request.
func (s *DurableApplicationTaskService) SubmitApplication(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (TaskResult, error) {
	snapshot, err := s.load(ctx, scope, applicationID, now)
	if err != nil {
		return TaskResult{}, err
	}
	switch snapshot.Status {
	case models.ApplicationQueued:
		return s.pause(ctx, scope, snapshot, models.ApplicationRequiresManualAction, TaskPauseEmployerAdapterUnavailable, now)
	case models.ApplicationRequiresManualAction:
		return persistedPause(snapshot, "employer adapter is unavailable"), nil
	case models.ApplicationSubmissionUnconfirmed:
		return persistedPause(snapshot, TaskPauseReconciliationUnavailable), nil
	case models.ApplicationSubmitted:
		return TaskResult{Outcome: TaskOutcomeCompleted}, nil
	default:
		return TaskResult{}, PermanentTaskFailure("invalid_application_state", ErrApplicationStateConflict)
	}
}

// ReconcileSubmission never repeats an uncertain submission. Without a
// verified reconciliation adapter it persists a manual-action state instead.
func (s *DurableApplicationTaskService) ReconcileSubmission(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (TaskResult, error) {
	snapshot, err := s.load(ctx, scope, applicationID, now)
	if err != nil {
		return TaskResult{}, err
	}
	switch snapshot.Status {
	case models.ApplicationSubmissionUnconfirmed:
		return s.pause(ctx, scope, snapshot, models.ApplicationRequiresManualAction, TaskPauseReconciliationUnavailable, now)
	case models.ApplicationRequiresManualAction:
		return persistedPause(snapshot, TaskPauseReconciliationUnavailable), nil
	case models.ApplicationSubmitted:
		return TaskResult{Outcome: TaskOutcomeCompleted}, nil
	default:
		return TaskResult{}, PermanentTaskFailure("invalid_application_state", ErrApplicationStateConflict)
	}
}

func (s *DurableApplicationTaskService) load(ctx context.Context, scope ActorScope, applicationID string, now time.Time) (ApplicationTaskSnapshot, error) {
	scope = normalizeActorScope(scope)
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	if err := validateActorScope(scope); err != nil || !isUUID(applicationID) || !isUTCTimestamp(now) {
		return ApplicationTaskSnapshot{}, PermanentTaskFailure("invalid_task", ErrApplicationRequest)
	}
	snapshot, err := s.store.LoadApplicationTask(ctx, scope, applicationID, now.UTC())
	if err != nil {
		return ApplicationTaskSnapshot{}, classifyApplicationTaskStoreError(err)
	}
	if err := snapshot.Validate(scope); err != nil || snapshot.ID != applicationID {
		return ApplicationTaskSnapshot{}, PermanentTaskFailure("invalid_store_result", ErrApplicationTaskStore)
	}
	return snapshot, nil
}

func (s *DurableApplicationTaskService) pause(ctx context.Context, scope ActorScope, snapshot ApplicationTaskSnapshot, to models.ApplicationStatus, reason string, now time.Time) (TaskResult, error) {
	if err := TransitionApplication(snapshot.Status, to, TransitionEvidence{}); err != nil {
		return TaskResult{}, PermanentTaskFailure("invalid_application_transition", err)
	}
	command := ApplicationTaskTransition{
		ApplicationID: snapshot.ID, ExpectedStatus: snapshot.Status,
		ExpectedVersion: snapshot.Version, ToStatus: to, PauseReason: reason,
		EventType: ApplicationTaskEventPaused, Now: now.UTC(),
	}
	if err := s.store.RecordApplicationTaskTransition(ctx, scope, command); err != nil {
		return TaskResult{}, classifyApplicationTaskStoreError(err)
	}
	return TaskResult{Outcome: TaskOutcomePaused, PauseReason: reason}, nil
}

func persistedPause(snapshot ApplicationTaskSnapshot, fallback string) TaskResult {
	reason := strings.TrimSpace(snapshot.PauseReason)
	if reason == "" {
		reason = fallback
	}
	return TaskResult{Outcome: TaskOutcomePaused, PauseReason: reason}
}

func classifyApplicationTaskStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrApplicationStore) || errors.Is(err, ErrApplicationTaskStore) {
		return RetryableTaskFailure("application_store_unavailable", 0, err)
	}
	if errors.Is(err, ErrApplicationNotFound) || errors.Is(err, ErrApplicationStateConflict) ||
		errors.Is(err, ErrConsentInvalid) || errors.Is(err, ErrApplicationPolicy) ||
		errors.Is(err, ErrApplicationResourceInvalid) || errors.Is(err, ErrApplicationRequest) {
		return PermanentTaskFailure("application_task_rejected", err)
	}
	return PermanentTaskFailure("unclassified_application_task_error", err)
}

func isUTCTimestamp(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC
}

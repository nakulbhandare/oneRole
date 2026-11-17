package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oneRole/models"
)

const (
	taskTenantID  = "11111111-1111-4111-8111-111111111111"
	taskUserID    = "22222222-2222-4222-8222-222222222222"
	taskAppID     = "33333333-3333-4333-8333-333333333333"
	taskJobID     = "44444444-4444-4444-8444-444444444444"
	taskCompanyID = "55555555-5555-4555-8555-555555555555"
	taskConsentID = "66666666-6666-4666-8666-666666666666"
)

type applicationTaskStoreFake struct {
	snapshot      ApplicationTaskSnapshot
	loadErr       error
	transitionErr error
	transitions   []ApplicationTaskTransition
	fanout        TaskResult
	fanoutErr     error
}

func (s *applicationTaskStoreFake) LoadApplicationTask(_ context.Context, scope ActorScope, applicationID string, now time.Time) (ApplicationTaskSnapshot, error) {
	if s.loadErr != nil {
		return ApplicationTaskSnapshot{}, s.loadErr
	}
	if scope.TenantID != s.snapshot.TenantID || scope.UserID != s.snapshot.UserID ||
		applicationID != s.snapshot.ID || now.Location() != time.UTC {
		return ApplicationTaskSnapshot{}, ErrApplicationNotFound
	}
	return s.snapshot, nil
}

func (s *applicationTaskStoreFake) RecordApplicationTaskTransition(_ context.Context, scope ActorScope, command ApplicationTaskTransition) error {
	if s.transitionErr != nil {
		return s.transitionErr
	}
	if scope.TenantID != s.snapshot.TenantID || scope.UserID != s.snapshot.UserID ||
		command.ApplicationID != s.snapshot.ID || command.ExpectedVersion != s.snapshot.Version ||
		command.ExpectedStatus != s.snapshot.Status {
		return ErrApplicationStateConflict
	}
	s.transitions = append(s.transitions, command)
	s.snapshot.Status = command.ToStatus
	s.snapshot.Version++
	s.snapshot.PauseReason = command.PauseReason
	return nil
}

func (s *applicationTaskStoreFake) FanoutApplicationBatch(_ context.Context, _ ActorScope, _ string, now time.Time) (TaskResult, error) {
	if s.fanoutErr != nil {
		return TaskResult{}, s.fanoutErr
	}
	if now.Location() != time.UTC {
		return TaskResult{}, ErrApplicationRequest
	}
	return s.fanout, nil
}

func TestDurableTaskServicePersistsPauseAndAcknowledgesRedelivery(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := newApplicationTaskStore(models.ApplicationPreparing)
	service, err := NewDurableApplicationTaskService(store)
	if err != nil {
		t.Fatal(err)
	}
	scope := ActorScope{TenantID: taskTenantID, UserID: taskUserID}
	result, err := service.PrepareApplication(context.Background(), scope, taskAppID, now)
	if err != nil {
		t.Fatalf("prepare task: %v", err)
	}
	if result.Outcome != TaskOutcomePaused || result.PauseReason != TaskPausePacketEngineUnavailable {
		t.Fatalf("unsupported preparation must be explicitly paused: %#v", result)
	}
	if store.snapshot.Status != models.ApplicationRequiresManualAction || store.snapshot.PauseReason != TaskPausePacketEngineUnavailable || len(store.transitions) != 1 {
		t.Fatalf("pause must be durable before task acknowledgement: snapshot=%#v transitions=%#v", store.snapshot, store.transitions)
	}

	// JetStream can redeliver after a crash between the database commit and ACK.
	// The second invocation observes the durable state and does not append another event.
	result, err = service.PrepareApplication(context.Background(), scope, taskAppID, now)
	if err != nil || result.Outcome != TaskOutcomePaused || len(store.transitions) != 1 {
		t.Fatalf("redelivery should return the persisted pause idempotently: result=%#v err=%v", result, err)
	}
}

func TestDurableTaskServiceDisablesSubmissionWithoutAuthorizedAdapter(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := newApplicationTaskStore(models.ApplicationQueued)
	service, err := NewDurableApplicationTaskService(store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.SubmitApplication(context.Background(), ActorScope{TenantID: taskTenantID, UserID: taskUserID}, taskAppID, now)
	if err != nil {
		t.Fatalf("submit task: %v", err)
	}
	if result.Outcome != TaskOutcomePaused || result.PauseReason != TaskPauseEmployerAdapterUnavailable {
		t.Fatalf("submission without a verified connector must pause: %#v", result)
	}
	if store.snapshot.Status != models.ApplicationRequiresManualAction || len(store.transitions) != 1 || store.transitions[0].ToStatus == models.ApplicationSubmitted {
		t.Fatalf("submission must not be reported successful: %#v", store)
	}
}

func TestDurableTaskServiceDoesNotBlindlyRetryAmbiguousSubmission(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := newApplicationTaskStore(models.ApplicationSubmissionUnconfirmed)
	service, _ := NewDurableApplicationTaskService(store)
	result, err := service.ReconcileSubmission(context.Background(), ActorScope{TenantID: taskTenantID, UserID: taskUserID}, taskAppID, now)
	if err != nil || result.Outcome != TaskOutcomePaused || result.PauseReason != TaskPauseReconciliationUnavailable {
		t.Fatalf("unknown outcome needs manual reconciliation, result=%#v err=%v", result, err)
	}
	if store.snapshot.Status != models.ApplicationRequiresManualAction || len(store.transitions) != 1 {
		t.Fatalf("unknown external outcome must never be resubmitted: %#v", store)
	}
}

func TestDurableTaskServiceRequiresDurableBatchFanoutResult(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := newApplicationTaskStore(models.ApplicationPreparing)
	store.fanout = TaskResult{Outcome: TaskOutcomeCompleted}
	service, _ := NewDurableApplicationTaskService(store)
	result, err := service.FanoutBatch(context.Background(), ActorScope{TenantID: taskTenantID, UserID: taskUserID}, "77777777-7777-4777-8777-777777777777", now)
	if err != nil || result.Outcome != TaskOutcomeCompleted {
		t.Fatalf("committed batch fanout should complete: result=%#v err=%v", result, err)
	}
	store.fanout = TaskResult{Outcome: TaskOutcomeReconciliationQueued}
	if _, err := service.FanoutBatch(context.Background(), ActorScope{TenantID: taskTenantID, UserID: taskUserID}, "77777777-7777-4777-8777-777777777777", now); err == nil || IsTaskRetryable(err) {
		t.Fatalf("fanout must reject a reconciliation result without a committed reconciliation operation: %v", err)
	}
}

func TestDurableTaskServiceClassifiesOnlyExplicitStoreFailuresAsRetryable(t *testing.T) {
	store := newApplicationTaskStore(models.ApplicationPreparing)
	store.loadErr = ErrApplicationStore
	service, _ := NewDurableApplicationTaskService(store)
	_, err := service.PrepareApplication(context.Background(), ActorScope{TenantID: taskTenantID, UserID: taskUserID}, taskAppID, time.Now().UTC())
	if !IsTaskRetryable(err) {
		t.Fatalf("store outage before side effects must be explicitly retryable: %v", err)
	}

	store.loadErr = errors.New("unclassified adapter error")
	_, err = service.PrepareApplication(context.Background(), ActorScope{TenantID: taskTenantID, UserID: taskUserID}, taskAppID, time.Now().UTC())
	if err == nil || IsTaskRetryable(err) {
		t.Fatalf("unclassified failures must not be blindly retried: %v", err)
	}
}

func newApplicationTaskStore(status models.ApplicationStatus) *applicationTaskStoreFake {
	return &applicationTaskStoreFake{snapshot: ApplicationTaskSnapshot{
		ID: taskAppID, TenantID: taskTenantID, UserID: taskUserID, JobID: taskJobID,
		CompanyID: taskCompanyID, ConsentID: taskConsentID,
		Status: status, Mode: ApplicationModeSubmitWhenReady, Version: 1,
	}}
}

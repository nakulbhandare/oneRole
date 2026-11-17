package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oneRole/queue"
	"github.com/oneRole/service"
)

type fakeApplicationTasks struct {
	prepareResult   service.TaskResult
	batchResult     service.TaskResult
	submitResult    service.TaskResult
	reconcileResult service.TaskResult
	err             error
	scope           service.ActorScope
	id              string
	now             time.Time
	called          string
}

func (f *fakeApplicationTasks) PrepareApplication(_ context.Context, scope service.ActorScope, id string, at time.Time) (service.TaskResult, error) {
	f.called, f.scope, f.id, f.now = "prepare", scope, id, at
	return f.prepareResult, f.err
}
func (f *fakeApplicationTasks) FanoutBatch(_ context.Context, scope service.ActorScope, id string, at time.Time) (service.TaskResult, error) {
	f.called, f.scope, f.id, f.now = "batch", scope, id, at
	return f.batchResult, f.err
}
func (f *fakeApplicationTasks) SubmitApplication(_ context.Context, scope service.ActorScope, id string, at time.Time) (service.TaskResult, error) {
	f.called, f.scope, f.id, f.now = "submit", scope, id, at
	return f.submitResult, f.err
}
func (f *fakeApplicationTasks) ReconcileSubmission(_ context.Context, scope service.ActorScope, id string, at time.Time) (service.TaskResult, error) {
	f.called, f.scope, f.id, f.now = "reconcile", scope, id, at
	return f.reconcileResult, f.err
}

func validApplicationEvent(eventType string) queue.Event {
	tenantID := "77777777-7777-4777-8777-777777777777"
	payload := map[string]string{"application_id": "22222222-2222-4222-8222-222222222222", "actor_user_id": "88888888-8888-4888-8888-888888888888"}
	aggregateType, aggregateID := "application", payload["application_id"]
	if eventType == queue.EventApplicationBatchRequested {
		payload = map[string]string{"batch_id": "33333333-3333-4333-8333-333333333333", "actor_user_id": payload["actor_user_id"]}
		aggregateType, aggregateID = "application_batch", payload["batch_id"]
	}
	return queue.Event{ID: "11111111-1111-4111-8111-111111111111", Type: eventType,
		SchemaVersion: 1, AggregateType: aggregateType, AggregateID: aggregateID,
		TenantID: &tenantID, CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC), Payload: payload}
}

func TestApplicationEventHandlerDispatchesWithPersistedActorScope(t *testing.T) {
	now := time.Date(2026, 10, 30, 13, 0, 0, 0, time.FixedZone("test", -7*60*60))
	for _, test := range []struct {
		eventType string
		called    string
	}{
		{queue.EventApplicationPrepareRequested, "prepare"},
		{queue.EventApplicationBatchRequested, "batch"},
		{queue.EventApplicationSubmitRequested, "submit"},
		{queue.EventApplicationReconcileRequested, "reconcile"},
	} {
		t.Run(test.called, func(t *testing.T) {
			service := &fakeApplicationTasks{
				prepareResult:   service.TaskResult{Outcome: service.TaskOutcomeCompleted},
				batchResult:     service.TaskResult{Outcome: service.TaskOutcomeCompleted},
				submitResult:    service.TaskResult{Outcome: service.TaskOutcomeReconciliationQueued},
				reconcileResult: service.TaskResult{Outcome: service.TaskOutcomePaused, PauseReason: "not_confirmed"},
			}
			handler := ApplicationEventHandler{Tasks: service, Now: func() time.Time { return now }}
			if err := handler.HandleEvent(context.Background(), validApplicationEvent(test.eventType)); err != nil {
				t.Fatal(err)
			}
			if service.called != test.called || service.scope.TenantID != "77777777-7777-4777-8777-777777777777" || service.scope.UserID != "88888888-8888-4888-8888-888888888888" {
				t.Fatalf("wrong service dispatch or actor scope: %+v", service)
			}
			if service.now.Location() != time.UTC || !service.now.Equal(now.UTC()) {
				t.Fatalf("service timestamp is not UTC: %s", service.now)
			}
		})
	}
}

func TestApplicationEventHandlerReturnsOnlyExplicitSafeRetry(t *testing.T) {
	service := &fakeApplicationTasks{err: service.RetryableTaskFailure("DB_TEMPORARY", 2*time.Second, errors.New("database details should not leak"))}
	handler := ApplicationEventHandler{Tasks: service}
	err := handler.HandleEvent(context.Background(), validApplicationEvent(queue.EventApplicationSubmitRequested))
	var failure *WorkFailure
	if !errors.As(err, &failure) || !failure.SafeToRetry || !failure.Retryable || failure.Class != "DB_TEMPORARY" || failure.RetryAfter != 2*time.Second {
		t.Fatalf("safe task failure lost its retry decision: %#v", err)
	}
	if failure.Error() != "DB_TEMPORARY" {
		t.Fatalf("failure error leaked details: %q", failure.Error())
	}
}

func TestApplicationEventHandlerRejectsUnknownOutcome(t *testing.T) {
	service := &fakeApplicationTasks{submitResult: service.TaskResult{Outcome: service.TaskOutcome("invented_success")}}
	handler := ApplicationEventHandler{Tasks: service}
	err := handler.HandleEvent(context.Background(), validApplicationEvent(queue.EventApplicationSubmitRequested))
	var failure *WorkFailure
	if !errors.As(err, &failure) || failure.Class != "INVALID_TASK_RESULT" {
		t.Fatalf("unknown outcome should be rejected, got %v", err)
	}
}

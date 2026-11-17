package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oneRole/queue"
	"github.com/oneRole/service"
)

// ApplicationEventHandler dispatches ID-only events to service-owned use cases.
// The service must reload the application, batch, actor, consent, job, and
// current employer capability under the supplied tenant scope before acting.
type ApplicationEventHandler struct {
	Tasks service.ApplicationTaskService
	Now   func() time.Time
}

func (h ApplicationEventHandler) HandleEvent(ctx context.Context, event queue.Event) error {
	if h.Tasks == nil {
		return &WorkFailure{Class: "APPLICATION_TASK_SERVICE_UNAVAILABLE", Retryable: true, SafeToRetry: true}
	}
	if err := event.Validate(); err != nil {
		return &WorkFailure{Class: "INVALID_APPLICATION_EVENT", Cause: err}
	}
	if event.TenantID == nil {
		return &WorkFailure{Class: "MISSING_TENANT_SCOPE"}
	}
	actorID := event.Payload["actor_user_id"]
	if actorID == "" {
		return &WorkFailure{Class: "MISSING_ACTOR_SCOPE"}
	}
	now := h.Now
	if now == nil {
		now = time.Now
	}
	scope := service.ActorScope{TenantID: *event.TenantID, UserID: actorID}
	at := now().UTC()

	var result service.TaskResult
	var err error
	switch event.Type {
	case queue.EventApplicationPrepareRequested:
		result, err = h.Tasks.PrepareApplication(ctx, scope, event.Payload["application_id"], at)
	case queue.EventApplicationBatchRequested:
		result, err = h.Tasks.FanoutBatch(ctx, scope, event.Payload["batch_id"], at)
	case queue.EventApplicationSubmitRequested:
		result, err = h.Tasks.SubmitApplication(ctx, scope, event.Payload["application_id"], at)
	case queue.EventApplicationReconcileRequested:
		result, err = h.Tasks.ReconcileSubmission(ctx, scope, event.Payload["application_id"], at)
	default:
		return &WorkFailure{Class: "UNSUPPORTED_APPLICATION_EVENT"}
	}
	if err != nil {
		var taskFailure service.TaskFailure
		if errors.As(err, &taskFailure) {
			return &WorkFailure{
				Class: taskFailure.Class, Retryable: service.IsTaskRetryable(err),
				SafeToRetry: taskFailure.SafeToRetry, RetryAfter: taskFailure.RetryAfter, Cause: err,
			}
		}
		if service.IsTaskRetryable(err) {
			return &WorkFailure{Class: "APPLICATION_TASK_TRANSIENT", Retryable: true, SafeToRetry: true, Cause: err}
		}
		return &WorkFailure{Class: "APPLICATION_TASK_FAILED", Cause: err}
	}
	if err := validateTaskResult(result); err != nil {
		return &WorkFailure{Class: "INVALID_TASK_RESULT", Cause: err}
	}
	// Completed, paused-for-user-input, and reconciliation-queued are all
	// durable outcomes. HandleDelivery marks the inbox complete before acking.
	return nil
}

func validateTaskResult(result service.TaskResult) error {
	switch result.Outcome {
	case service.TaskOutcomeCompleted:
		if result.PauseReason != "" {
			return fmt.Errorf("completed outcome cannot contain a pause reason")
		}
	case service.TaskOutcomePaused:
		if result.PauseReason == "" {
			return fmt.Errorf("paused outcome requires a reason")
		}
	case service.TaskOutcomeReconciliationQueued:
		if result.PauseReason != "" {
			return fmt.Errorf("reconciliation outcome cannot contain a pause reason")
		}
	default:
		return fmt.Errorf("unknown durable task outcome %q", result.Outcome)
	}
	return nil
}

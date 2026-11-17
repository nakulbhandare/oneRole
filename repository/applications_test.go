package repository

import (
	"testing"

	"github.com/oneRole/service"
)

func TestBatchFanoutMarksAllSuccessfulItemsCompleted(t *testing.T) {
	if got := batchTerminalState(0); got != "completed" {
		t.Fatalf("batch with all accepted fanout items status = %q; want completed", got)
	}
}

func TestBatchFanoutMarksAnyFailedOrDuplicateItemPartial(t *testing.T) {
	if got := batchTerminalState(1); got != "partial" {
		t.Fatalf("batch with failed or duplicate items status = %q; want partial", got)
	}
}

func TestApplicationAvailabilityRequiresConfirmedOpenJob(t *testing.T) {
	for _, test := range []struct {
		status  string
		allowed bool
	}{{"open", true}, {"stale", false}, {"closed", false}, {"removed", false}, {"unknown", false}} {
		if got := applicationJobStatusAllowed(test.status); got != test.allowed {
			t.Errorf("application status check for %q = %t; want %t", test.status, got, test.allowed)
		}
	}
}

func TestWorkerPauseAuditDoesNotImpersonateUser(t *testing.T) {
	actorType, actorID := applicationEventActor(service.ApplicationTaskEventPaused, "11111111-1111-4111-8111-111111111111")
	if actorType != "worker" || actorID != nil {
		t.Fatalf("worker pause actor = (%q, %#v); want worker with NULL actor ID", actorType, actorID)
	}

	actorType, actorID = applicationEventActor("application.created", "11111111-1111-4111-8111-111111111111")
	if actorType != "user" || actorID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("user creation actor = (%q, %#v); want originating user", actorType, actorID)
	}
}

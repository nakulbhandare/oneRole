package service

import (
	"testing"

	"github.com/oneRole/models"
)

func TestTransitionApplicationRequiresVerifiedReceipt(t *testing.T) {
	if err := TransitionApplication(models.ApplicationSubmitting, models.ApplicationSubmitted, TransitionEvidence{}); err == nil {
		t.Fatal("transition without receipt evidence must fail")
	}
	if err := TransitionApplication(models.ApplicationSubmitting, models.ApplicationSubmitted, TransitionEvidence{VerifiedReceiptID: "receipt-17"}); err != nil {
		t.Fatalf("transition with verified receipt evidence failed: %v", err)
	}
	if err := TransitionApplication(models.ApplicationSubmissionUnconfirmed, models.ApplicationSubmitted, TransitionEvidence{VerifiedReceiptID: "receipt-18"}); err != nil {
		t.Fatalf("reconciled receipt transition failed: %v", err)
	}
}

func TestTransitionApplicationRequiresAmbiguityAndSafeRetry(t *testing.T) {
	if err := TransitionApplication(models.ApplicationSubmitting, models.ApplicationSubmissionUnconfirmed, TransitionEvidence{}); err == nil {
		t.Fatal("submission_unconfirmed requires an ambiguous outcome")
	}
	if err := TransitionApplication(models.ApplicationSubmitting, models.ApplicationSubmissionUnconfirmed, TransitionEvidence{AmbiguousOutcome: true}); err != nil {
		t.Fatalf("ambiguous outcome was rejected: %v", err)
	}
	if err := TransitionApplication(models.ApplicationSubmissionUnconfirmed, models.ApplicationFailedRetryable, TransitionEvidence{
		KnownSafeToRetry: true, RetryBudgetRemaining: true,
	}); err == nil {
		t.Fatal("ambiguous outcome cannot be marked retryable without reconciliation")
	}
	evidence := TransitionEvidence{KnownSafeToRetry: true, RetryBudgetRemaining: true, ReconciledNoSubmission: true}
	if err := TransitionApplication(models.ApplicationSubmissionUnconfirmed, models.ApplicationFailedRetryable, evidence); err != nil {
		t.Fatalf("reconciled no-submission retry transition failed: %v", err)
	}
	if err := TransitionApplication(models.ApplicationFailedRetryable, models.ApplicationQueued, TransitionEvidence{}); err == nil {
		t.Fatal("retry queueing requires safe-to-retry evidence and remaining budget")
	}
}

func TestTransitionApplicationRejectsIllegalAndUnknownTransitions(t *testing.T) {
	if err := TransitionApplication(models.ApplicationSubmitted, models.ApplicationQueued, TransitionEvidence{}); err == nil {
		t.Fatal("terminal submitted state cannot return to queue")
	}
	if err := TransitionApplication("made_up", models.ApplicationPreparing, TransitionEvidence{}); err == nil {
		t.Fatal("unknown source status must fail")
	}
}

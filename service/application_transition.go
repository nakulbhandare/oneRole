package service

import (
	"fmt"

	"github.com/oneRole/models"
)

type TransitionEvidence struct {
	// VerifiedReceiptID identifies durable evidence that the employer accepted
	// the application. A successful network call alone is insufficient.
	VerifiedReceiptID      string
	AmbiguousOutcome       bool
	KnownSafeToRetry       bool
	RetryBudgetRemaining   bool
	ReconciledNoSubmission bool
}

type TransitionError struct {
	From   models.ApplicationStatus
	To     models.ApplicationStatus
	Reason string
}

func (e TransitionError) Error() string {
	return fmt.Sprintf("application transition %q -> %q rejected: %s", e.From, e.To, e.Reason)
}

// TransitionApplication validates one proposed state change. The caller owns
// persistence and must write the accepted event/state atomically.
func TransitionApplication(from, to models.ApplicationStatus, evidence TransitionEvidence) error {
	normFrom := models.NormalizeApplicationStatus(string(from))
	normTo := models.NormalizeApplicationStatus(string(to))

	if !isApplicationStatus(from) && !models.IsRule8Status(normFrom) {
		return TransitionError{From: from, To: to, Reason: "unknown application status"}
	}
	if !isApplicationStatus(to) && !models.IsRule8Status(normTo) {
		return TransitionError{From: from, To: to, Reason: "unknown application status"}
	}
	if !allowedTransition(from, to) && !allowedRule8Transition(normFrom, normTo) {
		return TransitionError{From: from, To: to, Reason: "transition is not allowed"}
	}

	switch {
	case to == models.ApplicationSubmitted || normTo == models.StatusSubmittedConfirmed:
		if evidence.VerifiedReceiptID == "" {
			return TransitionError{From: from, To: to, Reason: "verified employer receipt evidence is required"}
		}
	case (from == models.ApplicationSubmitting || from == models.ApplicationInFlight || normFrom == models.StatusSubmitting) && (to == models.ApplicationSubmissionUnconfirmed || normTo == models.StatusSubmittedUnverified):
		if !evidence.AmbiguousOutcome {
			return TransitionError{From: from, To: to, Reason: "ambiguous employer outcome is required"}
		}
	case to == models.ApplicationFailedRetryable || normTo == models.StatusFailed:
		if !evidence.KnownSafeToRetry || !evidence.RetryBudgetRemaining ||
			((from == models.ApplicationSubmissionUnconfirmed || normFrom == models.StatusSubmittedUnverified) && !evidence.ReconciledNoSubmission) {
			return TransitionError{From: from, To: to, Reason: "safe retry evidence and remaining budget are required"}
		}
	case (from == models.ApplicationFailedRetryable || normFrom == models.StatusFailed) && (to == models.ApplicationQueued || normTo == models.StatusQueued):
		if !evidence.KnownSafeToRetry || !evidence.RetryBudgetRemaining {
			return TransitionError{From: from, To: to, Reason: "safe retry evidence and remaining budget are required"}
		}
	}
	return nil
}

func allowedRule8Transition(from, to models.ApplicationStatus) bool {
	switch from {
	case models.StatusDiscovered:
		return oneStatusOf(to, models.StatusEligible, models.StatusRejected)
	case models.StatusEligible:
		return oneStatusOf(to, models.StatusPreparing, models.StatusRejected)
	case models.StatusPreparing:
		return oneStatusOf(to, models.StatusReady, models.StatusActionRequired, models.StatusFailed, models.StatusRejected)
	case models.StatusReady:
		return oneStatusOf(to, models.StatusQueued, models.StatusRejected)
	case models.StatusQueued:
		return oneStatusOf(to, models.StatusSubmitting, models.StatusActionRequired, models.StatusRejected)
	case models.StatusSubmitting:
		return oneStatusOf(to, models.StatusSubmittedConfirmed, models.StatusSubmittedUnverified,
			models.StatusActionRequired, models.StatusFailed, models.StatusRejected)
	case models.StatusSubmittedUnverified:
		return oneStatusOf(to, models.StatusSubmittedConfirmed, models.StatusActionRequired,
			models.StatusFailed, models.StatusRejected)
	case models.StatusFailed:
		return oneStatusOf(to, models.StatusQueued, models.StatusRejected)
	default:
		return false
	}
}

func allowedTransition(from, to models.ApplicationStatus) bool {
	switch from {
	case models.ApplicationDiscovered:
		return oneStatusOf(to, models.ApplicationMatched, models.ApplicationSkipped, models.ApplicationExpired)
	case models.ApplicationMatched:
		return oneStatusOf(to, models.ApplicationPreparing, models.ApplicationSkipped, models.ApplicationExpired, models.ApplicationRejectedByUser)
	case models.ApplicationPreparing:
		return oneStatusOf(to, models.ApplicationNeedsUserInput, models.ApplicationReadyForReview,
			models.ApplicationRequiresManualAction, models.ApplicationFailedRetryable,
			models.ApplicationFailedPermanent, models.ApplicationDuplicate, models.ApplicationExpired)
	case models.ApplicationNeedsUserInput:
		return oneStatusOf(to, models.ApplicationPreparing, models.ApplicationSkipped, models.ApplicationExpired, models.ApplicationRejectedByUser)
	case models.ApplicationReadyForReview:
		return oneStatusOf(to, models.ApplicationApproved, models.ApplicationSkipped, models.ApplicationExpired, models.ApplicationRejectedByUser)
	case models.ApplicationApproved:
		return oneStatusOf(to, models.ApplicationQueued, models.ApplicationExpired, models.ApplicationRejectedByUser)
	case models.ApplicationQueued:
		return oneStatusOf(to, models.ApplicationSubmitting, models.ApplicationRequiresManualAction,
			models.ApplicationSkipped, models.ApplicationExpired, models.ApplicationRejectedByUser)
	case models.ApplicationSubmitting:
		return oneStatusOf(to, models.ApplicationInFlight, models.ApplicationSubmitted, models.ApplicationSubmissionUnconfirmed,
			models.ApplicationRequiresManualAction, models.ApplicationFailedRetryable,
			models.ApplicationFailedPermanent, models.ApplicationDuplicate)
	case models.ApplicationInFlight:
		return oneStatusOf(to, models.ApplicationSubmitted, models.ApplicationSubmissionUnconfirmed,
			models.ApplicationRequiresManualAction, models.ApplicationFailedRetryable,
			models.ApplicationFailedPermanent, models.ApplicationDuplicate)
	case models.ApplicationSubmissionUnconfirmed:
		return oneStatusOf(to, models.ApplicationSubmitted, models.ApplicationRequiresManualAction,
			models.ApplicationFailedRetryable, models.ApplicationFailedPermanent, models.ApplicationDuplicate)
	case models.ApplicationFailedRetryable:
		return oneStatusOf(to, models.ApplicationQueued, models.ApplicationFailedPermanent,
			models.ApplicationRequiresManualAction, models.ApplicationExpired)
	default:
		return false
	}
}

func oneStatusOf(value models.ApplicationStatus, choices ...models.ApplicationStatus) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func isApplicationStatus(value models.ApplicationStatus) bool {
	return oneStatusOf(value,
		models.ApplicationDiscovered, models.ApplicationMatched, models.ApplicationPreparing,
		models.ApplicationNeedsUserInput, models.ApplicationReadyForReview, models.ApplicationApproved,
		models.ApplicationQueued, models.ApplicationSubmitting, models.ApplicationInFlight, models.ApplicationSubmitted,
		models.ApplicationSubmissionUnconfirmed, models.ApplicationRequiresManualAction,
		models.ApplicationFailedRetryable, models.ApplicationFailedPermanent, models.ApplicationDuplicate,
		models.ApplicationExpired, models.ApplicationSkipped, models.ApplicationRejectedByUser)
}

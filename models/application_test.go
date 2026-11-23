package models

import "testing"

func TestInternalStatusesAreNotExposedAsPublicStates(t *testing.T) {
	for _, internal := range []ApplicationStatus{ApplicationDiscovered, ApplicationMatched} {
		if _, ok := internal.PublicStatus(); ok {
			t.Errorf("internal state %q unexpectedly maps to a public application state", internal)
		}
	}
	status, ok := ApplicationSubmitted.PublicStatus()
	if !ok || status != PublicApplicationSubmitted {
		t.Fatalf("submitted public status = %q, %t", status, ok)
	}
	if _, ok := ApplicationStatus("unknown").PublicStatus(); ok {
		t.Fatal("unknown internal state must not be exposed")
	}
}

func TestRule8StatusesAndNormalization(t *testing.T) {
	expectedRule8 := []ApplicationStatus{
		StatusDiscovered, StatusEligible, StatusPreparing, StatusReady,
		StatusQueued, StatusSubmitting, StatusSubmittedUnverified, StatusSubmittedConfirmed,
		StatusActionRequired, StatusRejected, StatusFailed, StatusUnknown,
	}

	for _, s := range expectedRule8 {
		if !IsRule8Status(s) {
			t.Errorf("expected %q to be recognized as Rule 8 status", s)
		}
	}

	testCases := []struct {
		input    string
		expected ApplicationStatus
	}{
		{"discovered", StatusDiscovered},
		{"DISCOVERED", StatusDiscovered},
		{"matched", StatusEligible},
		{"eligible", StatusEligible},
		{"preparing", StatusPreparing},
		{"ready_for_review", StatusReady},
		{"approved", StatusReady},
		{"ready", StatusReady},
		{"queued", StatusQueued},
		{"submitting", StatusSubmitting},
		{"in_flight", StatusSubmitting},
		{"submitted_unverified", StatusSubmittedUnverified},
		{"submission_unconfirmed", StatusSubmittedUnverified},
		{"submitted", StatusSubmittedConfirmed},
		{"submitted_confirmed", StatusSubmittedConfirmed},
		{"requires_manual_action", StatusActionRequired},
		{"action_required", StatusActionRequired},
		{"rejected_by_user", StatusRejected},
		{"skipped", StatusRejected},
		{"duplicate", StatusRejected},
		{"expired", StatusRejected},
		{"failed_permanent", StatusFailed},
		{"failed_retryable", StatusFailed},
		{"stuck", StatusFailed},
		{"failed", StatusFailed},
		{"random_garbage", StatusUnknown},
	}

	for _, tc := range testCases {
		res := NormalizeApplicationStatus(tc.input)
		if res != tc.expected {
			t.Errorf("NormalizeApplicationStatus(%q) = %q, expected %q", tc.input, res, tc.expected)
		}
	}
}

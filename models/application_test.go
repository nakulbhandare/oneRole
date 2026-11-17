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

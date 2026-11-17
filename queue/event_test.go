package queue

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validSubmitEvent() Event {
	tenantID := "77777777-7777-4777-8777-777777777777"
	return Event{
		ID: "11111111-1111-4111-8111-111111111111", Type: EventApplicationSubmitRequested,
		SchemaVersion: 1, AggregateType: "application", AggregateID: "22222222-2222-4222-8222-222222222222",
		TenantID: &tenantID, CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		Payload: map[string]string{"application_id": "22222222-2222-4222-8222-222222222222", "actor_user_id": "88888888-8888-4888-8888-888888888888"},
	}
}

func TestEncodeDecodeEventRoundTrip(t *testing.T) {
	want := validSubmitEvent()
	data, err := EncodeEvent(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeEvent(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Type != want.Type || got.Payload["application_id"] != want.Payload["application_id"] || got.Payload["actor_user_id"] != want.Payload["actor_user_id"] {
		t.Fatalf("decoded event differs: %#v", got)
	}
}

func TestEventRejectsSensitivePayloadKeys(t *testing.T) {
	event := validSubmitEvent()
	event.Payload["resume_text"] = "private candidate data"
	if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected invalid event, got %v", err)
	}
}

func TestEventRequiresTenantForApplicationEvents(t *testing.T) {
	event := validSubmitEvent()
	event.TenantID = nil
	if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected invalid event, got %v", err)
	}
}

func TestDecodeEventRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	data, err := EncodeEvent(validSubmitEvent())
	if err != nil {
		t.Fatal(err)
	}
	withUnknown := strings.Replace(string(data), `"event_type":`, `"debug":"secret","event_type":`, 1)
	if _, err := DecodeEvent([]byte(withUnknown)); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
	if _, err := DecodeEvent(append(data, []byte(` {}`)...)); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected trailing JSON rejection, got %v", err)
	}
}

func TestSubjectForEventType(t *testing.T) {
	tests := map[string]string{
		EventApplicationPrepareRequested: SubjectApplicationPrepare,
		EventApplicationBatchRequested:   SubjectApplicationBatch,
		EventApplicationSubmitRequested:  SubjectApplicationSubmit,
	}
	for eventType, want := range tests {
		got, ok := SubjectForEventType(eventType)
		if !ok || got != want {
			t.Errorf("SubjectForEventType(%q) = %q, %t; want %q, true", eventType, got, ok, want)
		}
	}
	if _, ok := SubjectForEventType("unknown"); ok {
		t.Fatal("unknown event type unexpectedly has a subject")
	}
}

func TestGlobalSourceSyncEventAllowsNoTenantAndRequiresRunID(t *testing.T) {
	event := Event{
		ID: "11111111-1111-4111-8111-111111111111", Type: EventSourceSyncRequested,
		SchemaVersion: 1, AggregateType: "source_sync_run", AggregateID: "99999999-9999-4999-8999-999999999999",
		CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		Payload: map[string]string{
			"source_id":   "22222222-2222-4222-8222-222222222222",
			"sync_run_id": "99999999-9999-4999-8999-999999999999",
		},
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	delete(event.Payload, "sync_run_id")
	if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected missing sync run ID to fail, got %v", err)
	}
}

func TestEventAggregateMustMatchItsPayloadID(t *testing.T) {
	event := validSubmitEvent()
	event.AggregateID = "33333333-3333-4333-8333-333333333333"
	if err := event.Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("mismatched aggregate ID should be rejected, got %v", err)
	}
}

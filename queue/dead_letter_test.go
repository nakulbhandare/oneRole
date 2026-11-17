package queue

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestNewDeadLetterKeepsIdentifiersAndSanitizesClass(t *testing.T) {
	event := validSubmitEvent()
	raw, err := EncodeEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	first := NewDeadLetter(event, raw, SubjectApplicationSubmit, "HTTP_503", 4, time.Now().UTC())
	second := NewDeadLetter(event, raw, SubjectApplicationSubmit, "HTTP_503", 7, time.Now().UTC())
	if first.ID != second.ID {
		t.Fatalf("dead-letter ID must be stable across DLQ publish retries: %q != %q", first.ID, second.ID)
	}
	if first.ErrorClass != "HTTP_503" || first.EventID != event.ID || first.AggregateID != event.AggregateID || first.TenantID == nil {
		t.Fatalf("dead-letter fields incomplete: %+v", first)
	}
	if got := safeErrorClass("bearer token: super-secret-value"); got != "UNCLASSIFIED" {
		t.Fatalf("raw error content leaked into class %q", got)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "actor_user_id") || strings.Contains(string(encoded), "Payload") || strings.Contains(string(encoded), "super-secret") {
		t.Fatalf("dead letter contains input payload or sensitive content: %s", encoded)
	}
}

func TestDeadLetterPublisherUsesWorkloadSpecificSubject(t *testing.T) {
	client := &fakeSyncPublisher{ack: &jetstream.PubAck{Stream: "APPLICATION", Sequence: 2}}
	publisher, err := NewDeadLetterPublisher(client)
	if err != nil {
		t.Fatal(err)
	}
	record := DeadLetter{ID: "dlq-test", Subject: SubjectApplicationSubmit, ErrorClass: "PERMANENT_FAILURE", Attempts: 1, FailedAt: time.Now().UTC()}
	if err := publisher.Persist(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if client.subject != "application.dead_letter" {
		t.Fatalf("subject = %q", client.subject)
	}
	if _, err := deadLetterSubject("untrusted.subject"); err == nil {
		t.Fatal("unknown original subject should not be routed to a guessed DLQ")
	}
}

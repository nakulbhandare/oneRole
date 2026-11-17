package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// DeadLetter contains identifiers and a sanitized failure class only. It never
// stores the original message body or the raw error string.
type DeadLetter struct {
	ID            string    `json:"dead_letter_id"`
	EventID       string    `json:"event_id,omitempty"`
	EventType     string    `json:"event_type,omitempty"`
	Subject       string    `json:"subject"`
	AggregateType string    `json:"aggregate_type,omitempty"`
	AggregateID   string    `json:"aggregate_id,omitempty"`
	TenantID      *string   `json:"tenant_id,omitempty"`
	ErrorClass    string    `json:"error_class"`
	Attempts      uint64    `json:"attempts"`
	FailedAt      time.Time `json:"failed_at"`
}

var safeErrorClassPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_.-]{0,63}$`)

type DeadLetterPublisher struct {
	client  SyncPublisher
	timeout time.Duration
}

func NewDeadLetterPublisher(client SyncPublisher) (*DeadLetterPublisher, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: dead-letter publisher is required", ErrInvalidJetStreamConfig)
	}
	return &DeadLetterPublisher{client: client, timeout: 10 * time.Second}, nil
}

func (p *DeadLetterPublisher) Persist(ctx context.Context, record DeadLetter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.ID == "" || record.ErrorClass == "" || record.Attempts == 0 || record.FailedAt.IsZero() || record.FailedAt.Location() != time.UTC || safeErrorClass(record.ErrorClass) != record.ErrorClass {
		return fmt.Errorf("invalid dead-letter record")
	}
	subject, err := deadLetterSubject(record.Subject)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode dead-letter record: %w", err)
	}
	if len(payload) > MaxEventBytes {
		return fmt.Errorf("dead-letter record too large")
	}
	publishCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	ack, err := p.client.Publish(publishCtx, subject, payload, jetstream.WithMsgID(record.ID))
	if err != nil {
		return fmt.Errorf("persist dead-letter record: %w", err)
	}
	if ack == nil || ack.Stream == "" || ack.Sequence == 0 {
		return fmt.Errorf("persist dead-letter record: missing JetStream acknowledgement")
	}
	return nil
}

// NewDeadLetter creates a retry-stable record for a particular delivery
// attempt. Invalid messages are represented by a content hash instead of
// copying untrusted bytes into the dead-letter stream.
func NewDeadLetter(event Event, raw []byte, subject, errorClass string, attempts uint64, now time.Time) DeadLetter {
	if attempts == 0 {
		attempts = 1
	}
	digest := sha256.Sum256(raw)
	dedupeKey := hex.EncodeToString(digest[:16])
	if isUUID(event.ID) {
		dedupeKey = strings.ReplaceAll(strings.ToLower(event.ID), "-", "")
	}
	recordID := fmt.Sprintf("dlq-%s", dedupeKey)
	record := DeadLetter{
		ID: recordID, Subject: subject, ErrorClass: safeErrorClass(errorClass),
		Attempts: attempts, FailedAt: now.UTC(),
	}
	if isUUID(event.ID) {
		record.EventID = event.ID
	}
	if _, ok := eventContracts[event.Type]; ok {
		record.EventType = event.Type
	}
	if _, ok := map[string]bool{"application": true, "application_batch": true, "job_source": true, "source_sync_run": true, "job_observation": true, "job": true, "maintenance_task": true}[event.AggregateType]; ok {
		record.AggregateType = event.AggregateType
	}
	if isUUID(event.AggregateID) {
		record.AggregateID = event.AggregateID
	}
	if event.TenantID != nil && isUUID(*event.TenantID) {
		tenantID := *event.TenantID
		record.TenantID = &tenantID
	}
	return record
}

func deadLetterSubject(sourceSubject string) (string, error) {
	switch {
	case strings.HasPrefix(sourceSubject, "application."):
		return "application.dead_letter", nil
	case strings.HasPrefix(sourceSubject, "discovery."):
		return "discovery.dead_letter", nil
	case strings.HasPrefix(sourceSubject, "maintenance."):
		return "maintenance.dead_letter", nil
	default:
		return "", fmt.Errorf("unknown dead-letter source subject")
	}
}

func safeErrorClass(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if !safeErrorClassPattern.MatchString(value) {
		return "UNCLASSIFIED"
	}
	return value
}

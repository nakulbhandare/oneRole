package queue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

const MaxEventBytes = 64 << 10

var (
	ErrInvalidEvent    = errors.New("invalid queue event")
	traceparentPattern = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)
)

// Event types are stable, versioned outbox values. JetStream subjects are
// deliberately separate and are derived by SubjectForEventType.
const (
	EventApplicationPrepareRequested   = "application.packet.prepare.requested"
	EventApplicationBatchRequested     = "application.batch.fanout.requested"
	EventApplicationSubmitRequested    = "application.submit.requested"
	EventApplicationReconcileRequested = "application.reconcile.requested"
	EventSourceSyncRequested           = "discovery.source.sync.requested"
	EventJobNormalizeRequested         = "discovery.job.normalize.requested"
	EventJobIndexRequested             = "discovery.job.index.requested"
	EventMaintenanceRequested          = "maintenance.execute.requested"
)

const (
	SubjectApplicationPrepare   = "application.packet.prepare"
	SubjectApplicationBatch     = "application.batch.fanout"
	SubjectApplicationSubmit    = "application.submit"
	SubjectApplicationReconcile = "application.reconcile"
	SubjectSourceSync           = "discovery.source.sync"
	SubjectJobNormalize         = "discovery.job.normalize"
	SubjectJobIndex             = "discovery.job.index"
	SubjectMaintenance          = "maintenance.execute"
)

// Event contains only identity and routing metadata. Payload values are
// validated against the small per-event allowlist below, preventing accidental
// inclusion of resumes, answer text, credentials, or provider responses.
type Event struct {
	ID            string            `json:"event_id"`
	Type          string            `json:"event_type"`
	SchemaVersion int               `json:"schema_version"`
	AggregateType string            `json:"aggregate_type"`
	AggregateID   string            `json:"aggregate_id"`
	TenantID      *string           `json:"tenant_id"`
	CreatedAt     time.Time         `json:"created_at"`
	Traceparent   string            `json:"traceparent,omitempty"`
	Payload       map[string]string `json:"payload_ids_only"`
}

type eventContract struct {
	subject             string
	aggregateType       string
	aggregatePayloadKey string
	payloadKeys         []string
	global              bool
}

var eventContracts = map[string]eventContract{
	EventApplicationPrepareRequested:   {SubjectApplicationPrepare, "application", "application_id", []string{"application_id", "actor_user_id"}, false},
	EventApplicationBatchRequested:     {SubjectApplicationBatch, "application_batch", "batch_id", []string{"batch_id", "actor_user_id"}, false},
	EventApplicationSubmitRequested:    {SubjectApplicationSubmit, "application", "application_id", []string{"application_id", "actor_user_id"}, false},
	EventApplicationReconcileRequested: {SubjectApplicationReconcile, "application", "application_id", []string{"application_id", "actor_user_id"}, false},
	EventSourceSyncRequested:           {SubjectSourceSync, "source_sync_run", "sync_run_id", []string{"source_id", "sync_run_id"}, true},
	EventJobNormalizeRequested:         {SubjectJobNormalize, "job_observation", "observation_id", []string{"observation_id"}, true},
	EventJobIndexRequested:             {SubjectJobIndex, "job", "job_id", []string{"job_id"}, true},
	EventMaintenanceRequested:          {SubjectMaintenance, "maintenance_task", "task_id", []string{"task_id"}, true},
}

func SubjectForEventType(eventType string) (string, bool) {
	contract, ok := eventContracts[eventType]
	return contract.subject, ok
}

func (e Event) Validate() error {
	contract, ok := eventContracts[e.Type]
	if !ok {
		return fmt.Errorf("%w: unsupported event type %q", ErrInvalidEvent, e.Type)
	}
	if !isUUID(e.ID) || !isUUID(e.AggregateID) {
		return fmt.Errorf("%w: event_id and aggregate_id must be UUIDs", ErrInvalidEvent)
	}
	if e.SchemaVersion != 1 {
		return fmt.Errorf("%w: unsupported schema_version %d", ErrInvalidEvent, e.SchemaVersion)
	}
	if e.AggregateType != contract.aggregateType {
		return fmt.Errorf("%w: event_type %q requires aggregate_type %q", ErrInvalidEvent, e.Type, contract.aggregateType)
	}
	if e.TenantID != nil && !isUUID(*e.TenantID) {
		return fmt.Errorf("%w: tenant_id must be a UUID", ErrInvalidEvent)
	}
	if !contract.global && e.TenantID == nil {
		return fmt.Errorf("%w: tenant_id is required for %q", ErrInvalidEvent, e.Type)
	}
	if e.CreatedAt.IsZero() || e.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("%w: created_at must be a non-zero UTC timestamp", ErrInvalidEvent)
	}
	if e.Traceparent != "" && (!traceparentPattern.MatchString(e.Traceparent) || allZeroTraceparent(e.Traceparent)) {
		return fmt.Errorf("%w: traceparent is malformed", ErrInvalidEvent)
	}
	if len(e.Payload) != len(contract.payloadKeys) {
		return fmt.Errorf("%w: payload has unexpected or missing identifier fields", ErrInvalidEvent)
	}
	for _, key := range contract.payloadKeys {
		value, ok := e.Payload[key]
		if !ok || !isUUID(value) {
			return fmt.Errorf("%w: payload.%s must be a UUID", ErrInvalidEvent, key)
		}
	}
	if e.Payload[contract.aggregatePayloadKey] != e.AggregateID {
		return fmt.Errorf("%w: aggregate_id does not match payload.%s", ErrInvalidEvent, contract.aggregatePayloadKey)
	}
	return nil
}

func EncodeEvent(e Event) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode queue event: %w", err)
	}
	if len(data) > MaxEventBytes {
		return nil, fmt.Errorf("%w: encoded event exceeds %d bytes", ErrInvalidEvent, MaxEventBytes)
	}
	return data, nil
}

func DecodeEvent(data []byte) (Event, error) {
	var event Event
	if len(data) == 0 || len(data) > MaxEventBytes {
		return event, fmt.Errorf("%w: event size must be between 1 and %d bytes", ErrInvalidEvent, MaxEventBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, fmt.Errorf("%w: decode: %v", ErrInvalidEvent, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return event, fmt.Errorf("%w: trailing JSON content", ErrInvalidEvent)
	}
	if err := event.Validate(); err != nil {
		return event, err
	}
	return event, nil
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

func allZeroTraceparent(value string) bool {
	parts := strings.Split(value, "-")
	return parts[1] == strings.Repeat("0", 32) || parts[2] == strings.Repeat("0", 16)
}

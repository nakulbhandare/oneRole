package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

var ErrInvalidJetStreamConfig = errors.New("invalid JetStream configuration")

const (
	StreamApplications = "APPLICATIONS"
	StreamDiscovery    = "DISCOVERY"
	StreamMaintenance  = "MAINTENANCE"

	// StreamApplication retained for backward compatibility
	StreamApplication = StreamApplications

	// MaxTaskMessageBytes allocates 1MB for rich Chromium task payloads with HTML resumes
	MaxTaskMessageBytes = 1 << 20
)

const (
	SubjectApplicationsWildcard = "applications.>"
	SubjectApplicationsSubmit   = "applications.submit"
)

const maxDeadLetterDeliveryRetries = 3

type EventPublisher interface {
	Publish(context.Context, Event) error
}

// SyncPublisher is the synchronous subset needed by the outbox publisher.
// Publish returns only after JetStream has acknowledged durable receipt.
type SyncPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

type JetStreamPublisher struct {
	client SyncPublisher
}

func NewJetStreamPublisher(client SyncPublisher) (*JetStreamPublisher, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: publisher is required", ErrInvalidJetStreamConfig)
	}
	return &JetStreamPublisher{client: client}, nil
}

// Publish serializes a validated event and uses the immutable outbox ID as
// NATS' deduplication key. The caller must mark the outbox row published only
// after this method returns nil.
func (p *JetStreamPublisher) Publish(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	subject, ok := SubjectForEventType(event.Type)
	if !ok {
		return fmt.Errorf("%w: no subject for event type %q", ErrInvalidEvent, event.Type)
	}
	payload, err := EncodeEvent(event)
	if err != nil {
		return err
	}
	ack, err := p.client.Publish(ctx, subject, payload, jetstream.WithMsgID(event.ID))
	if err != nil {
		return fmt.Errorf("publish event %s to %s: %w", event.ID, subject, err)
	}
	if ack == nil || ack.Stream == "" || ack.Sequence == 0 {
		return fmt.Errorf("publish event %s to %s: missing JetStream acknowledgement", event.ID, subject)
	}
	return nil
}

// StreamConfigurations returns bounded, file-backed stream defaults. Production
// deployments may override retention/capacity in infrastructure configuration;
// they should not switch durable workflow streams to memory storage.
func StreamConfigurations(replicas int) ([]jetstream.StreamConfig, error) {
	if replicas < 1 || replicas > 5 {
		return nil, fmt.Errorf("%w: replicas must be 1..5", ErrInvalidJetStreamConfig)
	}
	base := func(name string, subjects []string, maxAge time.Duration, maxMsgSize int32) jetstream.StreamConfig {
		return jetstream.StreamConfig{
			Name: name, Subjects: subjects,
			Retention: jetstream.LimitsPolicy, Storage: jetstream.FileStorage,
			Discard: jetstream.DiscardOld, Replicas: replicas,
			MaxAge: maxAge, MaxMsgs: 10_000_000, MaxBytes: 20 << 30,
			MaxMsgSize: maxMsgSize, Duplicates: 10 * time.Minute,
			DenyDelete: true, DenyPurge: true,
		}
	}
	return []jetstream.StreamConfig{
		base(StreamDiscovery, []string{"discovery.>"}, 7*24*time.Hour, MaxEventBytes),
		base(StreamApplications, []string{"applications.>"}, 14*24*time.Hour, MaxTaskMessageBytes),
		base(StreamMaintenance, []string{"maintenance.>"}, 7*24*time.Hour, MaxEventBytes),
	}, nil
}

type StreamProvisioner interface {
	CreateOrUpdateStream(context.Context, jetstream.StreamConfig) (jetstream.Stream, error)
}

// EnsureStreams is an explicit infrastructure/bootstrap operation. A
// production worker identity can instead be granted access to pre-provisioned
// streams and omit create/update permissions.
func EnsureStreams(ctx context.Context, provisioner StreamProvisioner, replicas int) error {
	if provisioner == nil {
		return fmt.Errorf("%w: stream provisioner is required", ErrInvalidJetStreamConfig)
	}
	configs, err := StreamConfigurations(replicas)
	if err != nil {
		return err
	}
	for _, config := range configs {
		if _, err := provisioner.CreateOrUpdateStream(ctx, config); err != nil {
			return fmt.Errorf("ensure stream %s: %w", config.Name, err)
		}
	}
	return nil
}

// DurableConsumerConfig is the configuration shared by workload-specific
// consumers. FilterSubject must be an exact, documented JetStream subject.
func DurableConsumerConfig(name, filterSubject string, maxAttempts, maxPending int, ackWait time.Duration) (jetstream.ConsumerConfig, error) {
	if name == "" || filterSubject == "" || maxAttempts < 1 || maxAttempts > 1000 || maxPending < 1 || ackWait <= 0 {
		return jetstream.ConsumerConfig{}, fmt.Errorf("%w: consumer name, subject, attempts, pending count, and ack wait are required", ErrInvalidJetStreamConfig)
	}
	return jetstream.ConsumerConfig{
		Name: name, Durable: name, FilterSubject: filterSubject,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		// Reserve a small broker-delivery tail beyond the application retry
		// budget so transient DLQ persistence failures can recover. The worker
		// itself still stops safe task retries at maxAttempts.
		MaxDeliver:    maxAttempts + maxDeadLetterDeliveryRetries,
		MaxAckPending: maxPending, MaxRequestBatch: maxPending,
		ReplayPolicy: jetstream.ReplayInstantPolicy,
	}, nil
}

type ConsumerProvisioner interface {
	CreateOrUpdateConsumer(context.Context, string, jetstream.ConsumerConfig) (jetstream.Consumer, error)
}

type ConsumerLookup interface {
	Consumer(context.Context, string, string) (jetstream.Consumer, error)
}

func FindConsumer(ctx context.Context, lookup ConsumerLookup, stream, name string) (jetstream.Consumer, error) {
	if lookup == nil || stream == "" || name == "" {
		return nil, fmt.Errorf("%w: consumer lookup, stream, and durable name are required", ErrInvalidJetStreamConfig)
	}
	consumer, err := lookup.Consumer(ctx, stream, name)
	if err != nil {
		return nil, fmt.Errorf("find pre-provisioned consumer %s on %s: %w", name, stream, err)
	}
	if consumer == nil {
		return nil, fmt.Errorf("find pre-provisioned consumer %s on %s: broker returned no consumer", name, stream)
	}
	return consumer, nil
}

func EnsureConsumer(ctx context.Context, provisioner ConsumerProvisioner, stream string, config jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	if provisioner == nil || stream == "" || config.Durable == "" || config.FilterSubject == "" {
		return nil, fmt.Errorf("%w: consumer provisioner, stream, durable name, and filter are required", ErrInvalidJetStreamConfig)
	}
	consumer, err := provisioner.CreateOrUpdateConsumer(ctx, stream, config)
	if err != nil {
		return nil, fmt.Errorf("ensure consumer %s on %s: %w", config.Durable, stream, err)
	}
	if consumer == nil {
		return nil, fmt.Errorf("ensure consumer %s on %s: broker returned no consumer", config.Durable, stream)
	}
	return consumer, nil
}

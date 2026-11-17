package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type fakeSyncPublisher struct {
	subject string
	payload []byte
	opts    []jetstream.PublishOpt
	ack     *jetstream.PubAck
	err     error
}

func (p *fakeSyncPublisher) Publish(_ context.Context, subject string, payload []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	p.subject = subject
	p.payload = append([]byte(nil), payload...)
	p.opts = opts
	return p.ack, p.err
}

func TestJetStreamPublisherUsesSubjectAndDurableAck(t *testing.T) {
	client := &fakeSyncPublisher{ack: &jetstream.PubAck{Stream: "APPLICATION", Sequence: 9}}
	publisher, err := NewJetStreamPublisher(client)
	if err != nil {
		t.Fatal(err)
	}
	event := validSubmitEvent()
	if err := publisher.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if client.subject != SubjectApplicationSubmit {
		t.Fatalf("subject = %q", client.subject)
	}
	if len(client.opts) != 1 {
		t.Fatalf("expected one dedupe publish option, got %d", len(client.opts))
	}
	decoded, err := DecodeEvent(client.payload)
	if err != nil || decoded.ID != event.ID {
		t.Fatalf("published payload invalid: event=%+v err=%v", decoded, err)
	}
}

func TestJetStreamPublisherRequiresServerAck(t *testing.T) {
	client := &fakeSyncPublisher{}
	publisher, err := NewJetStreamPublisher(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Publish(context.Background(), validSubmitEvent()); err == nil {
		t.Fatal("expected nil-ack error")
	}
	client.ack = &jetstream.PubAck{Stream: "APPLICATION", Sequence: 1}
	client.err = errors.New("server unavailable")
	if err := publisher.Publish(context.Background(), validSubmitEvent()); err == nil {
		t.Fatal("expected publish error")
	}
}

func TestStreamConfigurationsAreDurableAndBounded(t *testing.T) {
	configs, err := StreamConfigurations(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 3 {
		t.Fatalf("got %d stream configs", len(configs))
	}
	for _, config := range configs {
		if config.Storage != jetstream.FileStorage || config.Replicas != 3 || config.MaxAge <= 0 || config.MaxMsgs <= 0 || config.MaxBytes <= 0 {
			t.Errorf("stream %s is not durably bounded: %+v", config.Name, config)
		}
		if !config.DenyDelete || !config.DenyPurge {
			t.Errorf("stream %s should protect stored history from delete/purge", config.Name)
		}
	}
	if _, err := StreamConfigurations(6); err == nil {
		t.Fatal("expected invalid replica count")
	}
}

func TestDurableConsumerConfigBoundsDelivery(t *testing.T) {
	config, err := DurableConsumerConfig("application-submit", SubjectApplicationSubmit, 8, 32, 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if config.AckPolicy != jetstream.AckExplicitPolicy || config.MaxDeliver != 8+maxDeadLetterDeliveryRetries || config.MaxAckPending != 32 || config.AckWait != 90*time.Second {
		t.Fatalf("unexpected consumer config: %+v", config)
	}
	if _, err := DurableConsumerConfig("", SubjectApplicationSubmit, 8, 32, time.Minute); err == nil {
		t.Fatal("expected invalid consumer config")
	}
}

type fakeConsumerLookup struct {
	stream string
	name   string
	err    error
}

func (lookup *fakeConsumerLookup) Consumer(_ context.Context, stream, name string) (jetstream.Consumer, error) {
	lookup.stream, lookup.name = stream, name
	return nil, lookup.err
}

func TestFindConsumerLooksUpExistingDurableAndFailsClosed(t *testing.T) {
	lookup := &fakeConsumerLookup{err: errors.New("consumer not found")}
	_, err := FindConsumer(context.Background(), lookup, "APPLICATION", "application-submit-v1")
	if err == nil || lookup.stream != "APPLICATION" || lookup.name != "application-submit-v1" {
		t.Fatalf("existing consumer lookup did not fail safely: lookup=%+v err=%v", lookup, err)
	}
	if _, err := FindConsumer(context.Background(), lookup, "APPLICATION", ""); err == nil {
		t.Fatal("empty durable name should be rejected")
	}
}

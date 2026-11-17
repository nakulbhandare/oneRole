package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
)

type fakeOutboxStore struct {
	events      []repository.OutboxEvent
	claimErr    error
	markErr     error
	failureErr  error
	claimed     int
	marked      int
	failed      int
	lastClass   string
	lastRetryAt time.Time
	callOrder   []string
}

func (s *fakeOutboxStore) ClaimPending(_ context.Context, _ string, _ int, _, _ time.Time) ([]repository.OutboxEvent, error) {
	s.claimed++
	s.callOrder = append(s.callOrder, "claim")
	return s.events, s.claimErr
}
func (s *fakeOutboxStore) MarkPublished(_ context.Context, _, _ string, _ time.Time) error {
	s.marked++
	s.callOrder = append(s.callOrder, "mark")
	return s.markErr
}
func (s *fakeOutboxStore) RecordPublishFailure(_ context.Context, _, _, class string, retryAt time.Time) error {
	s.failed++
	s.lastClass, s.lastRetryAt = class, retryAt
	s.callOrder = append(s.callOrder, "failure")
	return s.failureErr
}

type fakeEventPublisher struct {
	events []queue.Event
	err    error
}

func (p *fakeEventPublisher) Publish(_ context.Context, event queue.Event) error {
	p.events = append(p.events, event)
	return p.err
}

func validOutboxRow() repository.OutboxEvent {
	tenantID := "77777777-7777-4777-8777-777777777777"
	return repository.OutboxEvent{
		ID: "11111111-1111-4111-8111-111111111111", EventType: queue.EventApplicationSubmitRequested,
		SchemaVersion: 1, AggregateType: "application", AggregateID: "22222222-2222-4222-8222-222222222222",
		TenantID: &tenantID, PayloadIDsOnly: json.RawMessage(`{"application_id":"22222222-2222-4222-8222-222222222222","actor_user_id":"88888888-8888-4888-8888-888888888888"}`),
		CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC), ClaimToken: "99999999-9999-4999-8999-999999999999", PublishAttempts: 1,
	}
}

func validOutboxPublisher(store repository.OutboxStore, publisher queue.EventPublisher) OutboxPublisher {
	return OutboxPublisher{Store: store, Publisher: publisher, Owner: "worker-1", BatchSize: 10, Concurrency: 2,
		LeaseDuration: time.Minute, PublishTimeout: 10 * time.Second, PollInterval: time.Second, FailureDelay: time.Second, MaxFailureDelay: time.Minute,
		Now: func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }}
}

func TestOutboxPublisherMarksOnlyAfterSynchronousPublish(t *testing.T) {
	store := &fakeOutboxStore{events: []repository.OutboxEvent{validOutboxRow()}}
	publisher := &fakeEventPublisher{}
	sender := validOutboxPublisher(store, publisher)
	published, failed, err := sender.PublishOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 || failed != 0 || len(publisher.events) != 1 || store.marked != 1 || store.failed != 0 {
		t.Fatalf("outbox flow incomplete: published=%d failed=%d publisher=%+v store=%+v", published, failed, publisher, store)
	}
	if len(store.callOrder) != 2 || store.callOrder[0] != "claim" || store.callOrder[1] != "mark" {
		t.Fatalf("wrong durability order: %v", store.callOrder)
	}
	if publisher.events[0].Type != queue.EventApplicationSubmitRequested || publisher.events[0].Payload["actor_user_id"] != "88888888-8888-4888-8888-888888888888" {
		t.Fatalf("outbox envelope was not mapped: %+v", publisher.events[0])
	}
}

func TestOutboxPublishFailureDoesNotMarkPublished(t *testing.T) {
	store := &fakeOutboxStore{events: []repository.OutboxEvent{validOutboxRow()}}
	publisher := &fakeEventPublisher{err: errors.New("network error with private endpoint")}
	sender := validOutboxPublisher(store, publisher)
	published, failed, err := sender.PublishOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if published != 0 || failed != 1 || store.marked != 0 || store.failed != 1 || store.lastClass != "JETSTREAM_PUBLISH_FAILED" {
		t.Fatalf("failed publish was mishandled: published=%d failed=%d store=%+v", published, failed, store)
	}
	if store.lastRetryAt.Before(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Add(500*time.Millisecond)) || store.lastRetryAt.After(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Add(time.Second)) {
		t.Fatalf("outbox retry delay not bounded: %s", store.lastRetryAt)
	}
}

func TestOutboxPublisherRejectsSensitiveOrMalformedPayload(t *testing.T) {
	row := validOutboxRow()
	row.PayloadIDsOnly = json.RawMessage(`{"application_id":"22222222-2222-4222-8222-222222222222","actor_user_id":"88888888-8888-4888-8888-888888888888","resume_text":"secret"}`)
	store := &fakeOutboxStore{events: []repository.OutboxEvent{row}}
	publisher := &fakeEventPublisher{}
	published, failed, err := validOutboxPublisher(store, publisher).PublishOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if published != 0 || failed != 1 || store.lastClass != "INVALID_OUTBOX_EVENT" || len(publisher.events) != 0 {
		t.Fatalf("invalid payload escaped validation: published=%d failed=%d store=%+v", published, failed, publisher)
	}
}

func TestOutboxPublisherRetriesMarkFailureWithoutClaimingSuccess(t *testing.T) {
	store := &fakeOutboxStore{events: []repository.OutboxEvent{validOutboxRow()}, markErr: errors.New("lost DB connection")}
	publisher := &fakeEventPublisher{}
	published, failed, err := validOutboxPublisher(store, publisher).PublishOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if published != 0 || failed != 1 || len(publisher.events) != 1 || store.failed != 1 || store.lastClass != "OUTBOX_ACK_FAILED" {
		t.Fatalf("ack failure should leave event retryable: published=%d failed=%d store=%+v", published, failed, store)
	}
}

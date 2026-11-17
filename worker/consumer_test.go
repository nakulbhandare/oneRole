package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
)

type fakeAckMessage struct {
	data        []byte
	subject     string
	attempt     uint64
	metadataErr error
	acked       int
	nacked      time.Duration
	termed      string
}

func (m *fakeAckMessage) Data() []byte    { return m.data }
func (m *fakeAckMessage) Subject() string { return m.subject }
func (m *fakeAckMessage) Metadata() (*jetstream.MsgMetadata, error) {
	if m.metadataErr != nil {
		return nil, m.metadataErr
	}
	return &jetstream.MsgMetadata{NumDelivered: m.attempt}, nil
}
func (m *fakeAckMessage) DoubleAck(context.Context) error        { m.acked++; return nil }
func (m *fakeAckMessage) NakWithDelay(delay time.Duration) error { m.nacked = delay; return nil }
func (m *fakeAckMessage) TermWithReason(reason string) error     { m.termed = reason; return nil }

type fakeInbox struct {
	claimResult repository.InboxClaimResult
	claim       repository.InboxClaim
	claimErr    error
	completeErr error
	releaseErr  error
	completed   int
	released    int
	retryAt     time.Time
	errorClass  string
}

func (s *fakeInbox) Claim(_ context.Context, consumer, eventID, owner string, _ time.Time, until time.Time) (repository.InboxClaim, repository.InboxClaimResult, error) {
	if s.claimErr != nil {
		return repository.InboxClaim{}, 0, s.claimErr
	}
	claim := s.claim
	if claim.ConsumerName == "" {
		claim = repository.InboxClaim{ConsumerName: consumer, EventID: eventID, Owner: owner, Token: "fenced-token", LeaseUntil: until}
	}
	return claim, s.claimResult, nil
}
func (s *fakeInbox) Complete(context.Context, repository.InboxClaim, time.Time, *string) error {
	s.completed++
	return s.completeErr
}
func (s *fakeInbox) Release(_ context.Context, _ repository.InboxClaim, retryAt time.Time, class string) error {
	s.released++
	s.retryAt = retryAt
	s.errorClass = class
	return s.releaseErr
}

type fakeDeadLetterSink struct {
	record queue.DeadLetter
	calls  int
	err    error
}

func (s *fakeDeadLetterSink) Persist(_ context.Context, record queue.DeadLetter) error {
	s.calls++
	s.record = record
	return s.err
}

type fakeEventHandler struct {
	calls int
	err   error
}

func (h *fakeEventHandler) HandleEvent(context.Context, queue.Event) error { h.calls++; return h.err }

func validDelivery(t *testing.T) (*fakeAckMessage, queue.Event) {
	t.Helper()
	event := queue.Event{
		ID: "11111111-1111-4111-8111-111111111111", Type: queue.EventApplicationSubmitRequested,
		SchemaVersion: 1, AggregateType: "application", AggregateID: "22222222-2222-4222-8222-222222222222",
		Payload:  map[string]string{"application_id": "22222222-2222-4222-8222-222222222222", "actor_user_id": "88888888-8888-4888-8888-888888888888"},
		TenantID: stringPtr("77777777-7777-4777-8777-777777777777"), CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
	}
	data, err := queue.EncodeEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeAckMessage{data: data, subject: queue.SubjectApplicationSubmit, attempt: 1}, event
}

func stringPtr(value string) *string { return &value }

func TestHandleDeliveryCompletesInboxBeforeAck(t *testing.T) {
	message, _ := validDelivery(t)
	inbox := &fakeInbox{claimResult: repository.InboxClaimAcquired}
	handler := &fakeEventHandler{}
	dead := &fakeDeadLetterSink{}
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}
	err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, policy, 20*time.Minute, func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	if handler.calls != 1 || inbox.completed != 1 || message.acked != 1 || dead.calls != 0 {
		t.Fatalf("unexpected completion: calls=%d inbox_complete=%d ack=%d dlq=%d", handler.calls, inbox.completed, message.acked, dead.calls)
	}
}

func TestHandleDeliveryAcksOnlyProcessedDuplicate(t *testing.T) {
	message, _ := validDelivery(t)
	inbox := &fakeInbox{claimResult: repository.InboxAlreadyProcessed}
	handler := &fakeEventHandler{}
	dead := &fakeDeadLetterSink{}
	if err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, DefaultRetryPolicy(), 20*time.Minute, time.Now); err != nil {
		t.Fatal(err)
	}
	if message.acked != 1 || handler.calls != 0 || inbox.completed != 0 {
		t.Fatalf("processed duplicate was not safely acked: %+v", message)
	}
}

func TestHandleDeliveryDefersActiveInboxLease(t *testing.T) {
	message, _ := validDelivery(t)
	inbox := &fakeInbox{claimResult: repository.InboxLeaseHeld}
	handler := &fakeEventHandler{}
	dead := &fakeDeadLetterSink{}
	if err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}, 20*time.Minute, time.Now); err != nil {
		t.Fatal(err)
	}
	if message.nacked != time.Second || handler.calls != 0 || message.acked != 0 {
		t.Fatalf("active lease should be deferred: %+v", message)
	}
}

func TestHandleDeliveryRetriesOnlyExplicitSafeFailure(t *testing.T) {
	message, _ := validDelivery(t)
	inbox := &fakeInbox{claimResult: repository.InboxClaimAcquired}
	handler := &fakeEventHandler{err: &WorkFailure{Class: "SAFE_TEMPORARY", Retryable: true, SafeToRetry: true, RetryAfter: 3 * time.Second}}
	dead := &fakeDeadLetterSink{}
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}
	if err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, policy, 20*time.Minute, time.Now); err != nil {
		t.Fatal(err)
	}
	if message.nacked != 3*time.Second || inbox.released != 1 || inbox.errorClass != "SAFE_TEMPORARY" || dead.calls != 0 || inbox.completed != 0 {
		t.Fatalf("safe retry mismatch: message=%+v inbox=%+v", message, inbox)
	}
}

func TestHandleDeliveryDoesNotRetryUnsafeFailureAndPersistsDLQ(t *testing.T) {
	message, _ := validDelivery(t)
	inbox := &fakeInbox{claimResult: repository.InboxClaimAcquired}
	handler := &fakeEventHandler{err: &WorkFailure{Class: "AMBIGUOUS_SUBMIT", Retryable: true, SafeToRetry: false, Cause: errors.New("must not leak")}}
	dead := &fakeDeadLetterSink{}
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}
	if err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, policy, 20*time.Minute, time.Now); err != nil {
		t.Fatal(err)
	}
	if dead.calls != 1 || message.termed != "AMBIGUOUS_SUBMIT" || inbox.completed != 1 || message.nacked != 0 {
		t.Fatalf("unsafe failure was retried: message=%+v inbox=%+v dlq=%+v", message, inbox, dead)
	}
	if dead.record.ErrorClass != "AMBIGUOUS_SUBMIT" {
		t.Fatalf("wrong DLQ class %q", dead.record.ErrorClass)
	}
}

func TestHandleDeliveryNaksIfDLQCannotPersist(t *testing.T) {
	message, _ := validDelivery(t)
	inbox := &fakeInbox{claimResult: repository.InboxClaimAcquired}
	handler := &fakeEventHandler{err: &WorkFailure{Class: "PERMANENT_FAILURE"}}
	dead := &fakeDeadLetterSink{err: errors.New("broker unavailable")}
	if err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, RetryPolicy{MaxAttempts: 1, BaseDelay: time.Second, MaxDelay: time.Minute}, 20*time.Minute, time.Now); err != nil {
		t.Fatal(err)
	}
	if message.nacked != time.Second || inbox.released != 1 || inbox.completed != 0 || message.termed != "" {
		t.Fatalf("failed DLQ publish must remain recoverable: message=%+v inbox=%+v", message, inbox)
	}
}

func TestHandleDeliveryDLQsAtSafeRetryBudget(t *testing.T) {
	message, _ := validDelivery(t)
	message.attempt = 3
	inbox := &fakeInbox{claimResult: repository.InboxClaimAcquired}
	handler := &fakeEventHandler{err: &WorkFailure{Class: "SAFE_TEMPORARY", Retryable: true, SafeToRetry: true}}
	dead := &fakeDeadLetterSink{}
	if err := HandleDelivery(context.Background(), message, inbox, "submit-consumer", "worker-1", handler, dead, RetryPolicy{MaxAttempts: 3, BaseDelay: time.Second, MaxDelay: time.Minute}, 20*time.Minute, time.Now); err != nil {
		t.Fatal(err)
	}
	if dead.calls != 1 || message.termed == "" || message.nacked != 0 {
		t.Fatalf("retry budget did not dead-letter: %+v", message)
	}
}

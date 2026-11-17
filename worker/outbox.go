package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
)

var ErrInvalidOutboxPublisherConfig = errors.New("invalid outbox publisher configuration")

type OutboxPublisher struct {
	Store           repository.OutboxStore
	Publisher       queue.EventPublisher
	Owner           string
	BatchSize       int
	LeaseDuration   time.Duration
	PublishTimeout  time.Duration
	PollInterval    time.Duration
	FailureDelay    time.Duration
	MaxFailureDelay time.Duration
	Concurrency     int
	Now             func() time.Time
	OnError         func(error)
}

func (p OutboxPublisher) Validate() error {
	if p.Store == nil || p.Publisher == nil || p.Owner == "" || len(p.Owner) > 128 ||
		p.BatchSize < 1 || p.BatchSize > 500 || p.LeaseDuration <= p.PublishTimeout || p.PublishTimeout <= 0 ||
		p.PollInterval <= 0 || p.FailureDelay <= 0 || p.MaxFailureDelay < p.FailureDelay || p.Concurrency < 1 || p.Concurrency > p.BatchSize {
		return fmt.Errorf("%w: store, publisher, owner, batch, lease, polling, and retry settings are required", ErrInvalidOutboxPublisherConfig)
	}
	return nil
}

// PublishOnce leases committed events, publishes each synchronously, and marks
// an outbox row delivered only after JetStream confirms durable receipt.
func (p OutboxPublisher) PublishOnce(ctx context.Context) (published, failed int, err error) {
	if err := p.Validate(); err != nil {
		return 0, 0, err
	}
	nowFn := p.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	events, err := p.Store.ClaimPending(ctx, p.Owner, p.BatchSize, now, now.Add(p.LeaseDuration))
	if err != nil {
		return 0, 0, fmt.Errorf("claim outbox events: %w", err)
	}
	var publishedCount atomic.Int64
	var failedCount atomic.Int64
	var workers sync.WaitGroup
	semaphore := make(chan struct{}, p.Concurrency)
	for _, claimed := range events {
		row := claimed
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			failedCount.Add(1)
			p.recordFailure(ctx, row, nowFn().UTC(), "PUBLISHER_SHUTDOWN")
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			if err := p.publishOne(ctx, row, nowFn); err != nil {
				failedCount.Add(1)
				p.report(err)
				return
			}
			publishedCount.Add(1)
		}()
	}
	workers.Wait()
	return int(publishedCount.Load()), int(failedCount.Load()), nil
}

func (p OutboxPublisher) publishOne(ctx context.Context, row repository.OutboxEvent, nowFn func() time.Time) error {
	event, convertErr := eventFromOutbox(row)
	if convertErr != nil {
		p.recordFailure(ctx, row, nowFn().UTC(), "INVALID_OUTBOX_EVENT")
		return fmt.Errorf("invalid outbox row %s: INVALID_OUTBOX_EVENT", row.ID)
	}
	publishCtx, cancel := context.WithTimeout(ctx, p.PublishTimeout)
	publishErr := p.Publisher.Publish(publishCtx, event)
	cancel()
	if publishErr != nil {
		p.recordFailure(ctx, row, nowFn().UTC(), "JETSTREAM_PUBLISH_FAILED")
		return fmt.Errorf("publish outbox row %s: JETSTREAM_PUBLISH_FAILED", row.ID)
	}
	if markErr := p.Store.MarkPublished(ctx, row.ID, row.ClaimToken, nowFn().UTC()); markErr != nil {
		// The broker may already have the event. Leave the row retryable; the
		// stable NATS message ID and consumer inbox make redelivery safe.
		p.recordFailure(ctx, row, nowFn().UTC(), "OUTBOX_ACK_FAILED")
		return fmt.Errorf("mark outbox row %s published: OUTBOX_ACK_FAILED", row.ID)
	}
	return nil
}

func (p OutboxPublisher) Run(ctx context.Context) error {
	if err := p.Validate(); err != nil {
		return err
	}
	ticker := time.NewTicker(p.PollInterval)
	defer ticker.Stop()
	for {
		if _, _, err := p.PublishOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.report(err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func eventFromOutbox(row repository.OutboxEvent) (queue.Event, error) {
	var payload map[string]string
	if err := json.Unmarshal(row.PayloadIDsOnly, &payload); err != nil {
		return queue.Event{}, fmt.Errorf("decode payload_ids_only: %w", err)
	}
	event := queue.Event{
		ID: row.ID, Type: row.EventType, SchemaVersion: row.SchemaVersion,
		AggregateType: row.AggregateType, AggregateID: row.AggregateID,
		TenantID: row.TenantID, CreatedAt: row.CreatedAt.UTC(), Payload: payload,
	}
	if err := event.Validate(); err != nil {
		return queue.Event{}, err
	}
	return event, nil
}

func (p OutboxPublisher) recordFailure(ctx context.Context, row repository.OutboxEvent, now time.Time, errorClass string) {
	if err := p.Store.RecordPublishFailure(ctx, row.ID, row.ClaimToken, errorClass, now.Add(p.publishRetryDelay(row.PublishAttempts))); err != nil {
		p.report(fmt.Errorf("record outbox failure for %s: %w", row.ID, err))
	}
}

func (p OutboxPublisher) report(err error) {
	if p.OnError != nil && err != nil {
		p.OnError(err)
	}
}

func (p OutboxPublisher) publishRetryDelay(attempt int) time.Duration {
	capDelay := p.FailureDelay
	for index := 1; index < attempt && capDelay < p.MaxFailureDelay; index++ {
		if capDelay > p.MaxFailureDelay/2 {
			capDelay = p.MaxFailureDelay
			break
		}
		capDelay *= 2
	}
	if capDelay > p.MaxFailureDelay {
		capDelay = p.MaxFailureDelay
	}
	if capDelay <= time.Nanosecond {
		return capDelay
	}
	floor := capDelay / 2
	return floor + time.Duration(rand.Int63n(int64(capDelay-floor)+1))
}

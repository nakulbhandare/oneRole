package worker

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/oneRole/queue"
)

var ErrInvalidWorkerConfig = errors.New("invalid worker configuration")

const defaultTaskTimeout = 10 * time.Minute

// EventHandler reloads authoritative state from MongoDB and applies the
// domain/service workflow. It must persist a safe application state before
// returning nil; only then is the JetStream event acknowledged.
type EventHandler interface {
	HandleEvent(context.Context, queue.Event) error
}

type EventHandlerFunc func(context.Context, queue.Event) error

func (fn EventHandlerFunc) HandleEvent(ctx context.Context, event queue.Event) error {
	return fn(ctx, event)
}

// WorkFailure is returned only when the handler knows how the operation ended.
// A submission that may have reached an employer is not retryable: its handler
// must persist SUBMISSION_UNCONFIRMED and enqueue reconciliation, then return
// nil. SafeToRetry must be true before the queue runner retries a task.
type WorkFailure struct {
	Class       string
	Retryable   bool
	SafeToRetry bool
	RetryAfter  time.Duration
	Cause       error
}

func (e *WorkFailure) Error() string {
	if e == nil {
		return "worker failure"
	}
	if e.Class != "" {
		return e.Class
	}
	return "worker failure"
}

func (e *WorkFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type AckableMessage interface {
	Data() []byte
	Subject() string
	Metadata() (*jetstream.MsgMetadata, error)
	DoubleAck(context.Context) error
	NakWithDelay(time.Duration) error
	TermWithReason(string) error
}

type DeadLetterSink interface {
	Persist(context.Context, queue.DeadLetter) error
}

type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      func(time.Duration) time.Duration
}

func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 || p.BaseDelay <= 0 || p.MaxDelay < p.BaseDelay {
		return fmt.Errorf("%w: retry max attempts, base delay, and max delay are required", ErrInvalidWorkerConfig)
	}
	return nil
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 8,
		BaseDelay:   time.Second,
		MaxDelay:    5 * time.Minute,
		Jitter: func(cap time.Duration) time.Duration {
			if cap <= time.Nanosecond {
				return cap
			}
			floor := cap / 2
			return floor + time.Duration(rand.Int63n(int64(cap-floor)+1))
		},
	}
}

func (p RetryPolicy) delay(attempt uint64, retryAfter time.Duration) (time.Duration, bool) {
	capDelay := p.BaseDelay
	for index := uint64(1); index < attempt && capDelay < p.MaxDelay; index++ {
		if capDelay > p.MaxDelay/2 {
			capDelay = p.MaxDelay
			break
		}
		capDelay *= 2
	}
	if capDelay > p.MaxDelay {
		capDelay = p.MaxDelay
	}
	if retryAfter > 0 {
		if retryAfter > p.MaxDelay {
			return 0, false
		}
		return retryAfter, true
	}
	if p.Jitter == nil {
		return capDelay, true
	}
	delay := p.Jitter(capDelay)
	if delay <= 0 || delay > p.MaxDelay {
		return 0, false
	}
	return delay, true
}

// HandleMessage processes one delivery. Unknown errors and unsafe outcomes go
// to the dead-letter stream rather than being blindly retried.
func HandleMessage(ctx context.Context, message AckableMessage, handler EventHandler, deadLetters DeadLetterSink, policy RetryPolicy, now func() time.Time) error {
	return HandleDelivery(ctx, message, nil, "", "", handler, deadLetters, policy, 0, now)
}

// HandleDelivery adds durable inbox claims to one at-least-once JetStream delivery.
func HandleDelivery(ctx context.Context, message AckableMessage, inbox InboxStore, consumerName, owner string, handler EventHandler, deadLetters DeadLetterSink, policy RetryPolicy, leaseDuration time.Duration, now func() time.Time) error {
	if message == nil || handler == nil || deadLetters == nil {
		return fmt.Errorf("%w: message, handler, and dead-letter sink are required", ErrInvalidWorkerConfig)
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	if inbox != nil && (consumerName == "" || owner == "" || leaseDuration <= defaultTaskTimeout) {
		return fmt.Errorf("%w: inbox consumer, owner, and lease duration are required", ErrInvalidWorkerConfig)
	}
	if now == nil {
		now = time.Now
	}
	attempt := uint64(1)
	metadata, metadataErr := message.Metadata()
	if metadataErr == nil && metadata != nil && metadata.NumDelivered > 0 {
		attempt = metadata.NumDelivered
	}
	if metadataErr != nil {
		return message.NakWithDelay(policy.BaseDelay)
	}

	raw := message.Data()
	event, decodeErr := queue.DecodeEvent(raw)
	if decodeErr != nil {
		return persistAndTerm(ctx, message, deadLetters, queue.NewDeadLetter(event, raw, message.Subject(), "MALFORMED_EVENT", attempt, now().UTC()))
	}

	workCtx := ctx
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > defaultTaskTimeout {
		workCtx, cancel = context.WithTimeout(ctx, defaultTaskTimeout)
	}
	defer cancel()

	var claim InboxClaim
	if inbox != nil {
		stateNow := now().UTC()
		claimValue, state, claimErr := inbox.Claim(ctx, consumerName, event.ID, owner, stateNow, stateNow.Add(leaseDuration))
		claim = claimValue
		if claimErr != nil {
			return message.NakWithDelay(policy.BaseDelay)
		}
		switch state {
		case InboxAlreadyProcessed:
			return message.DoubleAck(ctx)
		case InboxLeaseHeld:
			return message.NakWithDelay(policy.BaseDelay)
		case InboxClaimAcquired:
			if claim.Token == "" {
				return message.NakWithDelay(policy.BaseDelay)
			}
		default:
			return message.NakWithDelay(policy.BaseDelay)
		}
	}

	processErr := invokeHandler(workCtx, handler, event)
	if processErr == nil {
		if inbox != nil {
			resourceID := event.AggregateID
			if err := inbox.Complete(ctx, claim, now().UTC(), &resourceID); err != nil {
				_ = inbox.Release(ctx, claim, now().UTC().Add(policy.BaseDelay), "INBOX_COMPLETE_FAILED")
				return message.NakWithDelay(policy.BaseDelay)
			}
		}
		return message.DoubleAck(ctx)
	}
	if ctx.Err() != nil {
		if inbox != nil {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = inbox.Release(cleanupCtx, claim, now().UTC().Add(policy.BaseDelay), "WORKER_SHUTDOWN")
			cleanupCancel()
		}
		return message.NakWithDelay(policy.BaseDelay)
	}

	var failure *WorkFailure
	class := "UNCLASSIFIED_FAILURE"
	if errors.As(processErr, &failure) && failure.Class != "" {
		class = failure.Class
	}
	if errors.As(processErr, &failure) && failure.Retryable && failure.SafeToRetry && attempt < uint64(policy.MaxAttempts) {
		delay, canRetry := policy.delay(attempt, failure.RetryAfter)
		if canRetry {
			if inbox != nil {
				if err := inbox.Release(ctx, claim, now().UTC().Add(delay), normalizeErrorClass(class)); err != nil {
					return message.NakWithDelay(policy.BaseDelay)
				}
			}
			return message.NakWithDelay(delay)
		}
	}
	record := queue.NewDeadLetter(event, raw, message.Subject(), class, attempt, now().UTC())
	if err := deadLetters.Persist(ctx, record); err != nil {
		if inbox != nil {
			_ = inbox.Release(ctx, claim, now().UTC().Add(policy.BaseDelay), "DEAD_LETTER_PERSIST_FAILED")
		}
		return message.NakWithDelay(policy.BaseDelay)
	}
	if inbox != nil {
		resourceID := event.AggregateID
		if err := inbox.Complete(ctx, claim, now().UTC(), &resourceID); err != nil {
			_ = inbox.Release(ctx, claim, now().UTC().Add(policy.BaseDelay), "INBOX_COMPLETE_FAILED")
			return message.NakWithDelay(policy.BaseDelay)
		}
	}
	return message.TermWithReason(record.ErrorClass)
}

func persistAndTerm(ctx context.Context, message AckableMessage, sink DeadLetterSink, record queue.DeadLetter) error {
	if err := sink.Persist(ctx, record); err != nil {
		return message.NakWithDelay(time.Second)
	}
	return message.TermWithReason(record.ErrorClass)
}

type StreamConsumer interface {
	Consume(jetstream.MessageHandler, ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error)
}

type ConsumerRunner struct {
	ConsumerName    string
	Owner           string
	LeaseDuration   time.Duration
	Concurrency     int
	ShutdownTimeout time.Duration
	RetryPolicy     RetryPolicy
	Now             func() time.Time
	OnError         func(error)
}

func (r ConsumerRunner) Run(ctx context.Context, consumer StreamConsumer, inbox InboxStore, handler EventHandler, deadLetters DeadLetterSink) error {
	if consumer == nil || inbox == nil || r.ConsumerName == "" || r.Owner == "" || r.LeaseDuration <= defaultTaskTimeout || r.Concurrency < 1 || r.ShutdownTimeout <= 0 {
		return fmt.Errorf("%w: consumer, inbox, owner, lease duration, concurrency, and shutdown timeout are required", ErrInvalidWorkerConfig)
	}
	if err := r.RetryPolicy.Validate(); err != nil {
		return err
	}
	semaphore := make(chan struct{}, r.Concurrency)
	var active sync.WaitGroup
	consumeErr := make(chan error, 1)

	consumeCtx, err := consumer.Consume(func(message jetstream.Msg) {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			_ = message.NakWithDelay(r.RetryPolicy.BaseDelay)
			return
		}
		active.Add(1)
		go func() {
			defer active.Done()
			defer func() { <-semaphore }()
			if err := HandleDelivery(ctx, message, inbox, r.ConsumerName, r.Owner, handler, deadLetters, r.RetryPolicy, r.LeaseDuration, r.Now); err != nil && r.OnError != nil {
				r.OnError(err)
			}
		}()
	}, jetstream.PullMaxMessages(r.Concurrency), jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		if err == nil || ctx.Err() != nil {
			return
		}
		select {
		case consumeErr <- err:
		default:
		}
	}))
	if err != nil {
		return fmt.Errorf("start JetStream consumer: %w", err)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case <-consumeCtx.Closed():
	case runErr = <-consumeErr:
	}
	consumeCtx.Drain()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), r.ShutdownTimeout)
	defer cancel()
	select {
	case <-consumeCtx.Closed():
	case <-shutdownCtx.Done():
		consumeCtx.Stop()
		if runErr == nil {
			runErr = fmt.Errorf("consumer drain timed out")
		}
	}
	finished := make(chan struct{})
	go func() {
		active.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-shutdownCtx.Done():
		if runErr == nil {
			runErr = fmt.Errorf("worker tasks did not finish before shutdown deadline")
		}
	}
	if runErr != nil {
		return fmt.Errorf("JetStream consumer stopped: %w", runErr)
	}
	return nil
}

var safeWorkerErrorClassPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func normalizeErrorClass(class string) string {
	class = strings.TrimSpace(class)
	if !safeWorkerErrorClassPattern.MatchString(class) {
		return "UNCLASSIFIED_FAILURE"
	}
	return class
}

func invokeHandler(ctx context.Context, handler EventHandler, event queue.Event) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &WorkFailure{Class: "HANDLER_PANIC", Cause: fmt.Errorf("panic: %v", recovered)}
		}
	}()
	return handler.HandleEvent(ctx, event)
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/oneRole/connectors/job_sources"
	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
)

var ErrInvalidSourceSyncConfig = errors.New("invalid source sync worker configuration")

// SourceSyncEventHandler reloads the persisted run and source configuration by
// ID. It fetches pages outside DB transactions, and commits each page together
// with its checkpoint so a redelivery resumes from the last durable cursor.
type SourceSyncEventHandler struct {
	Store         repository.JobSourceSyncStore
	Connector     job_sources.Connector
	MaxPages      int
	MaxRecords    int
	LeaseDuration time.Duration
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	Now           func() time.Time
	OnError       func(error)
}

func (h SourceSyncEventHandler) Validate() error {
	if h.Store == nil || h.Connector == nil || h.MaxPages < 1 || h.MaxRecords < 1 ||
		h.LeaseDuration <= 0 || h.LeaseDuration > 30*time.Minute ||
		h.RetryDelay <= 0 || h.MaxRetryDelay < h.RetryDelay {
		return fmt.Errorf("%w: store, connector, page/record caps, lease duration, and retry delays are required", ErrInvalidSourceSyncConfig)
	}
	return nil
}

func (h SourceSyncEventHandler) HandleEvent(ctx context.Context, event queue.Event) error {
	if err := h.Validate(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil || event.Type != queue.EventSourceSyncRequested || event.TenantID != nil {
		return &WorkFailure{Class: "INVALID_SOURCE_SYNC_EVENT"}
	}
	sourceID := event.Payload["source_id"]
	runID := event.Payload["sync_run_id"]
	if event.AggregateType != "source_sync_run" || event.AggregateID != runID {
		return &WorkFailure{Class: "INVALID_SOURCE_SYNC_AGGREGATE"}
	}
	nowFn := h.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	run, err := h.Store.GetSourceSyncRun(ctx, sourceID, runID, now)
	if errors.Is(err, repository.ErrSourceLeaseLost) || errors.Is(err, repository.ErrSourceSyncState) {
		// A replacement lease/run superseded this queued task. It is safe to ack
		// this stale event because the current run has its own durable outbox row.
		return nil
	}
	if err != nil {
		return &WorkFailure{Class: "SOURCE_SYNC_LOAD_FAILED", Retryable: true, SafeToRetry: true, Cause: err}
	}
	if run.ID != runID || run.Source.ID != sourceID {
		return h.failRun(ctx, run, "permanent_failure", "SOURCE_SYNC_RUN_INVALID", 0, nowFn)
	}
	keeper := newSourceLeaseKeeper(ctx, h.Store, run, h.LeaseDuration, nowFn)
	leaseStopped := false
	stopLease := func() error {
		if leaseStopped {
			return nil
		}
		leaseStopped = true
		return keeper.Stop()
	}
	defer func() { _ = stopLease() }()

	cursor := run.ResumeCursor
	seenCursors := map[string]struct{}{cursor: {}}
	processedRecords := 0
	for pageNumber := 0; pageNumber < h.MaxPages; pageNumber++ {
		if err := keeper.Context().Err(); err != nil {
			if leaseErr := stopLease(); leaseErr != nil {
				return leaseFailure(leaseErr)
			}
			return &WorkFailure{Class: "SOURCE_SYNC_CANCELLED", Retryable: true, SafeToRetry: true, Cause: err}
		}
		page, fetchErr := h.Connector.Fetch(keeper.Context(), run.Source, cursor)
		if fetchErr != nil {
			if leaseErr := stopLease(); leaseErr != nil {
				return leaseFailure(leaseErr)
			}
			status, class, retryAfter := classifySourceFetchError(fetchErr)
			return h.failRun(ctx, run, status, class, retryAfter, nowFn)
		}
		if processedRecords+len(page.Items) > h.MaxRecords {
			if leaseErr := stopLease(); leaseErr != nil {
				return leaseFailure(leaseErr)
			}
			return h.failRun(ctx, run, "retryable_failure", "SOURCE_RECORD_BUDGET_EXHAUSTED", h.RetryDelay, nowFn)
		}
		observedAt := nowFn().UTC()
		result, ingestErr := h.Store.IngestSourcePage(keeper.Context(), repository.SourcePageInput{
			Run: run, CursorBefore: cursor, Page: page, ObservedAt: observedAt,
		})
		if ingestErr != nil {
			if leaseErr := stopLease(); leaseErr != nil {
				return leaseFailure(leaseErr)
			}
			if errors.Is(ingestErr, repository.ErrSourceLeaseLost) || errors.Is(ingestErr, repository.ErrSourceSyncState) {
				return nil
			}
			return &WorkFailure{Class: "SOURCE_PAGE_COMMIT_FAILED", Retryable: true, SafeToRetry: true, Cause: ingestErr}
		}
		processedRecords += result.RecordsSeen
		if result.Complete {
			if leaseErr := stopLease(); leaseErr != nil {
				return leaseFailure(leaseErr)
			}
			if err := h.Store.CompleteSourceSync(ctx, run, nowFn().UTC()); err != nil {
				if errors.Is(err, repository.ErrSourceLeaseLost) || errors.Is(err, repository.ErrSourceSyncState) {
					return nil
				}
				return &WorkFailure{Class: "SOURCE_SYNC_COMPLETE_FAILED", Retryable: true, SafeToRetry: true, Cause: err}
			}
			return nil
		}
		if _, exists := seenCursors[result.CursorAfter]; exists {
			if leaseErr := stopLease(); leaseErr != nil {
				return leaseFailure(leaseErr)
			}
			return h.failRun(ctx, run, "permanent_failure", "SOURCE_CURSOR_LOOP", 0, nowFn)
		}
		seenCursors[result.CursorAfter] = struct{}{}
		cursor = result.CursorAfter
	}
	// The page cursor was checkpointed on every iteration; release the source
	// with a bounded retry time so the next scheduled run resumes, rather than
	// holding one queue message indefinitely.
	if leaseErr := stopLease(); leaseErr != nil {
		return leaseFailure(leaseErr)
	}
	return h.failRun(ctx, run, "retryable_failure", "SOURCE_PAGE_BUDGET_EXHAUSTED", h.RetryDelay, nowFn)
}

func leaseFailure(err error) error {
	if errors.Is(err, repository.ErrSourceLeaseLost) || errors.Is(err, repository.ErrSourceSyncState) {
		return nil
	}
	return &WorkFailure{Class: "SOURCE_LEASE_RENEW_FAILED", Retryable: true, SafeToRetry: true, Cause: err}
}

// sourceLeaseKeeper renews the fenced source/run lease independently of slow
// network fetches. A lease loss cancels the page context so no later page is
// committed with a stale owner or connector configuration.
type sourceLeaseKeeper struct {
	ctx         context.Context
	cancelWork  context.CancelFunc
	stopRenewal context.CancelFunc
	done        chan struct{}
	errMu       sync.Mutex
	err         error
}

func newSourceLeaseKeeper(parent context.Context, store repository.JobSourceSyncStore, run repository.SourceSyncRun, leaseDuration time.Duration, nowFn func() time.Time) *sourceLeaseKeeper {
	workCtx, cancelWork := context.WithCancel(parent)
	renewCtx, stopRenewal := context.WithCancel(context.Background())
	keeper := &sourceLeaseKeeper{ctx: workCtx, cancelWork: cancelWork, stopRenewal: stopRenewal, done: make(chan struct{})}
	interval := leaseDuration / 3
	if interval > time.Minute {
		interval = time.Minute
	}
	if interval < time.Second {
		interval = time.Second
	}
	go func() {
		defer close(keeper.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				now := nowFn().UTC()
				callCtx, cancelCall := context.WithTimeout(workCtx, 10*time.Second)
				err := store.RenewSourceLease(callCtx, run, now, now.Add(leaseDuration))
				cancelCall()
				if err != nil {
					keeper.errMu.Lock()
					keeper.err = err
					keeper.errMu.Unlock()
					cancelWork()
					return
				}
			}
		}
	}()
	return keeper
}

func (k *sourceLeaseKeeper) Context() context.Context { return k.ctx }

func (k *sourceLeaseKeeper) Stop() error {
	k.stopRenewal()
	<-k.done
	k.cancelWork()
	k.errMu.Lock()
	defer k.errMu.Unlock()
	return k.err
}

func (h SourceSyncEventHandler) failRun(ctx context.Context, run repository.SourceSyncRun, status, errorClass string, retryAfter time.Duration, nowFn func() time.Time) error {
	now := nowFn().UTC()
	retryAt := now
	if status == "retryable_failure" {
		if retryAfter <= 0 {
			retryAfter = h.RetryDelay
		}
		if retryAfter > h.MaxRetryDelay {
			retryAfter = h.MaxRetryDelay
		}
		retryAt = now.Add(retryAfter)
	}
	if err := h.Store.FailSourceSync(ctx, run, status, errorClass, retryAt, now); err != nil {
		if errors.Is(err, repository.ErrSourceLeaseLost) || errors.Is(err, repository.ErrSourceSyncState) {
			return nil
		}
		return &WorkFailure{Class: "SOURCE_SYNC_FAILURE_COMMIT_FAILED", Retryable: true, SafeToRetry: true, Cause: err}
	}
	return nil
}

func classifySourceFetchError(err error) (status, class string, retryAfter time.Duration) {
	var providerError *job_sources.HTTPError
	if errors.As(err, &providerError) {
		class = fmt.Sprintf("HTTP_%d", providerError.StatusCode)
		switch {
		case providerError.StatusCode == 408 || providerError.StatusCode == 429 || providerError.StatusCode >= 500:
			return "retryable_failure", class, providerError.RetryAfter
		default:
			return "permanent_failure", class, 0
		}
	}
	if errors.Is(err, job_sources.ErrSourceNotEnabled) || errors.Is(err, job_sources.ErrUnsupported) || errors.Is(err, job_sources.ErrInvalidConfig) {
		return "permanent_failure", "SOURCE_CONFIGURATION_INVALID", 0
	}
	var networkError net.Error
	if errors.As(err, &networkError) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "retryable_failure", "SOURCE_NETWORK_ERROR", 0
	}
	return "permanent_failure", "SOURCE_RESPONSE_INVALID", 0
}

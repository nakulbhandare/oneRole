package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oneRole/repository"
)

var ErrInvalidSourceSchedulerConfig = errors.New("invalid source scheduler configuration")

// SourceSyncScheduler leases due, reviewed sources and asks the repository to
// create a run plus its global outbox event atomically. The outbox publisher
// later delivers the fetch task to JetStream.
type SourceSyncScheduler struct {
	Store         repository.JobSourceSyncStore
	Owner         string
	BatchSize     int
	Concurrency   int
	LeaseDuration time.Duration
	PollInterval  time.Duration
	Now           func() time.Time
	OnError       func(error)
}

func (s SourceSyncScheduler) Validate() error {
	if s.Store == nil || s.Owner == "" || len(s.Owner) > 128 ||
		s.BatchSize < 1 || s.BatchSize > 500 || s.Concurrency < 1 || s.Concurrency > s.BatchSize ||
		s.LeaseDuration <= 0 || s.PollInterval <= 0 {
		return fmt.Errorf("%w: store, owner, batch, concurrency, lease, and poll interval are required", ErrInvalidSourceSchedulerConfig)
	}
	return nil
}

func (s SourceSyncScheduler) ScheduleOnce(ctx context.Context) (started, failed int, err error) {
	if err := s.Validate(); err != nil {
		return 0, 0, err
	}
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	sources, err := s.Store.ClaimDueSources(ctx, s.Owner, s.BatchSize, now, now.Add(s.LeaseDuration))
	if err != nil {
		return 0, 0, fmt.Errorf("claim due sources: %w", err)
	}
	var startedCount atomic.Int64
	var failedCount atomic.Int64
	var workers sync.WaitGroup
	semaphore := make(chan struct{}, s.Concurrency)
	for _, claimed := range sources {
		source := claimed
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			failedCount.Add(1)
			s.report(fmt.Errorf("source %s was not started before shutdown", source.ID))
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			if _, err := s.Store.StartSourceSync(ctx, source, nowFn().UTC()); err != nil {
				failedCount.Add(1)
				s.report(fmt.Errorf("start source sync %s: SOURCE_SYNC_START_FAILED", source.ID))
				return
			}
			startedCount.Add(1)
		}()
	}
	workers.Wait()
	return int(startedCount.Load()), int(failedCount.Load()), nil
}

func (s SourceSyncScheduler) Run(ctx context.Context) error {
	if err := s.Validate(); err != nil {
		return err
	}
	ticker := time.NewTicker(s.PollInterval)
	defer ticker.Stop()
	for {
		if _, _, err := s.ScheduleOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.report(err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s SourceSyncScheduler) report(err error) {
	if s.OnError != nil && err != nil {
		s.OnError(err)
	}
}

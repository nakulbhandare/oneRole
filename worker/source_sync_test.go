package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oneRole/connectors/job_sources"
	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
)

type fakeSourceStore struct {
	mu            sync.Mutex
	sources       []job_sources.SourceConfig
	run           repository.SourceSyncRun
	getErr        error
	pageResult    repository.SourcePageResult
	pageErr       error
	completeErr   error
	failErr       error
	renewErr      error
	claimedOwner  string
	claimCount    int
	startCount    int
	getCount      int
	pageCount     int
	completeCount int
	failCount     int
	renewCount    int
	failedStatus  string
	failedClass   string
	retryAt       time.Time
	seenCursor    string
}

func (s *fakeSourceStore) ClaimDueSources(_ context.Context, owner string, _ int, _, _ time.Time) ([]job_sources.SourceConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimedOwner = owner
	s.claimCount++
	return s.sources, nil
}
func (s *fakeSourceStore) StartSourceSync(_ context.Context, source job_sources.SourceConfig, at time.Time) (repository.SourceSyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCount++
	run := s.run
	if run.ID == "" {
		run.ID = "99999999-9999-4999-8999-999999999999"
	}
	run.Source = source
	run.StartedAt = at.UTC()
	return run, nil
}
func (s *fakeSourceStore) GetSourceSyncRun(_ context.Context, _, _ string, _ time.Time) (repository.SourceSyncRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCount++
	return s.run, s.getErr
}
func (s *fakeSourceStore) IngestSourcePage(_ context.Context, input repository.SourcePageInput) (repository.SourcePageResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageCount++
	s.seenCursor = input.CursorBefore
	return s.pageResult, s.pageErr
}
func (s *fakeSourceStore) CompleteSourceSync(context.Context, repository.SourceSyncRun, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completeCount++
	return s.completeErr
}
func (s *fakeSourceStore) RenewSourceLease(context.Context, repository.SourceSyncRun, time.Time, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewCount++
	return s.renewErr
}
func (s *fakeSourceStore) FailSourceSync(_ context.Context, _ repository.SourceSyncRun, status, errorClass string, retryAt, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCount++
	s.failedStatus, s.failedClass, s.retryAt = status, errorClass, retryAt
	return s.failErr
}

type fakeSourceConnector struct {
	pages  []job_sources.Page
	errs   []error
	calls  int
	cursor string
}

func (c *fakeSourceConnector) Fetch(_ context.Context, _ job_sources.SourceConfig, cursor string) (job_sources.Page, error) {
	c.calls++
	c.cursor = cursor
	index := c.calls - 1
	if index < len(c.errs) && c.errs[index] != nil {
		return job_sources.Page{}, c.errs[index]
	}
	if index < len(c.pages) {
		return c.pages[index], nil
	}
	return job_sources.Page{Complete: true}, nil
}

func validSourceEvent() queue.Event {
	return queue.Event{
		ID: "11111111-1111-4111-8111-111111111111", Type: queue.EventSourceSyncRequested,
		SchemaVersion: 1, AggregateType: "source_sync_run", AggregateID: "99999999-9999-4999-8999-999999999999",
		CreatedAt: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		Payload:   map[string]string{"source_id": "22222222-2222-4222-8222-222222222222", "sync_run_id": "99999999-9999-4999-8999-999999999999"},
	}
}

func sourceHandler(store repository.JobSourceSyncStore, connector job_sources.Connector) SourceSyncEventHandler {
	return SourceSyncEventHandler{Store: store, Connector: connector, MaxPages: 5, MaxRecords: 100,
		LeaseDuration: 20 * time.Minute, RetryDelay: time.Minute, MaxRetryDelay: time.Hour,
		Now: func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }}
}

func TestSourceSyncResumesAtPersistedCursorAndCompletes(t *testing.T) {
	store := &fakeSourceStore{
		run:        repository.SourceSyncRun{ID: "99999999-9999-4999-8999-999999999999", Source: job_sources.SourceConfig{ID: "22222222-2222-4222-8222-222222222222"}, CheckpointBefore: "before", ResumeCursor: "resume-here"},
		pageResult: repository.SourcePageResult{Complete: true, RecordsSeen: 1},
	}
	connector := &fakeSourceConnector{pages: []job_sources.Page{{Items: []job_sources.Posting{{ExternalID: "real-source-id"}}, Complete: true}}}
	if err := sourceHandler(store, connector).HandleEvent(context.Background(), validSourceEvent()); err != nil {
		t.Fatal(err)
	}
	if connector.cursor != "resume-here" || store.seenCursor != "resume-here" || store.completeCount != 1 || store.failCount != 0 {
		t.Fatalf("source sync did not resume and complete: connector=%+v store=%+v", connector, store)
	}
}

func TestSourceSyncHonorsProviderRetryAfter(t *testing.T) {
	retryAfter := 70 * time.Second
	store := &fakeSourceStore{run: repository.SourceSyncRun{ID: "99999999-9999-4999-8999-999999999999", Source: job_sources.SourceConfig{ID: "22222222-2222-4222-8222-222222222222"}}}
	connector := &fakeSourceConnector{errs: []error{&job_sources.HTTPError{StatusCode: 429, RetryAfter: retryAfter}}}
	if err := sourceHandler(store, connector).HandleEvent(context.Background(), validSourceEvent()); err != nil {
		t.Fatal(err)
	}
	if store.failCount != 1 || store.failedStatus != "retryable_failure" || store.failedClass != "HTTP_429" || !store.retryAt.Equal(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Add(retryAfter)) {
		t.Fatalf("provider retry policy mismatch: %+v", store)
	}
}

func TestSourceSyncPausesOnPermanentProviderFailure(t *testing.T) {
	store := &fakeSourceStore{run: repository.SourceSyncRun{ID: "99999999-9999-4999-8999-999999999999", Source: job_sources.SourceConfig{ID: "22222222-2222-4222-8222-222222222222"}}}
	connector := &fakeSourceConnector{errs: []error{&job_sources.HTTPError{StatusCode: 403}}}
	if err := sourceHandler(store, connector).HandleEvent(context.Background(), validSourceEvent()); err != nil {
		t.Fatal(err)
	}
	if store.failedStatus != "permanent_failure" || store.failedClass != "HTTP_403" {
		t.Fatalf("permanent provider response should pause source: %+v", store)
	}
}

func TestSourceSyncAcknowledgesSupersededRun(t *testing.T) {
	store := &fakeSourceStore{getErr: repository.ErrSourceLeaseLost}
	connector := &fakeSourceConnector{}
	if err := sourceHandler(store, connector).HandleEvent(context.Background(), validSourceEvent()); err != nil {
		t.Fatal(err)
	}
	if connector.calls != 0 || store.completeCount != 0 {
		t.Fatalf("superseded run was processed: connector=%d store=%+v", connector.calls, store)
	}
}

type blockingSourceConnector struct{ called chan struct{} }

func (c blockingSourceConnector) Fetch(ctx context.Context, _ job_sources.SourceConfig, _ string) (job_sources.Page, error) {
	close(c.called)
	<-ctx.Done()
	return job_sources.Page{}, ctx.Err()
}

func TestSourceSyncStopsWhenLeaseRenewalLosesFence(t *testing.T) {
	store := &fakeSourceStore{
		run:      repository.SourceSyncRun{ID: "99999999-9999-4999-8999-999999999999", Source: job_sources.SourceConfig{ID: "22222222-2222-4222-8222-222222222222"}},
		renewErr: repository.ErrSourceLeaseLost,
	}
	connector := blockingSourceConnector{called: make(chan struct{})}
	handler := sourceHandler(store, connector)
	handler.LeaseDuration = time.Second

	if err := handler.HandleEvent(context.Background(), validSourceEvent()); err != nil {
		t.Fatal(err)
	}
	if store.renewCount != 1 || store.pageCount != 0 || store.completeCount != 0 || store.failCount != 0 {
		t.Fatalf("lost lease should stop fetching and safely acknowledge stale work: %+v", store)
	}
}

func TestSourceSchedulerPersistsRunPerClaimedSource(t *testing.T) {
	store := &fakeSourceStore{sources: []job_sources.SourceConfig{{ID: "source-1"}, {ID: "source-2"}}}
	scheduler := SourceSyncScheduler{Store: store, Owner: "worker-1", BatchSize: 10, Concurrency: 2, LeaseDuration: 20 * time.Minute, PollInterval: time.Minute}
	started, failed, err := scheduler.ScheduleOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if started != 2 || failed != 0 || store.claimedOwner != "worker-1" || store.startCount != 2 {
		t.Fatalf("scheduler did not start each claimed run: started=%d failed=%d store=%+v", started, failed, store)
	}
}

func TestSourceSyncFailureCommitFailureIsExplicitlyRetryable(t *testing.T) {
	store := &fakeSourceStore{run: repository.SourceSyncRun{ID: "99999999-9999-4999-8999-999999999999", Source: job_sources.SourceConfig{ID: "22222222-2222-4222-8222-222222222222"}}, failErr: errors.New("db unavailable")}
	connector := &fakeSourceConnector{errs: []error{&job_sources.HTTPError{StatusCode: 401}}}
	err := sourceHandler(store, connector).HandleEvent(context.Background(), validSourceEvent())
	var failure *WorkFailure
	if !errors.As(err, &failure) || !failure.SafeToRetry || failure.Class != "SOURCE_SYNC_FAILURE_COMMIT_FAILED" {
		t.Fatalf("failure persistence error should retry the DB transaction, got %v", err)
	}
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/oneRole/connectors/job_sources"
	"github.com/oneRole/queue"
	"github.com/oneRole/repository"
	"github.com/oneRole/service"
)

var ErrInvalidRuntimeConfig = errors.New("invalid worker runtime configuration")

type RuntimeConfig struct {
	WorkerID string

	ProvisionStreams   bool
	ProvisionConsumers bool
	StreamReplicas     int

	OutboxBatchSize      int
	OutboxConcurrency    int
	OutboxLeaseDuration  time.Duration
	OutboxPublishTimeout time.Duration
	OutboxPollInterval   time.Duration
	OutboxFailureDelay   time.Duration
	OutboxMaxRetryDelay  time.Duration

	SourceBatchSize     int
	SourceConcurrency   int
	SourceLeaseDuration time.Duration
	SourcePollInterval  time.Duration
	SourceMaxPages      int
	SourceMaxRecords    int
	SourceRetryDelay    time.Duration
	SourceMaxRetryDelay time.Duration

	ConsumerConcurrency int
	ConsumerAckWait     time.Duration
	InboxLeaseDuration  time.Duration
	ShutdownTimeout     time.Duration
	RetryPolicy         RetryPolicy
}

func DefaultRuntimeConfig(workerID string) RuntimeConfig {
	return RuntimeConfig{
		WorkerID:         workerID,
		ProvisionStreams: true, ProvisionConsumers: true, StreamReplicas: 1,
		OutboxBatchSize: 100, OutboxConcurrency: 16,
		OutboxLeaseDuration: 2 * time.Minute, OutboxPublishTimeout: 30 * time.Second, OutboxPollInterval: 250 * time.Millisecond,
		OutboxFailureDelay: time.Second, OutboxMaxRetryDelay: time.Minute,
		SourceBatchSize: 100, SourceConcurrency: 16,
		SourceLeaseDuration: 20 * time.Minute, SourcePollInterval: 30 * time.Second,
		SourceMaxPages: 20, SourceMaxRecords: 10_000,
		SourceRetryDelay: 5 * time.Minute, SourceMaxRetryDelay: 24 * time.Hour,
		ConsumerConcurrency: 8, ConsumerAckWait: 15 * time.Minute,
		InboxLeaseDuration: 20 * time.Minute, ShutdownTimeout: 30 * time.Second,
		RetryPolicy: DefaultRetryPolicy(),
	}
}

func (c RuntimeConfig) Validate() error {
	if c.WorkerID == "" || len(c.WorkerID) > 128 || c.StreamReplicas < 1 || c.StreamReplicas > 5 ||
		c.OutboxBatchSize < 1 || c.OutboxBatchSize > 500 || c.OutboxConcurrency < 1 || c.OutboxConcurrency > c.OutboxBatchSize ||
		c.OutboxPublishTimeout <= 0 || c.OutboxLeaseDuration <= c.OutboxPublishTimeout || c.OutboxPollInterval <= 0 || c.OutboxFailureDelay <= 0 || c.OutboxMaxRetryDelay < c.OutboxFailureDelay ||
		c.SourceBatchSize < 1 || c.SourceBatchSize > 500 || c.SourceConcurrency < 1 || c.SourceConcurrency > c.SourceBatchSize ||
		c.SourceLeaseDuration <= defaultTaskTimeout || c.SourceLeaseDuration > 30*time.Minute || c.SourcePollInterval <= 0 || c.SourceMaxPages < 1 || c.SourceMaxRecords < 1 ||
		c.SourceRetryDelay <= 0 || c.SourceMaxRetryDelay < c.SourceRetryDelay ||
		c.ConsumerConcurrency < 1 || c.ConsumerConcurrency > 1000 || c.ConsumerAckWait <= defaultTaskTimeout ||
		c.InboxLeaseDuration <= c.ConsumerAckWait || c.InboxLeaseDuration <= defaultTaskTimeout || c.ShutdownTimeout <= 0 {
		return fmt.Errorf("%w: one or more worker pool, timeout, retry, stream, or owner settings are invalid", ErrInvalidRuntimeConfig)
	}
	if err := c.RetryPolicy.Validate(); err != nil {
		return err
	}
	return nil
}

type RuntimeDependencies struct {
	JetStream        jetstream.JetStream
	OutboxStore      repository.OutboxStore
	InboxStore       repository.InboxStore
	SourceSyncStore  repository.JobSourceSyncStore
	SourceConnector  job_sources.Connector
	ApplicationTasks service.ApplicationTaskService
}

type Runtime struct {
	config       RuntimeConfig
	dependencies RuntimeDependencies
	publisher    *OutboxPublisher
	scheduler    *SourceSyncScheduler
	runners      []configuredRunner
	router       EventHandler
	deadLetter   DeadLetterSink
	onError      func(error)
}

type configuredRunner struct {
	consumer jetstream.Consumer
	runner   ConsumerRunner
	handler  EventHandler
	dead     DeadLetterSink
}

// NewRuntime validates all required ports and constructs durable publishers,
// source scheduling, and workload-specific JetStream consumers. It does not
// connect to external employer systems or invent source records.
func NewRuntime(config RuntimeConfig, dependencies RuntimeDependencies, onError func(error)) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if dependencies.JetStream == nil || dependencies.OutboxStore == nil || dependencies.InboxStore == nil ||
		dependencies.SourceSyncStore == nil || dependencies.SourceConnector == nil || dependencies.ApplicationTasks == nil {
		return nil, fmt.Errorf("%w: JetStream, outbox, inbox, source sync, source connector, and application task services are required", ErrInvalidRuntimeConfig)
	}
	publisher, err := queue.NewJetStreamPublisher(dependencies.JetStream)
	if err != nil {
		return nil, err
	}
	outbox := &OutboxPublisher{
		Store: dependencies.OutboxStore, Publisher: publisher, Owner: config.WorkerID,
		BatchSize: config.OutboxBatchSize, Concurrency: config.OutboxConcurrency,
		LeaseDuration: config.OutboxLeaseDuration, PublishTimeout: config.OutboxPublishTimeout, PollInterval: config.OutboxPollInterval,
		FailureDelay: config.OutboxFailureDelay, MaxFailureDelay: config.OutboxMaxRetryDelay,
		OnError: onError,
	}
	scheduler := &SourceSyncScheduler{
		Store: dependencies.SourceSyncStore, Owner: config.WorkerID,
		BatchSize: config.SourceBatchSize, Concurrency: config.SourceConcurrency,
		LeaseDuration: config.SourceLeaseDuration, PollInterval: config.SourcePollInterval,
		OnError: onError,
	}
	appHandler := ApplicationEventHandler{Tasks: dependencies.ApplicationTasks}
	sourceHandler := SourceSyncEventHandler{
		Store: dependencies.SourceSyncStore, Connector: dependencies.SourceConnector,
		MaxPages: config.SourceMaxPages, MaxRecords: config.SourceMaxRecords,
		LeaseDuration: config.SourceLeaseDuration,
		RetryDelay:    config.SourceRetryDelay, MaxRetryDelay: config.SourceMaxRetryDelay,
		OnError: onError,
	}
	appTypes := []string{
		queue.EventApplicationPrepareRequested,
		queue.EventApplicationBatchRequested,
		queue.EventApplicationSubmitRequested,
		queue.EventApplicationReconcileRequested,
	}
	handlers := make(map[string]EventHandler, len(appTypes)+1)
	for _, eventType := range appTypes {
		handlers[eventType] = appHandler
	}
	handlers[queue.EventSourceSyncRequested] = sourceHandler
	router, err := NewEventRouter(handlers)
	if err != nil {
		return nil, err
	}
	deadLetter, err := queue.NewDeadLetterPublisher(dependencies.JetStream)
	if err != nil {
		return nil, err
	}

	runtime := &Runtime{
		config: config, dependencies: dependencies, publisher: outbox, scheduler: scheduler,
		router: router, deadLetter: deadLetter, onError: onError,
	}
	return runtime, nil
}

func (r *Runtime) ensureConsumers(ctx context.Context) error {
	consumerSpecs := []struct {
		stream  string
		name    string
		subject string
	}{
		{queue.StreamApplications, "application-packet-prepare-v1", queue.SubjectApplicationPrepare},
		{queue.StreamApplications, "application-batch-fanout-v1", queue.SubjectApplicationBatch},
		{queue.StreamApplications, "application-submit-v1", queue.SubjectApplicationSubmit},
		{queue.StreamApplications, "application-reconcile-v1", queue.SubjectApplicationReconcile},
		{queue.StreamDiscovery, "discovery-source-sync-v1", queue.SubjectSourceSync},
	}
	for _, spec := range consumerSpecs {
		consumerConfig, err := queue.DurableConsumerConfig(spec.name, spec.subject, r.config.RetryPolicy.MaxAttempts, r.config.ConsumerConcurrency, r.config.ConsumerAckWait)
		if err != nil {
			return err
		}
		var consumer jetstream.Consumer
		if r.config.ProvisionConsumers {
			consumer, err = queue.EnsureConsumer(ctx, r.dependencies.JetStream, spec.stream, consumerConfig)
		} else {
			consumer, err = queue.FindConsumer(ctx, r.dependencies.JetStream, spec.stream, spec.name)
		}
		if err != nil {
			return err
		}
		r.runners = append(r.runners, configuredRunner{
			consumer: consumer,
			runner: ConsumerRunner{
				ConsumerName: spec.name, Owner: r.config.WorkerID,
				LeaseDuration: r.config.InboxLeaseDuration, Concurrency: r.config.ConsumerConcurrency,
				ShutdownTimeout: r.config.ShutdownTimeout, RetryPolicy: r.config.RetryPolicy, OnError: r.onError,
			},
			handler: r.router, dead: r.deadLetter,
		})
	}
	return nil
}

// Run optionally provisions development streams, then starts the outbox relay,
// discovery scheduler, and durable workload consumers under one cancellation
// tree. The first unexpected worker exit stops the rest; root main owns signal
// handling and graceful process shutdown.
func (r *Runtime) Run(ctx context.Context) error {
	if r == nil || r.dependencies.JetStream == nil {
		return fmt.Errorf("%w: runtime is not configured", ErrInvalidRuntimeConfig)
	}
	if r.config.ProvisionStreams {
		if err := queue.EnsureStreams(ctx, r.dependencies.JetStream, r.config.StreamReplicas); err != nil {
			return err
		}
	}
	if len(r.runners) == 0 {
		if err := r.ensureConsumers(ctx); err != nil {
			return err
		}
	}
	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerFns := make([]func(context.Context) error, 0, len(r.runners)+2)
	workerFns = append(workerFns, r.publisher.Run, r.scheduler.Run)
	for _, configured := range r.runners {
		item := configured
		workerFns = append(workerFns, func(ctx context.Context) error {
			return item.runner.Run(ctx, item.consumer, r.dependencies.InboxStore, item.handler, item.dead)
		})
	}
	errs := make(chan error, len(workerFns))
	var workers sync.WaitGroup
	for _, runWorker := range workerFns {
		runWorker := runWorker
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := runWorker(groupCtx); err != nil {
				errs <- err
				return
			}
			if groupCtx.Err() == nil {
				errs <- errors.New("worker stopped unexpectedly")
			}
		}()
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errs:
		cancel()
	}
	cancel()
	workers.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return runErr
}

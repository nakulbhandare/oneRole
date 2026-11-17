# Go worker implementation

**Status:** implementation design. Workers do not exist yet.

## What a worker is

An API handler should finish quickly. Work that can take seconds or minutes—fetching jobs, parsing resumes, generating packets, or calling an employer—is stored as a durable task and performed by a Go worker.

The API and workers are the same Go application and the same root main.go. Production runs API and worker containers separately using APP_MODE=api and APP_MODE=worker. Local development may use APP_MODE=all.

## Worker map

| Worker | Receives | Does | Writes |
|---|---|---|---|
| Source scheduler | Due source rows | Claims sources under a short lease and publishes sync tasks | Next due time and run ID |
| Discovery fetcher | Source ID and checkpoint | Calls the documented provider endpoint, paginates, obeys rate limits | Source observations and checkpoint |
| Job normalizer | Observation ID | Maps provider values to OneRole's job shape, validates fields, deduplicates | Normalized job and job-change event |
| Search projection worker (optional) | Job-change event | Updates an external search index if enabled | Projection checkpoint and lag metrics |
| Resume parser | Resume version ID | Reads private object, validates format, extracts facts and page evidence | Facts, parse state, output object reference |
| Match worker | User/profile version and job IDs | Calculates explainable match against verified facts | Match score and evidence references |
| Packet worker | Application ID | Creates job-specific resume and question answers grounded in facts | Packet, answer evidence, missing-question rows |
| Batch fanout | Batch ID | Creates or claims one item task per selected job | Independent batch item/application rows |
| Submission worker | Application ID and consent version | Rechecks all policy and employer requirements, submits once when safe | Attempt result, receipt, application state |
| Reconciliation worker | Unconfirmed application ID | Checks source/ATS evidence without repeating an uncertain submission | Confirmed, still unknown, or manual-action result |
| Maintenance worker | Time window and tenant/source ID | Expires stale listings, cleans temporary objects, applies retention and retry budgets | Audited maintenance events |

Each worker has a narrow purpose. A worker calls the same service/domain rules as the HTTP API; it does not implement its own copy of application validation.

## Task delivery

The first durable queue is NATS JetStream. It stores messages and redelivers work if an explicit acknowledgment does not arrive. This is at-least-once processing, so duplicate delivery is expected and each consumer must be idempotent. [JetStream delivery model](https://docs.nats.io/concepts/jetstream).

Use durable pull consumers and bounded fetch batches. API handlers do not publish directly to the broker before a database commit.

### Event payload

Queue messages contain identifiers, version information, and trace context only. They do not contain resume text, full application answers, OAuth tokens, ATS passwords, or raw source documents.

~~~json
{
  "event_id": "uuid",
  "event_type": "application.submit.requested",
  "schema_version": 1,
  "aggregate_type": "application",
  "aggregate_id": "uuid",
  "tenant_id": "uuid",
  "created_at": "RFC3339 timestamp",
  "traceparent": "optional W3C trace context",
  "payload": {
    "application_id": "uuid",
    "consent_version": 4
  }
}
~~~

For global discovery events, tenant_id is null and the source/job ID is in payload. Consumers always reload authoritative data from PostgreSQL.

### Reliable database-to-queue delivery

1. The service starts a PostgreSQL transaction.
2. It saves the business record and an outbox event in that same transaction.
3. The transaction commits.
4. The outbox publisher sends the event to JetStream with the outbox event ID as the message ID.
5. After broker acknowledgment, the publisher marks the outbox row published.
6. A consumer records the event ID in its inbox table, handles the task, commits its state changes, then acknowledges the message.
7. If a process crashes between commit and acknowledgment, the broker redelivers it. The inbox record makes the repeat a no-op or safe continuation.

This avoids losing work between “database saved” and “message published.” It does not claim exactly-once effects at external employers.

## Subject and consumer plan

| Queue subject | Main consumer | Example tasks |
|---|---|---|
| discovery.source.sync | source-sync | Initial backfill, incremental poll, operator refresh |
| discovery.job.normalize | job-normalizer | Normalize a stored provider observation |
| discovery.job.index | search-indexer | Update or remove a search document |
| profile.resume.parse | resume-parser | Parse one immutable resume version |
| matching.calculate | matching-worker | Recompute a user/job match |
| application.packet.prepare | packet-worker | Generate and validate one evidence-backed packet |
| application.batch.fanout | batch-worker | Create per-job application workflows |
| application.submit | submission-worker | Submit one eligible application |
| application.reconcile | reconciliation-worker | Resolve an ambiguous employer response |
| maintenance.execute | maintenance-worker | Expiration, cleanup, retention, and repair |

Use separate streams/retention for discovery, application, and maintenance work so a large source backfill cannot block an application queue. Stream names and retention are deployment configuration and are versioned with infrastructure changes.

## Scheduler and source leasing

The scheduler selects sources where enabled=true and next_poll_at <= now. It claims each source with lease_owner and lease_until using a short transaction and row locking. It commits the lease before network I/O.

After a successful page is committed, the worker stores the next cursor and sets next_poll_at according to the source's configured interval. On retryable failure, it schedules a retry with bounded exponential backoff and jitter. On permanent failure or exhausted budget, it marks the source degraded and sends an operational alert.

The worker never holds a PostgreSQL transaction open during an HTTP request.

## Worker processing rules

- Every task has a deadline and context cancellation.
- Concurrency is bounded by worker pool size, provider limit, and tenant fairness limit.
- Each employer/source has its own token bucket and in-flight limit.
- Queue delivery count and retry budget are finite.
- Workers gracefully stop fetching new messages on shutdown, finish or release current leases, persist state, then acknowledge only completed work.
- A worker never sets SUBMITTED before receipt evidence is persisted.
- PII is loaded only when needed and is not logged.
- Long tasks expose progress in PostgreSQL, not an in-memory-only map.
- Search reindexing and discovery sync can be paused independently of submission work.

## Retry decisions

| Result | Worker action |
|---|---|
| DNS or connection timeout before request was sent | Retry within the task budget |
| Provider 429 | Honor Retry-After if present; slow that provider/source token bucket |
| Provider 5xx before any side effect is possible | Bounded retry with jitter |
| Invalid credential or revoked permission | Stop retrying; mark source as credential-required |
| Malformed provider response | Quarantine the response hash and mark the source degraded |
| Validation error in an application packet | Pause for the user; do not retry AI generation endlessly |
| Employer explicitly rejects the request before submission | Permanent failure or user correction depending on response |
| Timeout after employer may have received a submit request | Mark submission-unconfirmed; reconcile; do not blind retry |
| Duplicate queue message | Deduplicate by consumer + event ID and return the existing result |

A dead-letter queue is for exhausted or malformed tasks and preserves only sanitized identifiers and error class. Replays require an operator action and a documented reason.

## Handling 500 user-selected applications

A batch is a parent record, not one large external request.

1. API stores the batch and items in a single bounded transaction.
2. API responds with HTTP 202 and a batch ID.
3. Batch fanout creates one application task for each item.
4. Independent workers process eligible items under ATS, tenant, and user limits.
5. Each item can succeed, pause, fail, or require reconciliation independently.
6. Batch progress reports totals and per-item states.

The system can acknowledge a 500-item workflow quickly when capacity permits. Completing 500 external requests in ten minutes requires an average of at least 0.83 confirmed submissions per second across the whole batch, before retries or pauses. Employer rate limits and different ATS speeds may make that impossible for a particular batch. External submission completion depends on employer limits and adapter support. The API must not report 500 successes merely because 500 tasks were queued; it returns the true result for every item.

## 100,000-source capacity plan

Sharding is by source ID, not company-specific code. Add scheduler partitions and worker replicas horizontally. Track queue age and source poll lag by provider and shard.

A simple lower bound shows the work: 100,000 independent sources polled every five minutes require at least 333 requests per second if each sync fits in one request. Pagination and retries increase that. The provider's own quotas can make the target impossible at a given interval. The scheduler therefore applies provider-aware polling tiers and reports actual freshness rather than forcing all sources onto one interval.

Autoscaling signals:

- age of oldest task by stream/consumer
- pending message count
- source sync lag by priority tier
- 429 and timeout rates per provider
- database pool saturation
- search indexing lag
- application submission queue age by connector
- CPU, memory, and in-flight task count

Do not scale workers beyond the provider, database, or employer's allowed rate.

## Monitoring and operator controls

Every task carries event ID, source/application ID, tenant when applicable, connector version, attempt count, and trace ID. Logs exclude candidate answers, resume content, cookies, bearer tokens, and provider credentials.

Operators need controls to:

- pause one source or connector
- disable new submissions without blocking job discovery
- lower provider concurrency
- requeue a safe failed task
- hold a dead-letter task for inspection
- reconcile one unconfirmed application
- review per-source and per-company capability/health
- inspect queue age and worker heartbeat

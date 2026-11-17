# OneRole backend, explained simply

**Status:** design for review. The service code and database migrations have not been implemented yet.

## The short version

OneRole has two main jobs:

1. **Find real jobs.** It checks configured employer and job-source feeds, records what each source actually says, and makes the results searchable.
2. **Submit applications safely.** When a user applies, OneRole checks the user's consent, resume evidence, application answers, and employer support. A worker submits only when that company's flow is configured and allowed.

The app does not pretend that every company has a live feed or allows automated submissions. Each job shows its source freshness and application capability.

## Backend picture

~~~mermaid
flowchart LR
    UI[OneRole web app] --> API[Go API]
    API --> DB[(PostgreSQL)]
    API --> CACHE[Redis: short-lived cache and limits]
    DB --> OUTBOX[Outbox publisher]
    OUTBOX --> QUEUE[NATS JetStream]
    QUEUE --> DISCOVERY[Discovery workers]
    DISCOVERY --> SOURCES[Approved employer job feeds]
    SOURCES --> DISCOVERY
    DISCOVERY --> DB
    DB --> INDEXER[Search indexer]
    INDEXER --> SEARCH[(Optional OpenSearch projection after measurement)]
    API --> SEARCH
    QUEUE --> APPLICATION[Application workers]
    APPLICATION --> ATS[Supported employer or ATS flow]
    ATS --> APPLICATION
    APPLICATION --> DB
    APPLICATION --> FILES[Private resume storage]
    QUEUE --> EMAIL[OneRole email notification worker]
    EMAIL --> DB
~~~

PostgreSQL is the durable record. NATS holds work until a worker processes it. Redis is only a cache and coordination helper. PostgreSQL full-text and trigram search is the first search implementation. OpenSearch is an optional, rebuildable projection added only if measured query load, faceting, or relevance requires it. Resume files are private objects in S3-compatible storage.

## What each part does

| Part | What it does | What it receives | What it produces | If it fails |
|---|---|---|---|---|
| Go API | Accepts sign-in, job search, profile, and Apply requests | Authenticated HTTP request | Validated response or durable workflow ID | Returns a clear error; it does not claim background work succeeded |
| Company/source registry | Tells OneRole which employer feeds exist and what they support | Approved company and source configuration | Active connector schedule and capability status | Source is marked disabled or degraded |
| Discovery workers | Read real listings and changes from sources | Source ID, cursor, rate limit, schedule | Normalized job records and sync evidence | Retry if safe; retain last good data and show it as stale |
| PostgreSQL | Stores authoritative company, job, tenant, and application state | Transactions from API and workers | Durable rows, constraints, outbox events | API readiness fails or work waits; data is not replaced with demo jobs |
| Search | PostgreSQL full-text/trigram first; optional OpenSearch projection after measurement | Job updates | Search results and facet counts | Search can run from PostgreSQL; rebuild optional projection |
| Application workers | Prepare packets, validate them, submit authorized items, and reconcile results | Durable application task | State transition, receipt, or a specific pause/failure reason | Retry safe failures; never blindly repeat an uncertain employer submission |
| NATS JetStream | Keeps asynchronous tasks and redelivers unacknowledged work | Outbox publisher events | Durable messages for worker groups | Redelivery is expected; consumers deduplicate by event ID |
| Redis | Handles short-lived rate counters and hot cache entries | Cache/rate-limit operations | Temporary values | Cache misses or reduced rate-limit availability; PostgreSQL remains authoritative |
| Object storage | Holds original resumes and derived files privately | Validated file upload | Opaque object reference and checksum | Resume task pauses; the original is never replaced by a generated file |
| Observability | Shows API health, source lag, queue age, and submission outcomes | Traces, metrics, sanitized logs | Dashboards and alerts | Operational gaps are visible; secrets and resume contents stay out of logs |

## The three important flows

### A. Find and publish a job

1. A source is due for a sync.
2. A discovery worker calls that source using its approved connector and rate limit.
3. The worker saves each source observation and advances its checkpoint only after the page is durably stored.
4. Normalization maps source fields into OneRole fields and keeps evidence for uncertain attributes.
5. The search indexer updates the search document.
6. The API returns a listing with source, published time, first-seen time, last-seen time, and source freshness.

### B. Search with filters

1. The UI sends its selected filters to the job-search API.
2. The API validates the filter values and queries the search index.
3. The API returns matching real jobs, the applied filters, a cursor, and counts for each filter option.
4. The UI shows unknown or stale data as such. It does not silently turn missing facts into “no.”

### C. Apply to a job

1. The user clicks Apply, or has previously enabled a specific auto-submit policy.
2. The API records consent, the application, the idempotency key, and an outbox task in one database transaction.
3. A worker loads the current job, resume facts, packet, policy, and employer connector capability.
4. It pauses only that application if required facts are missing, an attestation needs review, or the employer flow is unsupported.
5. For a supported flow, the worker makes the employer request under that connector's rate limit.
6. The worker stores the employer response evidence before acknowledging the queue message.
7. OneRole marks the application submitted only when the response supplies sufficient confirmation. An unclear response becomes submission-unconfirmed and is reconciled before any retry.

## Runtime modes and code layout

There will be one Go entry point at the repository root. The same built program runs in API, worker, or local combined mode:

- APP_MODE=api starts HTTP routes.
- APP_MODE=worker starts schedulers and queue consumers.
- APP_MODE=all starts both for local development.
- The explicit migration command runs through the same root entry point.

The package direction is controller → service → repository → database. Workers call the same services as API handlers. External job boards, employer ATS products, AI, and storage are connectors behind interfaces.

## What “100,000 companies” means

OneRole can keep a registry for 100,000 or more company records. The number of companies in the registry is not the number of authorized, live feeds, and neither number equals the number of employers that allow automatic submission.

Each company and each job source has its own state:

- Is there an authorized, reachable job source?
- When did it last sync successfully?
- Does it have public job discovery?
- Can OneRole read application questions?
- Is application submission supported?
- Are company-specific credentials required and available?
- Which rate limits and terms apply?

OneRole will show these facts instead of labeling every registered company “live.”

## Local development to production

- **Local:** PostgreSQL, NATS JetStream, Redis, private S3-compatible storage, and the OneRole Go binary run in Docker Compose. A developer can opt into read-only calls to a real public job feed.
- **Staging:** same container image, test identity providers, a real approved job feed, and an ATS sandbox or employer-approved test account where available.
- **Production:** managed/high-availability database, secrets manager, private object storage, queue cluster, search cluster, monitoring, backups, and connector-specific rollout.
- Production application submission stays disabled for a company until its adapter, access method, field requirements, consent scope, and result-evidence rules have been verified.

## Related details

- [Company and source configuration](company-configuration.md)
- [Job discovery](job-discovery.md)
- [Worker implementation](worker-implementation.md)
- [Application submission](submission-engine.md)
- [Database architecture](database-architecture.md)
- [API contract: OAuth/JWT, filters, and endpoints](api-contract.md)
- [Database table guide](database-table-guide.md)
- [Proposed Database schema](database-schema.md)
- [Real data and coverage rules](../operations/live-data-and-coverage.md)

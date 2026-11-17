# OneRole database architecture

**Status:** design proposal. The schema file beside this document is a draft schema, not an applied migration.

## Simple explanation

PostgreSQL is the official record of what OneRole knows. It stores company/source configuration, job records, users, resumes, application workflows, receipts, and audit history.

PostgreSQL full-text/trigram is the initial search implementation. OpenSearch is an optional, rebuildable projection added only if measurement justifies it. NATS is the durable work queue. Redis holds temporary counters and cache values. Private object storage holds resume files. If Redis or OpenSearch is lost, OneRole can rebuild or degrade safely from PostgreSQL. It must never reconstruct an application receipt from a cache.

## Which store owns which data

| Store | Stores | Does not own |
|---|---|---|
| PostgreSQL | Companies, sources, normalized jobs, sync cursors, user/tenant records, resumes metadata/facts, application state, consent, attempts, receipts, outbox, audit, usage | Resume binary files or search-only temporary state |
| PostgreSQL full-text/trigram | Initial canonical search implementation | External queue or business state |
| Optional OpenSearch | Rebuildable projection for the global job catalog if measured workload requires it | Canonical job, user, resume, or application records |
| NATS JetStream | Durable asynchronous task events with bounded retention | The business state of an application |
| Redis | Short-lived cache, rate counters, and coordination leases where needed | Submission history, consent, source checkpoint, or receipt |
| S3-compatible object storage | Original resumes and derived documents in private tenant-scoped keys | Searchable metadata or authorization rules |
| Secret manager | Google/GitHub client secrets, connector credentials, database passwords, signing keys | Application records or job data |

## Logical database areas

### Global discovery area

- Company identity and aliases.
- One row per configured company source/board.
- Source health and sync-run history.
- One normalized global job row.
- Source observations linking external job IDs to normalized jobs.
- Normalized locations and filter facts with source evidence.

Global job rows can be returned to any authenticated user because they are public job data. Their source terms, source URL, and observation timestamps remain attached.

### Tenant-owned applicant area

- Tenant memberships and preferences.
- Resume versions, extracted facts, and evidence references.
- Match results and saved jobs.
- Application packets and answers.
- Applications, batches, consent, policies, attempts, receipts, usage, and audit events.

Every row in this area has tenant_id. Every repository method also receives tenant context. Foreign keys include tenant_id where practical so a tenant A row cannot reference a tenant B resume or application packet.

### Operational area

- Transactional outbox.
- Consumer inbox/deduplication.
- Idempotency records.
- Connector leases, health, retry counts, and checkpoints.

These tables are service-only. They do not expose arbitrary queue payloads to the browser.

## Transaction boundaries

### Creating an application workflow

In one PostgreSQL transaction:

1. Check tenant membership and user ownership.
2. Check the supplied idempotency key and request hash.
3. Insert application/batch item and audit event.
4. Insert an outbox event containing identifiers only.
5. Commit.
6. A separate outbox publisher delivers the event to JetStream.

No API transaction waits for an AI model or employer portal.

### Saving a source page

For each source page:

1. Start a bounded transaction.
2. Upsert source observations and normalized jobs.
3. Save index-update outbox rows.
4. Save the next source cursor only if the page data committed.
5. Commit.
6. Publish index events asynchronously.

This keeps the checkpoint from moving past uncommitted job data.

### Recording submission evidence

The worker persists attempt outcome, sanitized response evidence, state transition, and application event together. It acknowledges the queue message only after that transaction commits.

## Tenant isolation

1. JWT middleware validates the user identity.
2. The auth service sets transaction-local app.user_id and checks membership.
3. After selecting an authorized tenant, the service begins/uses a transaction with transaction-local app.tenant_id.
4. Repository queries always filter by tenant_id.
5. PostgreSQL row-level security provides another barrier for tenant-owned tables.
6. Workers reload tenant ID from the durable application record and set context for that task.

Use a restricted runtime database role that does not own tables and does not have BYPASSRLS. The migration role is separate. PostgreSQL table owners normally bypass row security unless forced, so production policies must be tested with the actual runtime role. [PostgreSQL row security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html).

Do not set tenant state for the whole database connection. Use transaction-local settings because connections are pooled and reused.

## Search and faceting

Start with PostgreSQL full-text/trigram search as required by ADR-0001. Add OpenSearch only when measured query volume, relevance, location-radius performance, or facet latency justifies its operational cost.

The job index contains:

- job_id and company_id
- title, description text, normalized role and skill terms
- company display name
- locations and workplace mode
- published_at and first_seen_at
- degree, experience, visa, job type, and employment type facts
- source status, last_seen_at, capability and data-quality fields

The index contains no resume data, answer text, or tenant-specific application status. Every index document is rebuildable from PostgreSQL. Index lag is measured and shown. A user-specific application badge can be joined after search using the returned job IDs and the tenant-scoped application table.

## Indexing and 100,000 companies

Initial PostgreSQL indexes:

- company_sources on enabled, next_poll_at, source_id
- job_source_observations unique on source_id + external_job_id
- jobs on company_id + status
- jobs on published_at and first_seen_at
- job_locations on country/region/city and spatial coordinates
- job_filter_facts on attribute_code + normalized_value + job_id
- applications on tenant_id + user_id + status + updated_at
- outbox on unpublished_at/created_at
- inbox unique on consumer_name + event_id

Do not make one table per company. Do not partition by company ID. First benchmark the real data volume, query plans, vacuum, and index size. If append-only observation-event, application-event, audit, or usage tables become too large, partition those tables by time with planned retention. PostgreSQL partitioning can help time-range retention and pruning, but over-partitioning increases planning and memory overhead. [PostgreSQL table partitioning](https://www.postgresql.org/docs/current/ddl-partitioning.html).

The company registry can scale independently from live source traffic. Source schedulers and workers shard by source ID. PostgreSQL keeps one source cursor and one sync-run record per source.

## Availability and backup plan

- Local PostgreSQL uses a named persistent volume.
- Staging and production use managed PostgreSQL with backups and point-in-time recovery.
- Search indexes are rebuilt from PostgreSQL after loss.
- Queue retention is long enough to recover bounded worker outages; business state remains in PostgreSQL.
- Resume object keys are private, encrypted, tenant-scoped, and backed up under a documented retention plan.
- Backups are encrypted, access controlled, and restore-tested.
- Migration steps are additive first, then data backfill, then contract/cleanup in a later release.
- Run migrations explicitly through a release step, never automatically on every API replica.

## Proposed schema file

[database-schema.md](database-schema.md) lists the proposed MongoDB collections, relationships, constraints, and core indexes. It is design input for review, not a migration to run against an existing database.

# OneRole backend architecture guide

Start with the picture below. Each step has a linked guide with the details and failure behavior.

~~~text
REAL COMPANY SOURCE
    ↓ scheduler + discovery worker
Go connector → normalize + deduplicate → PostgreSQL (official records)
                                         ↓ outbox → NATS JetStream
                                         ↓                     ↓
                               PostgreSQL search       Go workers
                                         ↓              prepare / submit
                               Optional search index             ↓
                                                         approved employer flow
Browser UI ← Go API ← JWT auth                              ↓
                                                       receipt + status
~~~

## In a few words

- **Go API:** checks sign-in and handles quick requests.
- **Go workers:** do slow or repeated work in the background.
- **PostgreSQL:** remembers the official state: sources, jobs, user choices, applications, and proof.
- **NATS JetStream:** carries durable work to workers; duplicate delivery is expected and handled safely.
- **Search:** PostgreSQL full-text/trigram first; optional OpenSearch is a rebuildable copy of public job fields if measurements justify it.
- **Redis:** temporary cache and rate counters. Losing it must not lose jobs or application receipts.
- **Private object storage:** holds resume files; PostgreSQL holds their private references.
- **Secret manager:** holds OAuth and employer integration credentials. The database stores references, not secret values.

## Read the architecture in this order

1. [Backend overview](overview.md) — the whole system and its main request flows.
2. [Company and source configuration](company-configuration.md) — how real company job feeds are registered and reviewed.
3. [Job discovery](job-discovery.md) — initial loads, frequent updates, freshness, filters, and last-minute listings.
4. [Go worker implementation](worker-implementation.md) — queues, leases, retries, scaling, and 500-item batches.
5. [Application submission](submission-engine.md) — consent, evidence-based answers, employer capability, and verified receipts.
6. [API contract](api-contract.md) — Google/GitHub sign-in, JWT sessions, endpoints, filters, and errors.
7. [Database architecture](database-architecture.md) — what each storage system owns and how tenant data is isolated.
8. [Database table guide](database-table-guide.md) — what each proposed table means.
9. [Proposed Database schema](database-schema.md) — detailed draft document schema, not an applied migration.
10. [Real-data and coverage rules](../operations/live-data-and-coverage.md) — what counts as live, fresh, and truly submitted.

## The three main user flows

**Find a job:** source → discovery worker → PostgreSQL → search index → API → user.

**Apply:** user click → API transaction → durable queue → packet/preflight worker → permitted employer connector → persisted receipt → application status and notification.

**Pass:** user click → API stores a user-specific pass. The listing remains open for other users and is not marked closed at the employer.

## Scope and honesty

OneRole can be designed to store a registry of 100,000+ companies and scale workers horizontally. Every company still needs a real, permitted source to count as live. Every automated submission needs a separately verified employer path and the required authorization. The architecture never fills gaps with fake jobs or claims success when the employer response is uncertain.

The repository has design documents and a local prototype. The Go service, production database, real connectors, and submission workers are not implemented yet.

## Existing repository contracts

This repository also preserves the original backend foundation and API artifacts:

- [Backend architecture notes](backend.md)
- [Logical data model](data-model.md)
- [API contract](api-contract.md)
- [OpenAPI 3.1 draft](../../api/openapi.yaml)
- [Backend foundation decision](../adr/0001-backend-foundation.md)

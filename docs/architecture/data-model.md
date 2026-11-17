# OneRole data model

**Status:** logical model for review; not yet a migration or implemented schema.

PostgreSQL is the system of record. UUIDs are generated server-side. Timestamps are UTC `timestamptz`. Tenant-owned foreign keys include `tenant_id` so a row cannot reference an object owned by a different tenant. Use optimistic version columns for mutable workflows and append-only events for audit history.

## Ownership classes

| Class | Examples | Tenant isolation |
|---|---|---|
| Global catalog | companies, source definitions, normalized jobs, source observations | Shared only when source terms permit it; read scope and provenance are explicit. |
| Tenant-owned | profiles, resumes, facts, matches, saved jobs, packets, applications, consent, usage | Every row includes `tenant_id`; composite constraints, repository predicates, and RLS apply. |
| Operational | outbox, inbox, sync runs, connector health, queue work | Tenant ID where work belongs to a tenant; global source jobs may be tenant-null by design. Access is service-only. |

## Identity and profile tables

- users: id, nullable normalized email, email verification state, display name, status, timestamps; enforce uniqueness on non-null verified email only. Stable external identity belongs in auth_identities.
- auth_identities: user_id, provider, immutable provider subject, verified email metadata, linked_at; unique (provider, subject); never use email as the identity key and never store provider secrets here.
- oauth_transactions: provider, hash of one-time state, encrypted short-lived PKCE verifier, OIDC nonce where required, browser binding, fixed return route, expiry and consumed_at. Delete/expire completed transactions quickly.
- sessions: user_id, session family, hashed rotating refresh token, expiry, last-used/revoked times, security metadata, and token-version/session ID used by access-token revocation checks. Store hashes, never raw refresh tokens or access JWTs.
- `tenants`: `id`, name, slug, plan/status, timestamps.
- `tenant_memberships`: `(tenant_id, user_id)`, role, status, created/updated; unique tenant/user pair.
- `user_profiles`: `(tenant_id, user_id)`, location, work preferences, salary preferences, job exclusions, profile version.

## Company and discovery tables

- `companies`: canonical employer identity, display name, normalized domain, country/region and verification state.
- `job_sources`: company/source type, external board identifier, authorized base URL, enabled state, capabilities, rate policy, polling schedule, last success/error.
- `source_sync_runs`: source ID, start/end, outcome, discovered/updated/closed counts, cursor/checkpoint before and after, sanitized error class.
- jobs: global normalized job. Fields include employer, title, description, requirements, skills, salary/currency, status, normalized search document, publication time and its quality, first/last observed times, and typed experience minimum/maximum where explicitly supported.
- job_locations: one or more normalized job locations, country/region/city/geopoint, workplace mode, and provenance; a remote job may have no fixed city.
- job_attribute_values: indexed normalized categorical values for role family, career level/job type, employment type, education level/requirement strength, and visa-sponsorship status; include source observation, confidence, taxonomy version, and extraction time. Represent unknown as absence plus explicit facet counts, not as a guessed value.
- `job_source_observations`: job ID, source ID, external requisition ID, canonical URL, source publication/update times, first/last seen, source status, payload checksum, provenance. Unique `(source_id, external_job_id)` where present; unique canonical source URL within source as appropriate.
- `source_checkpoints`: source ID, opaque connector cursor and update time. Checkpoints advance only after durable ingestion of the represented page.

Keep publication time nullable and separate from first observation. Do not store a raw source payload indefinitely unless the source permits it and retention is justified; retain a checksum and sanitized provenance by default.

## Filter behavior and indexes

Searchable filter attributes are normalized at ingestion/enrichment and retain their evidence. Index location country/region/city and geography; categorical values by (attribute, value, job_id); company/job status; publication/first-seen timestamps; and full-text/trigram search documents. Experience bounds use numeric indexes when query plans justify them. Keep separate values for published time and first observation.

The API facets include option counts and unknown/unclassified counts. Multi-select within a facet is OR; separate facets are AND; excluded terms/company IDs are subtractive. Company aliases resolve to a canonical company record. Visa sponsorship, degree, experience, role, job type, workplace, and employment type never become confident filter facts without provenance. Rebuild search projections from PostgreSQL so an index outage delays search freshness but does not lose canonical job data.

## Matching and resume tables

- `saved_jobs`: tenant/user/job relationship; unique `(tenant_id, user_id, job_id)`.
- `job_matches`: tenant/user/job, score, algorithm version, matched/missing requirements, evidence references, explanation, computed time; unique per user/job/algorithm version.
- `resumes`: tenant/user logical resume, current version, display name, status, created/deleted times.
- `resume_versions`: immutable version, private object key, MIME type, size, SHA-256, parser state, original/derived kind, parent version, creation time.
- `resume_facts`: tenant/user/resume version, fact type, normalized value, verification status, source location, confidence, user-confirmation timestamp.
- `resume_fact_evidence`: fact ID plus source page/section/character span or profile field provenance.
- `profile_facts`: user-confirmed structured facts independent of a resume file, with verification and history.

Original files are immutable. Tailored documents are separate derived versions with links to the source facts and job packet that caused generation. File objects use opaque keys such as `tenant/{tenant_uuid}/resume/{resume_uuid}/{version_uuid}` and remain private.

## Application preparation and workflow tables

- `application_packets`: tenant/user/job, source resume version, packet version, generation status, completeness, confidence summary, prompt/model metadata, created/validated/approved times.
- `application_answers`: packet ID, employer question ID/text, normalized field key, answer value, sensitivity/attestation class, validation state, answer source (`user`, `evidence`, `draft`), version.
- `answer_evidence`: answer ID, fact ID/profile field/source reference, quote/span metadata; no unsupported factual claim may be marked validated.
- `pending_questions`: tenant/user/application/answer ID, missing field, reason, status, response and timestamps.
- `applications`: tenant/user/job, packet version, lifecycle status, submission mode, authorized consent record, current version, capability snapshot, timestamps. Unique guard against duplicate active applications by `(tenant_id, user_id, job_id)`; allow a new attempt only through an explicit reapply policy after checking prior state.
- `application_events`: append-only state transition and consequential action history, actor type/ID, previous/new state, request ID, sanitized metadata, timestamp.
- `application_batches`: tenant/user, selected job/application count, requested mode, consent reference, idempotency key, state/counts, created/cancelled times.
- `application_batch_items`: batch ID, application ID, per-item result/status and timestamps; unique `(batch_id, application_id)`.
- `submission_attempts`: application ID, adapter and version, attempt number, request/idempotency correlation, started/finished time, outcome classification, retryability, sanitized response metadata, failure code.
- `submission_receipts`: attempt/application, employer reference/confirmation number, receipt type, evidence hash/object ref where allowed, observed time, verification status.

A successful submission is not inferred from HTTP 2xx alone. Store only the minimum evidence needed to prove/ reconcile the result; redact credentials and personal form values from response metadata.

## Consent, audit and operations tables

- `consent_records`: tenant/user, policy version, scope, accepted/revoked times, source UI/action, client request ID. Keep historical records; do not mutate old consent to imply a new scope.
- `automation_policies`: tenant/user, enabled state, allowed job filters, supported connector allowlist, per-run/daily limits, review requirements, version and updated time.
- `audit_events`: append-only tenant/user actor, action, object type/ID, request ID, timestamp, minimal sanitized metadata. Sensitive value contents are excluded.
- `usage_ledger`: tenant, metered event, quantity, idempotency/source reference, timestamp; derived aggregates may be cached but ledger remains authoritative.
- `tenant_quotas`: tenant, limit name, configured limit, period, effective time.
- `outbox_events`: event ID, aggregate ID/type/version, event type, payload containing IDs only, created/published times, publish attempts and last error class.
- `inbox_deduplication`: consumer, event ID, processed time; unique `(consumer, event_id)`.
- `connector_health`: source/adapter ID, health state, success/error times, lag, rolling counters and circuit-breaker state.

## Key constraints and indexes

- Every tenant-owned unique key includes `tenant_id`.
- Foreign keys for tenant entities include `(tenant_id, referenced_id)` to prevent cross-tenant references.
- Index global jobs on `(status, published_at DESC, id)` and `(first_seen_at DESC, id)`; add GIN full-text/trigram indexes based on measured query patterns.
- Index source observations on `(source_id, external_job_id)` and `(job_id, last_seen_at DESC)`.
- Index tenant workflows on `(tenant_id, user_id, status, updated_at DESC)` and `(tenant_id, created_at DESC)`.
- Index outbox pending rows on `(published_at, created_at)` with a partial index for unpublished rows; index inbox unique key.
- Use unique idempotency keys scoped by tenant, actor and operation. Store request hash plus response/workflow ID so key reuse with a different payload returns conflict.
- Use `CHECK` constraints for enum/range invariants that must survive all write paths, plus service-layer transition validation.

## Tenant enforcement

1. Authenticate the user.
2. Resolve the active tenant membership and role.
3. Begin a transaction and set transaction-local tenant context.
4. Repository methods require the tenant ID in their signatures and query predicates.
5. Enable RLS on tenant-owned tables and use a restricted application DB role. Explicitly account for PostgreSQL owner/superuser bypass behavior; test with the real runtime role.
6. Workers load the tenant ID from a durable record, re-validate ownership, set tenant context, and clear it at transaction end.

Global jobs are not exposed through a tenant-owned query join that leaks profile or application details. Search can return global listings, while personalization joins and scoring remain tenant-scoped.

## Retention and deletion

Deleting a tenant/user starts a tracked deletion workflow: revoke sessions and connector credentials, mark new work blocked, delete private objects and derived packets, remove or anonymize tenant-scoped rows according to retention requirements, and record completion. Backups expire on their defined lifecycle; deletion must not be described as immediate from immutable backups. Keep only legally required audit/receipt evidence, minimized and access-controlled.

Before implementation, choose concrete retention periods and legal requirements for each data class. No policy is inferred from this logical schema.

# OneRole database tables, in plain language

**Status:** guide to the proposed schema. See [database-schema.md](database-schema.md); it is not an applied migration.

## How to read the database

PostgreSQL is the durable source of truth. A row is the current record. Event and attempt rows preserve what happened over time. Foreign keys and unique constraints prevent many accidental duplicates. Tenant-owned rows carry `tenant_id`; row-level security and service checks keep one user's private data separate from another's.

## Accounts and sign-in

| Table | Purpose |
|---|---|
| `tenants` | Workspace boundary for private user data |
| `users` | OneRole user identity and account state |
| `tenant_memberships` | Which users can access which tenant |
| `auth_identities` | Stable Google/GitHub provider subject linked to a OneRole user |
| `auth_oauth_transactions` | Short-lived state/PKCE/nonce data used to safely finish OAuth |
| `auth_sessions` | Hashed refresh-token sessions, expiry, rotation, and revocation |

## Company catalog and real job sources

| Table | Purpose |
|---|---|
| `companies` | Canonical employer identity, domain, and verification state |
| `company_aliases` | Alternate names/domains used to avoid duplicate companies |
| `company_sources` | One actual job board/feed/account, capabilities, schedule, and health |
| `company_source_secrets` | Secret-manager reference and credential metadata; not the secret itself |
| `source_sync_runs` | One record per attempt to check a source, including outcome and counts |
| `source_checkpoints` | Provider cursor or continuation token for the next page/sync |
| `jobs` | Canonical normalized job currently searchable |
| `job_source_observations` | Evidence tying a canonical job to external IDs, URLs, and source timestamps |
| `job_locations` | Searchable normalized job locations |
| `job_filter_facts` | Evidence-backed degree, experience, sponsorship, role, and other filter facts |

## User profile and resume evidence

| Table | Purpose |
|---|---|
| `user_profiles` | User preferences and profile settings |
| `saved_jobs` | Jobs this user saved or passed |
| `job_matches` | Explainable user-to-job match result and source profile version |
| `resumes` | Resume identity and private object reference |
| `resume_versions` | Immutable uploaded or tailored resume versions |
| `resume_facts` | Structured candidate facts, review status, and version |
| `resume_fact_evidence` | Location/pointer showing where a fact came from |
| `consent_records` | What the user approved, for which purpose, and when |
| `automation_policies` | Versioned auto-apply boundaries and user opt-in |

Resume files live in private object storage; the database stores their metadata and references. A generated answer must link to facts, and tailoring creates a new version instead of replacing the original.

## Application workflow and proof

| Table | Purpose |
|---|---|
| `applications` | Current state of one user applying to one job |
| `application_packets` | Exact resume/answer packet prepared for that job |
| `application_answers` | One answer to one employer question and its review state |
| `application_answer_evidence` | Facts that support a generated or approved answer |
| `pending_questions` | Missing or review-required questions that pause one application |
| `application_batches` | Parent record for a multi-job request |
| `application_batch_items` | Independent state/result for each job in a batch |
| `application_events` | Append-only timeline of state changes |
| `submission_attempts` | Each external submit attempt, adapter version, and result class |
| `submission_receipts` | Employer/ATS confirmation reference or sanitized evidence |
| `notification_deliveries` | OneRole email delivery state for the candidate, including idempotent retries |
| `usage_ledger` | Auditable application and feature usage counts |

An application state is not proof of employer acceptance by itself. The receipt/attempt evidence is persisted first; only then can the application become `SUBMITTED`.

After OneRole has verified the employer result, an email worker can send a OneRole status notice and record its delivery. This is separate from an email sent by the employer; OneRole cannot guarantee that the employer sends one.

## Reliable background work and audit

| Table | Purpose |
|---|---|
| `idempotency_keys` | Makes repeated API requests return the first workflow |
| `outbox_events` | Stores work in the same transaction as a business change, ready for queue publishing |
| `inbox_deduplication` | Remembers processed queue event IDs so redelivery is safe |
| `audit_events` | Records important security and user actions without storing secrets |

The queue is not the source of truth. If a message is delivered twice, PostgreSQL constraints and inbox records let the worker resume safely. External employer systems do not provide universal exactly-once guarantees, so ambiguous submissions are reconciled instead of blindly repeated.

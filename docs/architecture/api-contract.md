# OneRole API contract

**Status:** design contract; [OpenAPI draft](../../api/openapi.yaml) covers the first auth, job-search, and application workflow slice. Public API version starts at /api/v1.

## Common rules

- JSON requests and responses use UTF-8. Timestamps are UTC ISO 8601.
- Authentication produces a user identity; the active tenant is authorized from membership. A body field cannot grant tenant access.
- Errors use a consistent error object with code, message, request ID, and safe details.
- Long-running work returns 202 Accepted with an ID and status URL. This means OneRole accepted the workflow, not that the employer accepted an application.
- Use opaque cursor pagination; default limit 25, maximum 100. Sort keys include a stable ID tie-breaker.
- Require an Idempotency-Key for application creation, submit, batch creation, approval, and other consequential writes. Scope it to tenant + actor + operation. Reusing the same key and body replays the original workflow result; the same key with a different body returns a conflict.
- Tenant membership failures should not reveal whether another tenant's object exists.
- Conditional transitions return a state conflict with the current state and permitted action when safe to disclose.

## Authentication contract

The application shows Continue with Google and Continue with GitHub buttons, backed by GET /api/v1/auth/oauth/google/start and GET /api/v1/auth/oauth/github/start. The backend owns the authorization-code callback and OneRole session.

- Use transaction-specific state and PKCE S256; Google also uses OIDC nonce and validates the signed ID token's issuer, audience, signature, expiry, nonce, and verified email. GitHub identity comes from the authenticated user subject, with only minimum scopes needed for sign-in and verified-email lookup (including user:email only when a private email address must be verified). Never request repository access.
- Redirect URIs are exact registered values; the callback consumes the code/state once and only redirects to a configured frontend route. No open redirect parameter is accepted.
- OneRole then issues its own short-lived signed JWT. Provider tokens are not OneRole API tokens. JWT middleware validates a fixed allowed asymmetric algorithm, key ID, signature, issuer, audience, expiry, not-before, token type, and active session.
- The browser keeps the access JWT in memory and sends Authorization: Bearer. A Secure, HttpOnly, SameSite refresh cookie is rotated on each refresh; its hashed value is stored server-side. Reuse revokes the refresh-token family. Refresh/logout endpoints validate Origin and CSRF protection.
- The JWT carries identity/session claims, not authoritative tenant roles. Every protected request still checks current tenant membership and resource ownership.
- Account linking requires an already authenticated OneRole session plus proving control of the other provider. Matching email strings do not auto-link identities.

## Core API groups

### Discovery

| Method and route | Purpose |
|---|---|
| GET /api/v1/jobs | Basic job search with bounded query parameters and cursor paging. |
| GET /api/v1/jobs/search | Linkable search using query parameters for common filters. |
| POST /api/v1/jobs/search | Full structured search request for all screenshot filters; returns jobs, applied filters, facets, unknown counts, and next cursor. This is read-only. |
| GET /api/v1/jobs/filters | Return filter vocabularies, normalized values, and availability/unknown counts; large company lists are searched separately. |
| GET /api/v1/companies/search | Search canonical company names for the Companies picker. |
| GET /api/v1/locations/suggest | Suggest normalized locations for the Location picker; optional radius uses stored geospatial data. |
| GET /api/v1/jobs/{jobID} | Read normalized listing, field provenance, and source/application capability metadata. |
| GET /api/v1/jobs/recent | Sort by source publication time where known; disclose freshness basis. |
| GET /api/v1/sources | Read source coverage and capabilities available to this client. |
| GET /api/v1/sources/{sourceID}/health | Read last success, last error class, sync lag, and source capability. Admin-only sync triggers are separate. |

### Search filter contract

The full structured body supports: query text; excluded terms/phrases; date preset (last_24_hours, last_7_days, last_30_days, last_60_days, custom, any) with custom publication bounds; locations and radius; workplace modes (remote, hybrid, on_site, unknown); included and excluded company IDs; degree levels and required/preferred strength; maximum experience years plus explicit per-facet include-unknown switches; visa sponsorship (yes, no, unknown, any); role IDs; job type/career level; employment types; sort; cursor; and page size.

Semantics:

- Multiple values inside one facet match any selected value; separate facets are combined.
- Exclude terms and excluded company IDs remove matching jobs.
- A selected publication-date window uses employer/source published time. Recently discovered is independently filtered by first-seen time. Unknown publication time is not rewritten as first-seen time.
- Max Experience compares against the explicit normalized upper bound. Open-ended requirements such as “5+ years” have no upper bound and are shown under open-ended/unknown; users may opt them into results.
- Visa sponsorship and degree options expose unknown. Unknown is never silently treated as no or yes.
- Filter options and counts are derived from indexed normalized job attributes. Result items include enough provenance/data-quality metadata to explain uncertain filters.
- Clicking Apply sends the complete selected filter object once; Clear clears the filter object and restores defaults. Pagination cursors are bound to the normalized filter hash so changing filters invalidates the old cursor.

### Resume and preparation

| Method and route | Purpose |
|---|---|
| `POST /api/v1/resumes/upload` | Create a private upload intent; client uploads through a short-lived signed URL. |
| `GET /api/v1/resumes/{resumeID}/status` | Read async parse state. |
| `GET /api/v1/resumes/{resumeID}/facts` | Read extracted facts with evidence and verification state. |
| `PATCH /api/v1/resumes/{resumeID}/facts/{factID}` | User confirms/corrects a fact; writes an audit event. |
| `POST /api/v1/application-packets` | Start job-specific preparation from an approved resume version. |
| `GET /api/v1/application-packets/{packetID}` | Read packet readiness, facts/evidence, missing questions, and downloadable derived resume. |
| `POST /api/v1/application-packets/{packetID}/validate` | Re-run deterministic schema, evidence, completeness, and policy validation. |
| `POST /api/v1/pending-questions/{questionID}/answer` | Supply a missing fact explicitly; this resumes only the affected workflow. |

### Applications

| Method and route | Purpose |
|---|---|
| `POST /api/v1/applications` | User Apply action: create an application for one job and enqueue preparation. If an approved packet and consent already satisfy the policy, submission may be queued after validation. |
| `POST /api/v1/applications/{applicationID}/submit` | Explicitly authorize submission of a ready application under the stored consent scope. Requires idempotency. |
| `GET /api/v1/applications` | List user's tenant-scoped applications with status/date filters. |
| `GET /api/v1/applications/{applicationID}` | Read current state, capability, pause reason, and next permitted action. |
| `GET /api/v1/applications/{applicationID}/timeline` | Read append-only application events and outcomes. |
| `GET /api/v1/applications/{applicationID}/receipt` | Read sanitized employer evidence where available. |
| `POST /api/v1/applications/{applicationID}/cancel` | Cancel only before the connector's irreversible boundary; otherwise start reconciliation/support handling. |
| `POST /api/v1/applications/{applicationID}/reconcile` | Request safe status reconciliation; never blindly resubmit an ambiguous attempt. |

### Batches

`POST /api/v1/application-batches` accepts a bounded list of selected jobs or existing applications, an explicit consent reference, and the requested review/submission mode. It returns `202` and a batch ID. Every batch item has an independent state, so a missing answer on one item does not fail other ready items. `GET /api/v1/application-batches/{batchID}` and `/items` return counts and per-item status; `POST .../cancel` stops work that has not crossed the submission boundary.

Batch endpoints accept workflow intent only; they cannot override employer restrictions, unresolved factual answers, attestations requiring review, connector rate limits, or tenant quotas. A 500-item batch means 500 items were accepted for processing, not 500 applications were confirmed.

## Planned route inventory

These routes are part of the target API surface. Implement complete vertical slices and add each implemented route to OpenAPI before shipping it; do not add empty handlers.

### Authentication and tenancy

- GET /auth/providers, GET /auth/oauth/google/start, GET /auth/oauth/google/callback, GET /auth/oauth/github/start, GET /auth/oauth/github/callback
- POST /auth/refresh, POST /auth/logout, GET /auth/me; email/password endpoints are not required for the first login slice unless separately selected
- Optional account-linking routes: GET /auth/identities, POST /auth/identities/{provider}/link/start, GET /auth/identities/{provider}/link/callback, DELETE /auth/identities/{provider}
- POST /tenants, GET /tenants, GET /tenants/{tenantID}, PATCH /tenants/{tenantID}
- POST /tenants/{tenantID}/members, GET /tenants/{tenantID}/members
- PATCH /tenants/{tenantID}/members/{memberID}, DELETE /tenants/{tenantID}/members/{memberID}

### User profile and resume

- GET|PATCH /profile, GET|PATCH /profile/preferences, GET /profile/completeness
- POST /resumes/upload, GET /resumes, GET /resumes/{resumeID}, GET /resumes/{resumeID}/versions
- GET|PATCH /resumes/{resumeID}/facts (fact correction is a user-confirmation action)
- POST /resumes/{resumeID}/parse, GET /resumes/{resumeID}/status, GET /resumes/{resumeID}/download, DELETE /resumes/{resumeID}

### Jobs and source operations

- GET /jobs, GET /jobs/search, GET /jobs/recent, GET /jobs/new, GET /jobs/filters
- GET /companies/search, GET /locations/suggest
- GET /jobs/{jobID}, GET /jobs/{jobID}/requirements, GET /jobs/{jobID}/match
- POST|DELETE /jobs/{jobID}/save, GET /saved-jobs
- GET /sources, GET /sources/{sourceID}, GET /sources/{sourceID}/health, GET /sources/{sourceID}/coverage
- POST /sources/{sourceID}/sync, GET /sources/{sourceID}/sync-runs, GET /ingestion/runs/{runID}

### Matching and application packets

- POST /matches, GET /matches, GET /recommendations, POST /matches/{matchID}/feedback
- POST /application-packets, GET /application-packets, GET /application-packets/{packetID}
- POST /application-packets/{packetID}/generate, GET /application-packets/{packetID}/status
- GET /application-packets/{packetID}/evidence, GET /application-packets/{packetID}/resume, GET /application-packets/{packetID}/answers
- PATCH /application-packets/{packetID}/answers/{answerID}, POST /application-packets/{packetID}/validate
- POST /application-packets/{packetID}/approve, POST /application-packets/{packetID}/reject

### Applications, questions and approval

- POST /applications, POST /applications/bulk, GET /applications, GET /applications/{applicationID}
- GET /applications/{applicationID}/status, GET /applications/{applicationID}/timeline, GET /applications/{applicationID}/receipt
- POST /applications/{applicationID}/prepare, /validate, /approve, /submit, /cancel, /retry, /reconcile
- POST /application-batches, GET /application-batches/{batchID}, GET /application-batches/{batchID}/items, GET /application-batches/{batchID}/metrics, POST /application-batches/{batchID}/cancel
- GET /pending-questions, GET /pending-questions/{questionID}, POST /pending-questions/{questionID}/answer, POST /pending-questions/{questionID}/dismiss
- GET /pending-approvals, POST /pending-approvals/{approvalID}/approve, POST /pending-approvals/{approvalID}/reject

### Automation, analytics and operations

- GET|PATCH /automation/settings, GET /automation/consents, POST /automation/consents, GET|POST /automation/policies
- PATCH|DELETE /automation/policies/{policyID}, POST /automation/pause, POST /automation/resume
- GET /analytics/jobs, /analytics/applications, /analytics/submissions, /analytics/performance
- GET /usage, /usage/quotas
- Protected operator routes: GET /admin/health, /admin/metrics, /admin/workers, /admin/queues, /admin/connectors, /admin/audit-events

Route shorthand above is relative to /api/v1. Admin, source sync, membership changes, automation policy, and consent routes have dedicated authorization checks and audit events. Retry is not permission to replay an ambiguous non-idempotent external request; the service first reconciles its outcome.

### Identity, preferences and policy

The route inventory above is the planned surface, not a claim that every route is already present in OpenAPI or implemented. Add each route's schema, authorization, validation, error cases, and idempotency rules to OpenAPI before implementing that vertical slice. Consent changes and automation policy changes are versioned and audited; disabling automation blocks new submissions and cancels queued work where cancellation is still safe.

## Apply-action request/response shape

A single Apply request supplies `job_id`, approved `resume_version_id` (or a selected default), an optional existing packet ID, the requested mode, and the consent record that authorizes the action. It does not supply trusted `tenant_id`, `status=SUBMITTED`, employer receipt, or arbitrary form answers that conflict with evidence.

Expected outcomes:

- `202`: workflow persisted; response includes application ID, status, and status URL.
- `409`: duplicate active application, idempotency body mismatch, or state conflict.
- `422`: structurally invalid input or a policy violation that can be corrected before creating the application.
- `401/403`: missing identity or insufficient permission.
- Later workflow status: `NEEDS_USER_INPUT`, `REQUIRES_MANUAL_ACTION`, `SUBMITTED`, retryable/permanent failure, or `SUBMISSION_UNCONFIRMED` based on worker evidence.

The response never reports `SUBMITTED` just because the API accepted the request or the queue acknowledged a message.

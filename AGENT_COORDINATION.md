# Agent Coordination Record

## 1. Project Overview

- Project: OneRole
- Repository: /Users/nakulbhandare/go/src/github.com/oneRole
- Project guide: README.md, docs/architecture/, api/openapi.yaml, docs/adr/0001-backend-foundation.md
- Primary language: Go 1.23 backend; static HTML/CSS/JavaScript browser prototype
- Current architecture: Go modular monolith; PostgreSQL system of record; NATS JetStream durable worker queue; exact-origin browser API; configured Greenhouse/Lever/Ashby read-only sources
- Build: go build -mod=readonly ./...
- Test: go test -mod=readonly ./...
- Static analysis: go vet -mod=readonly ./...
- Git branch: main; runtime integration e1fb200, coordination record b84140c, and PostgreSQL search-index fix 83c9969 are committed locally.
- Initial repository state: existing design/prototype files preserved in baseline commit 2e0a3f3; repository-local AGENTS.md and .codex/** instructions/config were left untouched
- Coordination owner: Codex lead
- Last updated: 2026-10-30
- Overall status: PARTIALLY_COMPLETE

### Project Objective

Implement and integrate the first real OneRole backend slice described by the repository: authenticated job discovery with the documented filters, tenant-scoped application workflow acceptance, PostgreSQL persistence, and durable discovery/application workers. A OneRole 202 response means the workflow was accepted by OneRole; it is not proof that an employer received an application.

### Repository Findings

- The repository began with architecture documents, a 941-line OpenAPI draft, proposed database schemas, and a static browser prototype.
- The integrated code now includes root runtime modes, typed config, migrations, PostgreSQL repositories, OAuth/JWT session code, catalog filters, application APIs, queue/workers, read-only Greenhouse/Lever/Ashby connectors, and a browser UI that calls the API.
- The UI sends the full documented structured filter request, shows API facets and unknown counts, and does not show fixture jobs.
- Provider OAuth is disabled until valid Google/GitHub credentials and callback URLs are configured. No source rows or live job data are seeded.
- No employer submission adapter is configured. Workers persist a safe manual-action pause when the packet preparer or authorized employer adapter is unavailable. The UI does not claim an employer confirmation.
- Pass and saved-job APIs are not present in the current OpenAPI/backend slice; the UI keeps those controls unavailable.
- The user-provided Docker PostgreSQL is available. Both opt-in PostgreSQL integration tests now pass against a disposable database; the database was removed after verification. RLS flags were confirmed on 24 tables; behavior under a production-like non-owner role remains unverified.
- Codex project discovery lists jobhunt and another OneRole folder, but not this exact repository path. The five ready-to-send prompts are in docs/coordination/AGENT_PROMPTS.md; local agents used separate worktrees. No dedicated Codex chats are claimed.
- There is no Git remote, so local integration is possible but push/PR delivery is not.

### Definition of Done

- Root Go runtime, mode-specific config, explicit migrations, graceful shutdown, health/readiness, and local run instructions are implemented.
- Existing OpenAPI API slice is implemented with JWT/session checks, tenant membership checks, CSRF, exact-origin CORS, and truthful application workflow status.
- Persistence, source connectors, outbox, and workers compile and have relevant tests.
- Browser filter controls use the documented API; no fake listings or success claims are shown.
- Full Go tests, build, vet, JavaScript syntax, OpenAPI parsing, and Git diff checks pass.
- Remaining environmental/provider/employer limitations and unsupported routes are explicitly listed; no claims of 100,000 live sources or 500 confirmed submissions in ten minutes are made.

## 2. Agent Ownership Registry

| Agent | Dedicated Chat | Workstream | Primary Owner | Status |
|---|---|---|---|---|
| Agent 1 | Not available for this unsaved repo path; prompt 1 | Core and Domain Functionality | TASK-002 | COMPLETE |
| Agent 2 | Not available for this unsaved repo path; prompt 2 | APIs and Application Logic | TASK-004, TASK-009, TASK-010 | COMPLETE |
| Agent 3 | Not available for this unsaved repo path; prompt 3 | Data, Persistence, and Integrations | TASK-003, TASK-005 | IN_REVIEW |
| Agent 4 | Not available for this unsaved repo path; prompt 4 | Background Processing and Supporting Systems | TASK-006 | IN_REVIEW |
| Agent 5 | Not available for this unsaved repo path; prompt 5 | Quality, Integration, and Remaining Work | TASK-007 | COMPLETE |

Five separate ready-to-send prompts are available in docs/coordination/AGENT_PROMPTS.md. Five local isolated worktrees were used in waves because the target path is not a saved Codex project and the collaboration runtime allows at most three child agents at once. Exact Codex project chats were not created.

### Agent Responsibilities and Handoffs

- Agent 1 owns domain types, filter normalization, and application transition rules in models/ and assigned pure service code. Handoff to Agents 2 and 3 is integrated and tested.
- Agent 2 owns controller/, middleware/, auth/application/catalog services, and coordinated OpenAPI changes. Auth, API, CORS, and CSRF handoffs are integrated.
- Agent 3 owns db/, migrations/, repository/, and connectors/job_sources/. Code is integrated; PostgreSQL workflow/lease integrations pass; production-like RLS role behavior remains unverified.
- Agent 4 owns queue/, worker/, retry/reconciliation behavior, and source scheduling. Code is integrated; live NATS/DB runtime remains unverified.
- Agent 5 owns index.html, tests/, and testutil/. API-backed search, filter controls, suggestions, CORS/CSRF/application contract tests are integrated.
- Lead owns main.go, config/, .env.example, README.md, AGENT_COORDINATION.md, final integration, and status reporting.

## 3. Master Task Registry

| Task ID | Description | Priority | Owner | Status | Depends On | Files/Components | Verification |
|---|---|---|---|---|---|---|---|
| TASK-001 | Root Go runtime, typed config, health/readiness, explicit migrations, local setup | P0 | Lead | COMPLETE | Shared contracts; persistence/runtime ports | main.go, config/, .env.example, Makefile, README.md | Full tests, build, vet pass; local service startup unavailable |
| TASK-002 | Domain types, filter normalization, application transitions | P0 | Agent 1 | COMPLETE | OpenAPI and data model | models/, assigned pure service code | Full Go tests pass |
| TASK-003 | PostgreSQL migrations, transaction helpers, tenant-safe repositories | P0 | Agent 3 | IN_REVIEW | TASK-002 and agreed ports | db/, migrations/, repository/ | Both opt-in PostgreSQL integrations pass on Docker; 24 tables show forced RLS; non-owner runtime-role enforcement remains unverified |
| TASK-004 | OAuth/JWT auth, tenant checks, application/catalog HTTP handlers | P1 | Agent 2 | IN_REVIEW | TASK-001 through TASK-003 | controller/, middleware/, service/, api/openapi.yaml | Go and PostgreSQL suites pass; live provider login still needs credentials |
| TASK-005 | Configurable read-only job-source connectors and persisted ingestion | P1 | Agent 3 | IN_REVIEW | TASK-002, TASK-003 | connectors/job_sources/, repository/ | Connector and repository package tests pass; no live configured source |
| TASK-006 | Outbox, JetStream workers, retries, batch fanout, source lease recovery | P1 | Agent 4 | IN_REVIEW | TASK-002, TASK-003 and TASK-004 ports | queue/, worker/ | Go and PostgreSQL tests pass; live NATS runtime remains unverified |
| TASK-007 | API-backed browser, complete documented filter panel, isolated integration tests | P1 | Agent 5 | COMPLETE | TASK-004, TASK-005, TASK-006 contracts | index.html, tests/, testutil/ | Full tests, build, JS syntax, diff checks pass |
| TASK-008 | Integrate worktrees, resolve cross-component defects, run final verification | P0 | Lead | COMPLETE | TASK-001 through TASK-007, TASK-009, TASK-010 | main branch | Full Go suite/build/vet and contract checks pass; lead runtime commit e1fb200 |
| TASK-009 | Exact-origin credentialed CORS and restricted preflight | P1 | Agent 2 | COMPLETE | TASK-001, TASK-007 | controller/, api/openapi.yaml | Controller and browser harness tests pass |
| TASK-010 | Same-site cross-origin CSRF bootstrap for API-host cookie | P1 | Agent 2 | COMPLETE | TASK-009, TASK-007 | controller/, api/openapi.yaml | Route tests verify origin, signed cookie, no-store, success/failure |

### Assignment Rules

Each task has one primary owner. File ownership and dependent contracts are recorded before edits. Completed work remains in the registry. New tasks require a unique ID, one primary owner, dependency and file entries, and verification criteria.

## 4. File and Component Ownership

| File/Component | Primary Owner | Other Contributors | Purpose | Coordination |
|---|---|---|---|---|
| main.go, config/, .env.example, Makefile, README.md | Lead | All agents notify lead for shared config changes | Runtime, local setup, configuration | Required |
| models/, assigned pure service domain code | Agent 1 | Agents 2/3/4 consume contracts | Domain rules and normalized filters | Required before interface changes |
| controller/, middleware/, auth/application/catalog services, api/openapi.yaml | Agent 2 | Agent 5 consumes API contracts; lead integrates | HTTP/auth/application behavior | Required |
| db/, migrations/, repository/, connectors/job_sources/ | Agent 3 | Agents 2/4 coordinate contracts | Persistence and read-only sources | Required |
| queue/, worker/ | Agent 4 | Agents 2/3 provide service/repository ports | Durable async work | Required |
| index.html, tests/, testutil/ | Agent 5 | Owners provide stable behavior under test | API-backed browser and cross-component tests | Required for API changes |
| AGENT_COORDINATION.md | Lead | Agents report changes to lead | Single coordination record | Lead serializes writes |
| docs/coordination/AGENT_PROMPTS.md | Lead | None | Five fallback prompts | Lead only |

Repository-local AGENTS.md and .codex/** files are untracked user/project content and have not been staged or changed.

## 5. Dependency and Sequencing Registry

| Dependency ID | Upstream | Downstream | Handoff | Status |
|---|---|---|---|---|
| DEP-001 | TASK-001 | TASK-004, TASK-005, TASK-006 | Module/config/runtime available | COMPLETE |
| DEP-002 | TASK-002 | TASK-003, TASK-004, TASK-005, TASK-006 | Domain types and behavior | COMPLETE |
| DEP-003 | TASK-003 | TASK-004, TASK-005, TASK-006 | PostgreSQL ports and schema integrated; workflow/lease tests pass; production-like RLS role check pending | IN_REVIEW |
| DEP-004 | TASK-004/005/006 | TASK-007 | Routes, search filters, and workflow contract | COMPLETE |
| DEP-005 | TASK-001–007 | TASK-008 | Integrated tests/build/static checks | COMPLETE |
| DEP-006 | TASK-010 | TASK-007 | CSRF bootstrap route and same-site behavior | COMPLETE |

Execution followed discovery → contracts → parallel implementation → incremental worktree integration → full local verification. No open merge or API-contract conflicts remain.

## 6. Shared Contracts and Decisions

| Decision ID | Topic | Agreed Contract or Decision | Affected Agents | Status | Date |
|---|---|---|---|---|---|
| DEC-001 | API | Existing `/api/v1` OpenAPI and API contract are authoritative; do not replace with `/v1` routes. | 1,2,5 | AGREED | 2026-10-30 |
| DEC-002 | Runtime | One root `main.go`; `APP_MODE=api|worker|all`; explicit migration command. | All | AGREED | 2026-10-30 |
| DEC-003 | Data and queue | MongoDB is canonical; transactional outbox; JetStream is at-least-once; message payloads use IDs; Redis is ephemeral. | 2,3,4,5 | AGREED | 2026-10-30 |
| DEC-004 | Search | PostgreSQL FTS/trigram first; OpenSearch only after measured need, per ADR-0001. | 1,2,3,5 | AGREED | 2026-10-30 |
| DEC-005 | Real data/submission | No live fixtures; read-only source adapters must be explicitly configured; employer submission is disabled unless a verified authorized flow is configured; never blind-retry ambiguous submits. | All | AGREED | 2026-10-30 |
| DEC-006 | Agent chats | Chat creation against this repo path is unavailable: it is not a saved Codex project; the saved OneRole project points elsewhere. Use five prompts plus isolated local agents; do not claim chat creation. | All | LIMITATION RECORDED | 2026-10-30 |
| DEC-007 | Concurrency | At most four active participants including lead. Run at most three child agents concurrently; rotate remaining distinct workstreams in as slots open. | All | LIMITATION RECORDED | 2026-10-30 |
| DEC-008 | Go module | Module path is `github.com/oneRole`, matching the selected repository path. Go directive is 1.23.0 for installed Go 1.23.2. | All | AGREED | 2026-10-30 |
| DEC-009 | Dependencies | Pin Go-1.23-compatible `pgx/v5 v5.7.2`, `nats.go v1.39.1`, `golang-jwt/jwt/v5 v5.3.1`, `coreos/go-oidc/v3 v3.15.0`, `golang.org/x/oauth2 v0.28.0`, and indirect `go-jose/v4 v4.0.5`, `pgpassfile v1.0.0`, `pgservicefile`, `klauspost/compress v1.17.9`, `nats-io/nkeys v0.4.9`, `nats-io/nuid v1.0.1`, `testify v1.10.0`, `x/crypto v0.36.0`, `x/net v0.37.0`, `x/sys v0.31.0`, and `x/text v0.23.0`; lead owns `go.mod`/`go.sum`. New dependencies require lead coordination. | 2,3,4 | AGREED | 2026-10-30 |
| DEC-010 | Auth persistence ports | Service-owned OAuthTransactionStore, AuthIdentityStore, AuthSessionStore, and MembershipReader contracts; exact signatures below. DB adapter owner is Agent 3. | 2,3 | AGREED | 2026-10-30 |
| DEC-011 | Tenant API scope | Every tenant-owned `/api/v1` operation requires `X-Tenant-ID`; middleware resolves the active membership on each request and hides cross-tenant resource existence. Public job search and auth callbacks are not tenant-scoped. | 2,3,5 | AGREED | 2026-10-30 |
| DEC-012 | Outbox contract | Outbox event types use documented dotted names `application.packet.prepare.requested`, `application.submit.requested`, `application.batch.fanout.requested`; JetStream subjects are mapped separately. Include event version; payloads contain identifiers only, no resume/question contents or secrets. | 2,3,4 | AGREED | 2026-10-30 |
| DEC-013 | Application workflow port | Service-owned `ApplicationWorkflowStore` durably accepts create, submit, and up-to-500-item batch workflows with tenant/user scope and idempotency hashes; app/batch, timeline event, and outbox write commit atomically. Reads are tenant-scoped. `202` means durable OneRole acceptance only. | 2,3,4 | AGREED | 2026-10-30 |
| DEC-014 | Consent scope | Versioned `ConsentScopeV1` requires `schema_version:1`; `application:{prepare,submit,modes,company_ids,job_ids?}`; `data_use:{resume_processing,evidence_based_answer_generation,sharing_with_employer}`. Company IDs are an explicit UUID list or explicit `"*"`; optional job IDs narrow scope. Active consent must match tenant/user/job/company/mode; submit and employer sharing are rechecked immediately before submission. | 2,3,4 | AGREED | 2026-10-30 |
| DEC-015 | Worker service port | Workers invoke service-owned `ApplicationTaskService` with tenant/user scope and IDs for prepare, batch fanout, submit, and reconciliation. Persisted business pauses and ambiguous outcomes are completed results; infrastructure failures are retried with bounds. | 2,4 | AGREED | 2026-10-30 |
| DEC-016 | Retry classification | Only typed `TaskFailure` values with `SafeToRetry=true` enter bounded worker retry; business pauses/ambiguous outcomes are persisted results, and unclassified errors go to controlled failure handling. | 2,4 | AGREED | 2026-10-30 |
| DEC-017 | Discovery run recovery | `StartSourceSync` commits the run and `discovery.source.sync.requested` outbox event together. Payload is source/run IDs only with null tenant for global catalog work. Leases expire if a crash happens before start; run records hold the lease token/checkpoint, and page/renew/finish/fail operations are fenced by that token. `RenewSourceLease` is bounded to 30 minutes. Worker heartbeat stops/cancels a run on lost fencing; stale work cannot commit pages or completion. Aggregate is `source_sync_run`, ID is `runID`. | 3,4 | AGREED | 2026-10-30 |
| DEC-018 | Worker runtime | Root main builds `worker.NewRuntime(RuntimeConfig, RuntimeDependencies, onError)` with JetStream, outbox/inbox/source stores, connector registry, and application task service. Development may provision streams and consumers; production sets both provisioning flags false and uses preprovisioned streams/durable consumers. Runtime coordinates relay, source scheduler, and durable consumers under root signal/shutdown handling. | 1,3,4 | AGREED | 2026-10-30 |
| DEC-019 | Browser CORS | Browser API calls use an exact allowlist from `ALLOWED_ORIGINS`, credentials are enabled only for listed origins, wildcard reflection is forbidden, and preflight exposes only the required API methods and headers. | 2,5 | AGREED | 2026-10-30 |
| DEC-020 | Cross-origin browser auth | Support same-site subdomains with exact-origin CORS and a no-store CSRF bootstrap endpoint for the API-host-only cookie. `SameSite=Strict` refresh cookies require frontend and API to share a site; different registrable domains are not supported by this slice. | 2,5 | AGREED | 2026-10-30 |

### Auth persistence contract (Agent 2 service ports; Agent 3 DB adapters)

- `OAuthTransactionStore.Create(ctx, OAuthTransaction) error`; `Consume(ctx, provider, stateHash, browserBindingHash []byte, now time.Time) (OAuthTransaction, error)`. Consume is atomic, one-time, provider-bound, expiry-checked, and browser-binding-checked.
- `AuthIdentityStore.ResolveOrCreate(ctx, AuthIdentityInput) (AuthUser, error)`. Identity key is `(provider, immutable provider subject)`, never email. Verified-email collision returns a conflict; no auto-linking.
- `AuthSessionStore.Create(ctx, AuthSession) error`; `Rotate(ctx, RefreshRotation) (AuthSession, error)`; `FindActive(ctx, sessionID string, now time.Time) (AuthSession, error)`; `Revoke(ctx, sessionID string, now time.Time, reason string) error`. Rotate atomically compare-and-swaps an active unexpired hashed token. Reuse revokes the family and returns `ErrRefreshTokenReuse`.
- `MembershipReader.ListActive(ctx, userID string) ([]TenantMembership, error)`; `FindActive(ctx, userID, tenantID string) (TenantMembership, error)`. `AuthUserReader.FindByID(ctx, userID string) (AuthUser, error)` supplies current profile data for `/auth/me`.
- All persistence times are explicit UTC `time.Time`; the service stores only hashes for state/refresh tokens and encrypted PKCE verifier bytes. API configuration uses RSA JWT signing with explicit issuer, audience, key ID, short access TTL, allowed verification key map, 32-byte PKCE AES key, CSRF and idempotency HMAC secrets, fixed frontend callback, exact allowed origins, and bounded refresh/OAuth TTLs.
- Agent 3 proposed `db.WithUserTenantTx(ctx, userID, tenantID, fn)`; its implementation must set local `app.user_id`, verify active membership, set local `app.tenant_id`, then call `fn` in the same transaction. Worker transactions use a distinct trusted service context.

### Application workflow contract (Agent 2 service ports; Agent 3 persistence; Agent 4 consumers)

- Scope is `service.ActorScope{TenantID, UserID}`. Idempotency carries only `KeyHash` and canonical `RequestHash` (`[32]byte` each); never persist the raw key.
- `ApplicationWorkflowStore` methods are `AcceptApplication`, `AcceptSubmission`, `AcceptBatch`, `GetApplication`, and `GetBatch`. Each accept method receives context, actor scope, command, idempotency identity, and explicit UTC `now`; each returns a workflow acceptance with ID/status/time.
- Application creation persists intent and `application.packet.prepare.requested`. Submission is separately authorized and queues `application.submit.requested` only after stored consent and readiness checks. Batch creation is capped at 500 and emits `application.batch.fanout.requested`; item outcomes remain independent.
- Consent scope is typed and versioned: `schema_version:1`; `application:{prepare,submit,modes,company_ids,job_ids?}`; `data_use:{resume_processing,evidence_based_answer_generation,sharing_with_employer}`. Explicit company IDs or `"*"` are required; optional job IDs narrow scope.
- The database transaction validates active membership and tenant ownership, applies idempotency replay/conflict rules, and commits the application/batch rows, timeline event, and outbox event together. A worker reloads all tenant-owned data by ID.

## 7. Shared Progress Metrics

### Task Progress

- Total assigned tasks: 10.
- Tasks with a primary owner: 10.
- Tasks completed: 6 (TASK-001, TASK-002, TASK-007, TASK-008, TASK-009, TASK-010).
- Tasks in review: 4 (TASK-003, TASK-004, TASK-005, TASK-006).
- Blocked by code dependencies: 0.
- Remaining verification actions: production-like non-owner RLS check, live configured-source check, live NATS runtime, and configured OAuth provider flow.

### Integration Health

- Required handoffs: 6; completed: 5; in review: 1.
- Open merge conflicts: 0.
- Open contract conflicts: 0.
- Open integration defects: 0 known from Go/browser contract suite.
- PostgreSQL validation defects found/fixed: 1 / 1 (full-text index immutability).
- Integrated implementation commits: runtime e1fb200; coordination b84140c; PostgreSQL search fix 83c9969; Agent 5 UI c3e2461 is an ancestor.

### Verification

- Full Go suite: 11 test packages passed, 0 failed; both opt-in PostgreSQL integration tests ran against the disposable Docker database using ONEROLE_TEST_DATABASE_URL.
- Full build: PASS; command was GOCACHE=/private/tmp/oneRole-final-gocache GOPROXY=off go build -mod=readonly ./...
- Static analysis: PASS; go vet -mod=readonly ./...
- JavaScript inline-script syntax: PASS with node --check.
- OpenAPI YAML parse/version: PASS; OpenAPI 3.1.0.
- Formatting: gofmt -d on root Go files produced no diff.
- Patch hygiene: git diff --check passed after final integration and coordination edits.
- PostgreSQL integration: PASS against the user's local Docker PostgreSQL in disposable database onerole_codex_test_20261030; both workflow and source-lease tests passed. The database was dropped after verification. Migration metadata shows one applied migration and 24 forced-RLS tables; a non-owner/no-BYPASSRLS policy test remains open.
- Live browser/service end-to-end: not run because a composed OneRole API/NATS stack, live source records, and OAuth credentials are not configured.
- Lint: no separate linter configured; go vet passed.

### Delivery

- Implemented and integrated: root runtime/config, JWT/OAuth handlers, CORS/CSRF, search filters/facets/suggestions, application workflow APIs, database schema/repos, source connectors, durable workers, API-backed browser filters, and integration tests.
- Verified behavior groups: config/security validation; authenticated API routing; tenant-scoped application acceptance; full structured filter normalization; suggestions/facets; CORS/CSRF; workers/queues/connectors package behavior; browser-to-API contract wiring.
- External claims not verified: live worldwide coverage, 100,000+ live source integrations, automatic employer submission, employer confirmation email, or 500 confirmed submissions in ten minutes.
- Local integration includes runtime commit e1fb200 and a follow-up documentation commit for this final coordination record.

## 8. Blockers and Conflicts

| Issue ID | Type | Description | Affected | Owner | Next Action | Status |
|---|---|---|---|---|---|---|
| ISSUE-001 | RLS ROLE VERIFICATION | PostgreSQL workflow/lease tests pass on Docker and 24 tables have forced RLS. The suite connects as superuser, so policy behavior under a non-owner, non-BYPASSRLS runtime role has not been demonstrated. | TASK-003 | Lead | Add/run a dedicated role-level RLS check. | OPEN |
| ISSUE-002 | CHAT AVAILABILITY | This exact repository path is not a saved Codex project; chat creation cannot target it. | All agents | Lead | Keep five ready-to-send prompts and local isolated worktrees; no chat creation claim. | RECORDED |
| ISSUE-003 | EXTERNAL CREDENTIALS | Google/GitHub OAuth, live job-source authorization, and employer submission access are not configured. | TASK-004/005/006 | Operator | Configure credentials and authorized source/employer access before live verification. | OPEN |
| ISSUE-004 | DELIVERY | No Git remote is configured, and no remote PR/push workflow is available. | All | Lead | Keep delivery local; do not claim remote publication. | OPEN |
| ISSUE-005 | PRODUCT API GAP | Pass and saved-job routes are absent from the current API/OpenAPI slice; UI keeps these actions unavailable. | Earlier Apply/Pass expectation, Agent 5 | Lead | Add a tenant-scoped decision/save API as a separately contracted task before enabling controls. | OPEN |

No local code ownership or merge conflict is open.

## 9. Handoff Register

| Handoff ID | From | To | Task | Deliverable | Verification | Status |
|---|---|---|---|---|---|---|
| HANDOFF-001 | Agent 1 | Agents 2/3 | TASK-002 | Search/filter models and legal application transitions | Full Go tests pass | COMPLETE |
| HANDOFF-002 | Lead | Agents 2/3/4 | TASK-001 | Go module, Go 1.23 dependency graph, config/runtime contract | Integrated build and tests pass | COMPLETE |
| HANDOFF-003 | Agent 2 | Agent 3 | TASK-004 | Auth persistence ports and tenant-scoped workflow contracts | PostgreSQL adapters and workflow integration verified against local Docker DB | COMPLETE |
| HANDOFF-004 | Agent 4 | Lead | TASK-006 | Worker runtime, durable queue, source leases and retry behavior | Go/PostgreSQL suites pass; live NATS runtime unverified | IN_REVIEW |
| HANDOFF-005 | Agent 2 | Agent 5 | TASK-009/010 | Exact-origin CORS and same-site CSRF bootstrap | Router/security integration tests pass | COMPLETE |
| HANDOFF-006 | Agent 5 | Lead | TASK-007 | API-backed UI, structured filter panel, suggestion routes, tests | Full Go suite/build, JS syntax, diff check pass | COMPLETE |

## 10. Git and Integration Status

- Baseline: 2e0a3f3 preserves the user's original design and prototype.
- Integration branch: main.
- Local integration commits: runtime/config/docs e1fb200; coordination b84140c; PostgreSQL search fix 83c9969; Agent 5 UI c3e2461 is an ancestor.
- Environment/config edits outside these commits remain unstaged and were preserved.
- User/project-local instructions AGENTS.md and .codex/** remain untracked and excluded. The refreshed graphify-out/ index is also local and untracked.
- Agent worktrees: /private/tmp/oneRole-w1 through /private/tmp/oneRole-w5. W5 commit c3e2461 is integrated; earlier domain/API/data/worker commits are ancestors of main.
- Initial user content: architecture/design docs and prototype assets; preserved.
- Git remote: none.
- No remote push or PR was attempted.

## 11. Final Acceptance Checklist

- [x] Repository guide, code, tests, API contract, and initial Git state inspected.
- [x] Existing user/project-local content preserved.
- [x] Five distinct workstreams and file owners recorded.
- [x] Five ready-to-send prompts supplied because exact-project chat creation is unavailable.
- [x] Domain, API, persistence, worker, and UI workstreams implemented and integrated locally.
- [x] Full Go tests, build, vet, JavaScript syntax, OpenAPI parse, and patch checks executed.
- [x] PostgreSQL workflow and source-lease integration tests pass against the user's Docker PostgreSQL in a disposable database.
- [ ] RLS enforcement tested as a non-owner, non-BYPASSRLS runtime role.
- [ ] Live OAuth provider, configured job source, and employer submission integrations verified.
- [x] Lead-owned backend implementation changes committed locally; separate environment/config edits remain unstaged.
- [x] Remaining limitations documented honestly.

## 12. Live Status Log

- 2026-10-30: Repository discovery confirmed a design-first repo, no Git remote, and no configured OAuth/source/employer credentials. Five separate workstreams and prompts were established; exact Codex chats could not target this unsaved repo path.
- 2026-10-30: Agent 1's domain/filter/state transition work, Agent 2's JWT/OAuth/API/CORS/CSRF work, Agent 3's migrations/repositories/read-only source connectors, and Agent 4's queue/worker work were integrated through isolated worktrees.
- 2026-10-30: Lead added the root runtime, bounded worker config, local environment defaults, and setup documentation.
- 2026-10-30: Agent 5 replaced runtime sample listings with API calls, added the complete documented filter panel and suggestions, and added router/JWT/service integration contract tests. Commit c3e2461 is integrated.
- 2026-10-30: The full Go suite passed across 11 packages, including both opt-in PostgreSQL integration tests against a disposable Docker database; build, vet, JS syntax, OpenAPI parsing, formatting, and patch checks passed.
- 2026-10-30: PostgreSQL integration exposed an immutable-expression error in the job-search GIN index. The migration and matching query expression were fixed in 83c9969; migration and search query plan now pass, and the GIN index is selected. The dedicated database was removed. A non-owner runtime-role RLS check remains open.
- 2026-10-30: Pass/saved-job routes, live OAuth credentials, configured sources, employer submission adapters, and production-scale external validation remain outstanding.

## 13. Final Implementation Report

### Completed Work

- Built the Go runtime composition for API, worker, combined, and explicit migration modes.
- Added typed runtime config checks, local environment setup, health/readiness, and graceful shutdown.
- Integrated JWT/session authentication, configurable Google/GitHub OAuth, tenant checks, exact-origin CORS, and CSRF bootstrap.
- Added PostgreSQL migrations/repositories, source connectors, outbox/inbox, and durable worker composition.
- Connected the browser prototype to API-backed auth, job search, all documented filters, company/location suggestions, and application workflow routes. Runtime demo jobs were removed.
- Added isolated HTTP contract tests that exercise real router, JWT, middleware, and service code with test-only in-memory ports.

### Agent Contributions

- Agent 1: domain filters and application transitions.
- Agent 2: auth/JWT/OAuth, application/catalog handlers, CORS, CSRF, OpenAPI.
- Agent 3: migrations, PostgreSQL repositories, connector implementations.
- Agent 4: durable outbox, JetStream consumers, discovery/application workers.
- Agent 5: API-backed prototype, complete filter panel, integration harness.
- Lead: runtime composition, config/README, final coordination and integration.

### Files Changed

Important changes are in main.go, config/, .env.example, README.md, migrations/, db/, repository/, connectors/job_sources/, controller/, middleware/, service/, queue/, worker/, index.html, tests/, testutil/, api/openapi.yaml, and AGENT_COORDINATION.md.

### Verification Results

- go test -mod=readonly -count=1 ./...: PASS, 11 test packages, including both opt-in PostgreSQL integrations against the disposable Docker test database.
- go build -mod=readonly ./...: PASS.
- go vet -mod=readonly ./...: PASS.
- Inline JavaScript node --check: PASS.
- OpenAPI YAML parse: PASS, 3.1.0.
- gofmt -d and git diff --check: PASS.
- PostgreSQL integration: PASS against local Docker PostgreSQL; 2 opt-in tests passed, the migration applied, and the full-text query used its GIN index.

### Remaining Issues

- RLS enforcement under a non-owner, non-BYPASSRLS role and local Compose startup remain unverified.
- Google/GitHub provider login requires configured credentials.
- Job discovery only covers explicitly configured, authorized feeds; no live source data is seeded.
- Employer submission adapters and employer confirmation email integration are absent.
- Pass and saved-job endpoints remain unavailable and their UI controls stay disabled.
- No evidence supports 100,000+ live companies or 500 confirmed submissions per ten minutes.
- Exact Codex project chats and remote publication were unavailable.

### Final Status

PARTIALLY_COMPLETE. The integrated local implementation passes unit/contract tests, live PostgreSQL workflow/lease checks, build, static analysis, JavaScript syntax, and OpenAPI parsing. Remaining RLS-role, provider, source, employer, and product-route gaps are explicitly recorded; production readiness is not established.

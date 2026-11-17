# ADR-0001: OneRole backend foundation

- **Status:** proposed for review
- **Date:** 2026-10-30
- **Decision owner:** OneRole product/engineering

## Context

OneRole needs a Go backend for job ingestion, tenant-scoped personal data, evidence-backed application preparation, and asynchronous employer workflows. The supplied simplified architecture requires exactly one root `main.go` and one binary with API/worker/all runtime modes. The larger platform brief favors a modular monolith, PostgreSQL, durable async processing, private object storage, observable connectors, and gradual scaling.

## Decisions

1. Build a modular monolith in Go with one root entry point. Start API and workers as independent replicas of the same image using validated `APP_MODE`; use an explicit migration subcommand.
2. Use PostgreSQL as the authoritative store for tenant data, jobs, workflow state, audit events, idempotency, and transactional outbox. Use application predicates plus transaction-scoped tenant context and PostgreSQL RLS as defense in depth.
3. Start with NATS JetStream for durable asynchronous tasks. Its official documentation describes persistence, consumer acknowledgements, replay, and at-least-once delivery. Therefore handlers must be idempotent and acknowledge only after durable completion. Evaluate Kafka only if measured stream retention, multi-consumer replay, throughput, or operating model requires it. Source: [NATS JetStream concepts](https://docs.nats.io/concepts/jetstream).
4. Start job search with PostgreSQL full-text and trigram indexes. Project to OpenSearch only after measured search relevance, faceting, or throughput requires separate indexing infrastructure.
5. Use Redis only for ephemeral coordination, limits, and cache; no business-critical state depends solely on Redis.
6. Store original resumes and generated artifacts in private S3-compatible object storage with tenant-scoped keys and short-lived signed access.
7. Define source discovery, ATS submission, AI and storage as separate connector boundaries. A discovery capability does not imply a submission capability.
8. Keep the local setup reproducible with Docker Compose and disposable credentials. Promote the same image through staging to production; production secrets and employer credentials are never copied into local configuration.

## Tenant enforcement note

PostgreSQL documents that RLS is default-deny when enabled with no applicable policy, and that table owners normally bypass policies unless configured to be subject to them. Application roles, transaction-local context, forced policy behavior, and cross-tenant tests must be designed together. RLS is not a replacement for authorization or tenant-aware repository APIs. Source: [PostgreSQL row security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html).

## Authentication decision

Use Google OpenID Connect and GitHub OAuth authorization-code sign-in in the OneRole UI, then issue OneRole's own short-lived JWT access token. Use PKCE S256, one-time state, exact callbacks, OIDC nonce and ID-token validation for Google, and stable GitHub subject lookup. Keep refresh credentials rotating, opaque, hashed server-side, and in an HttpOnly Secure SameSite cookie. Do not auto-link accounts on email equality. The API enforces current tenant membership separately from token identity.

This follows OAuth Security BCP guidance to protect redirect flows with state/PKCE, avoid implicit token flows, and rotate or sender-constrain refresh tokens. Sources: RFC 9700 at https://www.rfc-editor.org/rfc/rfc9700.html, Google's server-side OAuth flow at https://developers.google.com/identity/protocols/oauth2/web-server, and GitHub's OAuth web flow at https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps.

## Job-filter decision

Expose the screenshot's search bar, exclusions, date, location, workplace, companies, degree, maximum experience, visa sponsorship, role, job type, employment type, Apply, and Clear as functional API query behavior. Keep categorical values normalized and evidence-backed. Separate workplace from location, role from career level, and job type from employment type. Expose unknown values/counts rather than guessing missing information. Search filter semantics and field mappings are specified in the API contract.

## Consequences

- API and workers can scale separately without duplicate entry points or business logic.
- Queue processing is at-least-once, not a blanket exactly-once guarantee.
- PostgreSQL avoids adding a dedicated search cluster before there is evidence it is needed.
- The platform can accept bursts into durable workflows, while employer rate limits and policy determine external submission speed.
- A fully automated employer submission is enabled only for an explicit, supported adapter and valid consent; unsupported or ambiguous flows remain visible and recoverable.

## Revisit when

Revisit queue, search, deployment, and database partitioning choices only with measured workload data, connector contracts, or operational requirements. Do not use the 10,000 workflow requests/minute target as evidence that any component has already achieved that throughput.

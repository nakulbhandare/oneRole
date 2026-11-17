# OneRole

OneRole is a Go modular-monolith foundation for live job discovery and application workflows. The repository now contains a runnable API/worker, MongoDB migrations and repositories, authenticated catalog/application APIs, read-only Greenhouse/Lever/Ashby connectors, and an API-backed browser prototype.

OneRole records a user's application intent separately from an employer's receipt. A 202 Accepted response means OneRole queued work; only verified employer evidence may produce a submitted state.

## Implemented foundation

- Root Go entry point with api, worker, all, and explicit migrate modes.
- Google OIDC and GitHub OAuth sign-in, enabled only when provider credentials are configured; OneRole issues its own short-lived JWT and rotates a server-side refresh session.
- Exact-origin credentialed CORS, CSRF protection, tenant-membership checks, idempotency, and tenant-scoped application workflows.
- MongoDB migrations, transaction helpers, tenant isolation, repositories, job search/facets, source sync leases, outbox/inbox persistence, and audit/timeline records.
- Durable NATS JetStream workers for source refresh and application workflow tasks.
- Read-only job connectors for configured Greenhouse, Lever, and Ashby sources. They do not discover every employer automatically.
- A static browser UI that calls the API and displays only returned records. No job fixture is used as live data.

## Local quickstart

Prerequisites: Go 1.23+, Docker Compose, OpenSSL, and Python 3 for serving the static UI.

    cp .env.example .env
    make dev-up
    make dev-key

The checked-in .env.example contains local-only secrets. Replace AUTH_ENCRYPTION_KEY with the output of openssl rand -base64 32; replace CSRF_SIGNING_SECRET and IDEMPOTENCY_HMAC_SECRET with separate outputs from openssl rand -hex 32. Keep these values out of Git. make dev-key creates the local RSA signing key at .local/jwt-signing.pem.

Then apply the schema and start the API plus worker:

    make migrate
    make run

make run loads .env and uses its APP_MODE (the example uses all). The API listens at http://localhost:8080; health and readiness are /api/v1/healthz and /api/v1/readyz.

In a second terminal, serve the prototype from the repository root:

    python3 -m http.server 3000 --bind 127.0.0.1

Open http://localhost:3000. Keep the UI and API on localhost so browser cookies remain same-site. The local UI targets the API at http://localhost:8080/api/v1; deployments can set window.ONE_ROLE_API_BASE before the page script runs. The API origin must be listed exactly in ALLOWED_ORIGINS.

Google and GitHub sign-in remain unavailable until each provider's client ID, client secret, and callback URL are configured. Register callbacks at:

- http://localhost:8080/api/v1/auth/oauth/google/callback
- http://localhost:8080/api/v1/auth/oauth/github/callback

Set FRONTEND_AUTH_CALLBACK_URL to the UI callback route. The provider status endpoint tells the UI which providers are enabled.

The worker discovers jobs only from enabled, reviewed source records configured in MongoDB. The registry currently supports read-only Greenhouse, Lever, and Ashby feeds. No source records or employer credentials are seeded by default.

## Development checks

    make test
    make build
    go vet ./...

MongoDB workflow/lease integration tests run only when ONEROLE_TEST_DATABASE_URL points to a dedicated database whose name includes test. Do not point these tests at a development or production database. OpenAPI is in api/openapi.yaml.

## Design and operations documents

- [Plain-language backend overview](docs/architecture/overview.md)
- [Architecture map](docs/architecture/README.md)
- [Backend architecture](docs/architecture/backend.md)
- [Database architecture](docs/architecture/database-architecture.md)
- [Database table guide](docs/architecture/database-table-guide.md)
- [Data model](docs/architecture/data-model.md)
- [API contract](docs/architecture/api-contract.md)
- [Company/source configuration](docs/architecture/company-configuration.md)
- [Job discovery, freshness, and filters](docs/architecture/job-discovery.md)
- [Workers, queue delivery, retries, and batch processing](docs/architecture/worker-implementation.md)
- [Application preparation and submission flow](docs/architecture/submission-engine.md)
- [OpenAPI contract](api/openapi.yaml)
- [Foundation decisions](docs/adr/0001-backend-foundation.md)
- [Live data and coverage rules](docs/operations/live-data-and-coverage.md)
- [Local-first design plan](DESIGN.md)

## Limits of this implementation

This foundation does not claim 100,000 live employer integrations, worldwide job coverage, 50,000 independently polled company feeds, or 500 confirmed submissions in ten minutes. A company record is not automatically a live source, and source polling is constrained by authorization, provider limits, freshness, and configured concurrency.

Employer application submission adapters are not implemented or enabled. Application tasks pause for manual action when a packet preparer or authorized employer adapter is unavailable. The current API-backed UI also reports unsupported actions—such as Pass and saved jobs—as unavailable when no route exists. No employer confirmation email is fabricated.

OAuth credentials, employer/source authorization, a live MongoDB/NATS environment, and production secrets must be supplied by the operator before those integrations can run. See the coordination record for verified status and remaining tasks.

## Collaborative implementation record

- [Shared agent ownership, dependencies, verification, and status](AGENT_COORDINATION.md)
- [Five ready-to-send agent prompts](docs/coordination/AGENT_PROMPTS.md)

Codex cannot attach new chats to this repository path because it is not a saved project in the desktop app. The five separate prompts and five isolated local agent worktrees were used as the supported fallback; no unavailable Codex chats are claimed.

# Company and source configuration

**Status:** design for review. No company connector has been enabled by this document.

## The basic idea

A company is an employer identity. A source is one place where that company publishes jobs. A connector is Go code that understands a source's API or documented feed.

One company can have more than one source, for example, one job board per region or brand. A source can be used for discovery without being used for application submission.

## Company record versus source record

| Record | Example information | Why it is separate |
|---|---|---|
| Company | Canonical name, domains, aliases, country, verification status | The employer identity remains stable when its ATS vendor changes |
| Company source | ATS/provider, board identifier, public URL, region, current capability, sync schedule | A company may have several job boards, regions, or source providers |
| Connector version | Provider adapter name, supported operations, parser version | One connector implementation can serve many companies |
| Credential reference | Secret-manager key reference, scope, expiry, rotation state | Credentials are company-specific and must never be stored as plain text in PostgreSQL |
| Source run | Start/end time, result counts, cursor, sanitized failure class | Operations need evidence of when this feed was actually checked |
| Capability snapshot | Discovery and submit flags, required access, verified date, restrictions | The product must not infer that job discovery means auto-submit is allowed |

## Configuration fields

A source configuration needs the following values before it can be enabled:

| Field | What it means |
|---|---|
| company_id | The canonical employer record |
| provider_type | greenhouse, lever, ashby, licensed_feed, employer_feed, or another reviewed adapter |
| board_identifier | The employer's actual board slug, site name, or feed key |
| source_region | Region/instance if the provider has separate endpoints |
| source_endpoint | Exact documented endpoint or feed URL |
| allowed_hosts | Hostnames the connector may contact; redirects outside this set are rejected |
| discovery_enabled | Whether OneRole may read and index public listings |
| submit_capability | unsupported, manual, assisted, or automated_supported |
| submit_auth_mode | public, company credential, partner credential, or none |
| secret_reference | Secret-manager reference only; never the credential value |
| allowed_operations | Read jobs, read questions, submit application, reconcile status |
| requests_per_window | Connector rate ceiling approved for this source |
| concurrency_limit | Maximum in-flight requests for this source |
| poll_policy | Priority class, base interval, backoff, and freshness target |
| checkpoint | Provider cursor or last completed page marker |
| connector_version | Version of the source parser and adapter |
| policy_reviewed_at | Date access terms and capability were last reviewed |
| last_success_at / last_error_at | Last source health results |
| config_version | Monotonic version so workers can detect configuration changes |

The full configuration is validated before activation. An unknown provider type, unapproved host, absent required secret, invalid interval, or missing capability review keeps the source disabled.

## Onboarding a company source

1. **Resolve the employer.** Normalize its display name and domain. Check aliases so duplicate company records are not created.
2. **Identify a real source.** Use a documented ATS endpoint, employer-provided feed, licensed feed, or approved integration. Do not guess a board slug and call the result live.
3. **Check the provider.** Confirm supported operations, endpoint format, pagination, region, authentication, rate rules, field limitations, and application restrictions.
4. **Set credentials safely.** A privileged operator adds credentials to a secret manager. PostgreSQL stores only the reference and metadata.
5. **Run a read-only health check.** Confirm host, TLS, valid response, pagination, and source identity. The job records remain marked as unverified until normalized.
6. **Import real postings.** Persist the provider's external ID, source URL, timestamps, content checksum, and sync run ID.
7. **Review mapping.** Verify workplace, location, employment type, degree, experience, and sponsorship fields; unknown means unknown.
8. **Enable scheduled sync.** Assign a poll class within the provider's allowed rate.
9. **Evaluate submission separately.** Do not enable submission until the company's application flow, questions, credentials, consent handling, and receipt evidence have been tested.

## Capability matrix

Every job inherits an explicit capability from its source and may have additional job-specific restrictions.

| Capability | Meaning in OneRole |
|---|---|
| discovery_live | Source has a working, recently successful read path |
| discovery_degraded | Source exists but is failing or stale |
| questions_readable | Required application questions can be fetched through an approved interface |
| submit_public | Documented public submit operation exists and is permitted |
| submit_company_authorized | Employer credentials or partner authorization are required and configured |
| submit_assisted | OneRole can prepare the packet, but the user must finish the employer flow |
| submit_manual | User must apply on the employer's site |
| submit_unsupported | There is no verified supported submission path |
| requires_user_authentication | The employer requires the candidate to sign in |
| requires_user_action | The employer requires an attestation, CAPTCHA, MFA, or other user-only step |

Capability status is stored with the source and copied into a job/application snapshot. A later source config change does not rewrite old submission evidence.

## Scale for a 100,000-company registry

Use one company table and many company-source rows. Never create a database table or Go package per company.

Partition scheduled sources into deterministic worker shards, such as hash(source_id) mod shard_count. Store one schedule and one cursor per source. Workers claim due sources with a short lease, then release the lease when the sync completes or expires.

The source registry can contain 100,000 companies even when fewer have active feeds. The coverage dashboard separates:

- Registered companies.
- Companies with at least one configured source.
- Sources enabled for live discovery.
- Sources healthy within their freshness target.
- Sources with application questions available.
- Sources with verified live submission.
- Sources paused for credential, policy, or health reasons.

The target does not authorize indiscriminate crawling. Source access and submission limits are controlled per provider and per company.

## Real provider constraints

Provider support is not uniform. Greenhouse documents a Job Board API for listing jobs and a submit-application endpoint, but the submit endpoint requires an API key and OneRole must validate required fields itself. Lever documents a public Postings API that can list postings and programmatically apply, while its public API does not expose custom application questions. These differences mean a single generic “auto-apply supported” flag would be unsafe. [Greenhouse Job Board API](https://docs.greenhouse.io/job-board), [Lever Postings API](https://github.com/lever/postings-api).

## Source health states

- **Pending review:** source configured but not yet verified.
- **Healthy:** last successful sync is inside the configured freshness target.
- **Stale:** source is still available, but its last successful sync exceeds target.
- **Degraded:** recent failures are increasing or the source is partially responding.
- **Paused:** an operator or policy disabled the source.
- **Credential required:** the source needs an authorized secret that is missing or expired.
- **Unsupported:** there is no approved connector for this operation.

Never use a fallback demo connector when a real source is unavailable.

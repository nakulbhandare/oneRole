# Job discovery

**Status:** design for review. Live source integrations are not implemented yet.

## What discovery means

Discovery means reading jobs from a source that OneRole is allowed to access, storing what the source returned, and updating the searchable catalog. It is not an unrestricted crawler.

Discovery and application submission are separate. A live job can be visible even when its Apply button must open the employer's site.

## Discovery modes

| Mode | What starts it | What it does |
|---|---|---|
| Initial backfill | A source is approved for the first time | Reads available pages and imports real postings. It uses source publication dates for the last-60-day view when those dates exist. |
| Incremental sync | The source's next_poll_at time is due | Requests new or changed listings from the last provider cursor or modification timestamp. |
| Priority refresh | A source is in an explicitly configured high-freshness tier | Polls sooner within provider rules to reduce discovery delay for high-priority boards. |
| Webhook update | A documented provider webhook arrives | Verifies the signature, records the provider event, and schedules a fetch of the affected posting. |
| Manual sync | An authorized operator requests it | Creates a sync run subject to the same host allowlist, rate limit, and retry rules. |
| Reconciliation | A regular maintenance run is due | Checks source evidence for changed/closed jobs and corrects stale status. |

The first backfill does not invent dates. If a source supplies no publication time, OneRole records it as unknown and separately records first_seen_at.

## Discovery flow, step by step

1. **Scheduler finds due sources.** It reads active sources whose next_poll_at has passed and whose lease is free.
2. **Source lease is acquired.** A short database lease prevents multiple schedulers from starting the same sync. The lease expires if a worker crashes.
3. **Connector fetches a page.** It uses the source's documented endpoint, TLS, host allowlist, timeout, page limit, and per-source rate token.
4. **Response is checked.** Validate status code, content type, response size, required fields, provider board identity, and pagination. Reject malformed or unexpectedly large responses.
5. **Source observation is stored.** Keep external job ID, canonical posting/apply URLs, timestamps, payload hash, and sync run ID.
6. **Job is normalized.** Map title, employer, location, workplace, salary, employment type, and description. Unknown fields remain unknown. Derived degree, experience, role, and visa values keep provenance and confidence.
7. **Duplicates are matched.** First use provider + external job ID, then canonical URL, then verified employer requisition ID. Similar titles alone never merge two jobs.
8. **Search update is queued.** The transaction writes a search-index outbox event after the normalized job changes.
9. **Checkpoint advances.** Advance the provider cursor only after all records from that page are committed.
10. **Run is finalized.** Store counts, last success, lag, and sanitized errors. Update the next poll time using the source's rate policy and observed changes.

## Dates and open/closed state

OneRole stores these times separately:

- published_at: time the employer/source says the job was published; may be null.
- source_updated_at: source's modification time; may be null.
- first_seen_at: first time OneRole observed the job.
- last_seen_at: most recent successful observation of the job.
- closed_at: time OneRole received credible evidence that the job closed.

The 60-day filter uses published_at when available. Newly discovered uses first_seen_at. OneRole never substitutes first_seen_at for a missing employer publication date without telling the user.

A job is closed when the source explicitly marks it closed, a verified employer page says it is unavailable, or a documented reconciliation rule is satisfied. Missing from one paginated response does not close a job.

## Normalized job fields

A searchable job has:

- OneRole job ID and canonical company ID.
- Provider/source ID and provider's external job ID.
- Title, description, responsibilities, qualifications, skills.
- Original job URL and application URL.
- Country, region, city, remote/hybrid/on-site mode, and location provenance.
- Employment type and career level.
- Required/preferred degree and experience bounds when stated.
- Visa sponsorship yes/no/unknown with evidence.
- Salary range and currency when supplied.
- Source publication and update timestamps.
- First seen, last seen, current status, and source freshness.
- Filter taxonomy version, confidence, and evidence references.
- Application capability and any employer restrictions.

The original source description remains traceable. Normalization does not replace it with AI-generated content.

## How discovery supports the screenshot filters

The normalized fields feed the documented job-search API:

- Keyword and excluded-term search.
- Published-date and first-seen-date ranges.
- Location and radius; separate workplace mode.
- Canonical company include/exclude.
- Degree level and required/preferred strength.
- Maximum stated experience; open-ended and unknown experience are separate.
- Visa sponsorship yes/no/unknown.
- Role family, career level/job type, and employment type.

The UI sends all selections once when Apply is clicked. The API returns jobs, the applied filter object, counts, unknown counts, and a stable cursor. See the [API contract](api-contract.md) for the exact filter names and behavior.

## Search architecture at scale

PostgreSQL remains the source of truth. Local development can use PostgreSQL full-text search. Start with PostgreSQL full-text/trigram search as recorded in ADR-0001. Add a dedicated OpenSearch projection only when measured query volume, relevance, location radius, or facet latency warrants it. The index is a projection that can be rebuilt from PostgreSQL; it never owns the canonical job record.

If an external search index is enabled, an indexer consumes job-change events and writes one document per normalized job. Search documents contain public job fields only, not resumes, answers, tenant IDs, or application history.

Search can report indexing lag. If the index is behind, OneRole reports degraded freshness and can still return the last indexed snapshot. It does not generate fake listings to fill gaps.

## 100,000-company polling reality

A registry of 100,000 companies is reasonable as a directory target. Polling 100,000 independent sources every five minutes would require at least 20,000 requests per minute, or about 333 requests per second, assuming exactly one page per source and no retries. Pagination, regional boards, failed requests, and retries increase that load. The provider's allowed rate limits may be lower than OneRole's capacity.

Therefore the scheduler uses:

- Provider webhooks where documented.
- Adaptive intervals based on freshness tier, source changes, and provider limits.
- Per-company and per-provider rate budgets.
- Priority lanes with fair scheduling.
- Backpressure when source or database capacity is reached.
- Stale status when a source cannot be polled within its target.

“Instant” discovery is possible only when the source sends an event or a sufficiently fast poll is permitted. The UI always includes last sync and the timestamp basis.

## No mock jobs in the product

A job enters the live catalog only with a real source ID, real external ID or verified source URL, and recorded observation time. If the source fails, OneRole returns its last known job with a stale label or omits it according to the API policy. It never substitutes sample postings.

See [real-data and coverage rules](../operations/live-data-and-coverage.md).

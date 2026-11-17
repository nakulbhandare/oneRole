# Live data and coverage rules

**Status:** product/data contract for OneRole.

## What “live” means

A job is live in OneRole only when it was fetched from a real, identified source and its listing is not known to be closed. Each job keeps:

- source and company IDs
- source's external job ID or verified canonical URL
- original job/apply URL
- source published time, when given
- first and most recent observation times
- source health and last successful sync
- source capability and data-quality status

The product never labels a local fixture, generated posting, or guessed company page as a real job.

## No mock data in the product

- No seeded demo jobs enter the normal application database or search index.
- If a real source fails, show its last good records as stale or omit them according to the freshness policy.
- Never fall back to sample jobs, fabricated publication dates, guessed visa sponsorship, or invented application receipts.
- Local source sync is opt-in and read-only unless an employer-approved test integration is configured.
- Automated tests use a separate test database or a provider sandbox. Test records cannot be queried by the normal OneRole API and cannot be labelled live.
- A live application-success metric counts only evidence returned by a real employer system.

## How coverage is reported

Show counts separately:

1. Companies registered.
2. Companies with a configured source.
3. Sources enabled for live discovery.
4. Sources healthy within their freshness target.
5. Jobs seen in the last 24 hours, 7 days, and 60 days, with timestamp basis.
6. Sources with questions available.
7. Company/job flows with verified automated submission.
8. Companies requiring user login/manual action.
9. Sources degraded, paused, credential-required, or unsupported.

A count of companies in the directory is never presented as a count of live feeds or supported application submissions.

## Freshness promises

Freshness is defined per source tier and displayed with the last sync time. Webhooks can provide event-driven updates where documented. Polling sources are only as fresh as their permitted interval and queue capacity. The last-minute jobs view ranks by source publication time where known and separately shows first-seen time.

## External limits

Public APIs and employer flows differ. Some providers expose listing reads but require employer credentials for submissions; others do not expose the employer's custom questions. CAPTCHAs, MFA, login walls, API scopes, source rate limits, and employer restrictions are not bypassed.

OneRole can target a 100,000-company directory and scale its connector fleet, but cannot honestly promise that every company offers a live feed or automated submission. The user interface must display the actual per-company and per-job capability.

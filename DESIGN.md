# OneRole design approach

## What I understand

OneRole is a job discovery and opportunity management product. Its core value is to gather listings from supported, authorized sources, help a person find relevant roles across countries and work styles, and keep the search organized from discovery through application tracking. The product should also support opt-in automatic application submission within user-defined boundaries.

The product needs to be honest about how its data was obtained and how fresh it is. Publication time and the platform's first-seen time are different facts. Source coverage, sync health, and unavailable integrations should be visible. The product must not imply that it covers every employer or that sample listings are live.

Resume parsing and profile matching should reduce repetitive work while keeping the candidate in control. Extracted details need review and editing. Resumes and job-search profiles are private user data, so storage, access, deletion, and any external processing need to be designed in from the start.

## Design direction

- **Primary workflow:** Discover → inspect provenance and fit → save, track, or opt into bounded auto-application → receive matching alerts.
- **Information architecture:** Overview, Discover Jobs, New Jobs, Recommended, Saved Jobs, Applications, Resume & Profile, Job Alerts, and Settings.
- **Discovery screen:** Search by role, skill, company, and location; refine by recency and work preferences; compare listings; open a contextual detail panel without losing the result list.
- **Apply / Pass:** Put both actions on every job card and in its detail view. Clicking Apply is the instruction to send the approved resume/profile packet through a supported employer flow; do not add a second confirmation screen for already-approved answers. Pass hides that role from the candidate's results. A confirmed submission enters My Applications.
- **Job-specific tailoring:** With the candidate's opt-in, draft screening answers from reviewed resume/profile facts and create a role-specific resume version that emphasizes relevant evidence. Keep form answers in the employer form and tailor the resume by reordering or clarifying supported experience. Show evidence used and preserve the original; never invent qualifications, dates, metrics, or outcomes. New or ambiguous facts, sensitive questions, and attestations pause only that application for review.
- **Submission receipt:** OneRole should email the candidate after a successful, verified submission. Attach an employer confirmation email when one is actually received; do not promise that every employer sends one.
- **Trust cues:** Keep the employer/source attribution, published date, first-seen time, and source status visible near the listing. Use clear labels when a date is unknown or a source is unavailable.
- **Auto-apply:** Require explicit opt-in, reviewed resume/profile data, approved answers, and role/location rules. Submit once per unique listing, then keep monitoring its employer source until it confirms removal or closure; a temporary sync error does not count as removal. A missing answer, sensitive question, legal attestation, CAPTCHA, or disallowed flow queues only that listing for review; the overall run continues with other eligible jobs. Never invent answers, bypass employer restrictions, or submit duplicate applications. Record every application and let the user pause the overall run at any time.
- **Visual system:** Temporary OneRole brand, forest-green accents, warm neutral surfaces, compact readable type, restrained status colors, and an information-dense but calm desktop layout. Mobile uses a reduced navigation bar and touch-friendly job details.
- **Accessibility:** Keyboard-operable controls, clear focus states, readable contrast, responsive layouts, and explicit form errors.

## Prototype scope

The current preview includes the daily discovery loop, Apply / Pass actions, and an Auto-apply setup screen. Apply moves a role into a simulated My Applications tracker, while Pass dismisses it. The job detail includes an application-tailoring preview for an evidence-backed answer and tailored resume packet. It also includes the main navigation, role and location search, a remote filter, recent/saved tabs, job cards, a selected job detail view, save controls, alert toggles, auto-apply rules and safeguards, and representative secondary screens. Interactions stay in the browser and are not persisted; no employer is contacted and no email is sent.

All jobs, dates, source labels, and match scores in this prototype are illustrative fixtures. No employer feed, account, resume, email, or external service is connected. This lets us review hierarchy and interaction before committing to data-provider choices.

## Local-first build approach

1. **Keep the current repository as the source of truth.** The workspace was empty, so the prototype is a dependency-free HTML/CSS/JavaScript file that can be opened locally. It does not require credentials, a network call, or a database.
2. **Agree on the interaction model and design tokens.** Review the discovery view, navigation, mobile behavior, source-trust cues, and which profile fields need to be visible before adding production architecture.
3. **Build a local vertical slice.** Use React, TypeScript, and Vite for the product UI, with a typed API boundary and PostgreSQL for persistent domain data. Keep the frontend behind repository interfaces so sample fixtures can later be replaced by API data without reworking the screens.
4. **Run supporting services locally only when needed.** Add a local database, queue, private object-storage emulator, and email sink as the corresponding user journey is implemented. Keep safe defaults in `.env.example`; keep credentials out of the client and source control.
5. **Make demo and live data separate modes.** Demo fixtures should be labeled and isolated. The live mode should show real source records, `posted_at` and `first_seen_at` separately, and explicit errors or stale status when a provider has not synchronized.
6. **Add integrations behind adapters.** Implement one authorized provider at a time with pagination, rate limits, retry handling, stable external IDs, deduplication, and sync records. The UI should not depend on which provider produced a job.
7. **Promote the same boundaries to production.** Replace local services with managed database, private object storage, durable queue/worker, email delivery, and secret management. Deploy staging first, apply reviewed migrations, verify source permissions and refresh health, then promote. Keep workers and scheduled ingestion observable independently from the web app.

## Production decisions to settle after design review

- Which regions and job-source providers should be supported first, based on documented access and licensing?
- Should the first release prioritize resume-based recommendations, location breadth, or new-job freshness?
- Which authentication, email, object-storage, and queue services fit the deployment environment?
- What resume parsing can run locally, and which optional external processing (if any) should require explicit user disclosure?
- Which employer application flows permit automated submission, and what profile answers should be pre-approved versus require a one-time review?
- Which source health and data freshness indicators should candidates see?

The highest product risk is reliable, permitted job data coverage. The design should make narrow but verified coverage useful instead of promising universal discovery.

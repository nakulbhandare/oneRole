# Application preparation and submission engine

**Status:** design for review. No employer submission adapter is implemented or enabled by this document.

## Plain-language rule

The user chooses the jobs and OneRole prepares the application. A worker submits only when the user's consent, verified answers, employer capability, and external rules all allow it. Each job gets its own result and receipt.

OneRole cannot guarantee acceptance, immediate processing, or a confirmation email from an employer. It can accurately report what the employer's system returned.

## Application modes

| Mode | User control | What OneRole does |
|---|---|---|
| Prepare only | User asks for an application packet | Tailors a resume and drafts answers; does not submit |
| Review before submit | User reviews the generated packet and approves it | Waits for explicit approval, then queues one submission |
| Submit when ready | User enabled a specific automation policy or clicked Apply with this mode | Submits automatically only after all facts, consent, capabilities, and validations pass |
| Assisted/manual | Employer flow requires user authentication or action | Provides a prepared packet and a safe link to the employer; user finishes there |

Auto-submit is a policy for eligible jobs, not a way to bypass employer controls. A user can constrain it by role, location, company, employment type, salary, visa requirements, per-run limit, daily limit, and excluded employers. Each policy has a version and consent timestamp.

## What must be true before a worker submits

All checks happen again immediately before the external request:

1. The user/session is valid and the job is still open according to source evidence.
2. Consent is active and covers this company, application mode, and data use.
3. The selected resume version belongs to the user and tenant.
4. Required answers are supported by verified resume/profile facts or an explicit user answer.
5. No unresolved sensitive field, legal declaration, or employer attestation is being guessed.
6. The connector is enabled for this company and current job.
7. Its required credentials are present in the secret manager and have the necessary scope.
8. The employer flow is allowed by current adapter policy and source terms.
9. The job is not already applied to or in an uncertain prior submission state.
10. Per-user, per-tenant, per-company, and per-provider rate limits permit the attempt.

If one item fails, OneRole writes a specific pause reason. It does not block other jobs in the same batch.

## End-to-end Apply flow

1. **User action:** Apply button or authorized auto-apply policy selects a job.
2. **API transaction:** Validate user and tenant; write application intent, consent version, idempotency key, audit event, and outbox event.
3. **Preparation worker:** Load job and verified user evidence; create a versioned packet and answer set.
4. **Deterministic validation:** Check required fields, answer types, format, evidence links, user exclusions, and application policy.
5. **Capability check:** Read the current company/job adapter capability snapshot.
6. **Submission claim:** Atomically transition the application to queued/submitting and allocate one attempt ID.
7. **Connector call:** Send a bounded request through the company adapter with timeout, rate limit, trace ID, and external idempotency support if offered.
8. **Persist result:** Store sanitized response classification and any external reference/receipt before queue acknowledgment.
9. **Final status:** Use submitted only when evidence supports it. Use submission_unconfirmed for ambiguous responses.
10. **Notify:** Show the exact result and next action in OneRole. Employer confirmation email is outside OneRole's control.

## State machine

DISCOVERED → MATCHED → PREPARING → NEEDS_USER_INPUT or READY_FOR_REVIEW → APPROVED → QUEUED → SUBMITTING → SUBMITTED

Other supported outcomes are SKIPPED, REJECTED_BY_USER, DUPLICATE, EXPIRED, FAILED_RETRYABLE, FAILED_PERMANENT, REQUIRES_MANUAL_ACTION, and SUBMISSION_UNCONFIRMED.

Only the service layer can change status. It writes an append-only event for every transition. Clients cannot send status=SUBMITTED.

## Idempotency and uncertain outcomes

- The API requires an Idempotency-Key for Apply and batch creation. Repeated identical requests return the same workflow.
- A database unique constraint prevents two active applications for the same tenant/user/job.
- A queue inbox makes duplicate task delivery harmless.
- Each external attempt has a durable attempt ID and connector version.
- If an ATS supports an idempotency key, send it and store it.
- If the request may have reached an employer but the response was lost, mark the result unconfirmed and reconcile through a documented status/receipt path.
- Do not retry an ambiguous non-idempotent request. A retry could create duplicate external applications.

## Connector capability and real employer limits

Job listing access and application submit access are separate. Each company-source record must say what is supported.

- **Greenhouse:** The official Job Board API can list jobs and submit applications, but submission requires an API key. Its documentation says required-field validation must be performed by the submitting client. Therefore, OneRole needs an employer-authorized credential and its own exact preflight validation before submission. [Greenhouse Job Board API](https://docs.greenhouse.io/job-board)
- **Lever:** Its official Postings API supports listing published jobs and programmatic application, but the public API documentation says custom questions are not exposed. A job requiring those questions cannot be classified as fully automated through that public interface. [Lever Postings API](https://github.com/lever/postings-api)
- **Other ATS products:** Require current official documentation and a verified access method. Do not infer support from seeing an apply URL.

A connector capability snapshot includes discovery support, question access, submission method, authentication need, required scopes, per-company credential reference, form schema version, rate limit, last verified time, and unsupported steps.

## Answer and resume integrity

AI may rewrite or emphasize verified facts; it may not invent facts. Store each answer with source fact IDs, evidence locations, prompt/model version, validation status, and review state.

The following require explicit user input or review when asked by an employer:

- work authorization or sponsorship need
- salary expectation
- employment dates or years of experience
- degree/certification claims
- legal declarations
- criminal history
- disability or demographic information
- policy agreement or consent
- any question with no supporting evidence

An unresolved field pauses only the affected application. The user's original resume stays immutable; every tailored resume is a new derived artifact.

## Employer response and confirmation

Persist the attempt ID, adapter and version, started/finished time, response class, external application/reference ID, sanitized metadata, retry count, and failure reason.

A successful TCP/HTTP response is not by itself proof. OneRole marks the application submitted only when the ATS indicates acceptance or supplies a durable confirmation/reference in the supported flow. If the ATS is ambiguous, report unconfirmed and reconcile.

The employer controls confirmation emails. OneRole may optionally reconcile through a separately authorized mailbox connection, but it cannot make an employer send an email.

## 100,000-company live submission boundary

A company directory with 100,000 entries is a scale target. It does not imply that 100,000 employers give OneRole credentials, expose compatible questions, permit automated submission, or share compatible APIs. Submission support is enabled per verified adapter and per company source.

The product must report the number of companies with:

- live discovery
- fresh discovery
- readable application requirements
- authorized submission access
- verified confirmation evidence

No UI counter may label discovered jobs as submitted applications.

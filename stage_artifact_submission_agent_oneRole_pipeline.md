# Submission Workflow Agent - Improvements

The submission workflow has been enhanced to robustly handle stuck applications and improve diagnostic capabilities.

## Key Improvements

### 1. Stuck Application Detection & Reclamation
- **Watchdog Monitor**: A background monitor (`startStuckMonitor`) runs every 5 minutes to detect applications stuck in `in_flight` status for >15 minutes.
- **Optimized Batching**: Reclamation now processes applications in batches to ensure system stability when handling large volumes (100k+).
- **Status Transitions**: Applications detected as stuck are transitioned to `failed_retryable` with the pause reason `stuck_in_flight_timeout`.

### 2. Diagnostic & Resolution Enhancements
- **SubmissionContext**: Introduced `SubmissionContext` in `browser_submitter.js` to log granular submission steps and navigation timeouts for better failure analysis.
- **`requires_manual_action`**: Improved status handling to transition unrecoverable errors to `requires_manual_action` instead of `failed_retryable`, reducing unnecessary retries.
- **Generalized Solution Search**: Implemented `searchGeneralizedSolution` and `fetchKnowledgeBaseSolution` to facilitate automated resolution for known stuck scenarios.

### 3. Tooling & Visibility
- **`cmd/report_stuck`**: New CLI tool to identify degraded or paused sources for manual intervention.
- **Audit Logs**: Enhanced diagnostic audit logs for improved traceability of submission failures.

These changes collectively improve the reliability of the submission engine, reducing manual intervention requirements and providing better visibility into submission health.

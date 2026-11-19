# Implementation Plan: Company Job Tracking and Validation

To address the requirements:
1.  **Concurrent fetching per company**: Already largely addressed by `SourceSyncScheduler` (with company-level semaphore) and `SourceSyncEventHandler`.
2.  **Log count per company**: Implemented `CountJobsPerCompany` in `repository/sources.go`.
3.  **Maintain status DB**: Already supported by existing repository methods.
4.  **Expose APIs/UI for company job tracking**: Ensure existing APIs/UI are integrated or enhanced.
5.  **Periodic verification/validation of jobs**: Added skeleton for `JobValidatorService` in `service/validator/job_validator.go`.
6.  **Generalized process using JSON mappings**: Already supported by `connectors/ats_mappings`.

## Implemented Changes:
1.  **Repository**:
    - Added `CountJobsPerCompany` to `JobSourceSyncStore` interface in `repository/sources.go`.
    - Implemented `CountJobsPerCompany` in `JobSourceRepository` in `repository/sources.go`.
2.  **Validator**:
    - Created `service/validator/job_validator.go` with `JobValidator` interface and `JobValidatorService` skeleton.

## Next Steps (Future Workers):
- Implement `JobValidatorService.ValidateJobs` logic.
- Implement a `JobValidationWorker` to periodically call `JobValidatorService`.
- Expose the count via an API endpoint.

# Submission Workflow Agent - Stuck Detection Improvement

## Contribution
Implemented a watchdog mechanism in `AutoApplyEngine` to monitor applications that become stuck in `ApplicationInFlight` state. 

### Key Changes
- **`AutoApplyEngine.startStuckMonitor`**: Added a background ticker running every 5 minutes.
- **`AutoApplyEngine.reclaimStuckApplications`**: Added functionality to scan for `ApplicationInFlight` applications that haven't been updated for 15+ minutes.
- **Auto-Retry & Diagnostic**: Stuck applications are automatically marked as `ApplicationFailedRetryable` with the pause reason `stuck_in_flight_timeout`.
- **Generalized Solution Search**: Added a stub function `searchGeneralizedSolution` to facilitate future integration with generalized solution search mechanisms like TSenta for persistent issues.

This ensures the submission engine remains robust in the face of hung browser sessions or agent crashes.

## Files Modified
- `/Users/nakulbhandare/go/src/github.com/oneRole/service/auto_apply_engine.go`

## Next Steps
- Implement the actual TSenta integration or other automated resolution logic in `searchGeneralizedSolution`.
- Enhance logging for diagnostic purposes.

# Fixes for `connectors/ats_submit`

-   **Rate Limiting**: Implemented `Retry-After` header parsing in `client.go` to respect rate limits (HTTP 429) rather than simple exponential backoff.
-   **Tests**: Identified failures in `TestStressClient_Redirects301_302_Greenhouse` related to redirect loops.
-   **Future Work**: Further investigation into redirect loop error messages and robust context propagation.

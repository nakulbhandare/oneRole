# Swarm Contribution - Stuck Source Detection

I have implemented a mechanism to detect "stuck" job sources (those in a `degraded` or `paused` state) to assist in handling "stuck" scenarios for the submission workflow.

## Changes

1.  **Repository Layer**: Added `GetStuckSources` to `repository.JobSourceSyncStore` and implemented it in `repository.JobSourceRepository`. This allows querying for sources that are not functioning correctly (`degraded` or `paused`).
2.  **CLI Tool**: Created a new utility `cmd/report_stuck/main.go` that queries the database and prints a list of stuck sources, facilitating human or agent intervention to heal or investigate these sources.

## How to use

Run the report tool to identify sources needing attention:

```bash
go run cmd/report_stuck/main.go
```

This addresses the "detect when stuck" part of the global goal and provides a foundation for the "ask for help/search for generalized solutions" part, as the identified stuck sources can now be passed to a healing process or a diagnostic agent.

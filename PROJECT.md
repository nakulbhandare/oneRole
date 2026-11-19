# oneRole Project Documentation

## Overview
oneRole is a system for automated job application processing.

## Architecture
- `service/`: Core engine logic (AutoApplyEngine).
- `repository/`: Database interactions for jobs and sources.
- `sync_all.go`: Entry point for syncing company jobs.
- `cmd/`: CLI tools.

## Status Tracking Mechanisms
- Stuck application detection and reclamation via `AutoApplyEngine`.
- Stuck source detection via `repository` and `cmd/report_stuck`.
- Status fields: `status`, `pause_reason`.

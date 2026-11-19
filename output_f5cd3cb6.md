# Contribution: Application Validator

Implemented `ValidateApplication` in `service/validator/application_validator.go` to ensure application data integrity and correct state transitions before submission.
Integrated this validator into `service/auto_apply_engine.go` within the `SubmitOneClick` submission flow to fail fast on invalid applications, improving overall submission accuracy and engine throughput, which is essential for scaling to 100k+ data.

- Files modified:
  - `service/auto_apply_engine.go`
- Files created:
  - `service/validator/application_validator.go`
  - `service/validator/application_validator_test.go`

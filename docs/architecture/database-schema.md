# OneRole MongoDB Database Schema

This document details the MongoDB collection structures, document schemas, and indexes for OneRole.

## Collections

### 1. `auth_oauth_transactions`
- `_id`: String (state hash)
- `provider`: String ("google", "github")
- `state_hash`: Binary
- `pkce_verifier_ciphertext`: Binary
- `oidc_nonce_hash`: Binary (optional)
- `browser_binding_hash`: Binary
- `fixed_return_path`: String
- `expires_at`: Date (TTL index)
- `created_at`: Date
- `consumed_at`: Date (optional)

### 2. `auth_identities`
- `_id`: String (UUID)
- `user_id`: String (UUID)
- `provider`: String ("google", "github")
- `provider_subject`: String
- `provider_email`: String (optional)
- `provider_email_verified`: Boolean
- `linked_at`: Date
- `last_login_at`: Date
- **Indexes**: Unique on `(provider, provider_subject)`

### 3. `users`
- `_id`: String (UUID)
- `email`: String (optional)
- `email_verified_at`: Date (optional)
- `display_name`: String
- `status`: String ("active")
- `created_at`: Date
- `updated_at`: Date
- **Indexes**: Unique on `email`

### 4. `tenant_memberships`
- `_id`: String (UUID)
- `tenant_id`: String (UUID)
- `user_id`: String (UUID)
- `role`: String ("owner", "member")
- `status`: String ("active")
- `created_at`: Date
- **Indexes**: Unique on `(tenant_id, user_id)`

### 5. `auth_sessions`
- `_id`: String (UUID)
- `user_id`: String (UUID)
- `family_id`: String (UUID)
- `refresh_token_hash`: Binary
- `created_at`: Date
- `last_used_at`: Date
- `idle_expires_at`: Date
- `absolute_expires_at`: Date
- `revoked_at`: Date (optional)
- `revoke_reason`: String (optional)
- **Indexes**: Index on `refresh_token_hash`, `user_id`

### 6. `auth_session_refresh_tokens`
- `session_id`: String (UUID)
- `refresh_token_hash`: Binary
- `issued_at`: Date
- `consumed_at`: Date (optional)
- **Indexes**: Unique on `refresh_token_hash`

### 7. `jobs`
- `_id`: String (UUID)
- `company_id`: String (UUID, optional)
- `company_name`: String
- `canonical_title`: String
- `description_text`: String
- `responsibilities_text`: String
- `qualifications_text`: String
- `display_location`: String
- `country_code`: String (optional)
- `region`: String (optional)
- `city`: String (optional)
- `workplace_modes`: Array of String
- `role_ids`: Array of String
- `job_type`: String (optional)
- `employment_types`: Array of String
- `degree_levels`: Array of String
- `degree_requirement`: String (optional)
- `min_experience_years`: Double (optional)
- `max_experience_years`: Double (optional)
- `visa_sponsorship`: String
- `source_published_at`: Date (optional)
- `publication_time_quality`: String
- `first_seen_at`: Date
- `last_seen_at`: Date
- `application_capability`: String
- `source_url`: String
- `salary_max`: Double (optional)
- `status`: String ("open", "stale", "closed")
- **Indexes**: Text index on `canonical_title`, `description_text`, `responsibilities_text`, `qualifications_text`; compound indexes for status, sorting, and faceted filtering.

### 8. `companies`
- `_id`: String (UUID)
- `canonical_name`: String
- `canonical_domain`: String
- `logo_url`: String (optional)
- `verification_status`: String
- **Indexes**: Unique on `canonical_domain`, text index on `canonical_name`

### 9. `applications`
- `_id`: String (UUID)
- `tenant_id`: String (UUID)
- `user_id`: String (UUID)
- `job_id`: String (UUID)
- `batch_id`: String (UUID, optional)
- `resume_version_id`: String (UUID, optional)
- `consent_id`: String (UUID, optional)
- `packet_id`: String (UUID, optional)
- `status`: String ("preparing", "ready_for_review", "queued", "submitting", "submitted", "requires_manual_action", "failed_retryable", "failed_permanent")
- `submission_mode`: String
- `capability_snapshot`: String
- `capability_config_version`: Integer
- `version`: Long
- `pause_reason`: String (optional)
- `created_at`: Date
- `updated_at`: Date
- **Indexes**: Unique compound on `(tenant_id, user_id, job_id)`

### 10. `application_batches`
- `_id`: String (UUID)
- `tenant_id`: String (UUID)
- `user_id`: String (UUID)
- `consent_id`: String (UUID)
- `submission_mode`: String
- `requested_count`: Integer
- `status`: String
- `created_at`: Date
- `updated_at`: Date

### 11. `application_batch_items`
- `_id`: String (`batch_id:job_id`)
- `tenant_id`: String (UUID)
- `batch_id`: String (UUID)
- `job_id`: String (UUID)
- `status`: String
- `result_code`: String (optional)
- `application_id`: String (UUID, optional)
- `created_at`: Date
- `updated_at`: Date

### 12. `idempotency_keys`
- `_id`: String (`tenant_id:user_id:operation:key_hash`)
- `tenant_id`: String (UUID)
- `user_id`: String (UUID)
- `operation`: String
- `key_hash`: Binary
- `request_hash`: Binary
- `result_resource_id`: String (UUID, optional)
- `expires_at`: Date (TTL index)
- `created_at`: Date

### 13. `inbox_messages`
- `_id`: String (`consumer_name:event_id`)
- `consumer_name`: String
- `event_id`: String (UUID)
- `status`: String ("processing", "processed")
- `claim_owner`: String (optional)
- `claim_token`: String (UUID, optional)
- `lease_until`: Date (optional)
- `next_attempt_at`: Date
- `attempt_count`: Integer
- `processed_at`: Date (optional)
- `last_error_class`: String (optional)
- `result_resource_id`: String (UUID, optional)

### 14. `outbox_events`
- `_id`: String (UUID)
- `tenant_id`: String (UUID, optional)
- `event_type`: String
- `schema_version`: Integer
- `aggregate_type`: String
- `aggregate_id`: String
- `payload_ids_only`: Binary (JSON)
- `created_at`: Date
- `published_at`: Date (optional)
- `next_attempt_at`: Date
- `lease_owner`: String (optional)
- `lease_token`: String (UUID, optional)
- `lease_until`: Date (optional)
- `publish_attempts`: Integer
- `last_error_class`: String (optional)

### 15. `company_sources`
- `_id`: String (UUID)
- `company_id`: String (UUID)
- `provider_type`: String ("greenhouse", "lever", "ashby")
- `board_identifier`: String
- `source_endpoint`: String
- `allowed_hosts`: Array of String
- `allowed_operations`: Array of String
- `status`: String ("enabled", "degraded", "paused")
- `discovery_enabled`: Boolean
- `requests_per_window`: Integer
- `window_seconds`: Integer
- `max_concurrency`: Integer
- `config_version`: Integer
- `connector_version`: String
- `policy_reviewed_at`: Date (optional)
- `last_success_at`: Date (optional)
- `lease_owner`: String (optional)
- `lease_token`: String (UUID, optional)
- `lease_until`: Date (optional)
- `next_poll_at`: Date (optional)
- `poll_interval_seconds`: Integer

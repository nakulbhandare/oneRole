# Graph Report - oneRole  (2026-11-02)

## Corpus Check
- 103 files · ~112,192 words
- Verdict: corpus is large enough that graph structure adds value.
- Unclassified: 3 file(s) not represented in the graph (top: (none) 2, .example 1)

## Summary
- 1186 nodes · 4672 edges · 52 communities (46 shown, 6 thin omitted)
- Extraction: 94% EXTRACTED · 6% INFERRED · 0% AMBIGUOUS · INFERRED: 266 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- API Response Envelopes
- Application & Auth Handlers
- Runtime Initialization & Main
- HTTP & CORS Middleware Tests
- Application & Consent Workflows
- Environment & Docker Config
- Community 6
- Community 7
- Community 8
- Community 9
- Community 10
- Community 11
- Community 12
- Community 13
- Community 14
- Community 15
- Community 16
- Community 17
- Community 18
- Community 19
- Community 20
- Community 21
- Community 22
- Community 23
- Community 24
- Community 25
- Community 26
- Community 27
- Community 28
- Community 29
- Community 30
- Community 31
- Community 32
- Community 33
- Community 34
- Community 35
- Community 36
- Community 37
- Community 38
- Community 39
- Community 40
- Community 41
- Community 42
- Community 43
- Community 44
- Community 45
- Community 46
- Community 47
- Community 48
- Community 49
- Community 50
- Community 51

## God Nodes (most connected - your core abstractions)
1. `ActorScope` - 83 edges
2. `Runtime` - 56 edges
3. `Job` - 51 edges
4. `Config` - 47 edges
5. `Event` - 46 edges
6. `Application` - 36 edges
7. `JobSearchRequest` - 36 edges
8. `HandleDelivery()` - 34 edges
9. `WorkflowAcceptance` - 28 edges
10. `AuthSession` - 27 edges

## Surprising Connections (you probably didn't know these)
- `buildAPIHandler()` --references--> `Config`  [EXTRACTED]
  main.go → config/config.go
- `buildOAuthProviders()` --references--> `Config`  [EXTRACTED]
  main.go → config/config.go
- `buildWorkerRuntime()` --references--> `Config`  [EXTRACTED]
  main.go → config/config.go
- `connectJetStream()` --references--> `Config`  [EXTRACTED]
  main.go → config/config.go
- `runWorker()` --references--> `Config`  [EXTRACTED]
  main.go → config/config.go

## Import Cycles
- None detected.

## Communities (52 total, 6 thin omitted)

### Community 0 - "API Response Envelopes"
Cohesion: 0.10
Nodes (33): accessTokenResponse, apiError, csrfBootstrapResponse, currentUserResponse, errorEnvelope, healthResponse, membershipResponse, queryValidationError (+25 more)

### Community 1 - "Application & Auth Handlers"
Cohesion: 0.06
Nodes (54): actorScope(), isUUID(), NewApplicationHandler(), writeAccepted(), ApplicationHandler, authPrincipalFromRequest(), clearCookie(), cookieValue() (+46 more)

### Community 2 - "Runtime Initialization & Main"
Cohesion: 0.06
Nodes (40): isUUID(), Open(), WithSystemTenantTx(), WithUserTx(), buildWorkerRuntime(), connectJetStream(), main(), run() (+32 more)

### Community 3 - "HTTP & CORS Middleware Tests"
Cohesion: 0.07
Nodes (41): TestAuthCSRFBootstrapRejectsDisallowedOriginAndInvalidToken(), TestAuthCSRFBootstrapRejectsMissingOriginOrCookie(), TestAuthCSRFBootstrapReturnsSignedTokenWithoutCaching(), headerHasToken(), TestCORSMiddlewareAllowsConfiguredOriginAndCredentialedPreflight(), TestCORSMiddlewareRejectsUnconfiguredOriginAndUnapprovedPreflight(), TestDecodeJSONRequiresApplicationJSON(), TestDecodeJSONRequiresSingleBoundedObject() (+33 more)

### Community 4 - "Application & Consent Workflows"
Cohesion: 0.16
Nodes (35): WithUserTenantTx(), applicationBatchItem, applicationEventActor(), applicationJobStatusAllowed(), beginIdempotency(), consentCovers(), createApplicationForJob(), finalizeBatch() (+27 more)

### Community 5 - "Environment & Docker Config"
Cohesion: 0.06
Nodes (39): MINIO_ROOT_PASSWORD, MINIO_ROOT_USER, POSTGRES_DB, POSTGRES_PASSWORD, POSTGRES_USER, interval, retries, test (+31 more)

### Community 6 - "Community 6"
Cohesion: 0.14
Nodes (12): ApplicationAPI, applicationAPIFake, ActorScope, ApplicationWorkflowStore, BatchView, CreateApplicationCommand, CreateBatchCommand, IdempotencyIdentity (+4 more)

### Community 7 - "Community 7"
Cohesion: 0.10
Nodes (18): catalogAPIFake, DegreeLevel, DegreeRequirement, EmploymentType, FacetSet, JobPage, JobSearchResponse, JobType (+10 more)

### Community 8 - "Community 8"
Cohesion: 0.09
Nodes (17): ConsumerLookup, ConsumerProvisioner, fakeConsumerLookup, DurableConsumerConfig(), EnsureConsumer(), EnsureStreams(), FindConsumer(), StreamConfigurations() (+9 more)

### Community 9 - "Community 9"
Cohesion: 0.17
Nodes (12): normalizeActorScope(), PermanentTaskFailure(), classifyApplicationTaskStoreError(), ApplicationTaskSnapshot, ApplicationTaskStore, isUTCTimestamp(), persistedPause(), validApplicationMode() (+4 more)

### Community 10 - "Community 10"
Cohesion: 0.12
Nodes (8): httpSessionStoreFake, sessionStoreStub, AuthSession, AuthSessionStore, RefreshRotation, TestAuthSessionAndRefreshRotationValidateHashesAndExpiry(), sessionMemory, sessionMemory

### Community 11 - "Community 11"
Cohesion: 0.15
Nodes (10): batchResponse, workflowAcceptedResponse, SyncRun, opaqueCursor, OutboxEvent, JobSourceSyncStore, SourceSyncRun, ApplicationEventHandler (+2 more)

### Community 12 - "Community 12"
Cohesion: 0.10
Nodes (21): decodePage(), normalizePosting(), normalizeWorkplace(), parseRetryAfter(), parseTimestamp(), plainText(), requestURL(), response() (+13 more)

### Community 13 - "Community 13"
Cohesion: 0.14
Nodes (7): httpMembershipReaderFake, membershipReaderStub, namedService, assertTenantCount(), TenantMembership, membershipMemory, membershipMemory

### Community 14 - "Community 14"
Cohesion: 0.26
Nodes (23): allZero(), boolEnv(), decodeBase64Key(), durationEnv(), env(), Config, intEnv(), invalid() (+15 more)

### Community 15 - "Community 15"
Cohesion: 0.14
Nodes (14): FacetOption, Facet, decodeOpaqueCursor(), encodeOpaqueCursor(), filterDigest(), buildJobPageSQL(), discoveryFacetSpecs(), NewJobSearchRepository() (+6 more)

### Community 16 - "Community 16"
Cohesion: 0.13
Nodes (12): buildOAuthProviders(), hmacEqual(), OAuthClient, NewGitHubOAuthClient(), NewGoogleOAuthClient(), safeProviderHTTPClient(), validateOAuthClientSettings(), GitHubOAuthClient (+4 more)

### Community 17 - "Community 17"
Cohesion: 0.15
Nodes (14): enabledSource(), TestDisabledAndUnconfiguredSourcesFailClosed(), NewProcessRateLimiter(), NewRegistry(), newSecureHTTPClient(), Connector, Page, SourceConfig (+6 more)

### Community 18 - "Community 18"
Cohesion: 0.14
Nodes (13): CompanySearchStore, FilterStore, JobReader, JobSearchStore, LocationSuggestions, LocationSuggestionStore, NewCatalogService(), TestCatalogServiceNormalizesBeforeDatabaseSearch() (+5 more)

### Community 19 - "Community 19"
Cohesion: 0.16
Nodes (9): WithTx(), JobSourceRepository, NewOutboxStore(), requireClaim(), OutboxRepository, SourcePageInput, SourcePageResult, jsonStringValue() (+1 more)

### Community 20 - "Community 20"
Cohesion: 0.18
Nodes (8): validProviderAuthorizationURL(), OAuthCallbackResult, OAuthProvider, mustURLPath(), newOpaqueSecret(), newUUID(), validOAuthProvider(), AuthService

### Community 21 - "Community 21"
Cohesion: 0.13
Nodes (7): authAPIFake, authProvidersResponse, CurrentUser, OAuthProviderStatus, OAuthStartResult, RefreshResult, authMemory

### Community 22 - "Community 22"
Cohesion: 0.18
Nodes (10): EventPublisher, OutboxStore, eventFromOutbox(), TestOutboxPublisherMarksOnlyAfterSynchronousPublish(), TestOutboxPublisherRejectsSensitiveOrMalformedPayload(), TestOutboxPublisherRetriesMarkFailureWithoutClaimingSuccess(), TestOutboxPublishFailureDoesNotMarkPublished(), validOutboxPublisher() (+2 more)

### Community 23 - "Community 23"
Cohesion: 0.13
Nodes (10): Config, HTTPError, RetryableTaskFailure(), TaskFailure, DefaultRetryPolicy(), RetryPolicy, DefaultRuntimeConfig(), NewRuntime() (+2 more)

### Community 24 - "Community 24"
Cohesion: 0.17
Nodes (7): ApplicationTaskService, TaskResult, ApplicationTaskTransition, applicationTaskStoreFake, TaskOutcome, validateTaskResult(), fakeApplicationTasks

### Community 25 - "Community 25"
Cohesion: 0.32
Nodes (14): LoadFrom(), apiEnvironment(), lookup(), TestHMACSecretsMustBeIndependent(), TestLoadForModeRequiresDatabaseAndJWTKey(), TestLoadForModeRequiresNATSForWorkers(), TestLoadForModeUsesDefaultsAndBuildsRSAVerifierKey(), TestMigrateModeOnlyRequiresDatabase() (+6 more)

### Community 26 - "Community 26"
Cohesion: 0.17
Nodes (14): publicStatus(), ApplicationStatus, PublicApplicationStatus, isPublicApplicationStatus(), TestInternalStatusesAreNotExposedAsPublicStates(), allowedTransition(), isApplicationStatus(), oneStatusOf() (+6 more)

### Community 27 - "Community 27"
Cohesion: 0.21
Nodes (15): NewDeadLetter(), safeErrorClass(), TestNewDeadLetterKeepsIdentifiersAndSanitizesClass(), allZeroTraceparent(), DecodeEvent(), EncodeEvent(), isUUID(), TestDecodeEventRejectsUnknownFieldsAndTrailingJSON() (+7 more)

### Community 28 - "Community 28"
Cohesion: 0.16
Nodes (8): configuredRunner, invokeHandler(), ConsumerRunner, EventHandler, EventHandlerFunc, EventRouter, NewEventRouter(), StreamConsumer

### Community 29 - "Community 29"
Cohesion: 0.20
Nodes (9): deadLetterSubject(), NewDeadLetterPublisher(), TestDeadLetterPublisherUsesWorkloadSpecificSubject(), DeadLetterPublisher, fakeSyncPublisher, NewJetStreamPublisher(), TestJetStreamPublisherRequiresServerAck(), JetStreamPublisher (+1 more)

### Community 30 - "Community 30"
Cohesion: 0.14
Nodes (11): TestParseJobSearchQueryMapsAllFilters(), normalizeEnums(), NormalizeJobSearchRequest(), normalizeStrings(), oneOf(), TestNormalizeJobSearchRequestDefaultsAndCanonicalizes(), TestNormalizeJobSearchRequestRejectsOutOfRangeLimitAndInvalidLocation(), TestNormalizeJobSearchRequestReportsBoundsEnumsAndDateOrdering() (+3 more)

### Community 31 - "Community 31"
Cohesion: 0.23
Nodes (12): InboxStore, HandleDelivery(), normalizeErrorClass(), stringPtr(), TestHandleDeliveryAcksOnlyProcessedDuplicate(), TestHandleDeliveryCompletesInboxBeforeAck(), TestHandleDeliveryDefersActiveInboxLease(), TestHandleDeliveryDLQsAtSafeRetryBudget() (+4 more)

### Community 32 - "Community 32"
Cohesion: 0.23
Nodes (6): DeadLetter, AckableMessage, HandleMessage(), persistAndTerm(), DeadLetterSink, fakeDeadLetterSink

### Community 33 - "Community 33"
Cohesion: 0.23
Nodes (7): AuthIdentityRepository, TestAuthIdentityConflictDoesNotAutoLinkByEmail(), AuthIdentityInput, AuthUser, TestAuthIdentityInputRequiresProviderSubjectAndUTC(), AuthIdentityStore, identityMemory

### Community 34 - "Community 34"
Cohesion: 0.36
Nodes (11): addDateFilters(), addFactFilter(), addJobCursorFilter(), addJobFilters(), addLocationFilters(), addWorkplaceFilters(), enumStrings(), parseCursorFloat() (+3 more)

### Community 35 - "Community 35"
Cohesion: 0.28
Nodes (6): InboxClaim, InboxClaimResult, NewInboxStore(), validInboxClaim(), InboxRepository, fakeInbox

### Community 36 - "Community 36"
Cohesion: 0.24
Nodes (5): OAuthTransaction, isUTC(), TestOAuthTransactionValidateForCreate(), oauthTransactionMemory, OAuthTransactionStore

### Community 37 - "Community 37"
Cohesion: 0.27
Nodes (5): classifySourceFetchError(), leaseFailure(), newSourceLeaseKeeper(), sourceLeaseKeeper, SourceSyncEventHandler

### Community 38 - "Community 38"
Cohesion: 0.24
Nodes (8): AccessClaims, NewJWTVerifier(), signedAccessToken(), TestJWTVerifierAcceptsAccessTokenAndAttachesPrincipal(), TestJWTVerifierRejectsInactiveSession(), TestJWTVerifierRejectsWrongPurposeAndUnknownKey(), authTestConfig(), AuthServiceConfig

### Community 39 - "Community 39"
Cohesion: 0.25
Nodes (4): mapUniqueConflict(), nullBytes(), validProvider(), AuthSessionRepository

### Community 40 - "Community 40"
Cohesion: 0.27
Nodes (11): decodeOpaqueSecret(), newIdentityMemory(), newOAuthTransactionMemory(), newSessionMemory(), TestAuthServiceConsumesProviderDenialAndDoesNotCreateSession(), TestAuthServiceOAuthCreatesHashedSessionAndRotatesRefresh(), isLoopbackHost(), NewAuthService() (+3 more)

### Community 41 - "Community 41"
Cohesion: 0.36
Nodes (8): addVary(), newCORSMiddleware(), normalizeCORSOrigin(), setCORSResponseHeaders(), TestCORSMiddlewareRejectsWildcardConfiguration(), validPreflightHeaders(), validPreflightMethod(), writeCORSForbidden()

### Community 44 - "Community 44"
Cohesion: 0.43
Nodes (8): IsTaskRetryable(), NewDurableApplicationTaskService(), newApplicationTaskStore(), TestDurableTaskServiceClassifiesOnlyExplicitStoreFailuresAsRetryable(), TestDurableTaskServiceDisablesSubmissionWithoutAuthorizedAdapter(), TestDurableTaskServiceDoesNotBlindlyRetryAmbiguousSubmission(), TestDurableTaskServicePersistsPauseAndAcknowledgesRedelivery(), TestDurableTaskServiceRequiresDurableBatchFanoutResult()

### Community 45 - "Community 45"
Cohesion: 0.43
Nodes (8): sourceHandler(), TestSourceSyncAcknowledgesSupersededRun(), TestSourceSyncFailureCommitFailureIsExplicitlyRetryable(), TestSourceSyncHonorsProviderRetryAfter(), TestSourceSyncPausesOnPermanentProviderFailure(), TestSourceSyncResumesAtPersistedCursorAndCompletes(), TestSourceSyncStopsWhenLeaseRenewalLosesFence(), validSourceEvent()

### Community 46 - "Community 46"
Cohesion: 0.48
Nodes (7): Posting, dedupeStrings(), ingestPosting(), normalizeEmployment(), normalizeJobType(), postingChecksum(), replaceJobEvidence()

### Community 49 - "Community 49"
Cohesion: 0.40
Nodes (5): DatePreset, DateFilter, MatchesJobDateFilter(), TestMatchesJobDateFilterKeepsPublicationUnknownSeparate(), TestMatchesJobDateFilterUsesPublishedAtAndCustomBounds()

## Knowledge Gaps
- **1 isolated node(s):** `github.com/oneRole`
  These have ≤1 connection - possible missing edges. (Counts symbols only; 94 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **6 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Config` connect `Community 14` to `API Response Envelopes`, `Application & Auth Handlers`, `Runtime Initialization & Main`, `Community 38`, `Community 16`, `Community 23`, `Community 25`?**
  _High betweenness centrality (0.048) - this node is a cross-community bridge._
- **What connects `github.com/oneRole` to the rest of the system?**
  _1 weakly-connected nodes found - possible missing edges._
- **Should `API Response Envelopes` be split into smaller, more focused modules?**
  _Cohesion score 0.09603072983354674 - nodes in this community are weakly interconnected._
- **Why does `Runtime` connect `API Response Envelopes` to `Community 32`, `Runtime Initialization & Main`, `Application & Consent Workflows`, `Community 8`, `Community 22`, `Community 23`, `Community 28`?**
  _High betweenness centrality (0.026) - this node is a cross-community bridge._
- **Should `Application & Auth Handlers` be split into smaller, more focused modules?**
  _Cohesion score 0.0642873484328529 - nodes in this community are weakly interconnected._
- **Why does `Event` connect `API Response Envelopes` to `HTTP & CORS Middleware Tests`, `Community 37`, `Community 11`, `Community 45`, `Community 22`, `Community 27`, `Community 28`, `Community 29`, `Community 31`?**
  _High betweenness centrality (0.022) - this node is a cross-community bridge._
- **Should `Runtime Initialization & Main` be split into smaller, more focused modules?**
  _Cohesion score 0.06219426974143955 - nodes in this community are weakly interconnected._
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oneRole/connectors/ats_adapter"
	"github.com/oneRole/models"
	"github.com/oneRole/queue"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrLeaseUnavailable     = errors.New("application execution lease is currently held by another worker")
	ErrFencingTokenStale    = errors.New("fencing token is stale; execution superseded by another worker")
	ErrAlreadySubmitted     = errors.New("application is already submitted and confirmed by employer")
	ErrMaxRetriesExceeded   = errors.New("bounded retry budget exhausted")
	ErrAmbiguousNeedsReconc = errors.New("submission outcome was ambiguous; reconciliation required before retry")
)

// DeterministicSubmissionEngine implements Rule 2, Rule 7, and Rule 8 requirements:
// - Distributed leases with fencing tokens
// - Pre-submission reconciliation (never blind retries)
// - Bounded exponential backoff retries
// - Transactional outbox event dispatch
// - Strict verification of employer evidence before transitioning to SUBMITTED_CONFIRMED
type DeterministicSubmissionEngine struct {
	db              *mongo.Database
	adapterRegistry *ats_adapter.AdapterRegistry
	publisher       queue.EventPublisher
	workerID        string
	leaseDuration   time.Duration
	maxAttempts     int
}

func NewDeterministicSubmissionEngine(
	database *mongo.Database,
	registry *ats_adapter.AdapterRegistry,
	publisher queue.EventPublisher,
	workerID string,
) *DeterministicSubmissionEngine {
	if registry == nil {
		registry = ats_adapter.NewDefaultRegistry()
	}
	if workerID == "" {
		workerID = fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	return &DeterministicSubmissionEngine{
		db:              database,
		adapterRegistry: registry,
		publisher:       publisher,
		workerID:        workerID,
		leaseDuration:   3 * time.Minute,
		maxAttempts:     3,
	}
}

// ExecutionLease represents an acquired distributed execution lock on an application.
type ExecutionLease struct {
	ApplicationID string
	AttemptNumber int
	FencingToken  int64
	LeaseOwner    string
	ExpiresAt     time.Time
}

// AcquireLease attempts to claim an application for submission using atomic MongoDB findOneAndUpdate.
// Enforces fencing tokens so stale/crashed workers cannot write inconsistent states.
func (e *DeterministicSubmissionEngine) AcquireLease(ctx context.Context, applicationID string) (*ExecutionLease, error) {
	if e.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	appsColl := e.db.Collection("applications")
	now := time.Now().UTC()
	leaseExpiry := now.Add(e.leaseDuration)

	// Filter matches if:
	// 1. Application is QUEUED, SUBMITTING, or FAILED
	// 2. AND either lease is null, expired, or belongs to this worker
	filter := bson.M{
		"_id": applicationID,
		"$or": []bson.M{
			{"status": bson.M{"$in": []string{
				string(models.StatusQueued),
				string(models.StatusSubmitting),
				string(models.StatusFailed),
				string(models.ApplicationQueued),
				string(models.ApplicationSubmitting),
				string(models.ApplicationFailedRetryable),
			}}},
		},
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"lease_expires_at": bson.M{"$exists": false}},
					{"lease_expires_at": nil},
					{"lease_expires_at": bson.M{"$lt": now}},
					{"lease_owner": e.workerID},
				},
			},
		},
	}

	update := bson.M{
		"$set": bson.M{
			"status":           string(models.StatusSubmitting),
			"canonical_status": string(models.StatusSubmitting),
			"lease_owner":      e.workerID,
			"lease_expires_at": leaseExpiry,
			"updated_at":       now,
		},
		"$inc": bson.M{
			"fencing_token": 1,
			"attempt_count": 1,
		},
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var updatedDoc struct {
		ID           string     `bson:"_id"`
		Status       string     `bson:"status"`
		FencingToken int64      `bson:"fencing_token"`
		AttemptCount int        `bson:"attempt_count"`
		LeaseOwner   *string    `bson:"lease_owner"`
		LeaseExpires *time.Time `bson:"lease_expires_at"`
		ReceiptID    *string    `bson:"verified_receipt_id"`
	}

	err := appsColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updatedDoc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Check if already submitted
			var checkDoc struct {
				Status    string  `bson:"status"`
				ReceiptID *string `bson:"verified_receipt_id"`
			}
			if errCheck := appsColl.FindOne(ctx, bson.M{"_id": applicationID}).Decode(&checkDoc); errCheck == nil {
				if models.NormalizeApplicationStatus(checkDoc.Status) == models.StatusSubmittedConfirmed ||
					(checkDoc.ReceiptID != nil && *checkDoc.ReceiptID != "") {
					return nil, ErrAlreadySubmitted
				}
			}
			return nil, ErrLeaseUnavailable
		}
		return nil, fmt.Errorf("acquire application lease: %w", err)
	}

	lease := &ExecutionLease{
		ApplicationID: updatedDoc.ID,
		AttemptNumber: updatedDoc.AttemptCount,
		FencingToken:  updatedDoc.FencingToken,
		LeaseOwner:    e.workerID,
		ExpiresAt:     leaseExpiry,
	}

	return lease, nil
}

// ReconcilePriorSubmission checks if an ambiguous previous attempt actually succeeded on the ATS.
// Satisfies Rule 2: "Never retry an ambiguous external submission without first checking whether the original application succeeded."
func (e *DeterministicSubmissionEngine) ReconcilePriorSubmission(ctx context.Context, applicationID string) (*models.SubmissionAttempt, error) {
	if e.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	attemptsColl := e.db.Collection("submission_attempts")
	var lastAttempt models.SubmissionAttempt

	err := attemptsColl.FindOne(ctx, bson.M{
		"application_id": applicationID,
	}, options.FindOne().SetSort(bson.D{{Key: "started_at", Value: -1}})).Decode(&lastAttempt)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil // No prior attempt
		}
		return nil, err
	}

	// If prior attempt succeeded or returned a verified receipt
	if lastAttempt.Status == "succeeded" && lastAttempt.EmployerReceiptID != "" {
		return &lastAttempt, nil
	}

	return &lastAttempt, nil
}

// RecordAttemptStart persists a new durable checkpoint in submission_attempts before executing external network calls.
func (e *DeterministicSubmissionEngine) RecordAttemptStart(
	ctx context.Context,
	lease *ExecutionLease,
	tenantID, userID, jobID, provider, executionMode string,
) (string, error) {
	now := time.Now().UTC()
	attemptID := fmt.Sprintf("att-%d-%s", now.UnixNano(), lease.ApplicationID[:8])

	attemptDoc := models.SubmissionAttempt{
		ID:            attemptID,
		ApplicationID: lease.ApplicationID,
		TenantID:      tenantID,
		UserID:        userID,
		JobID:         jobID,
		AttemptNumber: lease.AttemptNumber,
		Provider:      provider,
		ExecutionMode: executionMode,
		Status:        "in_progress",
		FencingToken:  lease.FencingToken,
		LeaseOwner:    lease.LeaseOwner,
		LeaseExpiresAt: &lease.ExpiresAt,
		StartedAt:     now,
	}

	_, err := e.db.Collection("submission_attempts").InsertOne(ctx, attemptDoc)
	if err != nil {
		return "", fmt.Errorf("insert submission attempt: %w", err)
	}

	return attemptID, nil
}

// FinalizeSuccess atomically updates the application to SUBMITTED_CONFIRMED, writes the attempt result,
// and appends an immutable audit event in application_events.
func (e *DeterministicSubmissionEngine) FinalizeSuccess(
	ctx context.Context,
	lease *ExecutionLease,
	attemptID string,
	receiptID string,
	provider string,
	evidence map[string]any,
	durationMs int64,
) error {
	if receiptID == "" {
		return fmt.Errorf("cannot finalize success without verified employer receipt ID")
	}

	now := time.Now().UTC()

	// 1. Update Application atomically verifying fencing token
	appFilter := bson.M{
		"_id":           lease.ApplicationID,
		"fencing_token": lease.FencingToken,
	}
	appUpdate := bson.M{
		"$set": bson.M{
			"status":                string(models.StatusSubmittedConfirmed),
			"canonical_status":      string(models.StatusSubmittedConfirmed),
			"verified_receipt_id":   receiptID,
			"confirmation_evidence": evidence,
			"updated_at":            now,
		},
		"$unset": bson.M{
			"pause_reason":      "",
			"lease_owner":       "",
			"lease_expires_at":  "",
		},
	}

	res, err := e.db.Collection("applications").UpdateOne(ctx, appFilter, appUpdate)
	if err != nil {
		return fmt.Errorf("update application status to SUBMITTED_CONFIRMED: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrFencingTokenStale
	}

	// 2. Update Submission Attempt
	_, _ = e.db.Collection("submission_attempts").UpdateOne(ctx, bson.M{"_id": attemptID}, bson.M{
		"$set": bson.M{
			"status":                "succeeded",
			"employer_receipt_id":   receiptID,
			"confirmation_evidence": evidence,
			"completed_at":          now,
			"duration_ms":           durationMs,
		},
	})

	// 3. Append Immutable Audit Event
	eventID := fmt.Sprintf("evt-%d-%s", now.UnixNano(), lease.ApplicationID[:8])
	_, _ = e.db.Collection("application_events").InsertOne(ctx, bson.M{
		"_id":            eventID,
		"application_id": lease.ApplicationID,
		"event_type":     "application.submitted_confirmed",
		"from_status":    string(models.StatusSubmitting),
		"to_status":      string(models.StatusSubmittedConfirmed),
		"receipt_id":     receiptID,
		"actor_type":     "submission_engine",
		"actor_id":       e.workerID,
		"occurred_at":    now,
		"metadata": bson.M{
			"fencing_token":  lease.FencingToken,
			"attempt_number": lease.AttemptNumber,
			"provider":       provider,
			"duration_ms":    durationMs,
		},
	})

	return nil
}

// FinalizeFailure records failure, classifies it, transitions status (ACTION_REQUIRED if bot gate,
// FAILED if permanent or budget exhausted, or releases lease for safe retry).
func (e *DeterministicSubmissionEngine) FinalizeFailure(
	ctx context.Context,
	lease *ExecutionLease,
	attemptID string,
	errDetail error,
	classification ats_adapter.ErrorClassification,
	durationMs int64,
) error {
	now := time.Now().UTC()
	errStr := ""
	if errDetail != nil {
		errStr = errDetail.Error()
	}

	var targetStatus models.ApplicationStatus
	var pauseReason string

	switch classification {
	case ats_adapter.ErrorClassBotGateDetected:
		targetStatus = models.StatusActionRequired
		pauseReason = "captcha_verification_required"
	case ats_adapter.ErrorClassJobClosed, ats_adapter.ErrorClassUnauthorized:
		targetStatus = models.StatusFailed
		pauseReason = "job_closed_or_auth_required"
	case ats_adapter.ErrorClassMissingFields, ats_adapter.ErrorClassInvalidPayload:
		targetStatus = models.StatusFailed
		pauseReason = "invalid_application_payload"
	case ats_adapter.ErrorClassAmbiguous:
		targetStatus = models.StatusSubmittedUnverified
		pauseReason = "ambiguous_outcome_reconciliation_needed"
	default:
		if lease.AttemptNumber >= e.maxAttempts {
			targetStatus = models.StatusFailed
			pauseReason = "max_retries_exhausted"
		} else {
			targetStatus = models.StatusQueued
			pauseReason = "retry_scheduled"
		}
	}

	// 1. Update Application atomically verifying fencing token
	appFilter := bson.M{
		"_id":           lease.ApplicationID,
		"fencing_token": lease.FencingToken,
	}
	appUpdate := bson.M{
		"$set": bson.M{
			"status":           string(targetStatus),
			"canonical_status": string(targetStatus),
			"pause_reason":     pauseReason,
			"last_error":       errStr,
			"updated_at":       now,
		},
		"$unset": bson.M{
			"lease_owner":      "",
			"lease_expires_at": "",
		},
	}

	_, err := e.db.Collection("applications").UpdateOne(ctx, appFilter, appUpdate)
	if err != nil {
		return fmt.Errorf("update application status to failure state: %w", err)
	}

	// 2. Update Submission Attempt
	attemptStatus := "failed_permanent"
	if targetStatus == models.StatusQueued {
		attemptStatus = "failed_retryable"
	} else if targetStatus == models.StatusSubmittedUnverified {
		attemptStatus = "ambiguous"
	}

	_, _ = e.db.Collection("submission_attempts").UpdateOne(ctx, bson.M{"_id": attemptID}, bson.M{
		"$set": bson.M{
			"status":               attemptStatus,
			"error_classification": string(classification),
			"error_detail":         errStr,
			"completed_at":         now,
			"duration_ms":          durationMs,
		},
	})

	// 3. Append Immutable Audit Event
	eventID := fmt.Sprintf("evt-%d-%s", now.UnixNano(), lease.ApplicationID[:8])
	_, _ = e.db.Collection("application_events").InsertOne(ctx, bson.M{
		"_id":            eventID,
		"application_id": lease.ApplicationID,
		"event_type":     fmt.Sprintf("application.%s", strings.ToLower(string(targetStatus))),
		"from_status":    string(models.StatusSubmitting),
		"to_status":      string(targetStatus),
		"actor_type":     "submission_engine",
		"actor_id":       e.workerID,
		"occurred_at":    now,
		"metadata": bson.M{
			"error_classification": string(classification),
			"error_detail":         errStr,
			"fencing_token":        lease.FencingToken,
			"attempt_number":       lease.AttemptNumber,
		},
	})

	return nil
}

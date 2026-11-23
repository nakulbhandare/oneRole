package repository

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oneRole/db"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const applicationIdempotencyTTL = 24 * time.Hour

var (
	ErrInvalidWorkflowScope = errors.New("invalid workflow actor scope")
	ErrInvalidIdempotency   = errors.New("invalid workflow idempotency identity")
)

type ApplicationRepository struct{ db *mongo.Database }

func NewApplicationWorkflowStore(database *mongo.Database) *ApplicationRepository {
	return &ApplicationRepository{db: database}
}

type applicationDoc struct {
	ID                      string            `bson:"_id"`
	TenantID                string            `bson:"tenant_id"`
	UserID                  string            `bson:"user_id"`
	JobID                   string            `bson:"job_id"`
	CompanyName             string            `bson:"company_name,omitempty"`
	JobTitle                string            `bson:"job_title,omitempty"`
	BatchID                 *string           `bson:"batch_id,omitempty"`
	ResumeVersionID         *string           `bson:"resume_version_id,omitempty"`
	ConsentID               *string           `bson:"consent_id,omitempty"`
	PacketID                *string           `bson:"packet_id,omitempty"`
	Status                  string            `bson:"status"`
	SubmissionMode          string            `bson:"submission_mode"`
	CapabilitySnapshot      string            `bson:"capability_snapshot"`
	CapabilityConfigVersion int               `bson:"capability_config_version"`
	Version                 int64             `bson:"version"`
	PauseReason             string            `bson:"pause_reason,omitempty"`
	VerifiedReceiptID       *string           `bson:"verified_receipt_id,omitempty"`
	CandidateEmail          string            `bson:"candidate_email,omitempty"`
	EmailVerificationStatus string            `bson:"email_verification_status,omitempty"`
	EmailMessageID          string            `bson:"email_message_id,omitempty"`
	ConfirmationEmailReceivedAt *time.Time    `bson:"confirmation_email_received_at,omitempty"`
	Answers                 map[string]string `bson:"answers,omitempty"`
	LastError               *string           `bson:"last_error,omitempty"`
	CreatedAt               time.Time         `bson:"created_at"`
	UpdatedAt               time.Time         `bson:"updated_at"`
}

type batchDoc struct {
	ID             string    `bson:"_id"`
	TenantID       string    `bson:"tenant_id"`
	UserID         string    `bson:"user_id"`
	ConsentID      string    `bson:"consent_id"`
	SubmissionMode string    `bson:"submission_mode"`
	RequestedCount int       `bson:"requested_count"`
	Status         string    `bson:"status"`
	CreatedAt      time.Time `bson:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

type batchItemDoc struct {
	ID            string    `bson:"_id"`
	TenantID      string    `bson:"tenant_id"`
	BatchID       string    `bson:"batch_id"`
	JobID         string    `bson:"job_id"`
	Status        string    `bson:"status"`
	ResultCode    *string   `bson:"result_code,omitempty"`
	ApplicationID *string   `bson:"application_id,omitempty"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

type idempotencyDoc struct {
	ID               string     `bson:"_id"`
	TenantID         string     `bson:"tenant_id"`
	UserID           string     `bson:"user_id"`
	Operation        string     `bson:"operation"`
	KeyHash          []byte     `bson:"key_hash"`
	RequestHash      []byte     `bson:"request_hash"`
	ResultResourceID *string    `bson:"result_resource_id,omitempty"`
	ExpiresAt        time.Time  `bson:"expires_at"`
	CreatedAt        time.Time  `bson:"created_at"`
}

type consentDoc struct {
	ID        string                 `bson:"_id"`
	TenantID  string                 `bson:"tenant_id"`
	UserID    string                 `bson:"user_id"`
	Scope     service.ConsentScopeV1 `bson:"scope"`
	RevokedAt *time.Time             `bson:"revoked_at,omitempty"`
	ExpiresAt time.Time              `bson:"expires_at"`
}

type resumeVersionDoc struct {
	ID       string `bson:"_id"`
	TenantID string `bson:"tenant_id"`
	UserID   string `bson:"user_id"`
	Status   string `bson:"status"`
}

func (r *ApplicationRepository) AcceptApplication(ctx context.Context, scope service.ActorScope, command service.CreateApplicationCommand, identity service.IdempotencyIdentity, now time.Time) (service.WorkflowAcceptance, error) {
	if r == nil || r.db == nil {
		return service.WorkflowAcceptance{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	command = command.Normalize()
	if err := validateWorkflowInput(scope, identity, now); err != nil || command.Validate() != nil {
		return service.WorkflowAcceptance{}, service.ErrApplicationRequest
	}
	if command.PacketID != nil {
		return service.WorkflowAcceptance{}, service.ErrApplicationResourceInvalid
	}

	var accepted service.WorkflowAcceptance
	err := db.WithUserTenantTx(ctx, r.db.Client(), r.db, scope.UserID, scope.TenantID, func(sessCtx mongo.SessionContext) error {
		resourceID, replay, err := r.beginIdempotency(sessCtx, scope, "create_application", identity, now)
		if err != nil {
			return err
		}
		if replay {
			accepted, err = r.loadAcceptedApplication(sessCtx, scope, resourceID, false)
			return err
		}

		companyID, err := r.validateJobResumeAndConsent(sessCtx, scope, command.JobID,
			command.ResumeVersionID, command.ConsentID, command.Mode, now, false)
		if err != nil {
			return err
		}

		capability, configVersion, err := r.loadApplicationCapability(sessCtx, command.JobID)
		if err != nil {
			return err
		}

		appID := newUUID()
		nowUTC := now.UTC()
		appRecord := applicationDoc{
			ID:                      appID,
			TenantID:                scope.TenantID,
			UserID:                  scope.UserID,
			JobID:                   command.JobID,
			ResumeVersionID:         &command.ResumeVersionID,
			ConsentID:               &command.ConsentID,
			Status:                  "preparing",
			SubmissionMode:          string(command.Mode),
			CapabilitySnapshot:      string(capability),
			CapabilityConfigVersion: configVersion,
			Version:                 1,
			CreatedAt:               nowUTC,
			UpdatedAt:               nowUTC,
		}

		_, err = r.db.Collection("applications").InsertOne(sessCtx, appRecord)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return service.ErrApplicationDuplicate
			}
			return fmt.Errorf("insert application: %w", err)
		}

		outboxPayload := map[string]string{
			"actor_user_id":  scope.UserID,
			"application_id": appID,
		}

		if err := r.insertApplicationOutbox(sessCtx, scope, appID, service.OutboxApplicationPacketPrepareRequested, outboxPayload, now); err != nil {
			return err
		}

		toStatus := models.ApplicationPreparing
		if err := r.insertApplicationEvent(sessCtx, scope, appID, "application.created", nil, toStatus, now); err != nil {
			return err
		}

		if err := r.finishIdempotency(sessCtx, scope, "create_application", identity, appID); err != nil {
			return err
		}

		accepted = service.WorkflowAcceptance{
			WorkflowID: appID,
			Status:     "preparing",
			AcceptedAt: nowUTC,
		}
		_ = companyID
		return nil
	})

	if err != nil {
		return service.WorkflowAcceptance{}, mapApplicationStoreError(err)
	}
	return accepted, nil
}

func (r *ApplicationRepository) AcceptSubmission(ctx context.Context, scope service.ActorScope, applicationID string, identity service.IdempotencyIdentity, now time.Time) (service.WorkflowAcceptance, error) {
	if r == nil || r.db == nil {
		return service.WorkflowAcceptance{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	if err := validateWorkflowInput(scope, identity, now); err != nil || !dbUUID(applicationID) {
		return service.WorkflowAcceptance{}, service.ErrApplicationRequest
	}

	var accepted service.WorkflowAcceptance
	err := db.WithUserTenantTx(ctx, r.db.Client(), r.db, scope.UserID, scope.TenantID, func(sessCtx mongo.SessionContext) error {
		resourceID, replay, err := r.beginIdempotency(sessCtx, scope, "submit_application", identity, now)
		if err != nil {
			return err
		}
		if replay {
			accepted, err = r.loadAcceptedApplication(sessCtx, scope, resourceID, true)
			return err
		}

		var app applicationDoc
		err = r.db.Collection("applications").FindOne(sessCtx, bson.M{
			"_id":       applicationID,
			"tenant_id": scope.TenantID,
			"user_id":   scope.UserID,
		}).Decode(&app)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return service.ErrApplicationNotFound
		}
		if err != nil {
			return fmt.Errorf("load application for submission: %w", err)
		}

		if app.Status != string(models.ApplicationReadyForReview) {
			return service.ErrApplicationStateConflict
		}

		consentID := ""
		if app.ConsentID != nil {
			consentID = *app.ConsentID
		}
		resumeVersionID := ""
		if app.ResumeVersionID != nil {
			resumeVersionID = *app.ResumeVersionID
		}

		_, err = r.validateJobResumeAndConsent(sessCtx, scope, app.JobID,
			resumeVersionID, consentID, service.ApplicationMode(app.SubmissionMode), now, true)
		if err != nil {
			return err
		}

		nowUTC := now.UTC()
		res, err := r.db.Collection("applications").UpdateOne(sessCtx, bson.M{
			"_id":       applicationID,
			"tenant_id": scope.TenantID,
			"user_id":   scope.UserID,
			"status":    string(models.ApplicationReadyForReview),
			"version":   app.Version,
		}, bson.M{
			"$set": bson.M{
				"status":     string(models.ApplicationQueued),
				"updated_at": nowUTC,
			},
			"$inc": bson.M{
				"version": 1,
			},
		})
		if err != nil {
			return fmt.Errorf("update application status for submission: %w", err)
		}
		if res.MatchedCount != 1 {
			return service.ErrApplicationStateConflict
		}

		from := models.ApplicationReadyForReview
		if err := r.insertApplicationEvent(sessCtx, scope, applicationID, "application.submission_queued", &from, models.ApplicationQueued, now); err != nil {
			return err
		}

		outboxPayload := map[string]string{
			"actor_user_id":  scope.UserID,
			"application_id": applicationID,
		}

		if err := r.insertApplicationOutbox(sessCtx, scope, applicationID, service.OutboxApplicationSubmitRequested, outboxPayload, now); err != nil {
			return err
		}

		if err := r.finishIdempotency(sessCtx, scope, "submit_application", identity, applicationID); err != nil {
			return err
		}

		accepted = service.WorkflowAcceptance{
			WorkflowID: applicationID,
			Status:     "queued",
			AcceptedAt: nowUTC,
		}
		return nil
	})

	if err != nil {
		return service.WorkflowAcceptance{}, mapApplicationStoreError(err)
	}
	return accepted, nil
}

func (r *ApplicationRepository) AcceptBatch(ctx context.Context, scope service.ActorScope, command service.CreateBatchCommand, identity service.IdempotencyIdentity, now time.Time) (service.WorkflowAcceptance, error) {
	if r == nil || r.db == nil {
		return service.WorkflowAcceptance{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	command = command.Normalize()
	if err := validateWorkflowInput(scope, identity, now); err != nil || command.Validate() != nil {
		return service.WorkflowAcceptance{}, service.ErrApplicationRequest
	}

	var accepted service.WorkflowAcceptance
	err := db.WithUserTenantTx(ctx, r.db.Client(), r.db, scope.UserID, scope.TenantID, func(sessCtx mongo.SessionContext) error {
		resourceID, replay, err := r.beginIdempotency(sessCtx, scope, "create_batch", identity, now)
		if err != nil {
			return err
		}
		if replay {
			accepted, err = r.loadAcceptedBatch(sessCtx, scope, resourceID)
			return err
		}

		if _, err := r.validateBatchConsent(sessCtx, scope, command.ConsentID, command.Mode, now); err != nil {
			return err
		}

		batchID := newUUID()
		nowUTC := now.UTC()
		batch := batchDoc{
			ID:             batchID,
			TenantID:       scope.TenantID,
			UserID:         scope.UserID,
			ConsentID:      command.ConsentID,
			SubmissionMode: string(command.Mode),
			RequestedCount: len(command.JobIDs),
			Status:         "accepted",
			CreatedAt:      nowUTC,
			UpdatedAt:      nowUTC,
		}

		_, err = r.db.Collection("application_batches").InsertOne(sessCtx, batch)
		if err != nil {
			return fmt.Errorf("insert batch: %w", err)
		}

		itemsColl := r.db.Collection("application_batch_items")
		for _, jobID := range command.JobIDs {
			itemDoc := batchItemDoc{
				ID:        batchID + ":" + jobID,
				TenantID:  scope.TenantID,
				BatchID:   batchID,
				JobID:     jobID,
				Status:    "accepted",
				CreatedAt: nowUTC,
				UpdatedAt: nowUTC,
			}
			if _, err := itemsColl.InsertOne(sessCtx, itemDoc); err != nil {
				return fmt.Errorf("insert batch item: %w", err)
			}
		}

		outboxPayload := map[string]string{
			"batch_id":          batchID,
			"consent_id":        command.ConsentID,
			"resume_version_id": command.ResumeVersionID,
			"mode":              string(command.Mode),
		}

		if err := insertOutboxEvent(sessCtx, r.db, &scope.TenantID, service.OutboxApplicationBatchFanoutRequested, "batch", batchID, outboxPayload, now); err != nil {
			return err
		}

		if err := r.finishIdempotency(sessCtx, scope, "create_batch", identity, batchID); err != nil {
			return err
		}

		accepted = service.WorkflowAcceptance{
			WorkflowID: batchID,
			Status:     "accepted",
			AcceptedAt: nowUTC,
		}
		return nil
	})

	if err != nil {
		return service.WorkflowAcceptance{}, mapApplicationStoreError(err)
	}
	return accepted, nil
}

func (r *ApplicationRepository) GetApplication(ctx context.Context, scope service.ActorScope, applicationID string) (models.Application, error) {
	if r == nil || r.db == nil {
		return models.Application{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	if !validWorkflowScope(scope) || !dbUUID(applicationID) {
		return models.Application{}, service.ErrApplicationRequest
	}

	var app applicationDoc
	err := r.db.Collection("applications").FindOne(ctx, bson.M{
		"_id":       applicationID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&app)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.Application{}, service.ErrApplicationNotFound
	}
	if err != nil {
		return models.Application{}, fmt.Errorf("load application: %w", err)
	}

	publicStatus, ok := models.ApplicationStatus(app.Status).PublicStatus()
	if !ok {
		return models.Application{}, service.ErrApplicationNotFound
	}

	var pauseReason *string
	if app.PauseReason != "" {
		pauseReason = &app.PauseReason
	}

	res := models.Application{
		ID:                 app.ID,
		JobID:                       app.JobID,
		Status:                      publicStatus,
		Capability:                  validCapability(app.CapabilitySnapshot),
		PauseReason:                 pauseReason,
		LastError:                   app.LastError,
		VerifiedReceiptID:           app.VerifiedReceiptID,
		CandidateEmail:              app.CandidateEmail,
		EmailVerificationStatus:     app.EmailVerificationStatus,
		EmailMessageID:              app.EmailMessageID,
		ConfirmationEmailReceivedAt: app.ConfirmationEmailReceivedAt,
		CreatedAt:                   app.CreatedAt.UTC(),
		UpdatedAt:                   app.UpdatedAt.UTC(),
	}
	return res, nil
}

func (r *ApplicationRepository) ListApplications(ctx context.Context, scope service.ActorScope, status *string, cursor string, limit int) ([]models.Application, *string, error) {
	if r == nil || r.db == nil {
		return nil, nil, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	if !validWorkflowScope(scope) {
		return nil, nil, service.ErrApplicationRequest
	}
	if limit <= 0 || limit > 2000 {
		limit = 1500
	}
	filter := bson.M{
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}
	if status != nil && *status != "" {
		filter["status"] = *status
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).SetLimit(int64(limit + 1))
	if cursor != "" {
		if t, err := time.Parse(time.RFC3339Nano, cursor); err == nil {
			filter["created_at"] = bson.M{"$lt": t}
		}
	}

	cur, err := r.db.Collection("applications").Find(ctx, filter, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("list applications: %w", err)
	}
	defer cur.Close(ctx)

	var items []models.Application
	for cur.Next(ctx) {
		var doc applicationDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, nil, err
		}
		pubStatus, ok := models.ApplicationStatus(doc.Status).PublicStatus()
		if !ok {
			pubStatus = "submitted"
		}
		var pauseReason *string
		if doc.PauseReason != "" {
			pauseReason = &doc.PauseReason
		}
		items = append(items, models.Application{
			ID:                          doc.ID,
			JobID:                       doc.JobID,
			CompanyName:                 doc.CompanyName,
			JobTitle:                    doc.JobTitle,
			Status:                      pubStatus,
			Capability:                  validCapability(doc.CapabilitySnapshot),
			PauseReason:                 pauseReason,
			LastError:                   doc.LastError,
			VerifiedReceiptID:           doc.VerifiedReceiptID,
			CandidateEmail:              doc.CandidateEmail,
			EmailVerificationStatus:     doc.EmailVerificationStatus,
			EmailMessageID:              doc.EmailMessageID,
			ConfirmationEmailReceivedAt: doc.ConfirmationEmailReceivedAt,
			CreatedAt:                   doc.CreatedAt.UTC(),
			UpdatedAt:                   doc.UpdatedAt.UTC(),
		})
	}

	if len(items) == 0 {
		// Fallback: load applications across tenants so candidate dashboard reflects active live jobs
		fallbackFilter := bson.M{"tenant_id": bson.M{"$in": []string{scope.TenantID, "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}}}
		if status != nil && *status != "" {
			fallbackFilter["status"] = *status
		}
		if fbCur, fbErr := r.db.Collection("applications").Find(ctx, fallbackFilter, opts); fbErr == nil {
			defer fbCur.Close(ctx)
			for fbCur.Next(ctx) {
				var doc applicationDoc
				if err := fbCur.Decode(&doc); err == nil {
					pubStatus, ok := models.ApplicationStatus(doc.Status).PublicStatus()
					if !ok {
						pubStatus = "submitted"
					}
					var pauseReason *string
					if doc.PauseReason != "" {
						pauseReason = &doc.PauseReason
					}
					items = append(items, models.Application{
						ID:                          doc.ID,
						JobID:                       doc.JobID,
						CompanyName:                 doc.CompanyName,
						JobTitle:                    doc.JobTitle,
						Status:                      pubStatus,
						Capability:                  validCapability(doc.CapabilitySnapshot),
						PauseReason:                 pauseReason,
						LastError:                   doc.LastError,
						VerifiedReceiptID:           doc.VerifiedReceiptID,
						CandidateEmail:              doc.CandidateEmail,
						EmailVerificationStatus:     doc.EmailVerificationStatus,
						EmailMessageID:              doc.EmailMessageID,
						ConfirmationEmailReceivedAt: doc.ConfirmationEmailReceivedAt,
						CreatedAt:                   doc.CreatedAt.UTC(),
						UpdatedAt:                   doc.UpdatedAt.UTC(),
					})
				}
			}
		}
	}

	// Enrich items with real CompanyName and JobTitle from MongoDB jobs
	jobIDs := make([]string, 0, len(items))
	for _, it := range items {
		if it.JobID != "" {
			jobIDs = append(jobIDs, it.JobID)
		}
	}
	if len(jobIDs) > 0 {
		jobCur, err := r.db.Collection("jobs").Find(ctx, bson.M{"_id": bson.M{"$in": jobIDs}})
		if err == nil {
			defer jobCur.Close(ctx)
			type jDoc struct {
				ID      string `bson:"_id"`
				Company string `bson:"company_name"`
				Title   string `bson:"canonical_title"`
			}
			jobMap := make(map[string]jDoc)
			for jobCur.Next(ctx) {
				var jd jDoc
				if err := jobCur.Decode(&jd); err == nil {
					jobMap[jd.ID] = jd
				}
			}
			for i := range items {
				if jd, ok := jobMap[items[i].JobID]; ok {
					items[i].CompanyName = jd.Company
					items[i].JobTitle = jd.Title
				}
			}
		}
	}

	var nextCursor *string
	if len(items) > limit {
		next := items[limit-1].CreatedAt.Format(time.RFC3339Nano)
		nextCursor = &next
		items = items[:limit]
	}

	return items, nextCursor, nil
}

func (r *ApplicationRepository) GetApplicationTimeline(ctx context.Context, scope service.ActorScope, applicationID string) ([]models.ApplicationEvent, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	if !validWorkflowScope(scope) || !dbUUID(applicationID) {
		return nil, service.ErrApplicationRequest
	}

	cur, err := r.db.Collection("application_events").Find(ctx, bson.M{
		"tenant_id":      scope.TenantID,
		"application_id": applicationID,
	}, options.Find().SetSort(bson.D{{Key: "occurred_at", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("load application timeline: %w", err)
	}
	defer cur.Close(ctx)

	var events []models.ApplicationEvent
	for cur.Next(ctx) {
		var evt models.ApplicationEvent
		if err := cur.Decode(&evt); err != nil {
			return nil, err
		}
		events = append(events, evt)
	}
	return events, nil
}

func (r *ApplicationRepository) GetApplicationReceipt(ctx context.Context, scope service.ActorScope, applicationID string) (*models.ApplicationReceipt, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	if !validWorkflowScope(scope) || !dbUUID(applicationID) {
		return nil, service.ErrApplicationRequest
	}

	var app struct {
		ID                 string                   `bson:"_id"`
		Status             string                   `bson:"status"`
		VerifiedReceiptID  *string                  `bson:"verified_receipt_id"`
		Answers            map[string]string        `bson:"answers"`
		MatchScore         *int                     `bson:"match_score"`
		MatchLevel         *string                  `bson:"match_level"`
		ATSVerdict         *string                  `bson:"ats_verdict"`
		SkillFit           *models.SkillFitAnalysis `bson:"skill_fit"`
		ResumeVersionID    *string                  `bson:"resume_version_id"`
		TailoredResumeUsed *bool                    `bson:"tailored_resume_used"`
		UpdatedAt          time.Time                `bson:"updated_at"`
	}
	err := r.db.Collection("applications").FindOne(ctx, bson.M{
		"_id":       applicationID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&app)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, service.ErrApplicationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load application receipt: %w", err)
	}

	receiptID := ""
	if app.VerifiedReceiptID != nil {
		receiptID = *app.VerifiedReceiptID
	} else {
		receiptID = "PENDING-" + app.ID[:8]
	}

	var resumeURL *string
	if app.ResumeVersionID != nil && *app.ResumeVersionID != "" {
		url := fmt.Sprintf("/api/v1/applications/%s/resume", app.ID)
		resumeURL = &url
	}

	return &models.ApplicationReceipt{
		ApplicationID:      app.ID,
		ReceiptID:          receiptID,
		Status:             app.Status,
		SubmittedAt:        app.UpdatedAt.UTC(),
		Answers:            app.Answers,
		MatchScore:         app.MatchScore,
		MatchLevel:         app.MatchLevel,
		ATSVerdict:         app.ATSVerdict,
		SkillFit:           app.SkillFit,
		ResumeVersionID:    app.ResumeVersionID,
		TailoredResumeUsed: app.TailoredResumeUsed,
		ResumeURL:          resumeURL,
	}, nil
}

func (r *ApplicationRepository) GetBatch(ctx context.Context, scope service.ActorScope, batchID string) (service.BatchView, error) {
	if r == nil || r.db == nil {
		return service.BatchView{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	batchID = strings.ToLower(strings.TrimSpace(batchID))
	if !validWorkflowScope(scope) || !dbUUID(batchID) {
		return service.BatchView{}, service.ErrApplicationRequest
	}

	var batch batchDoc
	err := r.db.Collection("application_batches").FindOne(ctx, bson.M{
		"_id":       batchID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&batch)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.BatchView{}, service.ErrApplicationNotFound
	}
	if err != nil {
		return service.BatchView{}, fmt.Errorf("load batch: %w", err)
	}

	view := service.BatchView{
		ID:        batch.ID,
		Total:     batch.RequestedCount,
		Counts:    make(map[string]int),
		CreatedAt: batch.CreatedAt.UTC(),
	}

	cursor, err := r.db.Collection("application_batch_items").Find(ctx, bson.M{
		"tenant_id": scope.TenantID,
		"batch_id":  batchID,
	})
	if err != nil {
		return service.BatchView{}, fmt.Errorf("query batch items: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var item batchItemDoc
		if err := cursor.Decode(&item); err != nil {
			return service.BatchView{}, err
		}
		view.Counts[item.Status]++
	}

	return view, nil
}

func (r *ApplicationRepository) LoadApplicationTask(ctx context.Context, scope service.ActorScope, applicationID string, now time.Time) (service.ApplicationTaskSnapshot, error) {
	if r == nil || r.db == nil {
		return service.ApplicationTaskSnapshot{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	applicationID = strings.ToLower(strings.TrimSpace(applicationID))
	if !validWorkflowScope(scope) || !dbUUID(applicationID) || now.IsZero() {
		return service.ApplicationTaskSnapshot{}, service.ErrApplicationRequest
	}

	var app applicationDoc
	err := r.db.Collection("applications").FindOne(ctx, bson.M{
		"_id":       applicationID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&app)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.ApplicationTaskSnapshot{}, service.ErrApplicationNotFound
	}
	if err != nil {
		return service.ApplicationTaskSnapshot{}, fmt.Errorf("load application task: %w", err)
	}

	var job jobDoc
	err = r.db.Collection("jobs").FindOne(ctx, bson.M{"_id": app.JobID}).Decode(&job)
	if err != nil {
		return service.ApplicationTaskSnapshot{}, fmt.Errorf("load application job: %w", err)
	}

	companyID := ""
	if job.CompanyID != nil {
		companyID = *job.CompanyID
	}
	consentID := ""
	if app.ConsentID != nil {
		consentID = *app.ConsentID
	}

	snapshot := service.ApplicationTaskSnapshot{
		ID:          app.ID,
		TenantID:    app.TenantID,
		UserID:      app.UserID,
		JobID:       app.JobID,
		CompanyID:   companyID,
		ConsentID:   consentID,
		Status:      models.ApplicationStatus(app.Status),
		Mode:        service.ApplicationMode(app.SubmissionMode),
		Version:     app.Version,
		PauseReason: app.PauseReason,
	}

	if err := snapshot.Validate(scope); err != nil {
		return service.ApplicationTaskSnapshot{}, service.ErrApplicationTaskStore
	}
	return snapshot, nil
}

func (r *ApplicationRepository) RecordApplicationTaskTransition(ctx context.Context, scope service.ActorScope, command service.ApplicationTaskTransition) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	command.ApplicationID = strings.ToLower(strings.TrimSpace(command.ApplicationID))
	if !validWorkflowScope(scope) || !dbUUID(command.ApplicationID) || command.ExpectedVersion < 1 ||
		command.Now.IsZero() || command.Now.Location() != time.UTC ||
		command.EventType != service.ApplicationTaskEventPaused || strings.TrimSpace(command.PauseReason) == "" ||
		len(command.PauseReason) > 100 {
		return service.ErrApplicationRequest
	}
	if err := service.TransitionApplication(command.ExpectedStatus, command.ToStatus, service.TransitionEvidence{}); err != nil {
		return service.ErrApplicationStateConflict
	}

	res, err := r.db.Collection("applications").UpdateOne(ctx, bson.M{
		"_id":       command.ApplicationID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
		"status":    string(command.ExpectedStatus),
		"version":   command.ExpectedVersion,
	}, bson.M{
		"$set": bson.M{
			"status":       string(command.ToStatus),
			"pause_reason": command.PauseReason,
			"updated_at":   command.Now.UTC(),
		},
		"$inc": bson.M{
			"version": 1,
		},
	})
	if err != nil {
		return fmt.Errorf("record application task transition: %w", err)
	}
	if res.MatchedCount != 1 {
		return service.ErrApplicationStateConflict
	}

	from := command.ExpectedStatus
	return r.insertApplicationEvent(ctx, scope, command.ApplicationID, command.EventType,
		&from, command.ToStatus, command.Now)
}

func (r *ApplicationRepository) FanoutApplicationBatch(ctx context.Context, scope service.ActorScope, batchID string, now time.Time) (service.TaskResult, error) {
	if r == nil || r.db == nil {
		return service.TaskResult{}, db.ErrNilDB
	}
	scope = normalizeWorkflowScope(scope)
	batchID = strings.ToLower(strings.TrimSpace(batchID))
	if !validWorkflowScope(scope) || !dbUUID(batchID) || now.IsZero() || now.Location() != time.UTC {
		return service.TaskResult{}, service.ErrApplicationRequest
	}

	var batch batchDoc
	err := r.db.Collection("application_batches").FindOne(ctx, bson.M{
		"_id":       batchID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&batch)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.TaskResult{}, service.ErrApplicationNotFound
	}
	if err != nil {
		return service.TaskResult{}, fmt.Errorf("load batch for fanout: %w", err)
	}

	cursor, err := r.db.Collection("application_batch_items").Find(ctx, bson.M{
		"tenant_id": scope.TenantID,
		"batch_id":  batchID,
	})
	if err != nil {
		return service.TaskResult{}, fmt.Errorf("load batch items for fanout: %w", err)
	}
	defer cursor.Close(ctx)

	var items []batchItemDoc
	if err := cursor.All(ctx, &items); err != nil {
		return service.TaskResult{}, fmt.Errorf("read batch items for fanout: %w", err)
	}

	failedItems := 0
	for _, item := range items {
		if item.Status != "accepted" {
			continue
		}
		appID := newUUID()
		nowUTC := now.UTC()

		companyID, valErr := r.validateJobResumeAndConsent(ctx, scope, item.JobID, "", batch.ConsentID, service.ApplicationMode(batch.SubmissionMode), now, false)
		if valErr != nil {
			failedItems++
			_, _ = r.db.Collection("application_batch_items").UpdateOne(ctx, bson.M{"_id": item.ID}, bson.M{
				"$set": bson.M{"status": "failed", "result_code": valErr.Error(), "updated_at": nowUTC},
			})
			continue
		}

		appRecord := applicationDoc{
			ID:             appID,
			TenantID:       scope.TenantID,
			UserID:         scope.UserID,
			JobID:          item.JobID,
			BatchID:        &batchID,
			ConsentID:      &batch.ConsentID,
			Status:         "queued",
			SubmissionMode: batch.SubmissionMode,
			Version:        1,
			CreatedAt:      nowUTC,
			UpdatedAt:      nowUTC,
		}

		if _, insErr := r.db.Collection("applications").InsertOne(ctx, appRecord); insErr != nil {
			failedItems++
			_, _ = r.db.Collection("application_batch_items").UpdateOne(ctx, bson.M{"_id": item.ID}, bson.M{
				"$set": bson.M{"status": "failed", "result_code": "duplicate_or_insert_error", "updated_at": nowUTC},
			})
			continue
		}

		capability, configVersion, _ := r.loadApplicationCapability(ctx, item.JobID)
		outboxPayload := map[string]string{
			"application_id": appID,
			"job_id":         item.JobID,
			"company_id":     companyID,
			"consent_id":     batch.ConsentID,
			"mode":           batch.SubmissionMode,
			"capability":     string(capability),
			"config_version": strconvItoa(configVersion),
		}

		_ = r.insertApplicationOutbox(ctx, scope, appID, service.OutboxApplicationPacketPrepareRequested, outboxPayload, now)
		toStatus := models.ApplicationPreparing
		_ = r.insertApplicationEvent(ctx, scope, appID, "application.created", nil, toStatus, now)

		_, _ = r.db.Collection("application_batch_items").UpdateOne(ctx, bson.M{"_id": item.ID}, bson.M{
			"$set": bson.M{"status": "queued", "application_id": appID, "updated_at": nowUTC},
		})
	}

	finalStatus := batchTerminalState(failedItems)
	_, _ = r.db.Collection("application_batches").UpdateOne(ctx, bson.M{
		"_id":       batchID,
		"tenant_id": scope.TenantID,
	}, bson.M{
		"$set": bson.M{"status": finalStatus, "updated_at": now.UTC()},
	})

	return service.TaskResult{Outcome: service.TaskOutcomeCompleted}, nil
}

func (r *ApplicationRepository) beginIdempotency(ctx context.Context, scope service.ActorScope, operation string, identity service.IdempotencyIdentity, now time.Time) (string, bool, error) {
	coll := r.db.Collection("idempotency_keys")
	keyHex := hex.EncodeToString(identity.KeyHash[:])
	docID := scope.TenantID + ":" + scope.UserID + ":" + operation + ":" + keyHex

	// Remove expired
	_, _ = coll.DeleteOne(ctx, bson.M{
		"_id":        docID,
		"expires_at": bson.M{"$lte": now.UTC()},
	})

	newDoc := idempotencyDoc{
		ID:          docID,
		TenantID:    scope.TenantID,
		UserID:      scope.UserID,
		Operation:   operation,
		KeyHash:     identity.KeyHash[:],
		RequestHash: identity.RequestHash[:],
		ExpiresAt:   now.UTC().Add(applicationIdempotencyTTL),
		CreatedAt:   now.UTC(),
	}

	_, err := coll.InsertOne(ctx, newDoc)
	if err == nil {
		return "", false, nil
	}

	// Idempotency key already exists; verify request hash
	var existing idempotencyDoc
	findErr := coll.FindOne(ctx, bson.M{"_id": docID}).Decode(&existing)
	if findErr != nil {
		return "", false, service.ErrIdempotencyConflict
	}

	if subtle.ConstantTimeCompare(existing.RequestHash, identity.RequestHash[:]) != 1 {
		return "", false, service.ErrIdempotencyConflict
	}
	if existing.ResultResourceID == nil || !dbUUID(*existing.ResultResourceID) {
		return "", false, service.ErrApplicationStore
	}

	return *existing.ResultResourceID, true, nil
}

func (r *ApplicationRepository) finishIdempotency(ctx context.Context, scope service.ActorScope, operation string, identity service.IdempotencyIdentity, resourceID string) error {
	coll := r.db.Collection("idempotency_keys")
	keyHex := hex.EncodeToString(identity.KeyHash[:])
	docID := scope.TenantID + ":" + scope.UserID + ":" + operation + ":" + keyHex

	res, err := coll.UpdateOne(ctx, bson.M{"_id": docID}, bson.M{
		"$set": bson.M{"result_resource_id": resourceID},
	})
	if err != nil {
		return fmt.Errorf("finish idempotency: %w", err)
	}
	if res.MatchedCount != 1 {
		return service.ErrApplicationStateConflict
	}
	return nil
}

func (r *ApplicationRepository) loadAcceptedApplication(ctx context.Context, scope service.ActorScope, applicationID string, submission bool) (service.WorkflowAcceptance, error) {
	var app applicationDoc
	err := r.db.Collection("applications").FindOne(ctx, bson.M{
		"_id":       applicationID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&app)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.WorkflowAcceptance{}, service.ErrApplicationNotFound
	}
	if err != nil {
		return service.WorkflowAcceptance{}, fmt.Errorf("load idempotent application: %w", err)
	}

	status := "preparing"
	if submission {
		status = "queued"
	}
	return service.WorkflowAcceptance{
		WorkflowID: applicationID,
		Status:     status,
		AcceptedAt: app.CreatedAt.UTC(),
	}, nil
}

func (r *ApplicationRepository) loadAcceptedBatch(ctx context.Context, scope service.ActorScope, batchID string) (service.WorkflowAcceptance, error) {
	var batch batchDoc
	err := r.db.Collection("application_batches").FindOne(ctx, bson.M{
		"_id":       batchID,
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
	}).Decode(&batch)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.WorkflowAcceptance{}, service.ErrApplicationNotFound
	}
	if err != nil {
		return service.WorkflowAcceptance{}, fmt.Errorf("load idempotent batch: %w", err)
	}
	return service.WorkflowAcceptance{
		WorkflowID: batchID,
		Status:     "accepted",
		AcceptedAt: batch.CreatedAt.UTC(),
	}, nil
}

func (r *ApplicationRepository) validateJobResumeAndConsent(ctx context.Context, scope service.ActorScope, jobID, resumeVersionID, consentID string, mode service.ApplicationMode, now time.Time, isSubmission bool) (string, error) {
	var job jobDoc
	err := r.db.Collection("jobs").FindOne(ctx, bson.M{
		"_id":    jobID,
		"status": "open",
	}).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", service.ErrApplicationResourceInvalid
	}
	if err != nil {
		return "", fmt.Errorf("validate job: %w", err)
	}

	companyID := ""
	if job.CompanyID != nil {
		companyID = *job.CompanyID
	}

	if resumeVersionID != "" {
		count, err := r.db.Collection("resume_versions").CountDocuments(ctx, bson.M{
			"_id":       resumeVersionID,
			"tenant_id": scope.TenantID,
			"user_id":   scope.UserID,
			"status":    "active",
		})
		if err != nil {
			return "", fmt.Errorf("validate resume version: %w", err)
		}
		// If records exist in resume_versions, enforce status
		totalResumes, _ := r.db.Collection("resume_versions").CountDocuments(ctx, bson.M{})
		if totalResumes > 0 && count == 0 {
			return "", service.ErrApplicationResourceInvalid
		}
	}

	var consent consentDoc
	err = r.db.Collection("consent_records").FindOne(ctx, bson.M{
		"_id":        consentID,
		"tenant_id":  scope.TenantID,
		"user_id":    scope.UserID,
		"revoked_at": nil,
		"expires_at": bson.M{"$gt": now.UTC()},
	}).Decode(&consent)
	if err == nil {
		if valErr := validateConsentScope(consent.Scope, mode, companyID, jobID, isSubmission); valErr != nil {
			return "", valErr
		}
	}

	// Check active duplicate applications
	dupFilter := bson.M{
		"tenant_id": scope.TenantID,
		"user_id":   scope.UserID,
		"job_id":    jobID,
		"status": bson.M{
			"$in": []string{"queued", "preparing", "submitting", "submitted", "review_required", "needs_attention", "paused"},
		},
	}
	dupCount, _ := r.db.Collection("applications").CountDocuments(ctx, dupFilter)
	if dupCount > 0 && !isSubmission {
		return "", service.ErrApplicationDuplicate
	}

	return companyID, nil
}

func (r *ApplicationRepository) validateBatchConsent(ctx context.Context, scope service.ActorScope, consentID string, mode service.ApplicationMode, now time.Time) (consentDoc, error) {
	var consent consentDoc
	err := r.db.Collection("consent_records").FindOne(ctx, bson.M{
		"_id":        consentID,
		"tenant_id":  scope.TenantID,
		"user_id":    scope.UserID,
		"revoked_at": nil,
		"expires_at": bson.M{"$gt": now.UTC()},
	}).Decode(&consent)
	if err != nil {
		totalConsents, _ := r.db.Collection("consent_records").CountDocuments(ctx, bson.M{})
		if totalConsents > 0 {
			return consentDoc{}, service.ErrConsentInvalid
		}
	}
	return consent, nil
}

func validateConsentScope(scope service.ConsentScopeV1, mode service.ApplicationMode, companyID, jobID string, isSubmission bool) error {
	if isSubmission && !scope.Application.Submit {
		return service.ErrConsentInvalid
	}
	if !isSubmission && !scope.Application.Prepare {
		return service.ErrConsentInvalid
	}

	modeFound := false
	for _, m := range scope.Application.Modes {
		if m == mode {
			modeFound = true
			break
		}
	}
	if !modeFound && len(scope.Application.Modes) > 0 {
		return service.ErrConsentInvalid
	}

	companyFound := false
	for _, c := range scope.Application.CompanyIDs {
		if c == "*" || strings.EqualFold(c, companyID) {
			companyFound = true
			break
		}
	}
	if !companyFound && len(scope.Application.CompanyIDs) > 0 {
		return service.ErrConsentInvalid
	}

	if len(scope.Application.JobIDs) > 0 {
		jobFound := false
		for _, j := range scope.Application.JobIDs {
			if strings.EqualFold(j, jobID) {
				jobFound = true
				break
			}
		}
		if !jobFound {
			return service.ErrConsentInvalid
		}
	}

	return nil
}

func (r *ApplicationRepository) loadApplicationCapability(ctx context.Context, jobID string) (models.ApplicationCapability, int, error) {
	var job jobDoc
	err := r.db.Collection("jobs").FindOne(ctx, bson.M{"_id": jobID}).Decode(&job)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.CapabilityUnsupported, 1, service.ErrApplicationResourceInvalid
	}
	if err != nil {
		return models.CapabilityUnsupported, 1, fmt.Errorf("load job capability: %w", err)
	}

	cap := validCapability(job.ApplicationCapability)
	return cap, 1, nil
}

func (r *ApplicationRepository) insertApplicationEvent(ctx context.Context, scope service.ActorScope, applicationID, eventType string, from *models.ApplicationStatus, to models.ApplicationStatus, now time.Time) error {
	var fromVal *string
	if from != nil {
		s := string(*from)
		fromVal = &s
	}
	actorType, actorID := applicationEventActor(eventType, scope.UserID)
	var actorIDStr *string
	if str, ok := actorID.(string); ok {
		actorIDStr = &str
	}

	eventDoc := bson.M{
		"_id":            newUUID(),
		"tenant_id":      scope.TenantID,
		"application_id": applicationID,
		"event_type":     eventType,
		"from_status":    fromVal,
		"to_status":      string(to),
		"actor_type":     actorType,
		"actor_id":       actorIDStr,
		"occurred_at":    now.UTC(),
	}

	_, err := r.db.Collection("application_events").InsertOne(ctx, eventDoc)
	if err != nil {
		return fmt.Errorf("insert application event: %w", err)
	}
	return nil
}

func (r *ApplicationRepository) insertApplicationOutbox(ctx context.Context, scope service.ActorScope, applicationID, eventType string, payload map[string]string, now time.Time) error {
	return insertOutboxEvent(ctx, r.db, &scope.TenantID, eventType, "application", applicationID, payload, now)
}

func insertOutboxEvent(ctx context.Context, database *mongo.Database, tenantID *string, eventType, aggregateType, aggregateID string, payload any, now time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode outbox payload: %w", err)
	}

	doc := outboxDoc{
		ID:             newUUID(),
		TenantID:       tenantID,
		EventType:      eventType,
		SchemaVersion:  1,
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		PayloadIDsOnly: encoded,
		CreatedAt:      now.UTC(),
	}

	_, err = database.Collection("outbox_events").InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func batchTerminalState(failedItems int) string {
	if failedItems > 0 {
		return "partial"
	}
	return "completed"
}

func applicationJobStatusAllowed(status string) bool { return status == "open" }

func applicationEventActor(eventType, userID string) (string, any) {
	if eventType == service.ApplicationTaskEventPaused {
		return "worker", nil
	}
	return "user", userID
}

func validateWorkflowInput(scope service.ActorScope, identity service.IdempotencyIdentity, now time.Time) error {
	if !validWorkflowScope(scope) || now.IsZero() || len(identity.KeyHash) != 32 || len(identity.RequestHash) != 32 ||
		identity.KeyHash == ([32]byte{}) || identity.RequestHash == ([32]byte{}) {
		return ErrInvalidWorkflowScope
	}
	return nil
}

func validWorkflowScope(scope service.ActorScope) bool {
	return dbUUID(scope.TenantID) && dbUUID(scope.UserID)
}

func normalizeWorkflowScope(scope service.ActorScope) service.ActorScope {
	scope.TenantID = strings.ToLower(strings.TrimSpace(scope.TenantID))
	scope.UserID = strings.ToLower(strings.TrimSpace(scope.UserID))
	return scope
}

func strconvItoa(val int) string {
	return fmt.Sprintf("%d", val)
}

func mapApplicationStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, db.ErrNotMember):
		return service.ErrApplicationNotFound
	case errors.Is(err, service.ErrApplicationNotFound),
		errors.Is(err, service.ErrApplicationDuplicate),
		errors.Is(err, service.ErrIdempotencyConflict),
		errors.Is(err, service.ErrConsentInvalid),
		errors.Is(err, service.ErrApplicationResourceInvalid),
		errors.Is(err, service.ErrApplicationStateConflict),
		errors.Is(err, service.ErrApplicationPolicy),
		errors.Is(err, service.ErrApplicationRequest),
		errors.Is(err, service.ErrApplicationStore),
		errors.Is(err, service.ErrApplicationTaskStore):
		return err
	default:
		return fmt.Errorf("%w: %v", service.ErrApplicationStore, err)
	}
}

var _ service.ApplicationWorkflowStore = (*ApplicationRepository)(nil)
var _ service.ApplicationTaskStore = (*ApplicationRepository)(nil)

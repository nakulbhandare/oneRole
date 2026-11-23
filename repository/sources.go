package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oneRole/connectors/job_sources"
	"github.com/oneRole/db"
	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrSourceLeaseLost = errors.New("job source lease is no longer owned")
	ErrSourceSyncState = errors.New("job source sync is in an invalid state")
)

type SourceSyncRun struct {
	ID               string
	Source           job_sources.SourceConfig
	StartedAt        time.Time
	CheckpointBefore string
	ResumeCursor     string
}

type SourcePageInput struct {
	Run          SourceSyncRun
	CursorBefore string
	Page         job_sources.Page
	ObservedAt   time.Time
}

type SourcePageResult struct {
	RecordsSeen    int
	RecordsCreated int
	RecordsUpdated int
	CursorAfter    string
	Complete       bool
}

type JobSourceSyncStore interface {
	ClaimDueSources(ctx context.Context, owner string, limit int, now, leaseUntil time.Time) ([]job_sources.SourceConfig, error)
	StartSourceSync(ctx context.Context, source job_sources.SourceConfig, now time.Time) (SourceSyncRun, error)
	GetSourceSyncRun(ctx context.Context, sourceID, runID string, now time.Time) (SourceSyncRun, error)
	RenewSourceLease(ctx context.Context, run SourceSyncRun, now, leaseUntil time.Time) error
	IngestSourcePage(ctx context.Context, input SourcePageInput) (SourcePageResult, error)
	CompleteSourceSync(ctx context.Context, run SourceSyncRun, now time.Time) error
	FailSourceSync(ctx context.Context, run SourceSyncRun, status, errorClass string, retryAt, now time.Time) error
	CountJobsPerCompany(ctx context.Context, companyID string) (int64, error)
	GetStuckSourcesX(ctx context.Context, limit int) ([]models.SourceSummary, error)
	HealSource(ctx context.Context, sourceID string) error
}

const maxSourceLeaseExtension = 30 * time.Minute

type JobSourceRepository struct{ db *mongo.Database }

func NewJobSourceStore(database *mongo.Database) *JobSourceRepository {
	return &JobSourceRepository{db: database}
}

func (r *JobSourceRepository) ListSources(ctx context.Context, limit int) ([]models.SourceSummary, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cur, err := r.db.Collection("company_sources").Find(ctx, bson.M{}, options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "last_sync_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []models.SourceSummary
	for cur.Next(ctx) {
		var doc struct {
			ID           string     `bson:"_id"`
			CompanyID    string     `bson:"company_id"`
			CompanyName  string     `bson:"company_name"`
			SourceType   string     `bson:"source_type"`
			ProviderType string     `bson:"provider_type"`
			Endpoint     string     `bson:"source_endpoint"`
			Status       string     `bson:"status"`
			LastSyncAt   *time.Time `bson:"last_sync_at"`
			LogCount     int64      `bson:"log_count"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		st := doc.SourceType
		if st == "" {
			st = doc.ProviderType
		}
		items = append(items, models.SourceSummary{
			ID:           doc.ID,
			CompanyID:    doc.CompanyID,
			CompanyName:  doc.CompanyName,
			SourceType:   st,
			ProviderType: doc.ProviderType,
			BaseURL:      doc.Endpoint,
			Status:       doc.Status,
			LastSyncAt:   doc.LastSyncAt,
			LogCount:     doc.LogCount,
		})
	}
	return items, nil
}

func (r *JobSourceRepository) GetSourceHealth(ctx context.Context, sourceID string) (*models.SourceHealth, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	var doc struct {
		ID             string     `bson:"_id"`
		Status         string     `bson:"status"`
		LastErrorClass string     `bson:"last_error_class"`
		LastSyncAt     *time.Time `bson:"last_sync_at"`
		ProviderType   string     `bson:"provider_type"`
		LogCount       int64      `bson:"log_count"`
	}
	err := r.db.Collection("company_sources").FindOne(ctx, bson.M{"_id": sourceID}).Decode(&doc)
	if err != nil {
		return nil, err
	}
	var lag int64
	if doc.LastSyncAt != nil {
		lag = int64(time.Since(*doc.LastSyncAt).Seconds())
	}
	return &models.SourceHealth{
		SourceID:       doc.ID,
		Status:         doc.Status,
		LastErrorClass: doc.LastErrorClass,
		LastSyncAt:     doc.LastSyncAt,
		SyncLagSeconds: lag,
		Capability:     doc.ProviderType,
		LogCount:       doc.LogCount,
	}, nil
}

type sourceDoc struct {
	ID                  string     `bson:"_id"`
	CompanyID           string     `bson:"company_id"`
	ProviderType        string     `bson:"provider_type"`
	BoardIdentifier     string     `bson:"board_identifier"`
	SourceEndpoint      string     `bson:"source_endpoint"`
	AllowedHosts        []string   `bson:"allowed_hosts"`
	AllowedOperations   []string   `bson:"allowed_operations"`
	Status              string     `bson:"status"`
	DiscoveryEnabled    bool       `bson:"discovery_enabled"`
	RequestsPerWindow   int        `bson:"requests_per_window"`
	WindowSeconds       int        `bson:"window_seconds"`
	MaxConcurrency      int        `bson:"max_concurrency"`
	ConfigVersion       int        `bson:"config_version"`
	ConnectorVersion    string     `bson:"connector_version"`
	PolicyReviewedAt    *time.Time `bson:"policy_reviewed_at,omitempty"`
	LastSuccessAt       *time.Time `bson:"last_success_at,omitempty"`
	LeaseOwner          *string    `bson:"lease_owner,omitempty"`
	LeaseToken          *string    `bson:"lease_token,omitempty"`
	LeaseUntil          *time.Time `bson:"lease_until,omitempty"`
	NextPollAt          *time.Time `bson:"next_poll_at,omitempty"`
	PollIntervalSeconds int        `bson:"poll_interval_seconds"`
	LastErrorAt         *time.Time `bson:"last_error_at,omitempty"`
	LastErrorClass      *string    `bson:"last_error_class,omitempty"`
	LogCount            int64      `bson:"log_count"`
}

type sourceSyncRunDoc struct {
	ID               string     `bson:"_id"`
	SourceID         string     `bson:"source_id"`
	Status           string     `bson:"status"`
	LeaseToken       string     `bson:"lease_token"`
	ConfigVersion    int        `bson:"config_version"`
	ConnectorVersion string     `bson:"connector_version"`
	StartedAt        time.Time  `bson:"started_at"`
	FinishedAt       *time.Time `bson:"finished_at,omitempty"`
	CheckpointBefore string     `bson:"checkpoint_before,omitempty"`
	CheckpointAfter  string     `bson:"checkpoint_after,omitempty"`
	PagesRead        int        `bson:"pages_read"`
	RecordsSeen      int        `bson:"records_seen"`
	RecordsCreated   int        `bson:"records_created"`
	RecordsUpdated   int        `bson:"records_updated"`
	RecordsClosed    int        `bson:"records_closed"`
	CompleteSnapshot bool       `bson:"complete_snapshot"`
	ErrorClass       string     `bson:"error_class,omitempty"`
}

type sourceObservationDoc struct {
	ID                 string     `bson:"_id"`
	SourceID           string     `bson:"source_id"`
	ExternalJobID      string     `bson:"external_job_id"`
	JobID              string     `bson:"job_id"`
	CanonicalJobURL    string     `bson:"canonical_job_url"`
	ApplyURL           *string    `bson:"apply_url,omitempty"`
	SourcePublishedAt  *time.Time `bson:"source_published_at,omitempty"`
	SourceUpdatedAt    *time.Time `bson:"source_updated_at,omitempty"`
	FirstSeenAt        time.Time  `bson:"first_seen_at"`
	LastSeenAt         time.Time  `bson:"last_seen_at"`
	SourceStatus       string     `bson:"source_status"`
	ContentSHA256      []byte     `bson:"content_sha256"`
	SyncRunID          string     `bson:"sync_run_id"`
	LastObservationAt  time.Time  `bson:"last_observation_at"`
}

func (r *JobSourceRepository) ClaimDueSources(ctx context.Context, owner string, limit int, now, leaseUntil time.Time) ([]job_sources.SourceConfig, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	if owner == "" || len(owner) > 128 || limit < 1 || limit > 5000 || !leaseUntil.After(now) {
		return nil, ErrInvalidClaim
	}

	coll := r.db.Collection("company_sources")
	nowUTC := now.UTC()
	leaseUntilUTC := leaseUntil.UTC()

	filter := bson.M{
		"status":            bson.M{"$in": []string{"enabled", "degraded"}},
		"discovery_enabled": true,
		"policy_reviewed_at": bson.M{"$ne": nil},
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"next_poll_at": nil},
					{"next_poll_at": bson.M{"$lte": nowUTC}},
				},
			},
			{
				"$or": []bson.M{
					{"lease_until": nil},
					{"lease_until": bson.M{"$lte": nowUTC}},
				},
			},
		},
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "next_poll_at", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, fmt.Errorf("claim due job sources: %w", err)
	}
	defer cursor.Close(ctx)

	var candidates []sourceDoc
	if err := cursor.All(ctx, &candidates); err != nil {
		return nil, fmt.Errorf("read claimed job sources: %w", err)
	}

	var sources []job_sources.SourceConfig
	for _, cand := range candidates {
		token := newUUID()
		claimFilter := bson.M{
			"_id": cand.ID,
			"$or": []bson.M{
				{"lease_until": nil},
				{"lease_until": bson.M{"$lte": nowUTC}},
			},
		}
		claimUpdate := bson.M{
			"$set": bson.M{
				"lease_owner": owner,
				"lease_token": token,
				"lease_until": leaseUntilUTC,
			},
		}

		res, err := coll.UpdateOne(ctx, claimFilter, claimUpdate)
		if err != nil || res.MatchedCount == 0 {
			continue
		}

		var checkpointDoc struct {
			Cursor string `bson:"cursor"`
		}
		_ = r.db.Collection("source_checkpoints").FindOne(ctx, bson.M{"_id": cand.ID}).Decode(&checkpointDoc)

		cfg := job_sources.SourceConfig{
			ID:                 cand.ID,
			CompanyID:          cand.CompanyID,
			Provider:           job_sources.Provider(cand.ProviderType),
			BoardIdentifier:    cand.BoardIdentifier,
			Endpoint:           cand.SourceEndpoint,
			AllowedHosts:       cand.AllowedHosts,
			AllowedOperations:  cand.AllowedOperations,
			Status:             cand.Status,
			DiscoveryEnabled:   cand.DiscoveryEnabled,
			RequestsPerWindow:  cand.RequestsPerWindow,
			Window:             time.Duration(cand.WindowSeconds) * time.Second,
			MaxConcurrency:     cand.MaxConcurrency,
			ConfigVersion:      cand.ConfigVersion,
			ConnectorVersion:   cand.ConnectorVersion,
			PolicyReviewedAt:   cand.PolicyReviewedAt,
			LastSuccessfulSync: cand.LastSuccessAt,
			LeaseOwner:         owner,
			LeaseToken:         token,
			LeaseUntil:         &leaseUntilUTC,
			CheckpointCursor:   checkpointDoc.Cursor,
			MaxPageSize:        100,
			RequestTimeout:     10 * time.Second,
			MaxResponseBytes:   8 << 20,
		}
		sources = append(sources, cfg)
	}

	return sources, nil
}

func (r *JobSourceRepository) StartSourceSync(ctx context.Context, source job_sources.SourceConfig, now time.Time) (SourceSyncRun, error) {
	if r == nil || r.db == nil {
		return SourceSyncRun{}, db.ErrNilDB
	}

	runID := newUUID()
	nowUTC := now.UTC()

	var run SourceSyncRun
	err := db.WithTx(ctx, r.db.Client(), func(sessCtx mongo.SessionContext) error {
		if err := r.requireSourceLease(sessCtx, source, now); err != nil {
			return err
		}

		var checkpointDoc struct {
			Cursor string `bson:"cursor"`
		}
		_ = r.db.Collection("source_checkpoints").FindOne(sessCtx, bson.M{"_id": source.ID}).Decode(&checkpointDoc)
		completedCursor := checkpointDoc.Cursor

		var runningRun sourceSyncRunDoc
		findErr := r.db.Collection("source_sync_runs").FindOne(sessCtx, bson.M{
			"source_id": source.ID,
			"status":    "running",
		}).Decode(&runningRun)

		if findErr == nil {
			// If running run belongs to the active lease holder, resume it
			if runningRun.LeaseToken == source.LeaseToken && runningRun.ConfigVersion == source.ConfigVersion && runningRun.ConnectorVersion == source.ConnectorVersion {
				run = SourceSyncRun{
					ID:               runningRun.ID,
					Source:           source,
					StartedAt:        runningRun.StartedAt.UTC(),
					CheckpointBefore: runningRun.CheckpointBefore,
					ResumeCursor:     runningRun.CheckpointAfter,
				}
				return nil
			}

			// Otherwise, the previous lease expired or was abandoned; mark it timed_out and start fresh
			_, _ = r.db.Collection("source_sync_runs").UpdateOne(sessCtx,
				bson.M{"_id": runningRun.ID},
				bson.M{"$set": bson.M{
					"status":        "timed_out",
					"error_message": "Lease superseded by new sync run",
					"finished_at":   nowUTC,
				}},
			)
		}

		runRecord := sourceSyncRunDoc{
			ID:               runID,
			SourceID:         source.ID,
			Status:           "running",
			LeaseToken:       source.LeaseToken,
			ConfigVersion:    source.ConfigVersion,
			ConnectorVersion: source.ConnectorVersion,
			StartedAt:        nowUTC,
			CheckpointBefore: completedCursor,
		}

		if _, err := r.db.Collection("source_sync_runs").InsertOne(sessCtx, runRecord); err != nil {
			return fmt.Errorf("create source sync run: %w", err)
		}

		run = SourceSyncRun{
			ID:               runID,
			Source:           source,
			StartedAt:        nowUTC,
			CheckpointBefore: completedCursor,
			ResumeCursor:     completedCursor,
		}

		payload := map[string]string{
			"source_id":   source.ID,
			"sync_run_id": run.ID,
		}
		if err := insertOutboxEvent(sessCtx, r.db, nil, "discovery.source.sync.requested", "source_sync_run", run.ID, payload, now); err != nil {
			return fmt.Errorf("enqueue durable source sync request: %w", err)
		}

		run.Source.CheckpointCursor = run.ResumeCursor
		return nil
	})

	if err != nil {
		return SourceSyncRun{}, err
	}
	return run, nil
}

func (r *JobSourceRepository) GetSourceSyncRun(ctx context.Context, sourceID, runID string, now time.Time) (SourceSyncRun, error) {
	if r == nil || r.db == nil {
		return SourceSyncRun{}, db.ErrNilDB
	}

	var cand sourceDoc
	err := r.db.Collection("company_sources").FindOne(ctx, bson.M{"_id": sourceID}).Decode(&cand)
	if err != nil {
		return SourceSyncRun{}, ErrSourceSyncState
	}

	leaseOwner := ""
	if cand.LeaseOwner != nil {
		leaseOwner = *cand.LeaseOwner
	}
	leaseToken := ""
	if cand.LeaseToken != nil {
		leaseToken = *cand.LeaseToken
	}

	source := job_sources.SourceConfig{
		ID:                 cand.ID,
		CompanyID:          cand.CompanyID,
		Provider:           job_sources.Provider(cand.ProviderType),
		BoardIdentifier:    cand.BoardIdentifier,
		Endpoint:           cand.SourceEndpoint,
		AllowedHosts:       cand.AllowedHosts,
		AllowedOperations:  cand.AllowedOperations,
		Status:             cand.Status,
		DiscoveryEnabled:   cand.DiscoveryEnabled,
		RequestsPerWindow:  cand.RequestsPerWindow,
		Window:             time.Duration(cand.WindowSeconds) * time.Second,
		MaxConcurrency:     cand.MaxConcurrency,
		ConfigVersion:      cand.ConfigVersion,
		ConnectorVersion:   cand.ConnectorVersion,
		PolicyReviewedAt:   cand.PolicyReviewedAt,
		LastSuccessfulSync: cand.LastSuccessAt,
		LeaseOwner:         leaseOwner,
		LeaseToken:         leaseToken,
		LeaseUntil:         cand.LeaseUntil,
		MaxPageSize:        100,
		RequestTimeout:     10 * time.Second,
		MaxResponseBytes:   8 << 20,
	}

	if err := r.requireSourceLease(ctx, source, now); err != nil {
		return SourceSyncRun{}, err
	}

	var runDoc sourceSyncRunDoc
	err = r.db.Collection("source_sync_runs").FindOne(ctx, bson.M{
		"_id":       runID,
		"source_id": sourceID,
		"status":    "running",
	}).Decode(&runDoc)
	if err != nil {
		return SourceSyncRun{}, ErrSourceSyncState
	}

	if runDoc.LeaseToken != source.LeaseToken || runDoc.ConfigVersion != source.ConfigVersion || runDoc.ConnectorVersion != source.ConnectorVersion {
		return SourceSyncRun{}, ErrSourceLeaseLost
	}

	return SourceSyncRun{
		ID:               runID,
		Source:           source,
		StartedAt:        runDoc.StartedAt.UTC(),
		CheckpointBefore: runDoc.CheckpointBefore,
		ResumeCursor:     runDoc.CheckpointAfter,
	}, nil
}

func (r *JobSourceRepository) IngestSourcePage(ctx context.Context, input SourcePageInput) (SourcePageResult, error) {
	if r == nil || r.db == nil {
		return SourcePageResult{}, db.ErrNilDB
	}
	if input.Run.ID == "" || input.ObservedAt.IsZero() || (input.Page.Complete && input.Page.NextCursor != "") || (!input.Page.Complete && input.Page.NextCursor == "") || (input.Page.NextCursor != "" && input.Page.NextCursor == input.CursorBefore) {
		return SourcePageResult{}, ErrSourceSyncState
	}

	var result SourcePageResult
	err := db.WithTx(ctx, r.db.Client(), func(sessCtx mongo.SessionContext) error {
		if err := r.requireSourceLease(sessCtx, input.Run.Source, input.ObservedAt); err != nil {
			return err
		}

		var runDoc sourceSyncRunDoc
		err := r.db.Collection("source_sync_runs").FindOne(sessCtx, bson.M{
			"_id":       input.Run.ID,
			"source_id": input.Run.Source.ID,
			"status":    "running",
		}).Decode(&runDoc)
		if err != nil {
			return ErrSourceSyncState
		}

		if runDoc.LeaseToken != input.Run.Source.LeaseToken || runDoc.ConfigVersion != input.Run.Source.ConfigVersion || runDoc.ConnectorVersion != input.Run.Source.ConnectorVersion {
			return ErrSourceLeaseLost
		}

		if runDoc.CheckpointAfter != input.CursorBefore {
			return ErrSourceSyncState
		}

		nowUTC := input.ObservedAt.UTC()
		for _, posting := range input.Page.Items {
			created, err := r.ingestPosting(sessCtx, input.Run, posting, nowUTC)
			if err != nil {
				return err
			}
			result.RecordsSeen++
			if created {
				result.RecordsCreated++
			} else {
				result.RecordsUpdated++
			}
		}

		result.CursorAfter = input.Page.NextCursor
		result.Complete = input.Page.Complete
		completeSnapshot := input.Page.Complete && input.Page.FullSnapshot

		update := bson.M{
			"$inc": bson.M{
				"pages_read":      1,
				"records_seen":    result.RecordsSeen,
				"records_created": result.RecordsCreated,
				"records_updated": result.RecordsUpdated,
			},
			"$set": bson.M{
				"checkpoint_after":  result.CursorAfter,
				"complete_snapshot": runDoc.CompleteSnapshot || completeSnapshot,
			},
		}

		res, err := r.db.Collection("source_sync_runs").UpdateOne(sessCtx, bson.M{
			"_id":               input.Run.ID,
			"status":            "running",
			"lease_token":       input.Run.Source.LeaseToken,
			"config_version":    input.Run.Source.ConfigVersion,
			"connector_version": input.Run.Source.ConnectorVersion,
		}, update)
		if err != nil {
			return fmt.Errorf("update source sync counters: %w", err)
		}
		if res.MatchedCount != 1 {
			return ErrSourceLeaseLost
		}

		_, err = r.db.Collection("company_sources").UpdateOne(sessCtx, bson.M{
			"_id": input.Run.Source.ID,
		}, bson.M{
			"$inc": bson.M{"log_count": result.RecordsSeen},
		})
		if err != nil {
			return fmt.Errorf("increment source log count: %w", err)
		}

		return nil
	})

	if err != nil {
		return SourcePageResult{}, err
	}
	return result, nil
}

func (r *JobSourceRepository) RenewSourceLease(ctx context.Context, run SourceSyncRun, now, leaseUntil time.Time) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	now = now.UTC()
	leaseUntil = leaseUntil.UTC()
	if run.ID == "" || !leaseUntil.After(now) || leaseUntil.Sub(now) > maxSourceLeaseExtension {
		return ErrInvalidClaim
	}

	var runDoc sourceSyncRunDoc
	err := r.db.Collection("source_sync_runs").FindOne(ctx, bson.M{
		"_id":       run.ID,
		"source_id": run.Source.ID,
		"status":    "running",
	}).Decode(&runDoc)
	if err != nil {
		return ErrSourceSyncState
	}

	if runDoc.LeaseToken != run.Source.LeaseToken || runDoc.ConfigVersion != run.Source.ConfigVersion || runDoc.ConnectorVersion != run.Source.ConnectorVersion {
		return ErrSourceLeaseLost
	}

	res, err := r.db.Collection("company_sources").UpdateOne(ctx, bson.M{
		"_id":               run.Source.ID,
		"lease_owner":       run.Source.LeaseOwner,
		"lease_token":       run.Source.LeaseToken,
		"lease_until":       bson.M{"$gt": now},
		"config_version":    run.Source.ConfigVersion,
		"connector_version": run.Source.ConnectorVersion,
	}, bson.M{
		"$set": bson.M{
			"lease_until": leaseUntil,
		},
	})
	if err != nil {
		return fmt.Errorf("renew source sync lease: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrSourceLeaseLost
	}
	return nil
}

func (r *JobSourceRepository) CompleteSourceSync(ctx context.Context, run SourceSyncRun, now time.Time) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}

	nowUTC := now.UTC()
	var runDoc sourceSyncRunDoc
	err := r.db.Collection("source_sync_runs").FindOne(ctx, bson.M{
		"_id":       run.ID,
		"source_id": run.Source.ID,
		"status":    "running",
	}).Decode(&runDoc)
	if err != nil {
		return ErrSourceSyncState
	}

	if runDoc.LeaseToken != run.Source.LeaseToken || runDoc.ConfigVersion != run.Source.ConfigVersion || runDoc.ConnectorVersion != run.Source.ConnectorVersion {
		return ErrSourceLeaseLost
	}

	closed := 0
	if runDoc.CompleteSnapshot {
		obsFilter := bson.M{
			"source_id":     run.Source.ID,
			"source_status": "open",
			"sync_run_id":   bson.M{"$ne": run.ID},
			"last_seen_at":  bson.M{"$lt": runDoc.StartedAt},
		}
		cursor, err := r.db.Collection("job_source_observations").Find(ctx, obsFilter)
		if err == nil {
			var missingObs []sourceObservationDoc
			_ = cursor.All(ctx, &missingObs)
			cursor.Close(ctx)
			for _, obs := range missingObs {
				_, _ = r.db.Collection("job_source_observations").UpdateOne(ctx, bson.M{"_id": obs.ID}, bson.M{
					"$set": bson.M{"source_status": "removed", "last_observation_at": nowUTC},
				})
				// Check if other open observations exist
				openCount, _ := r.db.Collection("job_source_observations").CountDocuments(ctx, bson.M{
					"job_id":        obs.JobID,
					"source_status": "open",
				})
				if openCount == 0 {
					res, _ := r.db.Collection("jobs").UpdateOne(ctx, bson.M{
						"_id":    obs.JobID,
						"status": bson.M{"$in": []string{"open", "stale"}},
					}, bson.M{
						"$set": bson.M{"status": "closed", "closed_at": nowUTC, "updated_at": nowUTC},
					})
					if res != nil && res.ModifiedCount > 0 {
						closed++
					}
				}
			}
		}
	}

	_, err = r.db.Collection("source_sync_runs").UpdateOne(ctx, bson.M{
		"_id":               run.ID,
		"status":            "running",
		"lease_token":       run.Source.LeaseToken,
		"config_version":    run.Source.ConfigVersion,
		"connector_version": run.Source.ConnectorVersion,
	}, bson.M{
		"$set": bson.M{
			"status":      "succeeded",
			"finished_at": nowUTC,
		},
		"$inc": bson.M{
			"records_closed": closed,
		},
	})
	if err != nil {
		return fmt.Errorf("complete source sync run: %w", err)
	}

	_, _ = r.db.Collection("source_checkpoints").UpdateOne(ctx, bson.M{"_id": run.Source.ID}, bson.M{
		"$set": bson.M{
			"cursor":            runDoc.CheckpointAfter,
			"last_completed_at": nowUTC,
			"updated_at":        nowUTC,
		},
	}, options.Update().SetUpsert(true))

	intervalSec := run.Source.Window / time.Second
	if intervalSec <= 0 {
		intervalSec = 300
	}
	nextPoll := nowUTC.Add(intervalSec * time.Second)

	res, err := r.db.Collection("company_sources").UpdateOne(ctx, bson.M{
		"_id":         run.Source.ID,
		"lease_owner": run.Source.LeaseOwner,
		"lease_token": run.Source.LeaseToken,
	}, bson.M{
		"$set": bson.M{
			"status":          "enabled",
			"last_success_at": nowUTC,
			"next_poll_at":    nextPoll,
		},
		"$unset": bson.M{
			"lease_owner":      "",
			"lease_token":      "",
			"lease_until":      "",
			"last_error_at":    "",
			"last_error_class": "",
		},
	})
	if err != nil {
		return fmt.Errorf("release completed source lease: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrSourceLeaseLost
	}

	return nil
}

func (r *JobSourceRepository) FailSourceSync(ctx context.Context, run SourceSyncRun, status, errorClass string, retryAt, now time.Time) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	if status != "retryable_failure" && status != "permanent_failure" || !errClassPattern.MatchString(errorClass) {
		return ErrSourceSyncState
	}

	nowUTC := now.UTC()
	_, err := r.db.Collection("source_sync_runs").UpdateOne(ctx, bson.M{
		"_id":    run.ID,
		"status": "running",
	}, bson.M{
		"$set": bson.M{
			"status":      status,
			"finished_at": nowUTC,
			"error_class": errorClass,
		},
	})
	if err != nil {
		return fmt.Errorf("mark source sync failure: %w", err)
	}

	nextPoll := retryAt.UTC()
	sourceStatus := "degraded"
	if status == "permanent_failure" {
		nextPoll = nowUTC
		sourceStatus = "paused"
	}

	res, err := r.db.Collection("company_sources").UpdateOne(ctx, bson.M{
		"_id":         run.Source.ID,
		"lease_owner": run.Source.LeaseOwner,
		"lease_token": run.Source.LeaseToken,
	}, bson.M{
		"$set": bson.M{
			"status":           sourceStatus,
			"last_error_at":    nowUTC,
			"last_error_class": errorClass,
			"next_poll_at":     nextPoll,
		},
		"$unset": bson.M{
			"lease_owner": "",
			"lease_token": "",
			"lease_until": "",
		},
	})
	if err != nil {
		return fmt.Errorf("release failed source lease: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrSourceLeaseLost
	}
	return nil
}

func (r *JobSourceRepository) CountJobsPerCompany(ctx context.Context, companyID string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, db.ErrNilDB
	}
	return r.db.Collection("jobs").CountDocuments(ctx, bson.M{"company_id": companyID})
}

func (r *JobSourceRepository) GetStuckSources(ctx context.Context, limit int) ([]models.SourceSummary, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	filter := bson.M{
		"status": bson.M{"$in": []string{"degraded", "paused"}},
	}
	cur, err := r.db.Collection("company_sources").Find(ctx, filter, options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "last_error_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var items []models.SourceSummary
	for cur.Next(ctx) {
		var doc struct {
			ID           string     `bson:"_id"`
			CompanyID    string     `bson:"company_id"`
			CompanyName  string     `bson:"company_name"`
			SourceType   string     `bson:"source_type"`
			ProviderType string     `bson:"provider_type"`
			Endpoint     string     `bson:"source_endpoint"`
			Status       string     `bson:"status"`
			LastSyncAt   *time.Time `bson:"last_sync_at"`
			LogCount     int64      `bson:"log_count"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		items = append(items, models.SourceSummary{
			ID:           doc.ID,
			CompanyID:    doc.CompanyID,
			CompanyName:  doc.CompanyName,
			SourceType:   doc.SourceType,
			ProviderType: doc.ProviderType,
			BaseURL:      doc.Endpoint,
			Status:       doc.Status,
			LastSyncAt:   doc.LastSyncAt,
			LogCount:     doc.LogCount,
		})
	}
	return items, nil
}

func (r *JobSourceRepository) requireSourceLease(ctx context.Context, source job_sources.SourceConfig, now time.Time) error {
	var cand sourceDoc
	err := r.db.Collection("company_sources").FindOne(ctx, bson.M{
		"_id":               source.ID,
		"lease_owner":       source.LeaseOwner,
		"lease_token":       source.LeaseToken,
		"lease_until":       bson.M{"$gt": now.UTC()},
		"config_version":    source.ConfigVersion,
		"connector_version": source.ConnectorVersion,
		"discovery_enabled": true,
		"status":            bson.M{"$in": []string{"enabled", "degraded"}},
	}).Decode(&cand)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrSourceLeaseLost
	}
	if err != nil {
		return fmt.Errorf("verify job source lease: %w", err)
	}
	return nil
}

func (r *JobSourceRepository) ingestPosting(ctx context.Context, run SourceSyncRun, posting job_sources.Posting, now time.Time) (bool, error) {
	posting.ExternalID = strings.TrimSpace(posting.ExternalID)
	posting.Title = strings.TrimSpace(posting.Title)
	posting.Description = strings.TrimSpace(posting.Description)
	posting.CanonicalURL = strings.TrimSpace(posting.CanonicalURL)
	posting.ApplyURL = strings.TrimSpace(posting.ApplyURL)
	if posting.ExternalID == "" || posting.Title == "" || posting.CanonicalURL == "" {
		return false, fmt.Errorf("%w: listing requires source ID, title, and canonical URL", job_sources.ErrInvalidConfig)
	}
	checksum, err := postingChecksum(posting)
	if err != nil {
		return false, fmt.Errorf("hash job source observation: %w", err)
	}

	var existingObs sourceObservationDoc
	err = r.db.Collection("job_source_observations").FindOne(ctx, bson.M{
		"source_id":       run.Source.ID,
		"external_job_id": posting.ExternalID,
	}).Decode(&existingObs)

	created := false
	var jobID, observationID string

	if errors.Is(err, mongo.ErrNoDocuments) {
		created = true
		jobID = newUUID()
		observationID = newUUID()
		employmentTypes := normalizeEmployment(posting.Employment)

		quality := "unknown"
		if posting.PublishedAt != nil {
			quality = "source_provided"
		}

		var compDoc struct {
			CanonicalName string `bson:"canonical_name"`
		}
		_ = r.db.Collection("companies").FindOne(ctx, bson.M{"_id": run.Source.CompanyID}).Decode(&compDoc)

		locStr := ""
		if len(posting.Locations) > 0 {
			locStr = strings.Join(posting.Locations, ", ")
		}

		tier := ClassifyJobTier(compDoc.CanonicalName, posting.Title)
		cleanDesc := posting.Description
		if strings.Contains(cleanDesc, "<") && strings.Contains(cleanDesc, ">") {
			cleanDesc = cleanHTMLToText(cleanDesc)
		}

		newJob := jobDoc{
			ID:                     jobID,
			CompanyID:              &run.Source.CompanyID,
			CompanyName:            compDoc.CanonicalName,
			CanonicalTitle:         posting.Title,
			DescriptionText:        cleanDesc,
			DescriptionHTML:        posting.Description,
			DisplayLocation:        locStr,
			Location:               locStr,
			EmploymentTypes:        employmentTypes,
			WorkplaceModes:         []string{posting.WorkplaceMode},
			SourcePublishedAt:      posting.PublishedAt,
			FirstSeenAt:            now.UTC(),
			LastSeenAt:             now.UTC(),
			PublicationTimeQuality: quality,
			Status:                 "open",
			CompanyTier:            tier,
			JobDomain:              "software_engineering",
			SourceURL:              posting.CanonicalURL,
		}

		if _, insErr := r.db.Collection("jobs").InsertOne(ctx, newJob); insErr != nil {
			return false, fmt.Errorf("create normalized job: %w", insErr)
		}

		var applyURL *string
		if posting.ApplyURL != "" {
			applyURL = &posting.ApplyURL
		}

		newObs := sourceObservationDoc{
			ID:                observationID,
			SourceID:          run.Source.ID,
			ExternalJobID:     posting.ExternalID,
			JobID:             jobID,
			CanonicalJobURL:   posting.CanonicalURL,
			ApplyURL:          applyURL,
			SourcePublishedAt: posting.PublishedAt,
			FirstSeenAt:       now.UTC(),
			LastSeenAt:        now.UTC(),
			SourceStatus:      "open",
			ContentSHA256:     checksum[:],
			SyncRunID:         run.ID,
			LastObservationAt: now.UTC(),
		}

		if _, insErr := r.db.Collection("job_source_observations").InsertOne(ctx, newObs); insErr != nil {
			return false, fmt.Errorf("create job source observation: %w", insErr)
		}
	} else if err != nil {
		return false, fmt.Errorf("find job source observation: %w", err)
	} else {
		jobID = existingObs.JobID
		observationID = existingObs.ID
		changed := !bytes.Equal(existingObs.ContentSHA256, checksum[:])
		employmentTypes := normalizeEmployment(posting.Employment)

		var compDoc struct {
			CanonicalName string `bson:"canonical_name"`
		}
		_ = r.db.Collection("companies").FindOne(ctx, bson.M{"_id": run.Source.CompanyID}).Decode(&compDoc)

		locStr := ""
		if len(posting.Locations) > 0 {
			locStr = strings.Join(posting.Locations, ", ")
		}

		update := bson.M{
			"$set": bson.M{
				"company_id":       run.Source.CompanyID,
				"company_name":     compDoc.CanonicalName,
				"canonical_title":  posting.Title,
				"description_text": posting.Description,
				"display_location": locStr,
				"location":         locStr,
				"employment_types": employmentTypes,
				"workplace_modes":  []string{posting.WorkplaceMode},
				"last_seen_at":     now.UTC(),
				"status":           "open",
				"source_url":       posting.CanonicalURL,
			},
		}
		if posting.PublishedAt != nil {
			update["$set"].(bson.M)["source_published_at"] = posting.PublishedAt
			update["$set"].(bson.M)["publication_time_quality"] = "source_provided"
		}
		if changed {
			update["$set"].(bson.M)["updated_at"] = now.UTC()
		}

		_, _ = r.db.Collection("jobs").UpdateOne(ctx, bson.M{"_id": jobID}, update)

		var applyURL *string
		if posting.ApplyURL != "" {
			applyURL = &posting.ApplyURL
		}

		_, _ = r.db.Collection("job_source_observations").UpdateOne(ctx, bson.M{"_id": observationID}, bson.M{
			"$set": bson.M{
				"canonical_job_url":   posting.CanonicalURL,
				"apply_url":           applyURL,
				"last_seen_at":        now.UTC(),
				"source_status":       "open",
				"content_sha256":      checksum[:],
				"sync_run_id":         run.ID,
				"last_observation_at": now.UTC(),
			},
		})
	}

	return created, nil
}

func postingChecksum(posting job_sources.Posting) ([32]byte, error) {
	locations := dedupeStrings(posting.Locations)
	sort.Strings(locations)
	posting.Locations = locations
	encoded, err := json.Marshal(posting)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func normalizeEmployment(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	var normalized string
	switch value {
	case "full time", "fulltime":
		normalized = "full_time"
	case "part time", "parttime":
		normalized = "part_time"
	case "contract", "contractor":
		normalized = "contract"
	case "temporary", "temp":
		normalized = "temporary"
	case "seasonal":
		normalized = "seasonal"
	case "freelance", "freelancer":
		normalized = "freelance"
	case "internship", "intern":
		normalized = "internship"
	case "apprenticeship", "apprentice":
		normalized = "apprenticeship"
	}
	if normalized == "" {
		return nil
	}
	return []string{normalized}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (r *JobSourceRepository) HealSource(ctx context.Context, sourceID string) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}

	var doc struct {
		Status string `bson:"status"`
	}
	err := r.db.Collection("company_sources").FindOne(ctx, bson.M{"_id": sourceID}).Decode(&doc)
	if err != nil {
		return err
	}

	if doc.Status != "degraded" && doc.Status != "paused" {
		return fmt.Errorf("source %s is not in a healable state: %s", sourceID, doc.Status)
	}

	res, err := r.db.Collection("company_sources").UpdateOne(ctx, bson.M{"_id": sourceID}, bson.M{
		"$set": bson.M{
			"status":       "enabled",
			"next_poll_at": time.Now().UTC(),
		},
		"$unset": bson.M{
			"last_error_at":    "",
			"last_error_class": "",
		},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (r *JobSourceRepository) GetStuckSourcesX(ctx context.Context, limit int) ([]models.SourceSummary, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cur, err := r.db.Collection("company_sources").Find(ctx, bson.M{"status": "stuck"}, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	
	var items []models.SourceSummary
	for cur.Next(ctx) {
		var doc struct {
			ID           string     `bson:"_id"`
			CompanyID    string     `bson:"company_id"`
			CompanyName  string     `bson:"company_name"`
			SourceType   string     `bson:"source_type"`
			ProviderType string     `bson:"provider_type"`
			Endpoint     string     `bson:"source_endpoint"`
			Status       string     `bson:"status"`
			LastSyncAt   *time.Time `bson:"last_sync_at"`
			LogCount     int64      `bson:"log_count"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		items = append(items, models.SourceSummary{
			ID:           doc.ID,
			CompanyID:    doc.CompanyID,
			CompanyName:  doc.CompanyName,
			SourceType:   doc.SourceType,
			ProviderType: doc.ProviderType,
			BaseURL:      doc.Endpoint,
			Status:       doc.Status,
			LastSyncAt:   doc.LastSyncAt,
			LogCount:     doc.LogCount,
		})
	}
	return items, nil
}

func ClassifyJobTier(companyName, title string) string {
	titleLower := strings.ToLower(title)
	compLower := strings.ToLower(companyName)

	if strings.Contains(titleLower, "intern") || strings.Contains(titleLower, "internship") || strings.Contains(titleLower, "trainee") || strings.Contains(titleLower, "co-op") {
		return "internship"
	}

	faang := []string{
		"openai", "anthropic", "apple", "meta", "facebook", "instagram", "whatsapp",
		"google", "alphabet", "youtube", "waymo", "deepmind",
		"tesla", "nvidia", "amazon", "aws", "twitch", "netflix",
		"microsoft", "azure", "github", "linkedin",
		"amd", "intel", "qualcomm", "broadcom", "asml", "arm",
		"xai", "scale ai", "mistral", "cohere", "perplexity",
		"bytedance", "tiktok", "uber", "airbnb", "stripe", "palantir", "snowflake", "databricks", "salesforce", "adobe", "oracle",
	}
	for _, f := range faang {
		if compLower == f || strings.Contains(compLower, f) {
			return "faang"
		}
	}

	mnc := []string{
		"cisco", "ibm", "datadog", "figma", "coinbase",
		"sap", "servicenow", "workday", "paypal", "intuit", "shopify", "spotify",
		"twitter", "block", "atlassian", "twilio", "dropbox", "snap", "pinterest", "doordash", "instacart",
		"robinhood", "crowdstrike", "cloudflare", "mongodb", "elastic", "hubspot", "zendesk",
		"docusign", "zoom", "slack", "okta", "splunk", "vmware", "dell", "hp", "ebay", "walmart",
		"visa", "mastercard", "american express", "jpmorgan", "goldman sachs", "morgan stanley", "citigroup",
		"bank of america", "capital one", "bloomberg", "citadel", "jane street", "two sigma", "tcs", "infosys",
		"wipro", "hcl", "tech mahindra", "cognizant", "ltimindtree", "accenture", "capgemini", "deloitte", "pwc", "ey", "kpmg",
		"fluence energy", "fluenceenergy", "siemens", "schneider",
	}
	for _, m := range mnc {
		if compLower == m || strings.Contains(compLower, m) {
			return "mnc"
		}
	}

	return "startup"
}


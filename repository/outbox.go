package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/oneRole/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrClaimLost    = errors.New("work claim is no longer owned")
	ErrInvalidClaim = errors.New("invalid work claim")
	errClassPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,80}$`)
)

// OutboxEvent is the durable envelope published to JetStream. ClaimToken is
// private database fencing state and must not be sent to the broker.
type OutboxEvent struct {
	ID              string          `json:"id"`
	EventType       string          `json:"event_type"`
	SchemaVersion   int             `json:"schema_version"`
	AggregateType   string          `json:"aggregate_type"`
	AggregateID     string          `json:"aggregate_id"`
	TenantID        *string         `json:"tenant_id,omitempty"`
	PayloadIDsOnly  json.RawMessage `json:"payload_ids_only"`
	CreatedAt       time.Time       `json:"created_at"`
	ClaimToken      string          `json:"-"`
	PublishAttempts int             `json:"-"`
}

type OutboxStore interface {
	ClaimPending(ctx context.Context, owner string, limit int, now, leaseUntil time.Time) ([]OutboxEvent, error)
	MarkPublished(ctx context.Context, id, claimToken string, at time.Time) error
	RecordPublishFailure(ctx context.Context, id, claimToken, errorClass string, retryAt time.Time) error
}

type OutboxRepository struct{ db *mongo.Database }

func NewOutboxStore(database *mongo.Database) *OutboxRepository { return &OutboxRepository{db: database} }

type outboxDoc struct {
	ID              string     `bson:"_id"`
	EventType       string     `bson:"event_type"`
	SchemaVersion   int        `bson:"schema_version"`
	AggregateType   string     `bson:"aggregate_type"`
	AggregateID     string     `bson:"aggregate_id"`
	TenantID        *string    `bson:"tenant_id,omitempty"`
	PayloadIDsOnly  []byte     `bson:"payload_ids_only"`
	CreatedAt       time.Time  `bson:"created_at"`
	PublishedAt     *time.Time `bson:"published_at,omitempty"`
	NextAttemptAt   time.Time  `bson:"next_attempt_at"`
	LeaseOwner      *string    `bson:"lease_owner,omitempty"`
	LeaseToken      *string    `bson:"lease_token,omitempty"`
	LeaseUntil      *time.Time `bson:"lease_until,omitempty"`
	PublishAttempts int        `bson:"publish_attempts"`
	LastErrorClass  *string    `bson:"last_error_class,omitempty"`
}

func (r *OutboxRepository) ClaimPending(ctx context.Context, owner string, limit int, now, leaseUntil time.Time) ([]OutboxEvent, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	if owner == "" || len(owner) > 128 || limit < 1 || limit > 500 || !leaseUntil.After(now) {
		return nil, ErrInvalidClaim
	}

	coll := r.db.Collection("outbox_events")
	nowUTC := now.UTC()
	leaseUntilUTC := leaseUntil.UTC()

	filter := bson.M{
		"published_at": nil,
		"next_attempt_at": bson.M{
			"$lte": nowUTC,
		},
		"$or": []bson.M{
			{"lease_until": nil},
			{"lease_until": bson.M{"$lte": nowUTC}},
		},
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, fmt.Errorf("query outbox events: %w", err)
	}
	defer cursor.Close(ctx)

	var candidates []outboxDoc
	if err := cursor.All(ctx, &candidates); err != nil {
		return nil, fmt.Errorf("read outbox candidates: %w", err)
	}

	var claimed []OutboxEvent
	for _, cand := range candidates {
		token := newUUID()
		claimFilter := bson.M{
			"_id":          cand.ID,
			"published_at": nil,
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
			"$inc": bson.M{
				"publish_attempts": 1,
			},
		}

		res, err := coll.UpdateOne(ctx, claimFilter, claimUpdate)
		if err != nil || res.MatchedCount == 0 {
			continue
		}

		claimed = append(claimed, OutboxEvent{
			ID:              cand.ID,
			EventType:       cand.EventType,
			SchemaVersion:   cand.SchemaVersion,
			AggregateType:   cand.AggregateType,
			AggregateID:     cand.AggregateID,
			TenantID:        cand.TenantID,
			PayloadIDsOnly:  json.RawMessage(cand.PayloadIDsOnly),
			CreatedAt:       cand.CreatedAt.UTC(),
			ClaimToken:      token,
			PublishAttempts: cand.PublishAttempts + 1,
		})
	}

	return claimed, nil
}

func (r *OutboxRepository) MarkPublished(ctx context.Context, id, claimToken string, at time.Time) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}

	coll := r.db.Collection("outbox_events")
	filter := bson.M{
		"_id":          id,
		"lease_token":  claimToken,
		"published_at": nil,
	}
	update := bson.M{
		"$set": bson.M{
			"published_at": at.UTC(),
		},
		"$unset": bson.M{
			"lease_owner":      "",
			"lease_token":      "",
			"lease_until":      "",
			"last_error_class": "",
		},
	}

	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrClaimLost
	}
	return nil
}

func (r *OutboxRepository) RecordPublishFailure(ctx context.Context, id, claimToken, errorClass string, retryAt time.Time) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	if !errClassPattern.MatchString(errorClass) {
		return errors.New("error class must be a short sanitized identifier")
	}

	coll := r.db.Collection("outbox_events")
	filter := bson.M{
		"_id":          id,
		"lease_token":  claimToken,
		"published_at": nil,
	}
	update := bson.M{
		"$set": bson.M{
			"last_error_class": errorClass,
			"next_attempt_at":  retryAt.UTC(),
		},
		"$unset": bson.M{
			"lease_owner": "",
			"lease_token": "",
			"lease_until": "",
		},
	}

	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("record outbox publish failure: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrClaimLost
	}
	return nil
}

var _ OutboxStore = (*OutboxRepository)(nil)

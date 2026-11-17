package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oneRole/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type InboxClaimResult uint8

const (
	InboxClaimAcquired InboxClaimResult = iota + 1
	InboxAlreadyProcessed
	InboxLeaseHeld
)

type InboxClaim struct {
	ConsumerName string
	EventID      string
	Owner        string
	Token        string
	LeaseUntil   time.Time
}

type InboxStore interface {
	Claim(ctx context.Context, consumerName, eventID, owner string, now, leaseUntil time.Time) (InboxClaim, InboxClaimResult, error)
	Complete(ctx context.Context, claim InboxClaim, at time.Time, resultResourceID *string) error
	Release(ctx context.Context, claim InboxClaim, retryAt time.Time, errorClass string) error
}

type InboxRepository struct{ db *mongo.Database }

func NewInboxStore(database *mongo.Database) *InboxRepository { return &InboxRepository{db: database} }

type inboxDoc struct {
	ID               string     `bson:"_id"`
	ConsumerName     string     `bson:"consumer_name"`
	EventID          string     `bson:"event_id"`
	Status           string     `bson:"status"`
	ClaimOwner       *string    `bson:"claim_owner,omitempty"`
	ClaimToken       *string    `bson:"claim_token,omitempty"`
	LeaseUntil       *time.Time `bson:"lease_until,omitempty"`
	NextAttemptAt    time.Time  `bson:"next_attempt_at"`
	AttemptCount     int        `bson:"attempt_count"`
	ProcessedAt      *time.Time `bson:"processed_at,omitempty"`
	LastErrorClass   *string    `bson:"last_error_class,omitempty"`
	ResultResourceID *string    `bson:"result_resource_id,omitempty"`
}

func (r *InboxRepository) Claim(ctx context.Context, consumerName, eventID, owner string, now, leaseUntil time.Time) (InboxClaim, InboxClaimResult, error) {
	if r == nil || r.db == nil {
		return InboxClaim{}, 0, db.ErrNilDB
	}
	if consumerName == "" || len(consumerName) > 128 || owner == "" || len(owner) > 128 || !leaseUntil.After(now) {
		return InboxClaim{}, 0, ErrInvalidClaim
	}

	coll := r.db.Collection("inbox_messages")
	docID := consumerName + ":" + eventID
	token := newUUID()
	nowUTC := now.UTC()
	leaseUntilUTC := leaseUntil.UTC()

	// Try updating an existing unleased or expired record
	filter := bson.M{
		"_id":          docID,
		"processed_at": nil,
		"next_attempt_at": bson.M{
			"$lte": nowUTC,
		},
		"$or": []bson.M{
			{"lease_until": nil},
			{"lease_until": bson.M{"$lte": nowUTC}},
		},
	}
	update := bson.M{
		"$set": bson.M{
			"status":          "processing",
			"claim_owner":     owner,
			"claim_token":     token,
			"lease_until":     leaseUntilUTC,
			"next_attempt_at": nowUTC,
		},
		"$inc": bson.M{
			"attempt_count": 1,
		},
		"$unset": bson.M{
			"last_error_class": "",
		},
	}

	after := options.After
	var updated inboxDoc
	err := coll.FindOneAndUpdate(ctx, filter, update, &options.FindOneAndUpdateOptions{ReturnDocument: &after}).Decode(&updated)
	if err == nil {
		return InboxClaim{
			ConsumerName: consumerName,
			EventID:      eventID,
			Owner:        owner,
			Token:        token,
			LeaseUntil:   leaseUntilUTC,
		}, InboxClaimAcquired, nil
	}

	if !errors.Is(err, mongo.ErrNoDocuments) {
		return InboxClaim{}, 0, fmt.Errorf("claim inbox event: %w", err)
	}

	// Try inserting a new document
	newDoc := bson.M{
		"_id":             docID,
		"consumer_name":   consumerName,
		"event_id":        eventID,
		"status":          "processing",
		"claim_owner":     owner,
		"claim_token":     token,
		"lease_until":     leaseUntilUTC,
		"next_attempt_at": nowUTC,
		"attempt_count":   1,
		"processed_at":    nil,
	}
	_, insertErr := coll.InsertOne(ctx, newDoc)
	if insertErr == nil {
		return InboxClaim{
			ConsumerName: consumerName,
			EventID:      eventID,
			Owner:        owner,
			Token:        token,
			LeaseUntil:   leaseUntilUTC,
		}, InboxClaimAcquired, nil
	}

	// Inspect existing document
	var existing inboxDoc
	lookupErr := coll.FindOne(ctx, bson.M{"_id": docID}).Decode(&existing)
	if errors.Is(lookupErr, mongo.ErrNoDocuments) {
		return InboxClaim{}, InboxLeaseHeld, nil
	}
	if lookupErr != nil {
		return InboxClaim{}, 0, fmt.Errorf("inspect inbox claim: %w", lookupErr)
	}
	if existing.ProcessedAt != nil {
		return InboxClaim{}, InboxAlreadyProcessed, nil
	}
	if (existing.LeaseUntil != nil && existing.LeaseUntil.After(now)) || existing.NextAttemptAt.After(now) {
		return InboxClaim{}, InboxLeaseHeld, nil
	}

	return InboxClaim{}, InboxLeaseHeld, nil
}

func (r *InboxRepository) Complete(ctx context.Context, claim InboxClaim, at time.Time, resultResourceID *string) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	if !validInboxClaim(claim) {
		return ErrInvalidClaim
	}

	coll := r.db.Collection("inbox_messages")
	docID := claim.ConsumerName + ":" + claim.EventID
	filter := bson.M{
		"_id":          docID,
		"claim_owner":  claim.Owner,
		"claim_token":  claim.Token,
		"processed_at": nil,
	}
	update := bson.M{
		"$set": bson.M{
			"status":             "processed",
			"processed_at":       at.UTC(),
			"result_resource_id": resultResourceID,
		},
		"$unset": bson.M{
			"claim_owner":      "",
			"claim_token":      "",
			"lease_until":      "",
			"last_error_class": "",
		},
	}

	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("complete inbox event: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrClaimLost
	}
	return nil
}

func (r *InboxRepository) Release(ctx context.Context, claim InboxClaim, retryAt time.Time, errorClass string) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	if !validInboxClaim(claim) {
		return ErrInvalidClaim
	}
	if !errClassPattern.MatchString(errorClass) {
		return errors.New("error class must be a short sanitized identifier")
	}

	coll := r.db.Collection("inbox_messages")
	docID := claim.ConsumerName + ":" + claim.EventID
	filter := bson.M{
		"_id":          docID,
		"claim_owner":  claim.Owner,
		"claim_token":  claim.Token,
		"processed_at": nil,
	}
	update := bson.M{
		"$set": bson.M{
			"next_attempt_at":  retryAt.UTC(),
			"last_error_class": errorClass,
		},
		"$unset": bson.M{
			"claim_owner": "",
			"claim_token": "",
			"lease_until": "",
		},
	}

	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("release inbox event: %w", err)
	}
	if res.MatchedCount != 1 {
		return ErrClaimLost
	}
	return nil
}

func validInboxClaim(claim InboxClaim) bool {
	return claim.ConsumerName != "" && claim.EventID != "" && claim.Owner != "" && claim.Token != ""
}

var _ InboxStore = (*InboxRepository)(nil)

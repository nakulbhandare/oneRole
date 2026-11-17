package migrations

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const CurrentVersion = 1

// Apply creates collections and sets up all required indexes in MongoDB.
func Apply(ctx context.Context, database *mongo.Database) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}

	specs := map[string][]mongo.IndexModel{
		"auth_oauth_transactions": {
			{
				Keys:    bson.D{{Key: "state_hash", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys:    bson.D{{Key: "expires_at", Value: 1}},
				Options: options.Index().SetExpireAfterSeconds(0),
			},
		},
		"auth_identities": {
			{
				Keys:    bson.D{{Key: "provider", Value: 1}, {Key: "provider_subject", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "user_id", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "email", Value: 1}},
			},
		},
		"auth_users": {
			{
				Keys:    bson.D{{Key: "primary_email", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
		},
		"tenant_memberships": {
			{
				Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "user_id", Value: 1}},
			},
		},
		"auth_sessions": {
			{
				Keys: bson.D{{Key: "session_token_hash", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "user_id", Value: 1}},
			},
		},
		"auth_refresh_tokens": {
			{
				Keys:    bson.D{{Key: "token_hash", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "session_id", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "family_id", Value: 1}},
			},
		},
		"jobs": {
			{
				Keys: bson.D{
					{Key: "canonical_title", Value: "text"},
					{Key: "description_text", Value: "text"},
					{Key: "responsibilities_text", Value: "text"},
					{Key: "qualifications_text", Value: "text"},
				},
				Options: options.Index().SetWeights(bson.M{
					"canonical_title":       10,
					"description_text":      5,
					"responsibilities_text": 3,
					"qualifications_text":   3,
				}),
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "first_seen_at", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "first_seen_at", Value: -1}, {Key: "_id", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "source_published_at", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "source_published_at", Value: -1}, {Key: "_id", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "salary_max", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "salary_max", Value: -1}, {Key: "_id", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "company_id", Value: 1}, {Key: "status", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "company_name", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "company_name", Value: 1}, {Key: "source_published_at", Value: -1}, {Key: "_id", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "display_location", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "display_location", Value: 1}, {Key: "source_published_at", Value: -1}, {Key: "_id", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "workplace_modes", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "workplace_modes", Value: 1}, {Key: "source_published_at", Value: -1}, {Key: "_id", Value: -1}},
			},
			{
				Keys: bson.D{{Key: "workplace_mode", Value: 1}, {Key: "status", Value: 1}},
			},
		},
		"companies": {
			{
				Keys:    bson.D{{Key: "canonical_domain", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "name", Value: "text"}},
			},
		},
		"applications": {
			{
				Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "user_id", Value: 1}, {Key: "job_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "created_at", Value: -1}},
			},
		},
		"application_batches": {
			{
				Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}},
			},
		},
		"application_tasks": {
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "next_run_at", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "application_id", Value: 1}},
			},
		},
		"application_events": {
			{
				Keys: bson.D{{Key: "application_id", Value: 1}, {Key: "created_at", Value: 1}},
			},
		},
		"idempotency_keys": {
			{
				Keys:    bson.D{{Key: "tenant_id", Value: 1}, {Key: "scope", Value: 1}, {Key: "key_hash", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys:    bson.D{{Key: "expires_at", Value: 1}},
				Options: options.Index().SetExpireAfterSeconds(0),
			},
		},
		"timeline_events": {
			{
				Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "application_id", Value: 1}, {Key: "occurred_at", Value: -1}},
			},
		},
		"job_sources": {
			{
				Keys:    bson.D{{Key: "provider", Value: 1}, {Key: "board_token", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "is_enabled", Value: 1}, {Key: "lease_until", Value: 1}},
			},
		},
		"source_sync_runs": {
			{
				Keys: bson.D{{Key: "source_id", Value: 1}, {Key: "created_at", Value: -1}},
			},
		},
		"job_postings": {
			{
				Keys:    bson.D{{Key: "source_id", Value: 1}, {Key: "external_id", Value: 1}},
				Options: options.Index().SetUnique(true),
			},
			{
				Keys: bson.D{{Key: "job_id", Value: 1}},
			},
		},
		"job_evidence": {
			{
				Keys: bson.D{{Key: "job_id", Value: 1}},
			},
		},
		"inbox_messages": {
			{
				Keys: bson.D{{Key: "message_id", Value: 1}, {Key: "workload", Value: 1}},
			},
			{
				Keys: bson.D{{Key: "lease_expires_at", Value: 1}, {Key: "processed_at", Value: 1}},
			},
		},
		"outbox_events": {
			{
				Keys: bson.D{{Key: "status", Value: 1}, {Key: "lease_expires_at", Value: 1}, {Key: "created_at", Value: 1}},
			},
		},
	}

	for collName, indexes := range specs {
		coll := database.Collection(collName)
		_, err := coll.Indexes().CreateMany(ctx, indexes)
		if err != nil {
			return fmt.Errorf("create indexes for collection %s: %w", collName, err)
		}
	}

	// Record version in schema_versions
	versionColl := database.Collection("schema_versions")
	_, err := versionColl.UpdateOne(
		ctx,
		bson.M{"_id": "schema_state"},
		bson.M{
			"$set": bson.M{
				"version":    CurrentVersion,
				"applied_at": time.Now().UTC(),
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}

	return nil
}

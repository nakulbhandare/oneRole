package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

var (
	ErrInvalidUUID = errors.New("invalid UUID")
	ErrNotMember   = errors.New("user is not an active member of tenant")
	ErrNilDB       = errors.New("database is nil")
	uuidPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Config controls the MongoDB client connection pool.
type Config struct {
	URI             string
	Database        string
	MaxPoolSize     uint64
	MinPoolSize     uint64
	ConnMaxIdleTime time.Duration
	ConnectTimeout  time.Duration
}

// Open creates and verifies a MongoDB client connection. It does not run
// migrations; schema and index setup are performed through explicit migrate mode.
func Open(ctx context.Context, cfg Config) (*mongo.Client, *mongo.Database, error) {
	if cfg.URI == "" {
		return nil, nil, errors.New("database URI is required")
	}
	parsed, err := connstring.ParseAndValidate(cfg.URI)
	if err != nil {
		return nil, nil, fmt.Errorf("parse MongoDB URI: %w", err)
	}
	dbName := cfg.Database
	if dbName == "" {
		dbName = parsed.Database
	}
	if dbName == "" {
		dbName = "onerole"
	}

	clientOpts := options.Client().ApplyURI(cfg.URI)
	if cfg.MaxPoolSize > 0 {
		clientOpts.SetMaxPoolSize(cfg.MaxPoolSize)
	} else {
		clientOpts.SetMaxPoolSize(64)
	}
	if cfg.MinPoolSize > 0 {
		clientOpts.SetMinPoolSize(cfg.MinPoolSize)
	} else {
		clientOpts.SetMinPoolSize(4)
	}
	if cfg.ConnMaxIdleTime > 0 {
		clientOpts.SetMaxConnIdleTime(cfg.ConnMaxIdleTime)
	} else {
		clientOpts.SetMaxConnIdleTime(5 * time.Minute)
	}
	if cfg.ConnectTimeout > 0 {
		clientOpts.SetConnectTimeout(cfg.ConnectTimeout)
	} else {
		clientOpts.SetConnectTimeout(10 * time.Second)
	}

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("connect MongoDB: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return nil, nil, fmt.Errorf("ping MongoDB: %w", err)
	}

	database := client.Database(dbName)
	return client, database, nil
}

var (
	txSupportMu sync.RWMutex
	txSupported = make(map[*mongo.Client]bool)
)

func checkTxSupport(ctx context.Context, client *mongo.Client) bool {
	txSupportMu.RLock()
	supported, found := txSupported[client]
	txSupportMu.RUnlock()
	if found {
		return supported
	}

	txSupportMu.Lock()
	defer txSupportMu.Unlock()
	if supported, found := txSupported[client]; found {
		return supported
	}

	var helloResult bson.M
	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	err := client.Database("admin").RunCommand(cmdCtx, bson.D{{Key: "hello", Value: 1}}).Decode(&helloResult)
	if err != nil {
		_ = client.Database("admin").RunCommand(cmdCtx, bson.D{{Key: "isMaster", Value: 1}}).Decode(&helloResult)
	}

	isReplSet := false
	if setName, ok := helloResult["setName"].(string); ok && setName != "" {
		isReplSet = true
	}
	if msg, ok := helloResult["msg"].(string); ok && msg == "isdbgrid" {
		isReplSet = true
	}

	txSupported[client] = isReplSet
	return isReplSet
}

// WithTx runs fn within a MongoDB transaction when supported by the cluster,
// or inside a session context for standalone deployments.
func WithTx(ctx context.Context, client *mongo.Client, fn func(mongo.SessionContext) error) error {
	if client == nil {
		return ErrNilDB
	}
	if fn == nil {
		return errors.New("transaction callback is required")
	}

	session, err := client.StartSession()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	defer session.EndSession(ctx)

	if !checkTxSupport(ctx, client) {
		return mongo.WithSession(ctx, session, func(sessCtx mongo.SessionContext) error {
			return fn(sessCtx)
		})
	}

	_, txErr := session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		return nil, fn(sessCtx)
	})
	if txErr != nil {
		errStr := txErr.Error()
		if strings.Contains(errStr, "replica set") || strings.Contains(errStr, "Transaction numbers are only allowed") {
			txSupportMu.Lock()
			txSupported[client] = false
			txSupportMu.Unlock()

			fallbackSession, sErr := client.StartSession()
			if sErr != nil {
				return txErr
			}
			defer fallbackSession.EndSession(ctx)
			return mongo.WithSession(ctx, fallbackSession, func(sessCtx mongo.SessionContext) error {
				return fn(sessCtx)
			})
		}
		return txErr
	}
	return nil
}

// WithUserTx scopes the operation to the authenticated user.
func WithUserTx(ctx context.Context, client *mongo.Client, db *mongo.Database, userID string, fn func(mongo.SessionContext) error) error {
	if !isUUID(userID) {
		return ErrInvalidUUID
	}
	return WithTx(ctx, client, fn)
}

// WithUserTenantTx verifies active tenant membership before proceeding.
func WithUserTenantTx(ctx context.Context, client *mongo.Client, db *mongo.Database, userID, tenantID string, fn func(mongo.SessionContext) error) error {
	if !isUUID(userID) || !isUUID(tenantID) {
		return ErrInvalidUUID
	}
	if db == nil {
		return ErrNilDB
	}

	return WithTx(ctx, client, func(sessCtx mongo.SessionContext) error {
		memberships := db.Collection("tenant_memberships")
		count, err := memberships.CountDocuments(sessCtx, bson.M{
			"tenant_id": tenantID,
			"user_id":   userID,
			"status":    "active",
		})
		if err != nil {
			return fmt.Errorf("check tenant membership: %w", err)
		}
		if count == 0 {
			return ErrNotMember
		}
		return fn(sessCtx)
	})
}

// WithSystemTenantTx executes trusted background work for a tenant.
func WithSystemTenantTx(ctx context.Context, client *mongo.Client, db *mongo.Database, tenantID string, fn func(mongo.SessionContext) error) error {
	if !isUUID(tenantID) {
		return ErrInvalidUUID
	}
	return WithTx(ctx, client, fn)
}

func isUUID(value string) bool { return uuidPattern.MatchString(value) }

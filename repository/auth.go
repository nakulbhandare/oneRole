package repository

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"github.com/oneRole/db"
	"github.com/oneRole/service"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AuthRepositories provides separate adapters because OAuth and refresh stores
// both expose a Create method with distinct service-owned input types.
type AuthRepositories struct {
	OAuthTransactions *OAuthTransactionRepository
	Identities        *AuthIdentityRepository
	Users             *AuthIdentityRepository
	Sessions          *AuthSessionRepository
	Memberships       *MembershipReader
}

func NewAuthRepositories(database *mongo.Database) AuthRepositories {
	identities := &AuthIdentityRepository{db: database}
	return AuthRepositories{
		OAuthTransactions: &OAuthTransactionRepository{db: database},
		Identities:        identities,
		Users:             identities,
		Sessions:          &AuthSessionRepository{db: database},
		Memberships:       &MembershipReader{db: database},
	}
}

type OAuthTransactionRepository struct{ db *mongo.Database }
type AuthIdentityRepository struct{ db *mongo.Database }
type AuthSessionRepository struct{ db *mongo.Database }
type MembershipReader struct{ db *mongo.Database }

type oauthTxDoc struct {
	Provider                string     `bson:"provider"`
	StateHash               []byte     `bson:"state_hash"`
	PKCEVerifierCiphertext  []byte     `bson:"pkce_verifier_ciphertext"`
	OIDCNonceHash           []byte     `bson:"oidc_nonce_hash,omitempty"`
	BrowserBindingHash      []byte     `bson:"browser_binding_hash"`
	FixedReturnPath         string     `bson:"fixed_return_path"`
	ExpiresAt               time.Time  `bson:"expires_at"`
	CreatedAt               time.Time  `bson:"created_at"`
	ConsumedAt              *time.Time `bson:"consumed_at,omitempty"`
}

func (r *OAuthTransactionRepository) Create(ctx context.Context, transaction service.OAuthTransaction) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	if !validProvider(transaction.Provider) || len(transaction.StateHash) != 32 || len(transaction.BrowserBindingHash) != 32 || len(transaction.PKCEVerifierCiphertext) == 0 || transaction.FixedReturnPath == "" || !transaction.ExpiresAt.After(transaction.CreatedAt) {
		return service.ErrInvalidOAuthTransaction
	}

	doc := oauthTxDoc{
		Provider:               string(transaction.Provider),
		StateHash:              transaction.StateHash,
		PKCEVerifierCiphertext: transaction.PKCEVerifierCiphertext,
		OIDCNonceHash:          nullBytes(transaction.OIDCNonceHash),
		BrowserBindingHash:     transaction.BrowserBindingHash,
		FixedReturnPath:        transaction.FixedReturnPath,
		ExpiresAt:              transaction.ExpiresAt.UTC(),
		CreatedAt:              transaction.CreatedAt.UTC(),
	}

	_, err := r.db.Collection("auth_oauth_transactions").InsertOne(ctx, doc)
	if err != nil {
		return mapUniqueConflict(err, service.ErrInvalidOAuthTransaction)
	}
	return nil
}

func (r *OAuthTransactionRepository) Consume(ctx context.Context, provider service.OAuthProvider, stateHash, browserBindingHash []byte, now time.Time) (service.OAuthTransaction, error) {
	if r == nil || r.db == nil {
		return service.OAuthTransaction{}, db.ErrNilDB
	}
	if !validProvider(provider) || len(stateHash) != 32 || len(browserBindingHash) != 32 {
		return service.OAuthTransaction{}, service.ErrInvalidOAuthTransaction
	}

	nowUTC := now.UTC()
	coll := r.db.Collection("auth_oauth_transactions")

	filter := bson.M{
		"provider":             string(provider),
		"state_hash":           stateHash,
		"browser_binding_hash": browserBindingHash,
		"consumed_at":          nil,
		"expires_at":           bson.M{"$gt": nowUTC},
	}
	update := bson.M{
		"$set": bson.M{"consumed_at": nowUTC},
	}

	var doc oauthTxDoc
	after := options.After
	err := coll.FindOneAndUpdate(ctx, filter, update, &options.FindOneAndUpdateOptions{ReturnDocument: &after}).Decode(&doc)
	if err == nil {
		return service.OAuthTransaction{
			Provider:               service.OAuthProvider(doc.Provider),
			StateHash:              doc.StateHash,
			PKCEVerifierCiphertext: doc.PKCEVerifierCiphertext,
			OIDCNonceHash:          doc.OIDCNonceHash,
			BrowserBindingHash:     doc.BrowserBindingHash,
			FixedReturnPath:        doc.FixedReturnPath,
			CreatedAt:              doc.CreatedAt.UTC(),
			ExpiresAt:              doc.ExpiresAt.UTC(),
		}, nil
	}

	if !errors.Is(err, mongo.ErrNoDocuments) {
		return service.OAuthTransaction{}, fmt.Errorf("consume OAuth transaction: %w", err)
	}

	// Inspect why it failed
	var existing oauthTxDoc
	lookupErr := coll.FindOne(ctx, bson.M{
		"provider":   string(provider),
		"state_hash": stateHash,
	}).Decode(&existing)
	if errors.Is(lookupErr, mongo.ErrNoDocuments) || subtle.ConstantTimeCompare(existing.BrowserBindingHash, browserBindingHash) != 1 {
		return service.OAuthTransaction{}, service.ErrInvalidOAuthTransaction
	}
	if lookupErr != nil {
		return service.OAuthTransaction{}, fmt.Errorf("inspect OAuth transaction: %w", lookupErr)
	}
	if !existing.ExpiresAt.After(now) {
		return service.OAuthTransaction{}, service.ErrOAuthTransactionExpired
	}
	if existing.ConsumedAt != nil {
		return service.OAuthTransaction{}, service.ErrOAuthTransactionConsumed
	}
	return service.OAuthTransaction{}, service.ErrInvalidOAuthTransaction
}

type userDoc struct {
	ID              string     `bson:"_id"`
	Email           string     `bson:"email,omitempty"`
	EmailVerifiedAt *time.Time `bson:"email_verified_at,omitempty"`
	DisplayName     string     `bson:"display_name"`
	Status          string     `bson:"status"`
	CreatedAt       time.Time  `bson:"created_at"`
	UpdatedAt       time.Time  `bson:"updated_at"`
}

type identityDoc struct {
	ID                    string     `bson:"_id"`
	UserID                string     `bson:"user_id"`
	Provider              string     `bson:"provider"`
	ProviderSubject       string     `bson:"provider_subject"`
	ProviderEmail         string     `bson:"provider_email,omitempty"`
	ProviderEmailVerified bool       `bson:"provider_email_verified"`
	LinkedAt              time.Time  `bson:"linked_at"`
	LastLoginAt           time.Time  `bson:"last_login_at"`
}

func (r *AuthIdentityRepository) ResolveOrCreate(ctx context.Context, input service.AuthIdentityInput) (service.AuthUser, error) {
	if r == nil || r.db == nil {
		return service.AuthUser{}, db.ErrNilDB
	}
	if !validProvider(input.Provider) || input.ProviderSubject == "" {
		return service.AuthUser{}, service.ErrIdentityConflict
	}

	identitiesColl := r.db.Collection("auth_identities")
	usersColl := r.db.Collection("users")

	var existingIdentity identityDoc
	err := identitiesColl.FindOne(ctx, bson.M{
		"provider":         string(input.Provider),
		"provider_subject": input.ProviderSubject,
	}).Decode(&existingIdentity)

	if err == nil {
		var user userDoc
		userErr := usersColl.FindOne(ctx, bson.M{"_id": existingIdentity.UserID}).Decode(&user)
		if userErr != nil {
			return service.AuthUser{}, fmt.Errorf("find existing auth user: %w", userErr)
		}
		if user.Status != "active" {
			return service.AuthUser{}, service.ErrAuthUserUnavailable
		}

		_, _ = identitiesColl.UpdateOne(ctx, bson.M{"_id": existingIdentity.ID}, bson.M{
			"$set": bson.M{
				"provider_email":          input.Email,
				"provider_email_verified": input.EmailVerified,
				"last_login_at":           input.Now.UTC(),
			},
		})

		return service.AuthUser{
			ID:            user.ID,
			Email:         user.Email,
			DisplayName:   user.DisplayName,
			EmailVerified: user.EmailVerifiedAt != nil,
			Status:        user.Status,
		}, nil
	}

	if !errors.Is(err, mongo.ErrNoDocuments) {
		return service.AuthUser{}, fmt.Errorf("find auth identity: %w", err)
	}

	newUserID := newUUID()
	nowUTC := input.Now.UTC()
	var verifiedAt *time.Time
	if input.EmailVerified && input.Email != "" {
		verifiedAt = &nowUTC
	}

	userRecord := userDoc{
		ID:              newUserID,
		Email:           input.Email,
		EmailVerifiedAt: verifiedAt,
		DisplayName:     input.DisplayName,
		Status:          "active",
		CreatedAt:       nowUTC,
		UpdatedAt:       nowUTC,
	}

	_, insertUserErr := usersColl.InsertOne(ctx, userRecord)
	if insertUserErr != nil {
		return service.AuthUser{}, mapUniqueConflict(insertUserErr, service.ErrIdentityConflict)
	}

	identityRecord := identityDoc{
		ID:                    newUUID(),
		UserID:                newUserID,
		Provider:              string(input.Provider),
		ProviderSubject:       input.ProviderSubject,
		ProviderEmail:         input.Email,
		ProviderEmailVerified: input.EmailVerified,
		LinkedAt:              nowUTC,
		LastLoginAt:           nowUTC,
	}

	_, insertIdentityErr := identitiesColl.InsertOne(ctx, identityRecord)
	if insertIdentityErr != nil {
		// Clean up unlinked user
		_, _ = usersColl.DeleteOne(ctx, bson.M{"_id": newUserID})
		if mongo.IsDuplicateKeyError(insertIdentityErr) {
			// Identity was inserted concurrently, re-read
			var concIdentity identityDoc
			if readErr := identitiesColl.FindOne(ctx, bson.M{
				"provider":         string(input.Provider),
				"provider_subject": input.ProviderSubject,
			}).Decode(&concIdentity); readErr == nil {
				var u userDoc
				if uErr := usersColl.FindOne(ctx, bson.M{"_id": concIdentity.UserID}).Decode(&u); uErr == nil {
					return service.AuthUser{
						ID:            u.ID,
						Email:         u.Email,
						DisplayName:   u.DisplayName,
						EmailVerified: u.EmailVerifiedAt != nil,
						Status:        u.Status,
					}, nil
				}
			}
		}
		return service.AuthUser{}, mapUniqueConflict(insertIdentityErr, service.ErrIdentityConflict)
	}

	return service.AuthUser{
		ID:            newUserID,
		Email:         input.Email,
		DisplayName:   input.DisplayName,
		EmailVerified: verifiedAt != nil,
		Status:        "active",
	}, nil
}

type sessionDoc struct {
	ID                string     `bson:"_id"`
	UserID            string     `bson:"user_id"`
	FamilyID          string     `bson:"family_id"`
	RefreshTokenHash  []byte     `bson:"refresh_token_hash"`
	CreatedAt         time.Time  `bson:"created_at"`
	LastUsedAt        time.Time  `bson:"last_used_at"`
	IdleExpiresAt     time.Time  `bson:"idle_expires_at"`
	AbsoluteExpiresAt time.Time  `bson:"absolute_expires_at"`
	RevokedAt         *time.Time `bson:"revoked_at,omitempty"`
	RevokeReason      string     `bson:"revoke_reason,omitempty"`
}

type refreshTokenHistoryDoc struct {
	SessionID        string     `bson:"session_id"`
	RefreshTokenHash []byte     `bson:"refresh_token_hash"`
	IssuedAt         time.Time  `bson:"issued_at"`
	ConsumedAt       *time.Time `bson:"consumed_at,omitempty"`
}

func (r *AuthSessionRepository) Create(ctx context.Context, session service.AuthSession) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	if len(session.RefreshTokenHash) != 32 || !session.AbsoluteExpiresAt.After(session.CreatedAt) || !session.IdleExpiresAt.After(session.CreatedAt) {
		return service.ErrAuthSessionUnavailable
	}

	doc := sessionDoc{
		ID:                session.ID,
		UserID:            session.UserID,
		FamilyID:          session.FamilyID,
		RefreshTokenHash:  session.RefreshTokenHash,
		CreatedAt:         session.CreatedAt.UTC(),
		LastUsedAt:        session.LastUsedAt.UTC(),
		IdleExpiresAt:     session.IdleExpiresAt.UTC(),
		AbsoluteExpiresAt: session.AbsoluteExpiresAt.UTC(),
		RevokedAt:         session.RevokedAt,
		RevokeReason:      session.RevokeReason,
	}

	_, err := r.db.Collection("auth_sessions").InsertOne(ctx, doc)
	if err != nil {
		return mapUniqueConflict(err, service.ErrAuthSessionUnavailable)
	}

	history := refreshTokenHistoryDoc{
		SessionID:        session.ID,
		RefreshTokenHash: session.RefreshTokenHash,
		IssuedAt:         session.CreatedAt.UTC(),
	}
	_, err = r.db.Collection("auth_session_refresh_tokens").InsertOne(ctx, history)
	if err != nil {
		return mapUniqueConflict(err, service.ErrAuthSessionUnavailable)
	}

	return nil
}

func (r *AuthSessionRepository) Rotate(ctx context.Context, rotation service.RefreshRotation) (service.AuthSession, error) {
	if r == nil || r.db == nil {
		return service.AuthSession{}, db.ErrNilDB
	}
	if len(rotation.PresentedTokenHash) != 32 || len(rotation.ReplacementTokenHash) != 32 || bytes.Equal(rotation.PresentedTokenHash, rotation.ReplacementTokenHash) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}

	sessionsColl := r.db.Collection("auth_sessions")
	historyColl := r.db.Collection("auth_session_refresh_tokens")

	var session sessionDoc
	err := sessionsColl.FindOne(ctx, bson.M{
		"refresh_token_hash": rotation.PresentedTokenHash,
	}).Decode(&session)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return r.revokeOnRefreshReuse(ctx, rotation)
	}
	if err != nil {
		return service.AuthSession{}, fmt.Errorf("find auth session for rotation: %w", err)
	}

	if session.RevokedAt != nil || !session.IdleExpiresAt.After(rotation.Now) || !session.AbsoluteExpiresAt.After(rotation.Now) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}

	var history refreshTokenHistoryDoc
	histErr := historyColl.FindOne(ctx, bson.M{
		"session_id":         session.ID,
		"refresh_token_hash": rotation.PresentedTokenHash,
	}).Decode(&history)
	if histErr != nil {
		return service.AuthSession{}, fmt.Errorf("read refresh token history: %w", histErr)
	}

	if history.ConsumedAt != nil {
		return service.AuthSession{}, r.revokeFamily(ctx, session.FamilyID, rotation.Now, service.ErrRefreshTokenReuse)
	}

	nowUTC := rotation.Now.UTC()
	_, err = historyColl.UpdateOne(ctx, bson.M{
		"session_id":         session.ID,
		"refresh_token_hash": rotation.PresentedTokenHash,
		"consumed_at":        nil,
	}, bson.M{
		"$set": bson.M{"consumed_at": nowUTC},
	})
	if err != nil {
		return service.AuthSession{}, fmt.Errorf("consume previous refresh token: %w", err)
	}

	newIdleExpiry := rotation.NewIdleExpiresAt.UTC()
	if newIdleExpiry.After(session.AbsoluteExpiresAt) {
		newIdleExpiry = session.AbsoluteExpiresAt
	}
	if !newIdleExpiry.After(rotation.Now) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}

	_, err = sessionsColl.UpdateOne(ctx, bson.M{"_id": session.ID}, bson.M{
		"$set": bson.M{
			"refresh_token_hash": rotation.ReplacementTokenHash,
			"last_used_at":       nowUTC,
			"idle_expires_at":    newIdleExpiry,
		},
	})
	if err != nil {
		return service.AuthSession{}, mapUniqueConflict(err, service.ErrAuthSessionUnavailable)
	}

	newHistory := refreshTokenHistoryDoc{
		SessionID:        session.ID,
		RefreshTokenHash: rotation.ReplacementTokenHash,
		IssuedAt:         nowUTC,
	}
	_, err = historyColl.InsertOne(ctx, newHistory)
	if err != nil {
		return service.AuthSession{}, mapUniqueConflict(err, service.ErrAuthSessionUnavailable)
	}

	return service.AuthSession{
		ID:                session.ID,
		UserID:            session.UserID,
		FamilyID:          session.FamilyID,
		RefreshTokenHash:  append([]byte(nil), rotation.ReplacementTokenHash...),
		CreatedAt:         session.CreatedAt.UTC(),
		LastUsedAt:        nowUTC,
		IdleExpiresAt:     newIdleExpiry,
		AbsoluteExpiresAt: session.AbsoluteExpiresAt.UTC(),
	}, nil
}

func (r *AuthSessionRepository) revokeOnRefreshReuse(ctx context.Context, rotation service.RefreshRotation) (service.AuthSession, error) {
	historyColl := r.db.Collection("auth_session_refresh_tokens")
	sessionsColl := r.db.Collection("auth_sessions")

	var history refreshTokenHistoryDoc
	err := historyColl.FindOne(ctx, bson.M{
		"refresh_token_hash": rotation.PresentedTokenHash,
	}).Decode(&history)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}
	if err != nil {
		return service.AuthSession{}, fmt.Errorf("find refresh token history: %w", err)
	}

	var session sessionDoc
	err = sessionsColl.FindOne(ctx, bson.M{"_id": history.SessionID}).Decode(&session)
	if err != nil {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}

	if history.ConsumedAt == nil {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}

	if revErr := r.revokeFamily(ctx, session.FamilyID, rotation.Now, service.ErrRefreshTokenReuse); revErr != nil {
		return service.AuthSession{}, revErr
	}
	return service.AuthSession{}, service.ErrRefreshTokenReuse
}

func (r *AuthSessionRepository) revokeFamily(ctx context.Context, familyID string, now time.Time, result error) error {
	nowUTC := now.UTC()
	_, err := r.db.Collection("auth_sessions").UpdateMany(ctx, bson.M{
		"family_id":  familyID,
		"revoked_at": nil,
	}, bson.M{
		"$set": bson.M{
			"revoked_at":    nowUTC,
			"revoke_reason": "refresh_token_reuse",
		},
	})
	if err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return result
}

func (r *AuthSessionRepository) FindActive(ctx context.Context, sessionID string, now time.Time) (service.AuthSession, error) {
	if r == nil || r.db == nil {
		return service.AuthSession{}, db.ErrNilDB
	}
	nowUTC := now.UTC()
	var session sessionDoc
	err := r.db.Collection("auth_sessions").FindOne(ctx, bson.M{
		"_id":                 sessionID,
		"revoked_at":          nil,
		"idle_expires_at":     bson.M{"$gt": nowUTC},
		"absolute_expires_at": bson.M{"$gt": nowUTC},
	}).Decode(&session)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}
	if err != nil {
		return service.AuthSession{}, fmt.Errorf("find active auth session: %w", err)
	}

	return service.AuthSession{
		ID:                session.ID,
		UserID:            session.UserID,
		FamilyID:          session.FamilyID,
		RefreshTokenHash:  session.RefreshTokenHash,
		CreatedAt:         session.CreatedAt.UTC(),
		LastUsedAt:        session.LastUsedAt.UTC(),
		IdleExpiresAt:     session.IdleExpiresAt.UTC(),
		AbsoluteExpiresAt: session.AbsoluteExpiresAt.UTC(),
		RevokedAt:         session.RevokedAt,
		RevokeReason:      session.RevokeReason,
	}, nil
}

func (r *AuthSessionRepository) Revoke(ctx context.Context, sessionID string, now time.Time, reason string) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	nowUTC := now.UTC()
	res, err := r.db.Collection("auth_sessions").UpdateOne(ctx, bson.M{
		"_id":        sessionID,
		"revoked_at": nil,
	}, bson.M{
		"$set": bson.M{
			"revoked_at":    nowUTC,
			"revoke_reason": reason,
		},
	})
	if err != nil {
		return fmt.Errorf("revoke auth session: %w", err)
	}
	if res.MatchedCount == 0 {
		return service.ErrAuthSessionUnavailable
	}
	return nil
}

type membershipDoc struct {
	TenantID  string    `bson:"tenant_id"`
	UserID    string    `bson:"user_id"`
	Role      string    `bson:"role"`
	Status    string    `bson:"status"`
	CreatedAt time.Time `bson:"created_at"`
}

func (r *MembershipReader) ListActive(ctx context.Context, userID string) ([]service.TenantMembership, error) {
	if r == nil || r.db == nil {
		return nil, db.ErrNilDB
	}
	findOpts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "tenant_id", Value: 1}})
	cursor, err := r.db.Collection("tenant_memberships").Find(ctx, bson.M{
		"user_id": userID,
		"status":  "active",
	}, findOpts)
	if err != nil {
		return nil, fmt.Errorf("list active tenant memberships: %w", err)
	}
	defer cursor.Close(ctx)

	var memberships []service.TenantMembership
	for cursor.Next(ctx) {
		var doc membershipDoc
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		memberships = append(memberships, service.TenantMembership{
			TenantID: doc.TenantID,
			UserID:   doc.UserID,
			Role:     doc.Role,
			Status:   doc.Status,
		})
	}
	return memberships, cursor.Err()
}

func (r *MembershipReader) EnsureMembership(ctx context.Context, userID, tenantID, role string) error {
	if r == nil || r.db == nil {
		return db.ErrNilDB
	}
	now := time.Now().UTC()
	// Ensure tenant exists
	tenantFilter := bson.M{"_id": tenantID}
	tenantUpdate := bson.M{
		"$setOnInsert": bson.M{
			"_id":        tenantID,
			"name":       "OneRole Workspace",
			"status":     "active",
			"created_at": now,
		},
		"$set": bson.M{
			"updated_at": now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, _ = r.db.Collection("tenants").UpdateOne(ctx, tenantFilter, tenantUpdate, opts)

	filter := bson.M{
		"tenant_id": tenantID,
		"user_id":   userID,
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			"_id":        newUUID(),
			"tenant_id":  tenantID,
			"user_id":    userID,
			"role":       role,
			"created_at": now,
		},
		"$set": bson.M{
			"updated_at": now,
			"status":     "active",
		},
	}
	_, err := r.db.Collection("tenant_memberships").UpdateOne(ctx, filter, update, opts)
	return err
}

func (r *MembershipReader) FindActive(ctx context.Context, userID, tenantID string) (service.TenantMembership, error) {
	if r == nil || r.db == nil {
		return service.TenantMembership{}, db.ErrNilDB
	}
	var doc membershipDoc
	err := r.db.Collection("tenant_memberships").FindOne(ctx, bson.M{
		"tenant_id": tenantID,
		"user_id":   userID,
		"status":    "active",
	}).Decode(&doc)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return service.TenantMembership{}, service.ErrTenantMembershipNotFound
	}
	if err != nil {
		return service.TenantMembership{}, err
	}
	return service.TenantMembership{
		TenantID: doc.TenantID,
		UserID:   doc.UserID,
		Role:     doc.Role,
		Status:   doc.Status,
	}, nil
}

func (r *AuthIdentityRepository) FindByID(ctx context.Context, userID string) (service.AuthUser, error) {
	if r == nil || r.db == nil {
		return service.AuthUser{}, db.ErrNilDB
	}
	var user userDoc
	err := r.db.Collection("users").FindOne(ctx, bson.M{"_id": userID}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) || (err == nil && user.Status != "active") {
		return service.AuthUser{}, service.ErrAuthUserUnavailable
	}
	if err != nil {
		return service.AuthUser{}, fmt.Errorf("find auth user: %w", err)
	}
	return service.AuthUser{
		ID:            user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		EmailVerified: user.EmailVerifiedAt != nil,
		Status:        user.Status,
	}, nil
}

func validProvider(provider service.OAuthProvider) bool {
	return provider == service.OAuthProvider("google") || provider == service.OAuthProvider("github")
}

func nullBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	return value
}

func mapUniqueConflict(err error, mapped error) error {
	if err == nil {
		return nil
	}
	if mongo.IsDuplicateKeyError(err) {
		return mapped
	}
	return err
}

var _ interface {
	Create(context.Context, service.OAuthTransaction) error
	Consume(context.Context, service.OAuthProvider, []byte, []byte, time.Time) (service.OAuthTransaction, error)
} = (*OAuthTransactionRepository)(nil)

var _ interface {
	ResolveOrCreate(context.Context, service.AuthIdentityInput) (service.AuthUser, error)
	FindByID(context.Context, string) (service.AuthUser, error)
} = (*AuthIdentityRepository)(nil)

var _ interface {
	Create(context.Context, service.AuthSession) error
	Rotate(context.Context, service.RefreshRotation) (service.AuthSession, error)
	FindActive(context.Context, string, time.Time) (service.AuthSession, error)
	Revoke(context.Context, string, time.Time, string) error
} = (*AuthSessionRepository)(nil)

var _ interface {
	ListActive(context.Context, string) ([]service.TenantMembership, error)
	FindActive(context.Context, string, string) (service.TenantMembership, error)
} = (*MembershipReader)(nil)

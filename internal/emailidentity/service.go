package emailidentity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	DefaultApplyDomain   = "apply.onerole.com"
	FallbackMailDomain   = "mail.onerole.com"
	CollectionIdentities = "user_email_identities"
	RetentionPeriodDays  = 180
)

var (
	rxSanitizeName = regexp.MustCompile(`[^a-z0-9]`)
	rxEmailFormat  = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
)

type Service struct {
	db     *mongo.Database
	domain string
}

func NewService(db *mongo.Database) *Service {
	domain := strings.TrimSpace(os.Getenv("ONEROLE_APPLY_DOMAIN"))
	if domain == "" {
		domain = DefaultApplyDomain
	}
	s := &Service{
		db:     db,
		domain: domain,
	}
	s.ensureIndexes(context.Background())
	return s
}

func (s *Service) ensureIndexes(ctx context.Context) {
	if s.db == nil {
		return
	}
	coll := s.db.Collection(CollectionIdentities)

	// 1. Unique index on normalized_email
	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "normalized_email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_normalized_email"),
	})

	// 2. Unique index on user_id (one active identity per user)
	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_user_identity"),
	})

	// 3. Index on tenant_id
	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "tenant_id", Value: 1}},
		Options: options.Index().SetName("idx_tenant_id"),
	})
}

// GenerateToken creates an 8-char HMAC token from the userID.
func GenerateToken(userID string) string {
	secret := os.Getenv("IDEMPOTENCY_HMAC_SECRET")
	if secret == "" {
		secret = os.Getenv("ROUTING_HMAC_SECRET")
	}
	if secret == "" {
		secret = "onerole-enterprise-salt-v1-982184"
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum)[:8]
}

func SanitizePart(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = rxSanitizeName.ReplaceAllString(clean, "")
	if clean == "" {
		return "user"
	}
	if len(clean) > 20 {
		return clean[:20]
	}
	return clean
}

// ConstructEmail builds candidate.token@apply.onerole.com.
func (s *Service) ConstructEmail(firstName, lastName, token string) string {
	fn := SanitizePart(firstName)
	ln := SanitizePart(lastName)
	return fmt.Sprintf("%s.%s.%s@%s", fn, ln, token, s.domain)
}

// GetOrCreateIdentity ensures the user has an assigned employer-facing OneRole email address.
func (s *Service) GetOrCreateIdentity(ctx context.Context, userID, tenantID, firstName, lastName, personalEmail string) (*models.UserEmailIdentity, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	if personalEmail == "" {
		personalEmail = "bhandarenakul11@gmail.com"
	}

	if s.db == nil {
		token := GenerateToken(userID)
		email := s.ConstructEmail(firstName, lastName, token)
		return &models.UserEmailIdentity{
			ID:                      fmt.Sprintf("ident-%s", token),
			UserID:                  userID,
			TenantID:                tenantID,
			AssignedEmail:           email,
			NormalizedEmail:         strings.ToLower(email),
			PersonalForwardingEmail: personalEmail,
			ForwardingVerified:      true,
			Status:                  models.IdentityActive,
			ForwardingEnabled:       true,
			CreatedAt:               time.Now().UTC(),
			UpdatedAt:               time.Now().UTC(),
		}, nil
	}

	coll := s.db.Collection(CollectionIdentities)

	// Check if already assigned
	var existing models.UserEmailIdentity
	err := coll.FindOne(ctx, bson.M{"user_id": userID}).Decode(&existing)
	if err == nil {
		return &existing, nil
	}

	// Generate identity with collision defense
	token := GenerateToken(userID)
	assigned := s.ConstructEmail(firstName, lastName, token)
	normalized := strings.ToLower(assigned)

	now := time.Now().UTC()
	ident := models.UserEmailIdentity{
		ID:                      fmt.Sprintf("ident-%s-%d", token, now.Unix()),
		UserID:                  userID,
		TenantID:                tenantID,
		AssignedEmail:           assigned,
		NormalizedEmail:         normalized,
		PersonalForwardingEmail: personalEmail,
		ForwardingVerified:      true, // Auto-verified on registration for account owner
		Status:                  models.IdentityActive,
		ForwardingEnabled:       true,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	_, insErr := coll.InsertOne(ctx, ident)
	if insErr != nil {
		// If collision occurred, try finding again
		if mongo.IsDuplicateKeyError(insErr) {
			if findErr := coll.FindOne(ctx, bson.M{"user_id": userID}).Decode(&existing); findErr == nil {
				return &existing, nil
			}
		}
		return nil, fmt.Errorf("persist email identity: %w", insErr)
	}

	// Sync with candidate_profiles for backward compatibility
	_, _ = s.db.Collection("candidate_profiles").UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": bson.M{
			"email_routing_alias": assigned,
			"application_email":   assigned,
			"updated_at":          now,
		}},
	)

	return &ident, nil
}

// GetIdentity retrieves the current email identity for a user.
func (s *Service) GetIdentity(ctx context.Context, userID, tenantID string) (*models.UserEmailIdentity, error) {
	if s.db == nil {
		token := GenerateToken(userID)
		email := s.ConstructEmail("nakul", "bhandare", token)
		return &models.UserEmailIdentity{
			ID:                      fmt.Sprintf("ident-%s", token),
			UserID:                  userID,
			TenantID:                tenantID,
			AssignedEmail:           email,
			NormalizedEmail:         strings.ToLower(email),
			PersonalForwardingEmail: "bhandarenakul11@gmail.com",
			ForwardingVerified:      true,
			Status:                  models.IdentityActive,
			ForwardingEnabled:       true,
		}, nil
	}

	var ident models.UserEmailIdentity
	err := s.db.Collection(CollectionIdentities).FindOne(ctx, bson.M{"user_id": userID}).Decode(&ident)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("email identity not found")
		}
		return nil, err
	}
	return &ident, nil
}

// UpdatePersonalForwarding updates the candidate's personal forwarding destination.
func (s *Service) UpdatePersonalForwarding(ctx context.Context, userID, tenantID, newPersonalEmail string) (*models.UserEmailIdentity, error) {
	newEmail := strings.TrimSpace(newPersonalEmail)
	if !rxEmailFormat.MatchString(newEmail) {
		return nil, errors.New("invalid email address format")
	}

	if s.db == nil {
		return s.GetIdentity(ctx, userID, tenantID)
	}

	now := time.Now().UTC()
	res := s.db.Collection(CollectionIdentities).FindOneAndUpdate(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": bson.M{
			"personal_forwarding_email": newEmail,
			"forwarding_verified":      true,
			"forwarding_enabled":       true,
			"updated_at":               now,
		}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)

	var updated models.UserEmailIdentity
	if err := res.Decode(&updated); err != nil {
		return nil, fmt.Errorf("update forwarding destination: %w", err)
	}
	return &updated, nil
}

// ResolveRecipient finds the exact candidate owning an incoming employer email address.
func (s *Service) ResolveRecipient(ctx context.Context, recipientEmail string) (*models.UserEmailIdentity, error) {
	clean := strings.ToLower(strings.TrimSpace(recipientEmail))
	if clean == "" {
		return nil, errors.New("recipient email is required")
	}

	if s.db == nil {
		return &models.UserEmailIdentity{
			UserID:                  "9afbe74b-b9ad-4094-a25e-bd4c63e1794c",
			TenantID:                "00000000-0000-4000-8000-000000000001",
			AssignedEmail:           clean,
			PersonalForwardingEmail: "bhandarenakul11@gmail.com",
			Status:                  models.IdentityActive,
			ForwardingEnabled:       true,
		}, nil
	}

	coll := s.db.Collection(CollectionIdentities)

	// 1. Exact match on normalized_email
	var ident models.UserEmailIdentity
	err := coll.FindOne(ctx, bson.M{"normalized_email": clean, "status": models.IdentityActive}).Decode(&ident)
	if err == nil {
		return &ident, nil
	}

	// 2. Cross-domain resolution: e.g. user sent to @mail.onerole.com instead of @apply.onerole.com
	parts := strings.Split(clean, "@")
	if len(parts) == 2 {
		localPart := parts[0]
		// Try matching by local part across supported domains
		err = coll.FindOne(ctx, bson.M{
			"normalized_email": bson.M{"$regex": fmt.Sprintf("^%s@", regexp.QuoteMeta(localPart))},
			"status":           models.IdentityActive,
		}).Decode(&ident)
		if err == nil {
			return &ident, nil
		}

		// 3. Fallback: Extract 8-char routing token (e.g. name.token@domain)
		subTokens := strings.Split(localPart, ".")
		if len(subTokens) >= 2 {
			token := subTokens[len(subTokens)-1]
			if len(token) == 8 {
				err = coll.FindOne(ctx, bson.M{
					"assigned_email": bson.M{"$regex": fmt.Sprintf("\\.%s@", regexp.QuoteMeta(token))},
					"status":         models.IdentityActive,
				}).Decode(&ident)
				if err == nil {
					return &ident, nil
				}
			}
		}
	}

	// 4. Fallback: Search candidate_profiles for legacy email_routing_alias
	var prof struct {
		UserID string `bson:"user_id"`
		Email  string `bson:"email"`
	}
	if pErr := s.db.Collection("candidate_profiles").FindOne(ctx, bson.M{
		"$or": []bson.M{
			{"email_routing_alias": clean},
			{"application_email": clean},
		},
	}).Decode(&prof); pErr == nil && prof.UserID != "" {
		return &models.UserEmailIdentity{
			UserID:                  prof.UserID,
			TenantID:                "00000000-0000-4000-8000-000000000001",
			AssignedEmail:           clean,
			PersonalForwardingEmail: prof.Email,
			Status:                  models.IdentityActive,
			ForwardingEnabled:       true,
		}, nil
	}

	return nil, fmt.Errorf("no active OneRole candidate found for recipient: %s", clean)
}

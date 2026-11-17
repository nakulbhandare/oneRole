package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"
)

type oauthTransactionMemory struct {
	mu       sync.Mutex
	byState  map[string]OAuthTransaction
	consumed map[string]bool
}

func newOAuthTransactionMemory() *oauthTransactionMemory {
	return &oauthTransactionMemory{byState: make(map[string]OAuthTransaction), consumed: make(map[string]bool)}
}
func (s *oauthTransactionMemory) Create(_ context.Context, tx OAuthTransaction) error {
	if err := tx.ValidateForCreate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(tx.StateHash)
	if _, exists := s.byState[key]; exists {
		return ErrInvalidOAuthTransaction
	}
	s.byState[key] = tx
	return nil
}
func (s *oauthTransactionMemory) Consume(_ context.Context, provider OAuthProvider, stateHash, bindingHash []byte, now time.Time) (OAuthTransaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(stateHash)
	tx, exists := s.byState[key]
	if !exists || tx.Provider != provider || !hmacEqual(tx.BrowserBindingHash, bindingHash) {
		return OAuthTransaction{}, ErrInvalidOAuthTransaction
	}
	if s.consumed[key] {
		return OAuthTransaction{}, ErrOAuthTransactionConsumed
	}
	if !tx.ExpiresAt.After(now) {
		return OAuthTransaction{}, ErrOAuthTransactionExpired
	}
	s.consumed[key] = true
	return tx, nil
}

func (s *oauthTransactionMemory) getByStateHash(hash []byte) OAuthTransaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byState[string(hash)]
}

func hmacEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var difference byte
	for i := range left {
		difference |= left[i] ^ right[i]
	}
	return difference == 0
}

type identityMemory struct {
	mu              sync.Mutex
	bySubject       map[string]AuthUser
	byID            map[string]AuthUser
	byVerifiedEmail map[string]string
}

func newIdentityMemory() *identityMemory {
	return &identityMemory{bySubject: make(map[string]AuthUser), byID: make(map[string]AuthUser), byVerifiedEmail: make(map[string]string)}
}
func (s *identityMemory) ResolveOrCreate(_ context.Context, input AuthIdentityInput) (AuthUser, error) {
	if err := input.Validate(); err != nil {
		return AuthUser{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(input.Provider) + ":" + input.ProviderSubject
	if existing, ok := s.bySubject[key]; ok {
		return existing, nil
	}
	if input.EmailVerified {
		if _, exists := s.byVerifiedEmail[input.Email]; exists {
			return AuthUser{}, ErrIdentityConflict
		}
	}
	user := AuthUser{ID: "33333333-3333-4333-8333-333333333333", Email: input.Email, DisplayName: input.DisplayName, EmailVerified: input.EmailVerified, Status: "active"}
	s.bySubject[key] = user
	s.byID[user.ID] = user
	if input.EmailVerified {
		s.byVerifiedEmail[input.Email] = user.ID
	}
	return user, nil
}
func (s *identityMemory) FindByID(_ context.Context, id string) (AuthUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return AuthUser{}, ErrAuthUserUnavailable
	}
	return user, nil
}

// This memory adapter models historical refresh hash retention and family
// revocation. Production behavior is covered by the database adapter.
type sessionMemory struct {
	mu      sync.Mutex
	byID    map[string]AuthSession
	current map[string]string
	retired map[string]string
}

func newSessionMemory() *sessionMemory {
	return &sessionMemory{byID: make(map[string]AuthSession), current: make(map[string]string), retired: make(map[string]string)}
}
func (s *sessionMemory) Create(_ context.Context, session AuthSession) error {
	if err := session.ValidateForCreate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[session.ID] = session
	s.current[string(session.RefreshTokenHash)] = session.ID
	return nil
}
func (s *sessionMemory) Rotate(_ context.Context, rotation RefreshRotation) (AuthSession, error) {
	if err := rotation.Validate(); err != nil {
		return AuthSession{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	oldHash := string(rotation.PresentedTokenHash)
	id, ok := s.current[oldHash]
	if !ok {
		if sessionID, wasUsed := s.retired[oldHash]; wasUsed {
			family := s.byID[sessionID].FamilyID
			for otherID, session := range s.byID {
				if session.FamilyID == family {
					now := rotation.Now
					session.RevokedAt = &now
					session.RevokeReason = "refresh_reuse"
					s.byID[otherID] = session
				}
			}
			return AuthSession{}, ErrRefreshTokenReuse
		}
		return AuthSession{}, ErrAuthSessionUnavailable
	}
	session := s.byID[id]
	if session.RevokedAt != nil || !session.IdleExpiresAt.After(rotation.Now) || !session.AbsoluteExpiresAt.After(rotation.Now) {
		return AuthSession{}, ErrAuthSessionUnavailable
	}
	delete(s.current, oldHash)
	s.retired[oldHash] = id
	session.RefreshTokenHash = append([]byte(nil), rotation.ReplacementTokenHash...)
	session.LastUsedAt = rotation.Now
	session.IdleExpiresAt = rotation.NewIdleExpiresAt
	if session.IdleExpiresAt.After(session.AbsoluteExpiresAt) {
		session.IdleExpiresAt = session.AbsoluteExpiresAt
	}
	s.byID[id] = session
	s.current[string(session.RefreshTokenHash)] = id
	return session, nil
}
func (s *sessionMemory) FindActive(_ context.Context, id string, now time.Time) (AuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok || session.RevokedAt != nil || !session.IdleExpiresAt.After(now) || !session.AbsoluteExpiresAt.After(now) {
		return AuthSession{}, ErrAuthSessionUnavailable
	}
	return session, nil
}
func (s *sessionMemory) Revoke(_ context.Context, id string, now time.Time, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.byID[id]
	if !ok {
		return ErrAuthSessionUnavailable
	}
	session.RevokedAt = &now
	session.RevokeReason = reason
	s.byID[id] = session
	delete(s.current, string(session.RefreshTokenHash))
	return nil
}

type membershipMemory struct{ memberships []TenantMembership }

func (s membershipMemory) ListActive(_ context.Context, userID string) ([]TenantMembership, error) {
	result := make([]TenantMembership, 0)
	for _, membership := range s.memberships {
		if membership.UserID == userID && membership.Status == "active" {
			result = append(result, membership)
		}
	}
	return result, nil
}
func (s membershipMemory) FindActive(_ context.Context, userID, tenantID string) (TenantMembership, error) {
	for _, membership := range s.memberships {
		if membership.UserID == userID && membership.TenantID == tenantID && membership.Status == "active" {
			return membership, nil
		}
	}
	return TenantMembership{}, ErrTenantMembershipNotFound
}

type oauthClientFake struct {
	identity OAuthProviderIdentity
	verifier string
	nonce    string
}

func (c *oauthClientFake) AuthorizationURL(state, verifier, nonce string) (string, error) {
	c.verifier, c.nonce = verifier, nonce
	challengeHash := sha256.Sum256([]byte(verifier))
	return "https://accounts.google.com/o/oauth2/v2/auth?" + url.Values{
		"state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challengeHash[:])}, "code_challenge_method": {"S256"},
	}.Encode(), nil
}
func (c *oauthClientFake) Authenticate(_ context.Context, code, verifier string, expectedNonceHash []byte) (OAuthProviderIdentity, error) {
	if code != "authorization-code" || verifier != c.verifier {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	nonceHash := sha256.Sum256([]byte(c.nonce))
	if !hmacEqual(nonceHash[:], expectedNonceHash) {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	return c.identity, nil
}

func TestAuthServiceOAuthCreatesHashedSessionAndRotatesRefresh(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	transactions := newOAuthTransactionMemory()
	identities := newIdentityMemory()
	sessions := newSessionMemory()
	provider := &oauthClientFake{identity: OAuthProviderIdentity{
		ProviderSubject: "google-subject-1", Email: "person@example.com", EmailVerified: true, DisplayName: "One Person",
	}}
	auth, err := NewAuthService(authTestConfig(privateKey), transactions, identities, identities, sessions, membershipMemory{}, map[OAuthProvider]OAuthClient{
		OAuthProviderGoogle: provider,
	})
	if err != nil {
		t.Fatal(err)
	}

	statuses := auth.Providers()
	if len(statuses) != 2 || !statuses[0].Enabled || statuses[1].Enabled {
		t.Fatalf("missing credentials must leave GitHub disabled: %#v", statuses)
	}
	if _, err := auth.StartOAuth(ctx, OAuthProviderGitHub, now); !errors.Is(err, ErrOAuthProviderUnavailable) {
		t.Fatalf("disabled provider should not start login: %v", err)
	}

	started, err := auth.StartOAuth(ctx, OAuthProviderGoogle, now)
	if err != nil {
		t.Fatalf("start OAuth: %v", err)
	}
	parsed, err := url.Parse(started.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")
	stateBytes, err := decodeOpaqueSecret(state)
	if err != nil {
		t.Fatal(err)
	}
	stateHash := sha256.Sum256(stateBytes)
	tx := transactions.getByStateHash(stateHash[:])
	if hmacEqual(tx.StateHash, stateBytes) {
		t.Fatal("raw state must never be persisted")
	}
	if len(tx.PKCEVerifierCiphertext) == 0 || string(tx.PKCEVerifierCiphertext) == provider.verifier {
		t.Fatal("PKCE verifier must be encrypted at rest")
	}
	bindingBytes, err := decodeOpaqueSecret(started.BrowserBindingSecret)
	if err != nil {
		t.Fatal(err)
	}
	bindingHash := sha256.Sum256(bindingBytes)
	if !hmacEqual(tx.BrowserBindingHash, bindingHash[:]) {
		t.Fatal("only the browser binding hash should be persisted")
	}

	completed, err := auth.CompleteOAuth(ctx, OAuthProviderGoogle, state, started.BrowserBindingSecret, "authorization-code", "", now.Add(time.Second))
	if err != nil {
		t.Fatalf("complete OAuth: %v", err)
	}
	if completed.RedirectURL != authTestConfig(privateKey).FrontendRedirectURL || completed.RefreshToken == "" || completed.CSRFToken == "" {
		t.Fatalf("incomplete login result: %#v", completed)
	}
	refreshBytes, err := decodeOpaqueSecret(completed.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	refreshHash := sha256.Sum256(refreshBytes)
	if len(sessions.byID) != 1 {
		t.Fatalf("expected one persisted session, got %d", len(sessions.byID))
	}
	var session AuthSession
	for _, session = range sessions.byID {
	}
	if !hmacEqual(session.RefreshTokenHash, refreshHash[:]) {
		t.Fatal("session store must contain only the refresh hash")
	}
	if !auth.ValidCSRF("https://app.onerole.test", completed.CSRFToken, completed.CSRFToken) ||
		auth.ValidCSRF("https://attacker.test", completed.CSRFToken, completed.CSRFToken) ||
		auth.ValidCSRF("https://app.onerole.test", completed.CSRFToken, "attacker") {
		t.Fatal("CSRF validation did not enforce signed double-submit and exact origin")
	}

	refreshed, err := auth.Refresh(ctx, completed.RefreshToken, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" || refreshed.RefreshToken == completed.RefreshToken {
		t.Fatalf("refresh must rotate the opaque token and mint an access JWT: %#v", refreshed)
	}
	if _, err := auth.Refresh(ctx, completed.RefreshToken, now.Add(3*time.Second)); !errors.Is(err, ErrRefreshTokenReuse) {
		t.Fatalf("replayed refresh token must be detected: %v", err)
	}
}

func TestAuthServiceConsumesProviderDenialAndDoesNotCreateSession(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	transactions := newOAuthTransactionMemory()
	sessions := newSessionMemory()
	provider := &oauthClientFake{}
	identities := newIdentityMemory()
	auth, err := NewAuthService(authTestConfig(privateKey), transactions, identities, identities, sessions, membershipMemory{}, map[OAuthProvider]OAuthClient{
		OAuthProviderGoogle: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := auth.StartOAuth(ctx, OAuthProviderGoogle, now)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(started.AuthorizationURL)
	state := parsed.Query().Get("state")
	stateBytes, _ := decodeOpaqueSecret(state)
	stateHash := sha256.Sum256(stateBytes)
	if _, err := auth.CompleteOAuth(ctx, OAuthProviderGoogle, state, started.BrowserBindingSecret, "", "access_denied", now.Add(time.Second)); !errors.Is(err, ErrOAuthAuthenticationFailed) {
		t.Fatalf("provider denial should fail login after consuming state: %v", err)
	}
	if _, err := transactions.Consume(ctx, OAuthProviderGoogle, stateHash[:], func() []byte {
		binding, _ := decodeOpaqueSecret(started.BrowserBindingSecret)
		hash := sha256.Sum256(binding)
		return hash[:]
	}(), now.Add(2*time.Second)); !errors.Is(err, ErrOAuthTransactionConsumed) {
		t.Fatalf("denied flow state must be consumed: %v", err)
	}
	if len(sessions.byID) != 0 {
		t.Fatal("provider denial must not create a OneRole session")
	}
}

func TestAuthIdentityConflictDoesNotAutoLinkByEmail(t *testing.T) {
	store := newIdentityMemory()
	first, err := store.ResolveOrCreate(context.Background(), AuthIdentityInput{
		Provider: OAuthProviderGoogle, ProviderSubject: "google-subject", Email: "same@example.com", EmailVerified: true,
		Now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ResolveOrCreate(context.Background(), AuthIdentityInput{
		Provider: OAuthProviderGitHub, ProviderSubject: "github-subject", Email: "same@example.com", EmailVerified: true,
		Now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, ErrIdentityConflict) || second.ID != "" || first.ID == second.ID {
		t.Fatal("same email on a different provider must require explicit linking")
	}
}

func TestAuthServiceDevLogin(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 7, 30, 0, 0, time.UTC)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	sessions := newSessionMemory()
	transactions := newOAuthTransactionMemory()
	identities := newIdentityMemory()
	auth, err := NewAuthService(authTestConfig(privateKey), transactions, identities, identities, sessions, membershipMemory{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := auth.DevLogin(ctx, "onerole@gmail.com", "OneRole Candidate", now)
	if err != nil {
		t.Fatalf("DevLogin failed: %v", err)
	}
	if result.AccessToken == "" {
		t.Fatal("missing access token")
	}
	if result.TokenType != "Bearer" {
		t.Fatalf("expected Bearer token type, got %s", result.TokenType)
	}
	if result.Email != "onerole@gmail.com" {
		t.Fatalf("expected onerole@gmail.com, got %s", result.Email)
	}
	if result.TenantID != DefaultDevTenantID {
		t.Fatalf("expected default dev tenant ID, got %s", result.TenantID)
	}
	if result.RefreshToken == "" || result.CSRFToken == "" {
		t.Fatal("missing refresh or CSRF token")
	}
}

func authTestConfig(privateKey *rsa.PrivateKey) AuthServiceConfig {
	return AuthServiceConfig{
		PKCEEncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		CSRFSigningKey:    []byte("csrf-signing-secret-0123456789abcdef"),
		JWTIssuer:         "https://auth.onerole.test", JWTAudience: "onerole-api", JWTSigningKeyID: "key-1", JWTSigningPrivateKey: privateKey,
		AccessTokenTTL: 10 * time.Minute, OAuthTransactionTTL: 5 * time.Minute,
		RefreshIdleTTL: 24 * time.Hour, RefreshAbsoluteTTL: 30 * 24 * time.Hour,
		FrontendRedirectURL: "https://app.onerole.test/auth/complete", AllowedOrigins: []string{"https://app.onerole.test"},
	}
}

package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

// OAuthProvider identifies an external identity provider. Provider credentials
// and provider access tokens are deliberately kept outside OneRole sessions.
type OAuthProvider string

const (
	OAuthProviderGoogle OAuthProvider = "google"
	OAuthProviderGitHub OAuthProvider = "github"
)

// Auth errors are safe to classify at the HTTP boundary. Repository adapters
// should wrap infrastructure errors with context without replacing these
// sentinel values or returning database details to clients.
var (
	ErrInvalidOAuthTransaction   = errors.New("invalid OAuth transaction")
	ErrOAuthTransactionExpired   = errors.New("OAuth transaction expired")
	ErrOAuthTransactionConsumed  = errors.New("OAuth transaction already consumed")
	ErrOAuthProviderUnavailable  = errors.New("OAuth provider unavailable")
	ErrOAuthAuthenticationFailed = errors.New("OAuth authentication failed")
	ErrIdentityConflict          = errors.New("identity conflict requires explicit account linking")
	ErrAuthUserUnavailable       = errors.New("authentication user unavailable")
	ErrAuthSessionUnavailable    = errors.New("authentication session unavailable")
	ErrAuthDependencyUnavailable = errors.New("authentication dependency unavailable")
	ErrCSRFValidationFailed      = errors.New("CSRF validation failed")
	ErrRefreshTokenReuse         = errors.New("refresh token reuse detected")
	ErrTenantMembershipNotFound  = errors.New("active tenant membership not found")
)

// OAuthTransaction stores only hashes of browser-facing random values. The
// PKCE verifier is encrypted because it must be recovered to redeem the code.
// FixedReturnPath must be selected from the server's configured allowlist.
type OAuthTransaction struct {
	Provider               OAuthProvider
	StateHash              []byte
	PKCEVerifierCiphertext []byte
	OIDCNonceHash          []byte
	BrowserBindingHash     []byte
	FixedReturnPath        string
	CreatedAt              time.Time
	ExpiresAt              time.Time
}

// ValidateForCreate checks invariants common to OAuth transaction stores.
func (tx OAuthTransaction) ValidateForCreate() error {
	if !validOAuthProvider(tx.Provider) || len(tx.StateHash) != 32 ||
		len(tx.PKCEVerifierCiphertext) == 0 || len(tx.BrowserBindingHash) != 32 ||
		strings.TrimSpace(tx.FixedReturnPath) == "" ||
		!isUTC(tx.CreatedAt) || !isUTC(tx.ExpiresAt) || !tx.ExpiresAt.After(tx.CreatedAt) {
		return ErrInvalidOAuthTransaction
	}
	if tx.Provider == OAuthProviderGoogle && len(tx.OIDCNonceHash) != 32 {
		return ErrInvalidOAuthTransaction
	}
	if tx.Provider == OAuthProviderGitHub && len(tx.OIDCNonceHash) != 0 {
		return ErrInvalidOAuthTransaction
	}
	return nil
}

// OAuthTransactionStore persists and atomically consumes OAuth handshakes.
// Consume must match provider, state hash, and browser-binding hash, require
// ExpiresAt > now, and make the transaction unusable in the same transaction.
// It returns ErrOAuthTransactionExpired for an expired matching transaction,
// ErrOAuthTransactionConsumed for replay, and ErrInvalidOAuthTransaction for
// missing or mismatched input. State and binding arguments are SHA-256 hashes.
type OAuthTransactionStore interface {
	Create(ctx context.Context, tx OAuthTransaction) error
	Consume(ctx context.Context, provider OAuthProvider, stateHash, browserBindingHash []byte, now time.Time) (OAuthTransaction, error)
}

// AuthIdentityInput contains trusted claims returned by a provider after full
// provider validation. The store must identify accounts only by provider and
// immutable subject; it must never link accounts by email. Now must be UTC.
type AuthIdentityInput struct {
	Provider        OAuthProvider
	ProviderSubject string
	Email           string
	EmailVerified   bool
	DisplayName     string
	Now             time.Time
}

// Validate checks the input before it reaches an identity repository.
func (in AuthIdentityInput) Validate() error {
	if !validOAuthProvider(in.Provider) || strings.TrimSpace(in.ProviderSubject) == "" ||
		!isUTC(in.Now) || (in.EmailVerified && strings.TrimSpace(in.Email) == "") {
		return ErrIdentityConflict
	}
	return nil
}

// AuthUser is the minimum OneRole user view required for authentication.
type AuthUser struct {
	ID            string
	Email         string
	DisplayName   string
	EmailVerified bool
	Status        string
}

// AuthIdentityStore resolves a login by the unique pair (provider, subject),
// or creates a new OneRole user and identity atomically. A verified-email
// uniqueness collision must return ErrIdentityConflict and require explicit
// account linking; it must not attach the identity to the existing user.
type AuthIdentityStore interface {
	ResolveOrCreate(ctx context.Context, input AuthIdentityInput) (AuthUser, error)
}

// AuthSession stores the current refresh-token hash for a OneRole session.
// RefreshTokenHash is SHA-256(refresh secret), never the raw cookie value.
type AuthSession struct {
	ID                string
	UserID            string
	FamilyID          string
	RefreshTokenHash  []byte
	CreatedAt         time.Time
	LastUsedAt        time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
	RevokeReason      string
}

// ValidateForCreate verifies that a newly issued session has a valid token hash
// and bounded UTC expirations.
func (s AuthSession) ValidateForCreate() error {
	if !isUUID(s.ID) || !isUUID(s.UserID) || !isUUID(s.FamilyID) || len(s.RefreshTokenHash) != 32 ||
		!isUTC(s.CreatedAt) || !isUTC(s.LastUsedAt) || !isUTC(s.IdleExpiresAt) ||
		!isUTC(s.AbsoluteExpiresAt) || s.IdleExpiresAt.After(s.AbsoluteExpiresAt) ||
		!s.IdleExpiresAt.After(s.CreatedAt) || !s.AbsoluteExpiresAt.After(s.CreatedAt) {
		return ErrAuthSessionUnavailable
	}
	if s.RevokedAt != nil && !isUTC(*s.RevokedAt) {
		return ErrAuthSessionUnavailable
	}
	return nil
}

// RefreshRotation describes an atomic compare-and-swap of a presented refresh
// hash to a new hash. All timestamps are UTC. NewIdleExpiresAt is the requested
// idle deadline; the adapter clamps it to the session's immutable absolute
// expiry. The adapter must preserve enough token-hash history to detect replay;
// on a previously rotated hash it revokes the whole family and returns
// ErrRefreshTokenReuse.
type RefreshRotation struct {
	PresentedTokenHash   []byte
	ReplacementTokenHash []byte
	Now                  time.Time
	NewIdleExpiresAt     time.Time
}

// Validate checks cryptographic and timestamp invariants before rotation.
func (r RefreshRotation) Validate() error {
	if len(r.PresentedTokenHash) != 32 || len(r.ReplacementTokenHash) != 32 ||
		string(r.PresentedTokenHash) == string(r.ReplacementTokenHash) ||
		!isUTC(r.Now) || !isUTC(r.NewIdleExpiresAt) || !r.NewIdleExpiresAt.After(r.Now) {
		return ErrAuthSessionUnavailable
	}
	return nil
}

// AuthSessionStore owns session persistence. Rotate must atomically find and
// replace a current token whose idle and absolute expirations are both in the
// future, and must handle an old-token replay by revoking the token family.
// FindActive checks the access token's session ID against current storage.
// Revoke must be idempotent.
type AuthSessionStore interface {
	Create(ctx context.Context, session AuthSession) error
	Rotate(ctx context.Context, rotation RefreshRotation) (AuthSession, error)
	FindActive(ctx context.Context, sessionID string, now time.Time) (AuthSession, error)
	Revoke(ctx context.Context, sessionID string, now time.Time, reason string) error
}

// TenantMembership contains current authorization data. Roles are never
// trusted from access-token claims.
type TenantMembership struct {
	TenantID string
	UserID   string
	Role     string
	Status   string
}

// MembershipReader returns live membership state for every protected tenant
// operation and for the current-user response.
type MembershipReader interface {
	ListActive(ctx context.Context, userID string) ([]TenantMembership, error)
	FindActive(ctx context.Context, userID, tenantID string) (TenantMembership, error)
}

func validOAuthProvider(provider OAuthProvider) bool {
	return provider == OAuthProviderGoogle || provider == OAuthProviderGitHub
}

func isUTC(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC
}

func validTenantRole(role string) bool {
	switch role {
	case "owner", "admin", "member":
		return true
	default:
		return false
	}
}

// IsValidTenantRole reports whether a persisted membership role is part of the
// supported authorization model.
func IsValidTenantRole(role string) bool { return validTenantRole(role) }

// AuthUserReader reads the current OneRole user record for /auth/me and for
// account-state checks after provider sign-in or token refresh.
type AuthUserReader interface {
	FindByID(ctx context.Context, userID string) (AuthUser, error)
}

// OAuthProviderIdentity is a verified external identity. The ProviderSubject
// is the sole identity key; email is profile metadata and never links accounts.
type OAuthProviderIdentity struct {
	ProviderSubject string
	Email           string
	EmailVerified   bool
	DisplayName     string
}

// OAuthClient performs provider-specific authorization and identity validation.
// Google implementations must verify issuer, audience, signature, expiry, and
// nonce. GitHub implementations use the authenticated user ID as subject.
type OAuthClient interface {
	AuthorizationURL(state, verifier, nonce string) (string, error)
	Authenticate(ctx context.Context, code, verifier string, expectedNonceHash []byte) (OAuthProviderIdentity, error)
}

// OAuthClientSettings contains a provider's confidential OAuth client settings.
type OAuthClientSettings struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// AuthServiceConfig contains secrets and fixed public URL policy. Provider
// clients absent from the constructor remain disabled and are omitted from
// authorization attempts.
type AuthServiceConfig struct {
	PKCEEncryptionKey    []byte
	CSRFSigningKey       []byte
	JWTIssuer            string
	JWTAudience          string
	JWTSigningKeyID      string
	JWTSigningPrivateKey *rsa.PrivateKey
	AccessTokenTTL       time.Duration
	OAuthTransactionTTL  time.Duration
	RefreshIdleTTL       time.Duration
	RefreshAbsoluteTTL   time.Duration
	FrontendRedirectURL  string
	AllowedOrigins       []string
}

// OAuthProviderStatus backs GET /api/v1/auth/providers.
type OAuthProviderStatus struct {
	ID      OAuthProvider `json:"id"`
	Label   string        `json:"label"`
	Enabled bool          `json:"enabled"`
}

// OAuthStartResult contains the provider redirect and a one-time browser
// binding secret. The HTTP layer must set the binding secret as Secure,
// HttpOnly, SameSite=Lax cookie before redirecting.
type OAuthStartResult struct {
	AuthorizationURL     string
	BrowserBindingSecret string
	ExpiresAt            time.Time
}

// OAuthCallbackResult is returned only after OneRole identity and session state
// are persisted. The HTTP layer sets the refresh and CSRF cookies, then redirects
// to RedirectURL. No provider token or access JWT is returned from the callback.
type OAuthCallbackResult struct {
	RefreshToken     string
	RefreshExpiresAt time.Time
	CSRFToken        string
	RedirectURL      string
	AccessToken      string
	UserID           string
	Email            string
	TenantID         string
}

// RefreshResult contains the new OneRole access token and rotating refresh
// credential. The HTTP layer keeps the access token in the response body and the
// refresh token in an HttpOnly cookie.
type RefreshResult struct {
	AccessToken      string
	ExpiresIn        int
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// DevLoginResult provides credentials for local development and testing.
type DevLoginResult struct {
	AccessToken      string             `json:"access_token"`
	TokenType        string             `json:"token_type"`
	ExpiresIn        int                `json:"expires_in"`
	UserID           string             `json:"user_id"`
	Email            string             `json:"email"`
	DisplayName      string             `json:"display_name"`
	TenantID         string             `json:"tenant_id"`
	RefreshToken     string             `json:"refresh_token"`
	RefreshExpiresAt time.Time          `json:"refresh_expires_at"`
	CSRFToken        string             `json:"csrf_token"`
	Memberships      []TenantMembership `json:"memberships"`
}

// CurrentUser is the /auth/me response assembled from current database state.
type CurrentUser struct {
	UserID      string             `json:"user_id"`
	DisplayName string             `json:"display_name"`
	Email       *string            `json:"email"`
	Memberships []TenantMembership `json:"memberships"`
}

// AuthService owns OAuth state transitions and OneRole session issuance. It
// never uses provider tokens as API tokens and never links by matching email.
type AuthService struct {
	config         AuthServiceConfig
	transactions   OAuthTransactionStore
	identities     AuthIdentityStore
	users          AuthUserReader
	sessions       AuthSessionStore
	memberships    MembershipReader
	providers      map[OAuthProvider]OAuthClient
	allowedOrigins map[string]struct{}
	aead           cipher.AEAD
	now            func() time.Time
}

// NewAuthService validates the signing/encryption configuration once at
// startup. OAuth providers with missing credentials should be omitted from the
// providers map so that login remains disabled instead of faking success.
func NewAuthService(
	config AuthServiceConfig,
	transactions OAuthTransactionStore,
	identities AuthIdentityStore,
	users AuthUserReader,
	sessions AuthSessionStore,
	memberships MembershipReader,
	providers map[OAuthProvider]OAuthClient,
) (*AuthService, error) {
	if len(config.PKCEEncryptionKey) != 32 || len(config.CSRFSigningKey) < 32 ||
		strings.TrimSpace(config.JWTIssuer) == "" || strings.TrimSpace(config.JWTAudience) == "" ||
		strings.TrimSpace(config.JWTSigningKeyID) == "" || config.JWTSigningPrivateKey == nil ||
		config.JWTSigningPrivateKey.N == nil || config.JWTSigningPrivateKey.N.BitLen() < 2048 ||
		config.JWTSigningPrivateKey.E < 3 || transactions == nil || identities == nil || users == nil ||
		sessions == nil || memberships == nil || len(providers) > 2 {
		return nil, errors.New("invalid authentication service configuration")
	}
	if config.AccessTokenTTL <= 0 || config.AccessTokenTTL > 15*time.Minute ||
		config.OAuthTransactionTTL <= 0 || config.OAuthTransactionTTL > 15*time.Minute ||
		config.RefreshIdleTTL <= 0 || config.RefreshIdleTTL > 30*24*time.Hour ||
		config.RefreshAbsoluteTTL < config.RefreshIdleTTL || config.RefreshAbsoluteTTL > 90*24*time.Hour {
		return nil, errors.New("invalid authentication token lifetimes")
	}
	frontend, err := validateFixedRedirect(config.FrontendRedirectURL)
	if err != nil {
		return nil, errors.New("invalid fixed frontend callback URL")
	}
	config.FrontendRedirectURL = frontend.String()
	origins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, rawOrigin := range config.AllowedOrigins {
		origin, err := normalizeOrigin(rawOrigin)
		if err != nil {
			return nil, errors.New("invalid allowed browser origin")
		}
		origins[origin] = struct{}{}
	}
	if len(origins) == 0 {
		return nil, errors.New("at least one allowed browser origin is required")
	}
	providerCopy := make(map[OAuthProvider]OAuthClient, len(providers))
	for provider, client := range providers {
		if !validOAuthProvider(provider) || client == nil {
			return nil, errors.New("invalid OAuth provider client")
		}
		providerCopy[provider] = client
	}
	block, err := aes.NewCipher(config.PKCEEncryptionKey)
	if err != nil {
		return nil, errors.New("invalid PKCE encryption key")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("unable to initialize PKCE encryption")
	}
	config.PKCEEncryptionKey = append([]byte(nil), config.PKCEEncryptionKey...)
	config.CSRFSigningKey = append([]byte(nil), config.CSRFSigningKey...)
	return &AuthService{
		config: config, transactions: transactions, identities: identities, users: users,
		sessions: sessions, memberships: memberships, providers: providerCopy,
		allowedOrigins: origins, aead: aead, now: time.Now,
	}, nil
}

// Providers returns stable provider order and reports absent credentials as
// disabled. No provider is enabled with placeholder credentials.
func (s *AuthService) Providers() []OAuthProviderStatus {
	return []OAuthProviderStatus{
		{ID: OAuthProviderGoogle, Label: "Continue with Google", Enabled: s.providers[OAuthProviderGoogle] != nil},
		{ID: OAuthProviderGitHub, Label: "Continue with GitHub", Enabled: s.providers[OAuthProviderGitHub] != nil},
	}
}

// StartOAuth creates fresh state, PKCE, nonce, and browser-binding values and
// stores only hashes or encrypted verifier material before returning a redirect.
func (s *AuthService) StartOAuth(ctx context.Context, provider OAuthProvider, now time.Time) (OAuthStartResult, error) {
	client, ok := s.providers[provider]
	if !validOAuthProvider(provider) || !ok {
		return OAuthStartResult{}, ErrOAuthProviderUnavailable
	}
	now = now.UTC()
	if now.IsZero() {
		return OAuthStartResult{}, ErrInvalidOAuthTransaction
	}
	state, stateBytes, err := newOpaqueSecret()
	if err != nil {
		return OAuthStartResult{}, ErrAuthDependencyUnavailable
	}
	binding, bindingBytes, err := newOpaqueSecret()
	if err != nil {
		return OAuthStartResult{}, ErrAuthDependencyUnavailable
	}
	verifier := oauth2.GenerateVerifier()
	nonce := ""
	var nonceHash []byte
	if provider == OAuthProviderGoogle {
		nonce, _, err = newOpaqueSecret()
		if err != nil {
			return OAuthStartResult{}, ErrAuthDependencyUnavailable
		}
		hash := sha256.Sum256([]byte(nonce))
		nonceHash = hash[:]
	}
	stateHash := sha256.Sum256(stateBytes)
	bindingHash := sha256.Sum256(bindingBytes)
	ciphertext, err := s.encryptPKCE([]byte(verifier), stateHash[:])
	if err != nil {
		return OAuthStartResult{}, ErrAuthDependencyUnavailable
	}
	redirectURL, err := client.AuthorizationURL(state, verifier, nonce)
	if err != nil {
		return OAuthStartResult{}, ErrOAuthProviderUnavailable
	}
	parsed, err := url.Parse(redirectURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return OAuthStartResult{}, ErrOAuthProviderUnavailable
	}
	expiresAt := now.Add(s.config.OAuthTransactionTTL).UTC()
	fixedPath := mustURLPath(s.config.FrontendRedirectURL)
	tx := OAuthTransaction{
		Provider: provider, StateHash: stateHash[:], PKCEVerifierCiphertext: ciphertext,
		OIDCNonceHash: nonceHash, BrowserBindingHash: bindingHash[:],
		FixedReturnPath: fixedPath, CreatedAt: now, ExpiresAt: expiresAt,
	}
	if err := tx.ValidateForCreate(); err != nil {
		return OAuthStartResult{}, err
	}
	if err := s.transactions.Create(ctx, tx); err != nil {
		return OAuthStartResult{}, err
	}
	return OAuthStartResult{AuthorizationURL: parsed.String(), BrowserBindingSecret: binding, ExpiresAt: expiresAt}, nil
}

// CompleteOAuth consumes state exactly once, validates the provider identity,
// resolves a OneRole user by provider subject, and creates a rotating session.
// Provider errors are consumed too when they contain valid state and binding.
func (s *AuthService) CompleteOAuth(
	ctx context.Context,
	provider OAuthProvider,
	state, browserBinding, code, providerError string,
	now time.Time,
) (OAuthCallbackResult, error) {
	client, ok := s.providers[provider]
	if !validOAuthProvider(provider) || !ok {
		return OAuthCallbackResult{}, ErrOAuthProviderUnavailable
	}
	stateBytes, err := decodeOpaqueSecret(state)
	if err != nil {
		return OAuthCallbackResult{}, ErrInvalidOAuthTransaction
	}
	bindingBytes, err := decodeOpaqueSecret(browserBinding)
	if err != nil {
		return OAuthCallbackResult{}, ErrInvalidOAuthTransaction
	}
	stateHash := sha256.Sum256(stateBytes)
	bindingHash := sha256.Sum256(bindingBytes)
	now = now.UTC()
	tx, err := s.transactions.Consume(ctx, provider, stateHash[:], bindingHash[:], now)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	if tx.Provider != provider || tx.FixedReturnPath != mustURLPath(s.config.FrontendRedirectURL) ||
		!tx.ExpiresAt.After(now) || !hmac.Equal(tx.StateHash, stateHash[:]) || !hmac.Equal(tx.BrowserBindingHash, bindingHash[:]) {
		return OAuthCallbackResult{}, ErrInvalidOAuthTransaction
	}
	if strings.TrimSpace(providerError) != "" || strings.TrimSpace(code) == "" {
		return OAuthCallbackResult{}, ErrOAuthAuthenticationFailed
	}
	verifierBytes, err := s.decryptPKCE(tx.PKCEVerifierCiphertext, stateHash[:])
	if err != nil {
		return OAuthCallbackResult{}, ErrInvalidOAuthTransaction
	}
	identity, err := client.Authenticate(ctx, code, string(verifierBytes), tx.OIDCNonceHash)
	if err != nil {
		return OAuthCallbackResult{}, ErrOAuthAuthenticationFailed
	}
	if strings.TrimSpace(identity.ProviderSubject) == "" ||
		(provider == OAuthProviderGoogle && (!identity.EmailVerified || strings.TrimSpace(identity.Email) == "")) {
		return OAuthCallbackResult{}, ErrOAuthAuthenticationFailed
	}
	user, err := s.identities.ResolveOrCreate(ctx, AuthIdentityInput{
		Provider: provider, ProviderSubject: identity.ProviderSubject,
		Email: identity.Email, EmailVerified: identity.EmailVerified,
		DisplayName: identity.DisplayName, Now: now,
	})
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	if !isUUID(user.ID) || user.Status != "active" {
		return OAuthCallbackResult{}, ErrAuthUserUnavailable
	}
	if ensurer, ok := s.memberships.(membershipEnsurer); ok {
		_ = ensurer.EnsureMembership(ctx, user.ID, DefaultDevTenantID, "admin")
	}
	session, result, err := s.createSession(ctx, user, now)
	if err != nil {
		return OAuthCallbackResult{}, err
	}
	accessToken, _, _ := s.mintAccessToken(session, now)
	result.AccessToken = accessToken
	result.UserID = user.ID
	result.Email = user.Email
	result.TenantID = DefaultDevTenantID
	return result, nil
}

// Refresh rotates the opaque refresh cookie hash atomically and issues a new
// short-lived OneRole JWT. The old cookie is invalid after the transaction.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string, now time.Time) (RefreshResult, error) {
	presented, err := decodeOpaqueSecret(refreshToken)
	if err != nil {
		return RefreshResult{}, ErrAuthSessionUnavailable
	}
	presentedHash := sha256.Sum256(presented)
	replacementToken, replacementBytes, err := newOpaqueSecret()
	if err != nil {
		return RefreshResult{}, ErrAuthDependencyUnavailable
	}
	replacementHash := sha256.Sum256(replacementBytes)
	now = now.UTC()
	rotation := RefreshRotation{
		PresentedTokenHash: presentedHash[:], ReplacementTokenHash: replacementHash[:],
		Now: now, NewIdleExpiresAt: now.Add(s.config.RefreshIdleTTL).UTC(),
	}
	if err := rotation.Validate(); err != nil {
		return RefreshResult{}, err
	}
	session, err := s.sessions.Rotate(ctx, rotation)
	if err != nil {
		return RefreshResult{}, err
	}
	if session.ID == "" || session.UserID == "" || session.RevokedAt != nil || !session.IdleExpiresAt.After(now) || !session.AbsoluteExpiresAt.After(now) {
		return RefreshResult{}, ErrAuthSessionUnavailable
	}
	user, err := s.users.FindByID(ctx, session.UserID)
	if err != nil {
		return RefreshResult{}, err
	}
	if user.ID != session.UserID || user.Status != "active" {
		_ = s.sessions.Revoke(ctx, session.ID, now, "user_unavailable")
		return RefreshResult{}, ErrAuthUserUnavailable
	}
	accessToken, expiresIn, err := s.mintAccessToken(session, now)
	if err != nil {
		_ = s.sessions.Revoke(ctx, session.ID, now, "token_issuance_failed")
		return RefreshResult{}, ErrAuthDependencyUnavailable
	}
	return RefreshResult{
		AccessToken: accessToken, ExpiresIn: expiresIn, RefreshToken: replacementToken,
		RefreshExpiresAt: session.IdleExpiresAt,
	}, nil
}

// Logout revokes a session identified by a validated access token.
func (s *AuthService) Logout(ctx context.Context, sessionID string, now time.Time) error {
	if !isUUID(sessionID) {
		return ErrAuthSessionUnavailable
	}
	return s.sessions.Revoke(ctx, sessionID, now.UTC(), "user_logout")
}

// CurrentUser loads current account and active membership state. No tenant role
// or email value is trusted from access-token claims.
func (s *AuthService) CurrentUser(ctx context.Context, userID string) (CurrentUser, error) {
	if !isUUID(userID) {
		return CurrentUser{}, ErrAuthUserUnavailable
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return CurrentUser{}, err
	}
	if user.ID != userID || user.Status != "active" {
		return CurrentUser{}, ErrAuthUserUnavailable
	}
	memberships, err := s.memberships.ListActive(ctx, userID)
	if err != nil {
		return CurrentUser{}, err
	}
	active := make([]TenantMembership, 0, len(memberships))
	for _, membership := range memberships {
		if membership.UserID != userID || !isUUID(membership.TenantID) || membership.Status != "active" || !validTenantRole(membership.Role) {
			return CurrentUser{}, ErrAuthDependencyUnavailable
		}
		active = append(active, membership)
	}
	var email *string
	if user.Email != "" {
		value := user.Email
		email = &value
	}
	return CurrentUser{UserID: user.ID, DisplayName: user.DisplayName, Email: email, Memberships: active}, nil
}

// ValidCSRF enforces exact configured Origin, signed double-submit cookie, and
// constant-time header/cookie equality for refresh and logout.
func (s *AuthService) ValidCSRF(origin, cookieToken, headerToken string) bool {
	if _, ok := s.allowedOrigins[origin]; !ok || cookieToken == "" || headerToken == "" || len(cookieToken) > 256 || len(headerToken) > 256 {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) != 1 {
		return false
	}
	parts := strings.Split(cookieToken, ".")
	if len(parts) != 3 || parts[0] != "v1" || len(parts[1]) < 32 {
		return false
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(secret) != 32 {
		return false
	}
	providedMAC, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(providedMAC) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, s.config.CSRFSigningKey)
	_, _ = mac.Write([]byte("onerole-csrf-v1:" + parts[1]))
	return hmac.Equal(providedMAC, mac.Sum(nil))
}

type membershipEnsurer interface {
	EnsureMembership(ctx context.Context, userID, tenantID, role string) error
}

const DefaultDevTenantID = "00000000-0000-4000-8000-000000000001"

func (s *AuthService) DevLogin(ctx context.Context, email, name string, now time.Time) (DevLoginResult, error) {
	if strings.TrimSpace(email) == "" {
		email = "onerole@gmail.com"
	}
	if strings.TrimSpace(name) == "" {
		name = "OneRole Candidate"
	}
	now = now.UTC()
	if now.IsZero() {
		now = s.now().UTC()
	}

	user, err := s.identities.ResolveOrCreate(ctx, AuthIdentityInput{
		Provider:        OAuthProviderGoogle,
		ProviderSubject: "dev:" + strings.ToLower(strings.TrimSpace(email)),
		Email:           strings.TrimSpace(email),
		DisplayName:     strings.TrimSpace(name),
		EmailVerified:   true,
		Now:             now,
	})
	if err != nil {
		return DevLoginResult{}, fmt.Errorf("resolve dev user: %w", err)
	}
	if !isUUID(user.ID) || user.Status != "active" {
		return DevLoginResult{}, ErrAuthUserUnavailable
	}

	tenantID := DefaultDevTenantID
	if ensurer, ok := s.memberships.(membershipEnsurer); ok {
		if err := ensurer.EnsureMembership(ctx, user.ID, tenantID, "admin"); err != nil {
			return DevLoginResult{}, fmt.Errorf("ensure dev tenant membership: %w", err)
		}
	}

	session, callbackResult, err := s.createSession(ctx, user, now)
	if err != nil {
		return DevLoginResult{}, fmt.Errorf("create dev session: %w", err)
	}

	accessToken, expiresIn, err := s.mintAccessToken(session, now)
	if err != nil {
		return DevLoginResult{}, fmt.Errorf("mint dev access token: %w", err)
	}

	memberships, _ := s.memberships.ListActive(ctx, user.ID)

	return DevLoginResult{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresIn:        expiresIn,
		UserID:           user.ID,
		Email:            user.Email,
		DisplayName:      user.DisplayName,
		TenantID:         tenantID,
		RefreshToken:     callbackResult.RefreshToken,
		RefreshExpiresAt: callbackResult.RefreshExpiresAt,
		CSRFToken:        callbackResult.CSRFToken,
		Memberships:      memberships,
	}, nil
}

func (s *AuthService) createSession(ctx context.Context, user AuthUser, now time.Time) (AuthSession, OAuthCallbackResult, error) {
	sessionID, err := newUUID()
	if err != nil {
		return AuthSession{}, OAuthCallbackResult{}, ErrAuthDependencyUnavailable
	}
	familyID, err := newUUID()
	if err != nil {
		return AuthSession{}, OAuthCallbackResult{}, ErrAuthDependencyUnavailable
	}
	refreshToken, refreshBytes, err := newOpaqueSecret()
	if err != nil {
		return AuthSession{}, OAuthCallbackResult{}, ErrAuthDependencyUnavailable
	}
	refreshHash := sha256.Sum256(refreshBytes)
	csrfToken, err := s.newCSRFToken()
	if err != nil {
		return AuthSession{}, OAuthCallbackResult{}, ErrAuthDependencyUnavailable
	}
	absoluteExpiresAt := now.Add(s.config.RefreshAbsoluteTTL).UTC()
	idleExpiresAt := now.Add(s.config.RefreshIdleTTL).UTC()
	if idleExpiresAt.After(absoluteExpiresAt) {
		idleExpiresAt = absoluteExpiresAt
	}
	session := AuthSession{
		ID: sessionID, UserID: user.ID, FamilyID: familyID, RefreshTokenHash: refreshHash[:],
		CreatedAt: now, LastUsedAt: now, IdleExpiresAt: idleExpiresAt, AbsoluteExpiresAt: absoluteExpiresAt,
	}
	if err := session.ValidateForCreate(); err != nil {
		return AuthSession{}, OAuthCallbackResult{}, err
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return AuthSession{}, OAuthCallbackResult{}, err
	}
	return session, OAuthCallbackResult{
		RefreshToken: refreshToken, RefreshExpiresAt: idleExpiresAt,
		CSRFToken: csrfToken, RedirectURL: s.config.FrontendRedirectURL,
	}, nil
}

func (s *AuthService) mintAccessToken(session AuthSession, now time.Time) (string, int, error) {
	expiresAt := now.Add(s.config.AccessTokenTTL).UTC()
	claims := jwt.RegisteredClaims{
		Issuer: s.config.JWTIssuer, Subject: session.UserID,
		Audience: jwt.ClaimStrings{s.config.JWTAudience},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	jwtClaims := struct {
		SessionID string `json:"sid"`
		TokenUse  string `json:"token_use"`
		jwt.RegisteredClaims
	}{SessionID: session.ID, TokenUse: "access", RegisteredClaims: claims}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims)
	token.Header["kid"] = s.config.JWTSigningKeyID
	token.Header["typ"] = "JWT"
	encoded, err := token.SignedString(s.config.JWTSigningPrivateKey)
	if err != nil {
		return "", 0, err
	}
	return encoded, int(math.Ceil(s.config.AccessTokenTTL.Seconds())), nil
}

func (s *AuthService) encryptPKCE(plaintext, associatedData []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := s.aead.Seal(nil, nonce, plaintext, associatedData)
	return append(nonce, ciphertext...), nil
}

func (s *AuthService) decryptPKCE(ciphertext, associatedData []byte) ([]byte, error) {
	if len(ciphertext) <= s.aead.NonceSize() {
		return nil, errors.New("invalid encrypted verifier")
	}
	nonce := ciphertext[:s.aead.NonceSize()]
	return s.aead.Open(nil, nonce, ciphertext[s.aead.NonceSize():], associatedData)
}

func (s *AuthService) newCSRFToken() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	encodedSecret := base64.RawURLEncoding.EncodeToString(secret)
	mac := hmac.New(sha256.New, s.config.CSRFSigningKey)
	_, _ = mac.Write([]byte("onerole-csrf-v1:" + encodedSecret))
	return "v1." + encodedSecret + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *AuthService) allowedProvider(provider OAuthProvider) bool {
	return validOAuthProvider(provider) && s.providers[provider] != nil
}

// GoogleOAuthClient validates Google's OIDC ID token through go-oidc's
// discovery/JWKS verifier and checks the transaction nonce hash explicitly.
type GoogleOAuthClient struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
}

// NewGoogleOAuthClient performs OIDC discovery once at startup. Call it only
// when Google client credentials are configured; otherwise leave Google absent.
func NewGoogleOAuthClient(ctx context.Context, settings OAuthClientSettings, client *http.Client) (*GoogleOAuthClient, error) {
	if err := validateOAuthClientSettings(settings); err != nil {
		return nil, err
	}
	client = safeProviderHTTPClient(client)
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("discover Google OIDC provider: %w", err)
	}
	return &GoogleOAuthClient{
		config: oauth2.Config{
			ClientID: settings.ClientID, ClientSecret: settings.ClientSecret,
			RedirectURL: settings.RedirectURL, Endpoint: provider.Endpoint(),
			Scopes: []string{oidc.ScopeOpenID, "email", "profile"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: settings.ClientID}), client: client,
	}, nil
}

func (c *GoogleOAuthClient) AuthorizationURL(state, verifier, nonce string) (string, error) {
	if state == "" || verifier == "" || nonce == "" {
		return "", ErrInvalidOAuthTransaction
	}
	return c.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce)), nil
}

func (c *GoogleOAuthClient) Authenticate(ctx context.Context, code, verifier string, expectedNonceHash []byte) (OAuthProviderIdentity, error) {
	if code == "" || verifier == "" || len(expectedNonceHash) != sha256.Size {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	ctx = oidc.ClientContext(ctx, c.client)
	token, err := c.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Nonce         string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil || strings.TrimSpace(claims.Subject) == "" ||
		!claims.EmailVerified || strings.TrimSpace(claims.Email) == "" || claims.Nonce == "" {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	nonceHash := sha256.Sum256([]byte(claims.Nonce))
	if !hmac.Equal(nonceHash[:], expectedNonceHash) {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	return OAuthProviderIdentity{
		ProviderSubject: claims.Subject, Email: strings.TrimSpace(claims.Email),
		EmailVerified: true, DisplayName: strings.TrimSpace(claims.Name),
	}, nil
}

// GitHubOAuthClient requests only read:user and uses the stable numeric user ID
// as provider subject. It never requests repository access or private email.
type GitHubOAuthClient struct {
	config oauth2.Config
	client *http.Client
}

// NewGitHubOAuthClient constructs a minimal GitHub OAuth client when credentials
// are present. A public profile email is treated as unverified metadata.
func NewGitHubOAuthClient(settings OAuthClientSettings, client *http.Client) (*GitHubOAuthClient, error) {
	if err := validateOAuthClientSettings(settings); err != nil {
		return nil, err
	}
	client = safeProviderHTTPClient(client)
	return &GitHubOAuthClient{
		config: oauth2.Config{
			ClientID: settings.ClientID, ClientSecret: settings.ClientSecret,
			RedirectURL: settings.RedirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:   "https://github.com/login/oauth/authorize",
				TokenURL:  "https://github.com/login/oauth/access_token",
				AuthStyle: oauth2.AuthStyleInParams,
			},
			Scopes: []string{"read:user"},
		}, client: client,
	}, nil
}

func (c *GitHubOAuthClient) AuthorizationURL(state, verifier, _ string) (string, error) {
	if state == "" || verifier == "" {
		return "", ErrInvalidOAuthTransaction
	}
	return c.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), nil
}

func (c *GitHubOAuthClient) Authenticate(ctx context.Context, code, verifier string, expectedNonceHash []byte) (OAuthProviderIdentity, error) {
	if code == "" || verifier == "" || len(expectedNonceHash) != 0 {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.client)
	token, err := c.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	client := c.config.Client(ctx, token)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, response.Body, 1<<20))
	decoder.UseNumber()
	var profile struct {
		ID    json.Number `json:"id"`
		Login string      `json:"login"`
		Name  string      `json:"name"`
		Email string      `json:"email"`
	}
	if err := decoder.Decode(&profile); err != nil || strings.TrimSpace(profile.ID.String()) == "" || profile.ID.String() == "0" {
		return OAuthProviderIdentity{}, ErrOAuthAuthenticationFailed
	}
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.TrimSpace(profile.Login)
	}
	return OAuthProviderIdentity{
		ProviderSubject: profile.ID.String(), Email: strings.TrimSpace(profile.Email),
		EmailVerified: false, DisplayName: name,
	}, nil
}

func validateOAuthClientSettings(settings OAuthClientSettings) error {
	if strings.TrimSpace(settings.ClientID) == "" || strings.TrimSpace(settings.ClientSecret) == "" {
		return ErrOAuthProviderUnavailable
	}
	redirect, err := url.Parse(settings.RedirectURL)
	if err != nil || !redirect.IsAbs() || redirect.Host == "" || redirect.User != nil || redirect.Fragment != "" || redirect.RawQuery != "" {
		return ErrOAuthProviderUnavailable
	}
	if redirect.Scheme != "https" && !(redirect.Scheme == "http" && isLoopbackHost(redirect.Hostname())) {
		return ErrOAuthProviderUnavailable
	}
	return nil
}

func safeProviderHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	} else {
		copy := *client
		client = &copy
		if client.Timeout <= 0 {
			client.Timeout = 10 * time.Second
		}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

func validateFixedRedirect(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid fixed redirect")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return nil, errors.New("fixed redirect must use HTTPS or loopback HTTP")
	}
	if parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/") || strings.Contains(parsed.Path, "\\") || strings.HasPrefix(parsed.Path, "//") {
		return nil, errors.New("invalid redirect path")
	}
	return parsed, nil
}

func normalizeOrigin(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid origin")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", errors.New("origin must use HTTPS or loopback HTTP")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func isLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1" || strings.HasPrefix(host, "[::1]")
}

func newOpaqueSecret() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", nil, err
	}
	return base64.RawURLEncoding.EncodeToString(value), value, nil
}

func decodeOpaqueSecret(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, ErrInvalidOAuthTransaction
	}
	return decoded, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}

func mustURLPath(raw string) string {
	parsed, _ := url.Parse(raw)
	return parsed.EscapedPath()
}

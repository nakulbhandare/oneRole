package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oneRole/service"
)

type sessionStoreStub struct {
	session service.AuthSession
	err     error
}

func (s sessionStoreStub) Create(context.Context, service.AuthSession) error { return nil }
func (s sessionStoreStub) Rotate(context.Context, service.RefreshRotation) (service.AuthSession, error) {
	return service.AuthSession{}, nil
}
func (s sessionStoreStub) FindActive(_ context.Context, id string, _ time.Time) (service.AuthSession, error) {
	if s.err != nil {
		return service.AuthSession{}, s.err
	}
	if s.session.ID != id {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}
	return s.session, nil
}
func (s sessionStoreStub) Revoke(context.Context, string, time.Time, string) error { return nil }

func TestJWTVerifierAcceptsAccessTokenAndAttachesPrincipal(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := sessionStoreStub{session: service.AuthSession{
		ID: "44444444-4444-4444-8444-444444444444", UserID: "33333333-3333-4333-8333-333333333333", IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(24 * time.Hour),
	}}
	verifier, err := NewJWTVerifier("https://auth.onerole.test", "onerole-api", map[string]*rsa.PublicKey{"key-1": &privateKey.PublicKey}, store)
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }

	token := signedAccessToken(t, privateKey, "key-1", now, AccessClaims{
		SessionID: "44444444-4444-4444-8444-444444444444", TokenUse: accessTokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "https://auth.onerole.test", Subject: "33333333-3333-4333-8333-333333333333", Audience: jwt.ClaimStrings{"onerole-api"},
			IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	})

	var received Principal
	handler := verifier.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected valid access token, status=%d body=%s", response.Code, response.Body.String())
	}
	if received != (Principal{UserID: "33333333-3333-4333-8333-333333333333", SessionID: "44444444-4444-4444-8444-444444444444"}) {
		t.Fatalf("unexpected principal: %#v", received)
	}
}

func TestJWTVerifierRejectsWrongPurposeAndUnknownKey(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := sessionStoreStub{session: service.AuthSession{
		ID: "44444444-4444-4444-8444-444444444444", UserID: "33333333-3333-4333-8333-333333333333", IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(24 * time.Hour),
	}}
	verifier, err := NewJWTVerifier("https://auth.onerole.test", "onerole-api", map[string]*rsa.PublicKey{"key-1": &privateKey.PublicKey}, store)
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }

	claims := AccessClaims{
		SessionID: "44444444-4444-4444-8444-444444444444", TokenUse: "provider",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "https://auth.onerole.test", Subject: "33333333-3333-4333-8333-333333333333", Audience: jwt.ClaimStrings{"onerole-api"},
			IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	wrongPurpose := signedAccessToken(t, privateKey, "key-1", now, claims)
	unknownKey := signedAccessToken(t, privateKey, "rotated-key", now, AccessClaims{
		SessionID: "44444444-4444-4444-8444-444444444444", TokenUse: accessTokenUse, RegisteredClaims: claims.RegisteredClaims,
	})

	for name, token := range map[string]string{"wrong token use": wrongPurpose, "unknown key ID": unknownKey} {
		t.Run(name, func(t *testing.T) {
			handler := verifier.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("protected handler ran for an invalid token")
			}))
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", response.Code)
			}
		})
	}
}

func TestJWTVerifierRejectsInactiveSession(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	verifier, err := NewJWTVerifier("https://auth.onerole.test", "onerole-api", map[string]*rsa.PublicKey{"key-1": &privateKey.PublicKey}, sessionStoreStub{err: service.ErrAuthSessionUnavailable})
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }
	token := signedAccessToken(t, privateKey, "key-1", now, AccessClaims{
		SessionID: "44444444-4444-4444-8444-444444444444", TokenUse: accessTokenUse,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "https://auth.onerole.test", Subject: "33333333-3333-4333-8333-333333333333", Audience: jwt.ClaimStrings{"onerole-api"},
			IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	})
	handler := verifier.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler ran for an inactive session")
	}))
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

type membershipReaderStub struct {
	membership service.TenantMembership
	err        error
}

func (m membershipReaderStub) ListActive(context.Context, string) ([]service.TenantMembership, error) {
	return []service.TenantMembership{m.membership}, m.err
}
func (m membershipReaderStub) FindActive(_ context.Context, userID, tenantID string) (service.TenantMembership, error) {
	if m.err != nil {
		return service.TenantMembership{}, m.err
	}
	if m.membership.UserID != userID || m.membership.TenantID != tenantID {
		return service.TenantMembership{}, service.ErrTenantMembershipNotFound
	}
	return m.membership, nil
}

func TestRequireTenantMembershipLoadsCurrentMembership(t *testing.T) {
	reader := membershipReaderStub{membership: service.TenantMembership{
		TenantID: "11111111-1111-4111-8111-111111111111", UserID: "33333333-3333-4333-8333-333333333333", Role: "member", Status: "active",
	}}
	var received service.TenantMembership
	handler := RequireTenantMembership(reader, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = TenantMembershipFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/applications", nil)
	request.Header.Set(tenantHeader, reader.membership.TenantID)
	ctx := context.WithValue(request.Context(), principalContextKey{}, Principal{UserID: "33333333-3333-4333-8333-333333333333", SessionID: "44444444-4444-4444-8444-444444444444"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusNoContent || received != reader.membership {
		t.Fatalf("membership was not attached: status=%d membership=%#v", response.Code, received)
	}
}

func TestRequireTenantMembershipHidesUnrelatedTenant(t *testing.T) {
	reader := membershipReaderStub{membership: service.TenantMembership{
		TenantID: "22222222-2222-4222-8222-222222222222", UserID: "another-user", Role: "owner", Status: "active",
	}}
	handler := RequireTenantMembership(reader, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected tenant handler ran without a current membership")
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/applications", nil)
	request.Header.Set(tenantHeader, "22222222-2222-4222-8222-222222222222")
	ctx := context.WithValue(request.Context(), principalContextKey{}, Principal{UserID: "33333333-3333-4333-8333-333333333333", SessionID: "44444444-4444-4444-8444-444444444444"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request.WithContext(ctx))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unrelated tenant, got %d", response.Code)
	}
}

func signedAccessToken(t *testing.T, key *rsa.PrivateKey, keyID string, now time.Time, claims AccessClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	token.Header["typ"] = "JWT"
	encoded, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

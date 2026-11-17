package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oneRole/service"
)

const (
	accessTokenUse = "access"
	tenantHeader   = "X-Tenant-ID"
)

var errInvalidAccessToken = errors.New("invalid access token")

// AccessClaims contains identity and session references only. Tenant roles are
// always loaded from the current membership store for each request.
type AccessClaims struct {
	SessionID string `json:"sid"`
	TokenUse  string `json:"token_use"`
	jwt.RegisteredClaims
}

// Principal is attached to request context after a JWT and active session have
// both been validated.
type Principal struct {
	UserID    string
	SessionID string
}

type principalContextKey struct{}
type membershipContextKey struct{}

// JWTVerifier checks a strict RS256 access token and its backing session.
type JWTVerifier struct {
	issuer   string
	audience string
	keys     map[string]*rsa.PublicKey
	sessions service.AuthSessionStore
	now      func() time.Time
}

// NewJWTVerifier builds a verifier for a fixed issuer/audience and current RSA
// public keys indexed by key ID. Key IDs must be explicit and non-empty.
func NewJWTVerifier(issuer, audience string, keys map[string]*rsa.PublicKey, sessions service.AuthSessionStore) (*JWTVerifier, error) {
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(audience) == "" || len(keys) == 0 || sessions == nil {
		return nil, errors.New("invalid JWT verifier configuration")
	}
	keyCopy := make(map[string]*rsa.PublicKey, len(keys))
	for keyID, key := range keys {
		if strings.TrimSpace(keyID) == "" || key == nil || key.N == nil || key.N.BitLen() < 2048 || key.E < 3 {
			return nil, errors.New("invalid JWT verification key")
		}
		keyCopy[keyID] = key
	}
	return &JWTVerifier{
		issuer: issuer, audience: audience, keys: keyCopy, sessions: sessions, now: time.Now,
	}, nil
}

// Middleware rejects malformed, expired, incorrectly signed, wrong-purpose,
// or revoked access tokens before a protected handler runs.
func (v *JWTVerifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := v.authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			if errors.Is(err, service.ErrAuthDependencyUnavailable) {
				writeAPIError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication could not be checked.")
				return
			}
			writeAPIError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal)))
	})
}

func (v *JWTVerifier) authenticate(ctx context.Context, authorization string) (Principal, error) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return Principal{}, errInvalidAccessToken
	}
	claims := new(AccessClaims)
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(30*time.Second),
		jwt.WithTimeFunc(func() time.Time { return v.now().UTC() }),
	)
	token, err := parser.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 || token.Header["alg"] != jwt.SigningMethodRS256.Alg() {
			return nil, errors.New("unexpected signing algorithm")
		}
		keyID, ok := token.Header["kid"].(string)
		if !ok || keyID == "" {
			return nil, errors.New("missing signing key ID")
		}
		key, ok := v.keys[keyID]
		if !ok {
			return nil, errors.New("unknown signing key ID")
		}
		return key, nil
	})
	if err != nil || token == nil || !token.Valid || claims.TokenUse != accessTokenUse ||
		!isUUID(claims.Subject) || !isUUID(claims.SessionID) || claims.IssuedAt == nil || claims.ExpiresAt == nil {
		return Principal{}, errInvalidAccessToken
	}

	now := v.now().UTC()
	userID, sessionID := strings.ToLower(claims.Subject), strings.ToLower(claims.SessionID)
	session, err := v.sessions.FindActive(ctx, sessionID, now)
	if err != nil {
		if errors.Is(err, service.ErrAuthSessionUnavailable) {
			return Principal{}, service.ErrAuthSessionUnavailable
		}
		return Principal{}, service.ErrAuthDependencyUnavailable
	}
	if strings.ToLower(session.ID) != sessionID || strings.ToLower(session.UserID) != userID || session.RevokedAt != nil ||
		!session.IdleExpiresAt.After(now) || !session.AbsoluteExpiresAt.After(now) {
		return Principal{}, service.ErrAuthSessionUnavailable
	}
	return Principal{UserID: userID, SessionID: sessionID}, nil
}

// PrincipalFromContext reads the authenticated principal, if present.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

// RequireTenantMembership resolves the selected tenant from X-Tenant-ID and
// checks its current membership for every request. The tenant ID never comes
// from a trusted JWT claim or application body.
func RequireTenantMembership(memberships service.MembershipReader, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || memberships == nil || !isUUID(principal.UserID) || !isUUID(principal.SessionID) {
			writeAPIError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		tenantID := strings.ToLower(strings.TrimSpace(r.Header.Get(tenantHeader)))
		if !isUUID(tenantID) {
			writeAPIError(w, r, http.StatusBadRequest, "tenant_required", "A valid X-Tenant-ID is required.")
			return
		}
		membership, err := memberships.FindActive(r.Context(), principal.UserID, tenantID)
		if errors.Is(err, service.ErrTenantMembershipNotFound) {
			writeAPIError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
			return
		}
		if err != nil {
			writeAPIError(w, r, http.StatusServiceUnavailable, "authorization_unavailable", "Authorization could not be checked.")
			return
		}
		if strings.ToLower(membership.UserID) != principal.UserID || strings.ToLower(membership.TenantID) != tenantID || membership.Status != "active" || !service.IsValidTenantRole(membership.Role) {
			writeAPIError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
			return
		}
		ctx := context.WithValue(r.Context(), membershipContextKey{}, membership)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TenantMembershipFromContext returns the membership resolved by the tenant
// boundary middleware.
func TenantMembershipFromContext(ctx context.Context) (service.TenantMembership, bool) {
	membership, ok := ctx.Value(membershipContextKey{}).(service.TenantMembership)
	return membership, ok
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for i, ch := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

type errorResponse struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" || len(requestID) > 128 {
		requestID = randomRequestID()
	}
	var body errorResponse
	body.Error.Code = code
	body.Error.Message = message
	body.Error.RequestID = requestID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func randomRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

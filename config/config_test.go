package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"testing"
	"time"
)

func lookup(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func apiEnvironment(t *testing.T) map[string]string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return map[string]string{
		"APP_MODE":                    "api",
		"DATABASE_URL":                "mongodb://localhost/onerole",
		"JWT_SIGNING_PRIVATE_KEY_B64": base64.StdEncoding.EncodeToString(privatePEM),
		"AUTH_ENCRYPTION_KEY":         base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		"CSRF_SIGNING_SECRET":         "local-csrf-secret-012345678901234567890123456789",
		"IDEMPOTENCY_HMAC_SECRET":     "local-idempotency-secret-012345678901234567890123",
	}
}

func TestLoadForModeRequiresDatabaseAndJWTKey(t *testing.T) {
	_, err := LoadForMode(ModeAPI, lookup(map[string]string{}))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config, got %v", err)
	}
	values := apiEnvironment(t)
	delete(values, "DATABASE_URL")
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected database validation error, got %v", err)
	}
}

func TestLoadForModeUsesDefaultsAndBuildsRSAVerifierKey(t *testing.T) {
	values := apiEnvironment(t)
	cfg, err := LoadFrom(lookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.WorkerConcurrency != 10 || cfg.OutboxBatchSize != 100 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.GoogleOAuth.Enabled() || cfg.GitHubOAuth.Enabled() {
		t.Fatal("OAuth must remain disabled when provider credentials are absent")
	}
	if cfg.JWTSigningPrivateKey == nil || cfg.JWTSigningPrivateKey.N.BitLen() < 2048 || cfg.JWTVerificationKeys[cfg.JWTSigningKeyID] == nil {
		t.Fatal("RSA signing and verification keys were not configured")
	}
	if cfg.AccessTokenTTL != 10*time.Minute || cfg.RefreshIdleTTL != 7*24*time.Hour {
		t.Fatalf("unexpected auth defaults: %+v", cfg)
	}
}

func TestLoadForModeRequiresNATSForWorkers(t *testing.T) {
	_, err := LoadForMode(ModeWorker, lookup(map[string]string{"DATABASE_URL": "mongodb://localhost/onerole"}))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config, got %v", err)
	}
}

func TestOAuthNeedsCompleteCredentialsAndEncryptionKey(t *testing.T) {
	values := apiEnvironment(t)
	values["GOOGLE_OAUTH_CLIENT_ID"] = "client"
	values["GOOGLE_OAUTH_CLIENT_SECRET"] = "secret"
	values["GOOGLE_OAUTH_REDIRECT_URL"] = "https://localhost/oauth/callback"
	delete(values, "AUTH_ENCRYPTION_KEY")
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected encryption key error, got %v", err)
	}
	values["AUTH_ENCRYPTION_KEY"] = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	cfg, err := LoadFrom(lookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GoogleOAuth.Enabled() {
		t.Fatal("Google OAuth configuration was not enabled")
	}
}

func TestProductionRequiresHTTPSAndSecureCookies(t *testing.T) {
	values := apiEnvironment(t)
	values["APP_ENV"] = "production"
	values["JWT_ISSUER"] = "https://api.example.com"
	values["FRONTEND_AUTH_CALLBACK_URL"] = "https://app.example.com/auth/callback"
	values["ALLOWED_ORIGINS"] = "https://app.example.com"
	values["COOKIE_SECURE"] = "false"
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected secure cookie validation error, got %v", err)
	}
	values["COOKIE_SECURE"] = "true"
	values["FRONTEND_AUTH_CALLBACK_URL"] = "http://app.example/auth/callback"
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected HTTPS callback validation error, got %v", err)
	}
}

func TestMigrateModeOnlyRequiresDatabase(t *testing.T) {
	cfg, err := LoadForMode(ModeMigrate, lookup(map[string]string{"DATABASE_URL": "mongodb://localhost/onerole"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeMigrate {
		t.Fatalf("expected migrate mode, got %s", cfg.Mode)
	}
}

func TestRefreshAbsoluteLifetimeMustCoverIdleLifetime(t *testing.T) {
	values := apiEnvironment(t)
	values["REFRESH_IDLE_TTL"] = "48h"
	values["REFRESH_ABSOLUTE_TTL"] = "24h"
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected lifetime validation error, got %v", err)
	}
}

func TestHMACSecretsMustBeIndependent(t *testing.T) {
	values := apiEnvironment(t)
	values["IDEMPOTENCY_HMAC_SECRET"] = values["CSRF_SIGNING_SECRET"]
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected separate HMAC key error, got %v", err)
	}
}

func TestOutboxBatchSizeMatchesWorkerRuntimeLimit(t *testing.T) {
	values := apiEnvironment(t)
	values["OUTBOX_BATCH_SIZE"] = "501"
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected worker-compatible batch bound, got %v", err)
	}
	values["OUTBOX_BATCH_SIZE"] = "500"
	if _, err := LoadFrom(lookup(values)); err != nil {
		t.Fatalf("expected maximum supported batch size, got %v", err)
	}
}

func TestProductionRejectsZeroEncryptionKey(t *testing.T) {
	values := apiEnvironment(t)
	values["APP_ENV"] = "production"
	values["JWT_ISSUER"] = "https://api.example.com"
	values["FRONTEND_AUTH_CALLBACK_URL"] = "https://app.example.com/auth/callback"
	values["ALLOWED_ORIGINS"] = "https://app.example.com"
	values["COOKIE_SECURE"] = "true"
	if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected random-key validation error, got %v", err)
	}
}

func TestPublicAuthURLsRejectCredentialsQueriesAndFragments(t *testing.T) {
	tests := []struct {
		name string
		key  string
		url  string
	}{
		{name: "issuer credentials", key: "JWT_ISSUER", url: "https://user:pass@example.com"},
		{name: "issuer query", key: "JWT_ISSUER", url: "https://example.com?tenant=1"},
		{name: "issuer fragment", key: "JWT_ISSUER", url: "https://example.com#callback"},
		{name: "frontend credentials", key: "FRONTEND_AUTH_CALLBACK_URL", url: "https://user:pass@example.com/auth/callback"},
		{name: "frontend query", key: "FRONTEND_AUTH_CALLBACK_URL", url: "https://example.com/auth/callback?next=/"},
		{name: "oauth credentials", key: "GOOGLE_OAUTH_REDIRECT_URL", url: "https://user:pass@example.com/oauth/callback"},
		{name: "oauth query", key: "GOOGLE_OAUTH_REDIRECT_URL", url: "https://example.com/oauth/callback?next=/"},
		{name: "oauth fragment", key: "GOOGLE_OAUTH_REDIRECT_URL", url: "https://example.com/oauth/callback#fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := apiEnvironment(t)
			values[test.key] = test.url
			if test.key == "GOOGLE_OAUTH_REDIRECT_URL" {
				values["GOOGLE_OAUTH_CLIENT_ID"] = "client"
				values["GOOGLE_OAUTH_CLIENT_SECRET"] = "secret"
			}
			if _, err := LoadFrom(lookup(values)); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected invalid public auth URL, got %v", err)
			}
		})
	}
}

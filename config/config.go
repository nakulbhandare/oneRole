package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModeAPI      Mode = "api"
	ModeWorker   Mode = "worker"
	ModeAll      Mode = "all"
	ModeMigrate  Mode = "migrate"
	ModeDiscover Mode = "discover"
)

type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentProduction  Environment = "production"
)

type OAuthProvider struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func (p OAuthProvider) Enabled() bool {
	return p.ClientID != "" && p.ClientSecret != "" && p.RedirectURL != ""
}

type Config struct {
	Mode                 Mode
	Environment          Environment
	HTTPAddr             string
	DatabaseURL          string
	NATSURL              string
	RedisURL             string
	JWTIssuer            string
	JWTAudience          string
	JWTSigningKeyID      string
	JWTSigningPrivateKey *rsa.PrivateKey
	JWTVerificationKeys  map[string]*rsa.PublicKey
	AuthEncryptionKey    []byte
	CSRFSigningKey       []byte
	IdempotencyHMACKey   []byte
	GoogleOAuth          OAuthProvider
	GitHubOAuth          OAuthProvider
	FrontendAuthCallback string
	AllowedOrigins       []string
	SecureCookies        bool
	RequestTimeout       time.Duration
	ShutdownTimeout      time.Duration
	AccessTokenTTL       time.Duration
	OAuthTransactionTTL  time.Duration
	RefreshIdleTTL       time.Duration
	RefreshAbsoluteTTL   time.Duration
	WorkerConcurrency    int
	OutboxBatchSize      int
}

type LookupEnv func(string) (string, bool)

var ErrInvalidConfig = errors.New("invalid OneRole configuration")

func Load() (Config, error) { return LoadFrom(os.LookupEnv) }

func LoadFrom(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, invalid("environment lookup is nil")
	}
	mode := Mode(env(lookup, "APP_MODE", string(ModeAll)))
	return LoadForMode(mode, lookup)
}

func LoadForMode(mode Mode, lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, invalid("environment lookup is nil")
	}
	if mode == "" {
		mode = ModeAll
	}
	if mode != ModeAPI && mode != ModeWorker && mode != ModeAll && mode != ModeMigrate && mode != ModeDiscover {
		return Config{}, invalid("APP_MODE must be api, worker, all, migrate, or discover")
	}

	cfg := Config{Mode: mode}
	cfg.Environment = Environment(env(lookup, "APP_ENV", string(EnvironmentDevelopment)))
	if cfg.Environment != EnvironmentDevelopment && cfg.Environment != EnvironmentTest && cfg.Environment != EnvironmentProduction {
		return Config{}, invalid("APP_ENV must be development, test, or production")
	}
	cfg.DatabaseURL = env(lookup, "DATABASE_URL", "")
	if err := validateURL(cfg.DatabaseURL, "mongodb", "mongodb+srv", "DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if mode == ModeMigrate || mode == ModeDiscover {
		return cfg, nil
	}

	cfg.HTTPAddr = env(lookup, "HTTP_ADDR", ":8080")
	if mode == ModeAPI || mode == ModeAll {
		if _, _, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
			return Config{}, invalid("HTTP_ADDR must be a host:port address")
		}
	}
	cfg.NATSURL = env(lookup, "NATS_URL", "")
	if mode == ModeWorker || mode == ModeAll {
		if err := validateURL(cfg.NATSURL, "nats", "tls", "NATS_URL"); err != nil {
			return Config{}, err
		}
	}
	cfg.RedisURL = env(lookup, "REDIS_URL", "")
	if cfg.RedisURL != "" {
		if err := validateURL(cfg.RedisURL, "redis", "rediss", "REDIS_URL"); err != nil {
			return Config{}, err
		}
	}

	cfg.GoogleOAuth = oauthProvider(lookup, "GOOGLE_OAUTH_CLIENT_ID", "GOOGLE_OAUTH_CLIENT_SECRET", "GOOGLE_OAUTH_REDIRECT_URL")
	cfg.GitHubOAuth = oauthProvider(lookup, "GITHUB_OAUTH_CLIENT_ID", "GITHUB_OAUTH_CLIENT_SECRET", "GITHUB_OAUTH_REDIRECT_URL")
	if err := validateOAuth(lookup, cfg.Environment, "Google", "GOOGLE_OAUTH_CLIENT_ID", "GOOGLE_OAUTH_CLIENT_SECRET", "GOOGLE_OAUTH_REDIRECT_URL"); err != nil {
		return Config{}, err
	}
	if err := validateOAuth(lookup, cfg.Environment, "GitHub", "GITHUB_OAUTH_CLIENT_ID", "GITHUB_OAUTH_CLIENT_SECRET", "GITHUB_OAUTH_REDIRECT_URL"); err != nil {
		return Config{}, err
	}

	if mode == ModeAPI || mode == ModeAll {
		if err := loadAPISecrets(&cfg, lookup); err != nil {
			return Config{}, err
		}
	}
	secureDefault := cfg.Environment == EnvironmentProduction
	var err error
	cfg.SecureCookies, err = boolEnv(lookup, "COOKIE_SECURE", secureDefault)
	if err != nil {
		return Config{}, err
	}
	if cfg.Environment == EnvironmentProduction && !cfg.SecureCookies {
		return Config{}, invalid("COOKIE_SECURE must be true in production")
	}
	cfg.RequestTimeout, err = durationEnv(lookup, "REQUEST_TIMEOUT", 90*time.Second, time.Second, 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.ShutdownTimeout, err = durationEnv(lookup, "SHUTDOWN_TIMEOUT", 15*time.Second, time.Second, 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.AccessTokenTTL, err = durationEnv(lookup, "ACCESS_TOKEN_TTL", 10*time.Minute, time.Second, 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.OAuthTransactionTTL, err = durationEnv(lookup, "OAUTH_TRANSACTION_TTL", 5*time.Minute, time.Second, 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cfg.RefreshIdleTTL, err = durationEnv(lookup, "REFRESH_IDLE_TTL", 7*24*time.Hour, time.Second, 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	cfg.RefreshAbsoluteTTL, err = durationEnv(lookup, "REFRESH_ABSOLUTE_TTL", 30*24*time.Hour, time.Second, 90*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if cfg.RefreshAbsoluteTTL < cfg.RefreshIdleTTL {
		return Config{}, invalid("REFRESH_ABSOLUTE_TTL must be at least REFRESH_IDLE_TTL")
	}
	cfg.WorkerConcurrency, err = intEnv(lookup, "WORKER_CONCURRENCY", 10, 1, 1000)
	if err != nil {
		return Config{}, err
	}
	// Worker runtime bounds outbox and source batches at 500, so reject a
	// configuration that would otherwise fail only after process startup.
	cfg.OutboxBatchSize, err = intEnv(lookup, "OUTBOX_BATCH_SIZE", 100, 1, 500)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadAPISecrets(cfg *Config, lookup LookupEnv) error {
	cfg.JWTIssuer = env(lookup, "JWT_ISSUER", "http://localhost:8080")
	cfg.JWTAudience = env(lookup, "JWT_AUDIENCE", "onerole-web")
	cfg.JWTSigningKeyID = env(lookup, "JWT_SIGNING_KEY_ID", "local-1")
	if err := validatePublicHTTPURL(cfg.JWTIssuer, "JWT_ISSUER"); err != nil {
		return err
	}
	if cfg.Environment == EnvironmentProduction && !strings.HasPrefix(strings.ToLower(cfg.JWTIssuer), "https://") {
		return invalid("JWT_ISSUER must use HTTPS in production")
	}
	if cfg.JWTAudience == "" || cfg.JWTSigningKeyID == "" {
		return invalid("JWT_AUDIENCE and JWT_SIGNING_KEY_ID are required")
	}

	privateKeyPEM, err := readPrivateKeyPEM(lookup)
	if err != nil {
		return err
	}
	cfg.JWTSigningPrivateKey, err = parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return err
	}
	if cfg.JWTSigningPrivateKey.N.BitLen() < 2048 || cfg.JWTSigningPrivateKey.E < 3 {
		return invalid("JWT signing key must be RSA with at least 2048 bits")
	}
	if err := cfg.JWTSigningPrivateKey.Validate(); err != nil {
		return invalid("JWT signing RSA key is invalid")
	}
	cfg.JWTVerificationKeys = map[string]*rsa.PublicKey{cfg.JWTSigningKeyID: &cfg.JWTSigningPrivateKey.PublicKey}

	var keyErr error
	cfg.AuthEncryptionKey, keyErr = decodeBase64Key(env(lookup, "AUTH_ENCRYPTION_KEY", ""), "AUTH_ENCRYPTION_KEY")
	if keyErr != nil {
		return keyErr
	}
	if cfg.Environment == EnvironmentProduction && allZero(cfg.AuthEncryptionKey) {
		return invalid("AUTH_ENCRYPTION_KEY must be randomly generated in production")
	}
	cfg.CSRFSigningKey = []byte(env(lookup, "CSRF_SIGNING_SECRET", ""))
	if len(cfg.CSRFSigningKey) < 32 {
		return invalid("CSRF_SIGNING_SECRET must contain at least 32 bytes")
	}
	cfg.IdempotencyHMACKey = []byte(env(lookup, "IDEMPOTENCY_HMAC_SECRET", ""))
	if len(cfg.IdempotencyHMACKey) < 32 {
		return invalid("IDEMPOTENCY_HMAC_SECRET must contain at least 32 bytes")
	}
	if string(cfg.CSRFSigningKey) == string(cfg.IdempotencyHMACKey) {
		return invalid("CSRF_SIGNING_SECRET and IDEMPOTENCY_HMAC_SECRET must use separate key material")
	}

	cfg.FrontendAuthCallback = env(lookup, "FRONTEND_AUTH_CALLBACK_URL", "http://localhost:8080/auth/callback")
	if err := validatePublicHTTPURL(cfg.FrontendAuthCallback, "FRONTEND_AUTH_CALLBACK_URL"); err != nil {
		return err
	}
	if cfg.Environment == EnvironmentProduction && !strings.HasPrefix(strings.ToLower(cfg.FrontendAuthCallback), "https://") {
		return invalid("FRONTEND_AUTH_CALLBACK_URL must use HTTPS in production")
	}
	cfg.AllowedOrigins, err = parseOrigins(env(lookup, "ALLOWED_ORIGINS", "http://localhost:8080,http://127.0.0.1:8080"), cfg.Environment)
	if err != nil {
		return err
	}
	return nil
}

func readPrivateKeyPEM(lookup LookupEnv) ([]byte, error) {
	encoded := env(lookup, "JWT_SIGNING_PRIVATE_KEY_B64", "")
	path := env(lookup, "JWT_SIGNING_PRIVATE_KEY_FILE", "")
	if encoded != "" && path != "" {
		return nil, invalid("set only one of JWT_SIGNING_PRIVATE_KEY_B64 or JWT_SIGNING_PRIVATE_KEY_FILE")
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, invalid("JWT_SIGNING_PRIVATE_KEY_FILE could not be read")
		}
		return data, nil
	}
	if encoded == "" {
		return nil, invalid("JWT signing private key is required in API mode")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(encoded)
	}
	if err != nil {
		return nil, invalid("JWT_SIGNING_PRIVATE_KEY_B64 must be base64-encoded PEM")
	}
	return data, nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, invalid("JWT signing key must contain a PEM-encoded RSA private key")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if ok {
			return rsaKey, nil
		}
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, invalid("JWT signing key must be a valid RSA private key")
	}
	return key, nil
}

func decodeBase64Key(encoded, name string) ([]byte, error) {
	if encoded == "" {
		return nil, invalid(name + " is required in API mode")
	}
	key, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		key, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil || len(key) != 32 {
		return nil, invalid(name + " must be a base64-encoded 32-byte key")
	}
	return key, nil
}

func oauthProvider(lookup LookupEnv, idKey, secretKey, redirectKey string) OAuthProvider {
	return OAuthProvider{
		ClientID:     env(lookup, idKey, ""),
		ClientSecret: env(lookup, secretKey, ""),
		RedirectURL:  env(lookup, redirectKey, ""),
	}
}

func validateOAuth(lookup LookupEnv, environment Environment, providerName, idKey, secretKey, redirectKey string) error {
	id, secret, redirect := env(lookup, idKey, ""), env(lookup, secretKey, ""), env(lookup, redirectKey, "")
	if id == "" && secret == "" && redirect == "" {
		return nil
	}
	if id == "" || secret == "" || redirect == "" {
		return invalid("all client ID, client secret, and redirect URL values are required when enabling " + providerName + " OAuth")
	}
	if err := validatePublicHTTPURL(redirect, providerName+" OAuth redirect URL"); err != nil {
		return err
	}
	if environment == EnvironmentProduction {
		parsed, _ := url.Parse(redirect)
		if parsed.Scheme != "https" {
			return invalid(providerName + " OAuth redirect URL must use HTTPS in production")
		}
	}
	return nil
}

func parseOrigins(raw string, environment Environment) ([]string, error) {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
			(parsed.Scheme != "https" && parsed.Scheme != "http") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, invalid("ALLOWED_ORIGINS must be comma-separated exact HTTP origins")
		}
		if environment == EnvironmentProduction && parsed.Scheme != "https" {
			return nil, invalid("ALLOWED_ORIGINS must use HTTPS in production")
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return nil, invalid("ALLOWED_ORIGINS must include at least one origin")
	}
	return origins, nil
}

func validateURL(raw, schemeA, schemeB, name string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != schemeA && parsed.Scheme != schemeB) {
		return invalid(name + " must be an absolute URL using an allowed scheme")
	}
	return nil
}

// validatePublicHTTPURL checks URLs that become public identity or browser
// redirect values. Unlike service URLs (which may contain credentials), these
// must not carry user information, a query, or a fragment.
func validatePublicHTTPURL(raw, name string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return invalid(name + " must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return nil
}

func env(lookup LookupEnv, key, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func boolEnv(lookup LookupEnv, key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, invalid(key + " must be true or false")
	}
	return parsed, nil
}

func durationEnv(lookup LookupEnv, key string, fallback, minimum, maximum time.Duration) (time.Duration, error) {
	value := env(lookup, key, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, invalid(fmt.Sprintf("%s must be between %s and %s", key, minimum, maximum))
	}
	return parsed, nil
}

func intEnv(lookup LookupEnv, key string, fallback, minimum, maximum int) (int, error) {
	value := env(lookup, key, "")
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, invalid(fmt.Sprintf("%s must be between %d and %d", key, minimum, maximum))
	}
	return parsed, nil
}

func allZero(value []byte) bool {
	for _, current := range value {
		if current != 0 {
			return false
		}
	}
	return len(value) > 0
}

func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalidConfig, message) }

package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oneRole/middleware"
	"github.com/oneRole/service"
)

const (
	refreshCookieName      = "onerole_refresh"
	csrfCookieName         = "onerole_csrf"
	oauthBindingCookieName = "onerole_oauth_binding"
	authCookiePath         = "/api/v1/auth"
	csrfCookiePath         = "/"
	oauthBindingCookiePath = "/api/v1/auth/oauth"
)

// AuthAPI is the controller-facing auth use case. service.AuthService
// implements this contract; tests can provide deterministic fakes.
type AuthAPI interface {
	Providers() []service.OAuthProviderStatus
	StartOAuth(context.Context, service.OAuthProvider, time.Time) (service.OAuthStartResult, error)
	CompleteOAuth(context.Context, service.OAuthProvider, string, string, string, string, time.Time) (service.OAuthCallbackResult, error)
	Refresh(context.Context, string, time.Time) (service.RefreshResult, error)
	Logout(context.Context, string, time.Time) error
	CurrentUser(context.Context, string) (service.CurrentUser, error)
	ValidCSRF(origin, cookieToken, headerToken string) bool
	DevLogin(context.Context, string, string, time.Time) (service.DevLoginResult, error)
}

type authProvidersResponse struct {
	Providers []service.OAuthProviderStatus `json:"providers"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type membershipResponse struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

type currentUserResponse struct {
	UserID      string               `json:"user_id"`
	DisplayName string               `json:"display_name"`
	Email       *string              `json:"email"`
	Memberships []membershipResponse `json:"memberships"`
}

type csrfBootstrapResponse struct {
	CSRFToken string `json:"csrf_token"`
}

// AuthHTTPConfig contains cookie and fixed redirect settings shared with the
// service. SecureCookies must be true outside local development.
type AuthHTTPConfig struct {
	SecureCookies       bool
	FrontendRedirectURL string
	IsProduction        bool
}

type AuthHandler struct {
	api    AuthAPI
	config AuthHTTPConfig
}

func NewAuthHandler(api AuthAPI, config AuthHTTPConfig) (*AuthHandler, error) {
	if api == nil || !validFixedFrontendURL(config.FrontendRedirectURL) {
		return nil, errors.New("invalid authentication HTTP handler configuration")
	}
	return &AuthHandler{api: api, config: config}, nil
}

func (h *AuthHandler) DevLogin(w http.ResponseWriter, r *http.Request) {
	if h.config.IsProduction {
		writeError(w, r, http.StatusForbidden, "dev_login_disabled", "Dev login is disabled in production.")
		return
	}
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		_ = decodeJSON(r, &req)
	}

	result, err := h.api.DevLogin(r.Context(), req.Email, req.Name, time.Now().UTC())
	if err != nil {
		slog.Error("DevLogin failed", "error", err)
		h.writeAuthError(w, r, err)
		return
	}

	http.SetCookie(w, refreshCookie(result.RefreshToken, result.RefreshExpiresAt, h.config.SecureCookies))
	http.SetCookie(w, csrfCookie(result.CSRFToken, result.RefreshExpiresAt, h.config.SecureCookies))

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": result.AccessToken,
		"token_type":   result.TokenType,
		"expires_in":   result.ExpiresIn,
		"user_id":      result.UserID,
		"email":        result.Email,
		"display_name": result.DisplayName,
		"tenant_id":    result.TenantID,
		"csrf_token":   result.CSRFToken,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_email", "Email address is required.")
		return
	}

	name := strings.Split(req.Email, "@")[0]
	result, err := h.api.DevLogin(r.Context(), req.Email, name, time.Now().UTC())
	if err != nil {
		slog.Error("Login failed", "error", err)
		h.writeAuthError(w, r, err)
		return
	}

	http.SetCookie(w, refreshCookie(result.RefreshToken, result.RefreshExpiresAt, h.config.SecureCookies))
	http.SetCookie(w, csrfCookie(result.CSRFToken, result.RefreshExpiresAt, h.config.SecureCookies))

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": result.AccessToken,
		"token_type":   result.TokenType,
		"expires_in":   result.ExpiresIn,
		"user_id":      result.UserID,
		"email":        result.Email,
		"display_name": result.DisplayName,
		"tenant_id":    result.TenantID,
		"csrf_token":   result.CSRFToken,
		"message":      "Signed in successfully",
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_email", "Email address is required.")
		return
	}
	displayName := strings.TrimSpace(req.Name)
	if displayName == "" {
		displayName = strings.Split(req.Email, "@")[0]
	}

	result, err := h.api.DevLogin(r.Context(), req.Email, displayName, time.Now().UTC())
	if err != nil {
		slog.Error("Registration failed", "error", err)
		h.writeAuthError(w, r, err)
		return
	}

	http.SetCookie(w, refreshCookie(result.RefreshToken, result.RefreshExpiresAt, h.config.SecureCookies))
	http.SetCookie(w, csrfCookie(result.CSRFToken, result.RefreshExpiresAt, h.config.SecureCookies))

	writeJSON(w, http.StatusCreated, map[string]any{
		"access_token": result.AccessToken,
		"token_type":   result.TokenType,
		"expires_in":   result.ExpiresIn,
		"user_id":      result.UserID,
		"email":        result.Email,
		"display_name": result.DisplayName,
		"tenant_id":    result.TenantID,
		"csrf_token":   result.CSRFToken,
		"message":      "Account created successfully",
	})
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Invalid request body.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "If an account exists with that email, a password reset link has been dispatched.",
		"status":  "dispatched",
	})
}

func (h *AuthHandler) Providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, authProvidersResponse{Providers: h.api.Providers()})
}

// CSRF returns the signed double-submit token only to an exact configured
// Origin that presents its valid CSRF cookie. This supports same-site
// subdomain frontends that cannot read the API host's host-only cookie.
func (h *AuthHandler) CSRF(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	origin := r.Header.Get("Origin")
	csrfToken := cookieValue(r, csrfCookieName)
	if origin == "" || csrfToken == "" || !h.api.ValidCSRF(origin, csrfToken, csrfToken) {
		writeError(w, r, http.StatusForbidden, "csrf_validation_failed", "The request could not be verified.")
		return
	}
	writeJSON(w, http.StatusOK, csrfBootstrapResponse{CSRFToken: csrfToken})
}

func (h *AuthHandler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	provider, ok := parseProvider(r.PathValue("provider"))
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_provider", "The selected sign-in provider is not supported.")
		return
	}
	result, err := h.api.StartOAuth(r.Context(), provider, time.Now().UTC())
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	if !validProviderAuthorizationURL(provider, result.AuthorizationURL) || result.BrowserBindingSecret == "" || !result.ExpiresAt.After(time.Now()) {
		writeError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "Sign-in could not be started.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthBindingCookieName, Value: result.BrowserBindingSecret,
		Path: oauthBindingCookiePath, HttpOnly: true, Secure: h.config.SecureCookies,
		SameSite: http.SameSiteLaxMode, Expires: result.ExpiresAt.UTC(), MaxAge: maxAgeSeconds(result.ExpiresAt),
	})
	http.Redirect(w, r, result.AuthorizationURL, http.StatusFound)
}

func (h *AuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider, ok := parseProvider(r.PathValue("provider"))
	if !ok {
		writeError(w, r, http.StatusBadRequest, "invalid_provider", "The selected sign-in provider is not supported.")
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	providerError := r.URL.Query().Get("error")
	bindingCookie, _ := r.Cookie(oauthBindingCookieName)
	binding := ""
	if bindingCookie != nil {
		binding = bindingCookie.Value
	}
	clearCookie(w, oauthBindingCookieName, oauthBindingCookiePath, h.config.SecureCookies, true)
	if len(state) > 128 || len(code) > 4096 || len(providerError) > 128 {
		writeError(w, r, http.StatusBadRequest, "invalid_oauth_callback", "The sign-in response was invalid.")
		return
	}
	result, err := h.api.CompleteOAuth(r.Context(), provider, state, binding, code, providerError, time.Now().UTC())
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	if result.RedirectURL != h.config.FrontendRedirectURL || result.RefreshToken == "" || result.CSRFToken == "" || !result.RefreshExpiresAt.After(time.Now()) {
		writeError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "Sign-in could not be completed.")
		return
	}
	http.SetCookie(w, refreshCookie(result.RefreshToken, result.RefreshExpiresAt, h.config.SecureCookies))
	http.SetCookie(w, csrfCookie(result.CSRFToken, result.RefreshExpiresAt, h.config.SecureCookies))

	redirectURL := result.RedirectURL
	if result.AccessToken != "" && (strings.Contains(redirectURL, "/docs") || strings.Contains(redirectURL, "/swagger")) {
		redirectURL = fmt.Sprintf("%s#access_token=%s&email=%s&tenant_id=%s&refresh_token=%s",
			redirectURL,
			url.QueryEscape(result.AccessToken),
			url.QueryEscape(result.Email),
			url.QueryEscape(result.TenantID),
			url.QueryEscape(result.RefreshToken),
		)
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	csrfCookieValue := cookieValue(r, csrfCookieName)
	if !h.api.ValidCSRF(r.Header.Get("Origin"), csrfCookieValue, r.Header.Get("X-CSRF-Token")) {
		writeError(w, r, http.StatusForbidden, "csrf_validation_failed", "The request could not be verified.")
		return
	}
	refreshToken := cookieValue(r, refreshCookieName)
	if refreshToken == "" {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	result, err := h.api.Refresh(r.Context(), refreshToken, time.Now().UTC())
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	if result.AccessToken == "" || result.ExpiresIn <= 0 || result.RefreshToken == "" || !result.RefreshExpiresAt.After(time.Now()) {
		writeError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "The session could not be refreshed.")
		return
	}
	http.SetCookie(w, refreshCookie(result.RefreshToken, result.RefreshExpiresAt, h.config.SecureCookies))
	// The signed double-submit CSRF value remains valid across refresh rotation;
	// extend its cookie to the new idle session expiry.
	http.SetCookie(w, csrfCookie(csrfCookieValue, result.RefreshExpiresAt, h.config.SecureCookies))
	writeJSON(w, http.StatusOK, accessTokenResponse{
		AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: result.ExpiresIn,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	csrfCookieValue := cookieValue(r, csrfCookieName)
	if !h.api.ValidCSRF(r.Header.Get("Origin"), csrfCookieValue, r.Header.Get("X-CSRF-Token")) {
		writeError(w, r, http.StatusForbidden, "csrf_validation_failed", "The request could not be verified.")
		return
	}
	principal, ok := authPrincipalFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	if err := h.api.Logout(r.Context(), principal.SessionID, time.Now().UTC()); err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	clearCookie(w, refreshCookieName, authCookiePath, h.config.SecureCookies, true)
	clearCookie(w, csrfCookieName, csrfCookiePath, h.config.SecureCookies, false)
	clearCookie(w, oauthBindingCookieName, oauthBindingCookiePath, h.config.SecureCookies, true)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := authPrincipalFromRequest(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	user, err := h.api.CurrentUser(r.Context(), principal.UserID)
	if err != nil {
		h.writeAuthError(w, r, err)
		return
	}
	memberships := make([]membershipResponse, 0, len(user.Memberships))
	for _, membership := range user.Memberships {
		memberships = append(memberships, membershipResponse{TenantID: membership.TenantID, Role: membership.Role})
	}
	writeJSON(w, http.StatusOK, currentUserResponse{
		UserID: user.UserID, DisplayName: user.DisplayName, Email: user.Email, Memberships: memberships,
	})
}

func (h *AuthHandler) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrOAuthProviderUnavailable), errors.Is(err, service.ErrInvalidOAuthTransaction):
		writeError(w, r, http.StatusBadRequest, "invalid_oauth_request", "The sign-in request could not be validated.")
	case errors.Is(err, service.ErrOAuthTransactionExpired), errors.Is(err, service.ErrOAuthTransactionConsumed),
		errors.Is(err, service.ErrOAuthAuthenticationFailed), errors.Is(err, service.ErrAuthSessionUnavailable),
		errors.Is(err, service.ErrAuthUserUnavailable):
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
	case errors.Is(err, service.ErrIdentityConflict):
		writeError(w, r, http.StatusConflict, "identity_conflict", "This identity must be linked from an authenticated account.")
	case errors.Is(err, service.ErrAuthDependencyUnavailable):
		writeError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication could not be completed.")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "Authentication could not be completed.")
	}
}

func authPrincipalFromRequest(r *http.Request) (middleware.Principal, bool) {
	return middleware.PrincipalFromContext(r.Context())
}

func parseProvider(value string) (service.OAuthProvider, bool) {
	provider := service.OAuthProvider(value)
	return provider, provider == service.OAuthProviderGoogle || provider == service.OAuthProviderGitHub
}

func validProviderAuthorizationURL(provider service.OAuthProvider, raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	switch provider {
	case service.OAuthProviderGoogle:
		return strings.EqualFold(parsed.Hostname(), "accounts.google.com")
	case service.OAuthProviderGitHub:
		return strings.EqualFold(parsed.Hostname(), "github.com")
	default:
		return false
	}
}

func validFixedFrontendURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path == "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")
}

func refreshCookie(value string, expires time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: refreshCookieName, Value: value, Path: authCookiePath,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: expires.UTC(), MaxAge: maxAgeSeconds(expires),
	}
}

func csrfCookie(value string, expires time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name: csrfCookieName, Value: value, Path: csrfCookiePath,
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: expires.UTC(), MaxAge: maxAgeSeconds(expires),
	}
}

func clearCookie(w http.ResponseWriter, name, path string, secure, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: "", Path: path, HttpOnly: httpOnly, Secure: secure,
		SameSite: http.SameSiteStrictMode, Expires: time.Unix(1, 0).UTC(), MaxAge: -1,
	})
}

func cookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func maxAgeSeconds(expires time.Time) int {
	seconds := int(time.Until(expires).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthCSRFBootstrapReturnsSignedTokenWithoutCaching(t *testing.T) {
	handler, _, authAPI := newTestHTTPHandler(t)
	csrfToken := strings.Repeat("s", 64)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	request.Header.Set("Origin", "https://app.onerole.test")
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("Access-Control-Allow-Origin") != "https://app.onerole.test" ||
		response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("CSRF bootstrap should return a no-store credentialed response: status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	var body csrfBootstrapResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.CSRFToken != csrfToken {
		t.Fatalf("CSRF bootstrap returned an invalid token body: %#v err=%v", body, err)
	}
	if authAPI.csrfCalls != 1 || authAPI.csrfOrigin != "https://app.onerole.test" ||
		authAPI.csrfCookie != csrfToken || authAPI.csrfHeader != csrfToken {
		t.Fatalf("CSRF cookie must be verified against itself for the exact Origin: %#v", authAPI)
	}
}

func TestAuthCSRFBootstrapRejectsMissingOriginOrCookie(t *testing.T) {
	handler, _, _ := newTestHTTPHandler(t)

	missingOrigin := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	missingOrigin.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "signed-csrf"})
	missingOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingOriginResponse, missingOrigin)
	if missingOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("missing Origin must be rejected, got %d", missingOriginResponse.Code)
	}

	missingCookie := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	missingCookie.Header.Set("Origin", "https://app.onerole.test")
	missingCookieResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingCookieResponse, missingCookie)
	if missingCookieResponse.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF cookie must be rejected, got %d", missingCookieResponse.Code)
	}
}

func TestAuthCSRFBootstrapRejectsDisallowedOriginAndInvalidToken(t *testing.T) {
	handler, _, authAPI := newTestHTTPHandler(t)

	disallowed := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	disallowed.Header.Set("Origin", "https://attacker.example")
	disallowed.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "signed-csrf"})
	disallowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(disallowedResponse, disallowed)
	if disallowedResponse.Code != http.StatusForbidden || disallowedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed Origin must be denied without CORS permission: status=%d headers=%v", disallowedResponse.Code, disallowedResponse.Header())
	}

	authAPI.csrfValid = false
	invalidToken := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	invalidToken.Header.Set("Origin", "https://app.onerole.test")
	invalidToken.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "invalid-token"})
	invalidTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidTokenResponse, invalidToken)
	if invalidTokenResponse.Code != http.StatusForbidden {
		t.Fatalf("invalid signed CSRF token must be rejected, got %d", invalidTokenResponse.Code)
	}
}

package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSMiddlewareAllowsConfiguredOriginAndCredentialedPreflight(t *testing.T) {
	handler, _, _ := newTestHTTPHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	request.Header.Set("Origin", "https://app.onerole.test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "https://app.onerole.test" ||
		response.Header().Get("Access-Control-Allow-Credentials") != "true" || !headerHasToken(response.Header().Values("Vary"), "Origin") {
		t.Fatalf("configured origin should receive credentialed CORS response: status=%d headers=%v", response.Code, response.Header())
	}

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
		t.Run("preflight "+method, func(t *testing.T) {
			preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/applications", nil)
			preflight.Header.Set("Origin", "https://app.onerole.test")
			preflight.Header.Set("Access-Control-Request-Method", method)
			preflight.Header.Set("Access-Control-Request-Headers", "authorization, content-type, idempotency-key, x-tenant-id, x-csrf-token, x-request-id")
			preflightResponse := httptest.NewRecorder()
			handler.ServeHTTP(preflightResponse, preflight)
			if preflightResponse.Code != http.StatusNoContent {
				t.Fatalf("preflight failed: status=%d body=%s", preflightResponse.Code, preflightResponse.Body.String())
			}
			if preflightResponse.Header().Get("Access-Control-Allow-Origin") != "https://app.onerole.test" ||
				preflightResponse.Header().Get("Access-Control-Allow-Credentials") != "true" ||
				preflightResponse.Header().Get("Access-Control-Allow-Methods") != corsAllowedMethods ||
				preflightResponse.Header().Get("Access-Control-Allow-Headers") != corsAllowedHeaders {
				t.Fatalf("preflight returned unexpected CORS headers: %v", preflightResponse.Header())
			}
			for _, vary := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"} {
				if !headerHasToken(preflightResponse.Header().Values("Vary"), vary) {
					t.Fatalf("preflight response must vary on %s: %v", vary, preflightResponse.Header().Values("Vary"))
				}
			}
		})
	}
}

func TestCORSMiddlewareRejectsUnconfiguredOriginAndUnapprovedPreflight(t *testing.T) {
	handler, _, _ := newTestHTTPHandler(t)

	disallowed := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	disallowed.Header.Set("Origin", "https://app.onerole.test.attacker.example")
	disallowedResponse := httptest.NewRecorder()
	handler.ServeHTTP(disallowedResponse, disallowed)
	if disallowedResponse.Code != http.StatusForbidden || disallowedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unconfigured origin must not be reflected: status=%d headers=%v", disallowedResponse.Code, disallowedResponse.Header())
	}

	for name, headers := range map[string]string{
		"unapproved header": "authorization, x-attacker-header",
		"unapproved method": "authorization",
	} {
		t.Run(name, func(t *testing.T) {
			preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/applications", nil)
			preflight.Header.Set("Origin", "https://app.onerole.test")
			requestedMethod := http.MethodPost
			if name == "unapproved method" {
				requestedMethod = http.MethodDelete
			}
			preflight.Header.Set("Access-Control-Request-Method", requestedMethod)
			preflight.Header.Set("Access-Control-Request-Headers", headers)
			preflightResponse := httptest.NewRecorder()
			handler.ServeHTTP(preflightResponse, preflight)
			if preflightResponse.Code != http.StatusForbidden || preflightResponse.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("invalid preflight must be rejected without CORS permission: status=%d headers=%v", preflightResponse.Code, preflightResponse.Header())
			}
		})
	}
}

func TestCORSMiddlewareRejectsWildcardConfiguration(t *testing.T) {
	if _, err := newCORSMiddleware([]string{"*"}); err == nil {
		t.Fatal("wildcard origins must not be accepted with credentials")
	}
	if _, err := newCORSMiddleware([]string{"https://*.example.test"}); err == nil {
		t.Fatal("wildcard subdomain origins must not be accepted")
	}
}

func headerHasToken(values []string, token string) bool {
	for _, value := range values {
		for _, candidate := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(candidate), token) {
				return true
			}
		}
	}
	return false
}

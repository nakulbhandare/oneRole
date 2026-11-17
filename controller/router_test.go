package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oneRole/middleware"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

const (
	testTenantID  = "11111111-1111-4111-8111-111111111111"
	testUserID    = "22222222-2222-4222-8222-222222222222"
	testJobID     = "33333333-3333-4333-8333-333333333333"
	testAppID     = "44444444-4444-4444-8444-444444444444"
	testResumeID  = "55555555-5555-4555-8555-555555555555"
	testConsentID = "66666666-6666-4666-8666-666666666666"
	testBatchID   = "77777777-7777-4777-8777-777777777777"
)

type httpSessionStoreFake struct {
	session service.AuthSession
}

func (s httpSessionStoreFake) Create(context.Context, service.AuthSession) error { return nil }
func (s httpSessionStoreFake) Rotate(context.Context, service.RefreshRotation) (service.AuthSession, error) {
	return service.AuthSession{}, nil
}
func (s httpSessionStoreFake) FindActive(_ context.Context, id string, now time.Time) (service.AuthSession, error) {
	if id != s.session.ID || s.session.UserID == "" || !s.session.IdleExpiresAt.After(now) || !s.session.AbsoluteExpiresAt.After(now) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}
	return s.session, nil
}
func (s httpSessionStoreFake) Revoke(context.Context, string, time.Time, string) error { return nil }

type httpMembershipReaderFake struct {
	membership service.TenantMembership
}

func (m httpMembershipReaderFake) ListActive(context.Context, string) ([]service.TenantMembership, error) {
	return []service.TenantMembership{m.membership}, nil
}
func (m httpMembershipReaderFake) FindActive(_ context.Context, userID, tenantID string) (service.TenantMembership, error) {
	if m.membership.UserID != userID || m.membership.TenantID != tenantID || m.membership.Status != "active" {
		return service.TenantMembership{}, service.ErrTenantMembershipNotFound
	}
	return m.membership, nil
}

type authAPIFake struct {
	start         service.OAuthStartResult
	complete      service.OAuthCallbackResult
	refresh       service.RefreshResult
	current       service.CurrentUser
	csrfValid     bool
	logoutID      string
	startCalls    int
	completeCalls int
	refreshCalls  int
	csrfCalls     int
	csrfOrigin    string
	csrfCookie    string
	csrfHeader    string
}

func (a *authAPIFake) Providers() []service.OAuthProviderStatus {
	return []service.OAuthProviderStatus{{ID: service.OAuthProviderGoogle, Label: "Continue with Google", Enabled: true}}
}
func (a *authAPIFake) StartOAuth(context.Context, service.OAuthProvider, time.Time) (service.OAuthStartResult, error) {
	a.startCalls++
	return a.start, nil
}
func (a *authAPIFake) CompleteOAuth(context.Context, service.OAuthProvider, string, string, string, string, time.Time) (service.OAuthCallbackResult, error) {
	a.completeCalls++
	return a.complete, nil
}
func (a *authAPIFake) Refresh(context.Context, string, time.Time) (service.RefreshResult, error) {
	a.refreshCalls++
	return a.refresh, nil
}
func (a *authAPIFake) Logout(_ context.Context, sessionID string, _ time.Time) error {
	a.logoutID = sessionID
	return nil
}
func (a *authAPIFake) CurrentUser(context.Context, string) (service.CurrentUser, error) {
	return a.current, nil
}
func (a *authAPIFake) ValidCSRF(origin, cookie, header string) bool {
	a.csrfCalls++
	a.csrfOrigin, a.csrfCookie, a.csrfHeader = origin, cookie, header
	return a.csrfValid && origin == "https://app.onerole.test" && cookie != "" && cookie == header
}
func (a *authAPIFake) DevLogin(_ context.Context, email, name string, _ time.Time) (service.DevLoginResult, error) {
	if email == "" {
		email = "onerole@gmail.com"
	}
	if name == "" {
		name = "OneRole Candidate"
	}
	return service.DevLoginResult{
		AccessToken:      "dev-access-token",
		TokenType:        "Bearer",
		ExpiresIn:        600,
		UserID:           testUserID,
		Email:            email,
		DisplayName:      name,
		TenantID:         testTenantID,
		RefreshToken:     "dev-refresh-token",
		RefreshExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CSRFToken:        "v1.dev-csrf-token",
		Memberships:      []service.TenantMembership{{TenantID: testTenantID, UserID: testUserID, Role: "admin", Status: "active"}},
	}, nil
}

type applicationAPIFake struct {
	scope       service.ActorScope
	createCalls int
	submitCalls int
	batchCalls  int
	getCalls    int
	batchReads  int
}

func (a *applicationAPIFake) CreateApplication(_ context.Context, scope service.ActorScope, _ service.CreateApplicationCommand, _ string) (service.WorkflowAcceptance, error) {
	a.scope = scope
	a.createCalls++
	return service.WorkflowAcceptance{WorkflowID: testAppID, Status: "preparing", AcceptedAt: time.Now().UTC()}, nil
}
func (a *applicationAPIFake) SubmitApplication(_ context.Context, scope service.ActorScope, _ string, _ string) (service.WorkflowAcceptance, error) {
	a.scope = scope
	a.submitCalls++
	return service.WorkflowAcceptance{WorkflowID: testAppID, Status: "queued", AcceptedAt: time.Now().UTC()}, nil
}
func (a *applicationAPIFake) CreateBatch(_ context.Context, scope service.ActorScope, _ service.CreateBatchCommand, _ string) (service.WorkflowAcceptance, error) {
	a.scope = scope
	a.batchCalls++
	return service.WorkflowAcceptance{WorkflowID: testBatchID, Status: "accepted", AcceptedAt: time.Now().UTC()}, nil
}
func (a *applicationAPIFake) GetApplication(_ context.Context, scope service.ActorScope, _ string) (models.Application, error) {
	a.scope = scope
	a.getCalls++
	return models.Application{ID: testAppID, JobID: testJobID, Status: models.PublicApplicationPreparing, Capability: models.CapabilityAssistedSupported, UpdatedAt: time.Now().UTC()}, nil
}
func (a *applicationAPIFake) GetBatch(_ context.Context, scope service.ActorScope, _ string) (service.BatchView, error) {
	a.scope = scope
	a.batchReads++
	return service.BatchView{ID: testBatchID, Total: 1, Counts: map[string]int{"accepted": 1}, CreatedAt: time.Now().UTC()}, nil
}
func (a *applicationAPIFake) ListApplications(_ context.Context, scope service.ActorScope, _ *string, _ string, _ int) (service.ApplicationListPage, error) {
	a.scope = scope
	return service.ApplicationListPage{
		Items: []models.Application{{ID: testAppID, JobID: testJobID, Status: models.PublicApplicationPreparing, Capability: models.CapabilityAssistedSupported, UpdatedAt: time.Now().UTC()}},
		Total: 1,
	}, nil
}
func (a *applicationAPIFake) GetApplicationTimeline(_ context.Context, scope service.ActorScope, _ string) ([]models.ApplicationEvent, error) {
	a.scope = scope
	return []models.ApplicationEvent{{ID: "evt-1", TenantID: scope.TenantID, ApplicationID: testAppID, EventType: "application.created", ToStatus: "preparing", OccurredAt: time.Now().UTC()}}, nil
}
func (a *applicationAPIFake) GetApplicationReceipt(_ context.Context, scope service.ActorScope, _ string) (*models.ApplicationReceipt, error) {
	a.scope = scope
	return &models.ApplicationReceipt{ApplicationID: testAppID, ReceiptID: "RCPT-123", Status: "submitted", SubmittedAt: time.Now().UTC()}, nil
}

func TestNewHTTPHandlerAppliesCurrentTenantBoundaryAndDurable202(t *testing.T) {
	handler, appAPI, authAPI := newTestHTTPHandler(t)
	body, _ := json.Marshal(service.CreateApplicationCommand{
		JobID: testJobID, ResumeVersionID: testResumeID, ConsentID: testConsentID,
		Mode: service.ApplicationModeReviewBeforeSubmit,
	})
	request := authenticatedRequest(t, http.MethodPost, "/api/v1/applications", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "stable-idempotency-key-001")
	request.Header.Set("X-Tenant-ID", testTenantID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || appAPI.createCalls != 1 {
		t.Fatalf("expected durable 202 acceptance, status=%d body=%s", response.Code, response.Body.String())
	}
	var accepted workflowAcceptedResponse
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.WorkflowID != testAppID || accepted.StatusURL != "/api/v1/applications/"+testAppID || !strings.Contains(accepted.Message, "employer submission is not yet confirmed") {
		t.Fatalf("unexpected acceptance response: %#v", accepted)
	}
	if appAPI.scope != (service.ActorScope{TenantID: testTenantID, UserID: testUserID}) {
		t.Fatalf("actor scope must come from JWT and current membership: %#v", appAPI.scope)
	}
	if authAPI.startCalls != 0 {
		t.Fatal("application route must not invoke auth start")
	}

	wrongTenant := authenticatedRequest(t, http.MethodGet, "/api/v1/applications/"+testAppID, nil)
	wrongTenant.Header.Set("X-Tenant-ID", "99999999-9999-4999-8999-999999999999")
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrongTenant)
	if wrongResponse.Code != http.StatusNotFound || appAPI.getCalls != 0 {
		t.Fatalf("cross-tenant existence must be hidden, status=%d", wrongResponse.Code)
	}
}

func TestApplicationSubmitBatchAndTenantScopedReads(t *testing.T) {
	handler, appAPI, _ := newTestHTTPHandler(t)
	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPost, "/api/v1/applications/" + testAppID + "/submit", "", http.StatusAccepted},
		{http.MethodPost, "/api/v1/application-batches", `{"job_ids":["` + testJobID + `"],"resume_version_id":"` + testResumeID + `","consent_id":"` + testConsentID + `","mode":"submit_when_ready"}`, http.StatusAccepted},
		{http.MethodGet, "/api/v1/applications/" + testAppID, "", http.StatusOK},
		{http.MethodGet, "/api/v1/application-batches/" + testBatchID, "", http.StatusOK},
		{http.MethodGet, "/api/v1/applications", "", http.StatusOK},
		{http.MethodGet, "/api/v1/applications/" + testAppID + "/timeline", "", http.StatusOK},
		{http.MethodGet, "/api/v1/applications/" + testAppID + "/receipt", "", http.StatusOK},
	} {
		request := authenticatedRequest(t, tc.method, tc.path, strings.NewReader(tc.body))
		request.Header.Set("X-Tenant-ID", testTenantID)
		if tc.method == http.MethodPost && strings.Contains(tc.path, "application-batches") {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "batch-idempotency-key-001")
		} else if tc.method == http.MethodPost {
			request.Header.Set("Idempotency-Key", "submit-idempotency-key-001")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("%s %s: expected %d, got %d: %s", tc.method, tc.path, tc.want, response.Code, response.Body.String())
		}
	}
	if appAPI.submitCalls != 1 || appAPI.batchCalls != 1 || appAPI.getCalls != 1 || appAPI.batchReads != 1 {
		t.Fatalf("not all tenant-scoped routes ran: %#v", appAPI)
	}
}

func TestAuthDevLogin(t *testing.T) {
	handler, _, _ := newTestHTTPHandler(t)
	body := `{"email":"onerole@gmail.com","name":"OneRole User"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev-login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/auth/dev-login expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if res["access_token"] != "dev-access-token" {
		t.Errorf("expected access_token 'dev-access-token', got %v", res["access_token"])
	}
	if res["email"] != "onerole@gmail.com" {
		t.Errorf("expected email 'onerole@gmail.com', got %v", res["email"])
	}

	cookies := rec.Result().Cookies()
	var foundRefresh, foundCSRF bool
	for _, c := range cookies {
		if c.Name == refreshCookieName {
			foundRefresh = true
		}
		if c.Name == csrfCookieName {
			foundCSRF = true
		}
	}
	if !foundRefresh || !foundCSRF {
		t.Errorf("expected refresh and csrf cookies, got: %#v", cookies)
	}
}

func TestAuthHTTPRoutesUseFixedRedirectCookiesAndCSRF(t *testing.T) {
	handler, _, authAPI := newTestHTTPHandler(t)
	start := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/start", nil)
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, start)
	if startResponse.Code != http.StatusFound || startResponse.Header().Get("Location") != authAPI.start.AuthorizationURL || authAPI.startCalls != 1 {
		t.Fatalf("OAuth start failed: status=%d location=%q", startResponse.Code, startResponse.Header().Get("Location"))
	}
	bindingCookie := startResponse.Result().Cookies()[0]
	if bindingCookie.Name != oauthBindingCookieName || !bindingCookie.HttpOnly || !bindingCookie.Secure || bindingCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe OAuth binding cookie: %#v", bindingCookie)
	}

	callback := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/google/callback?state=state&code=code", nil)
	callback.AddCookie(&http.Cookie{Name: oauthBindingCookieName, Value: "binding"})
	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, callback)
	if callbackResponse.Code != http.StatusFound || callbackResponse.Header().Get("Location") != authAPI.complete.RedirectURL || authAPI.completeCalls != 1 {
		t.Fatalf("OAuth callback failed: status=%d location=%q", callbackResponse.Code, callbackResponse.Header().Get("Location"))
	}
	cookies := callbackResponse.Result().Cookies()
	if len(cookies) < 3 {
		t.Fatalf("expected binding clear, refresh, and CSRF cookies: %#v", cookies)
	}
	var sawRefresh, sawCSRF bool
	for _, cookie := range cookies {
		if cookie.Name == refreshCookieName {
			sawRefresh = cookie.HttpOnly && cookie.Secure && cookie.SameSite == http.SameSiteStrictMode && cookie.Path == authCookiePath
		}
		if cookie.Name == csrfCookieName {
			sawCSRF = !cookie.HttpOnly && cookie.Secure && cookie.SameSite == http.SameSiteStrictMode && cookie.Path == csrfCookiePath
		}
	}
	if !sawRefresh || !sawCSRF {
		t.Fatal("refresh/CSRF cookies must have the expected security attributes")
	}

	refresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refresh.Header.Set("Origin", "https://app.onerole.test")
	refresh.Header.Set("X-CSRF-Token", "signed-csrf")
	refresh.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "signed-csrf"})
	refresh.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-refresh"})
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK || authAPI.refreshCalls != 1 || !strings.Contains(refreshResponse.Body.String(), "access-token") {
		t.Fatalf("refresh failed: status=%d body=%s", refreshResponse.Code, refreshResponse.Body.String())
	}

	invalidCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	invalidCSRF.Header.Set("Origin", "https://attacker.test")
	invalidCSRF.Header.Set("X-CSRF-Token", "wrong")
	invalidCSRF.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "signed-csrf"})
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalidCSRF)
	if invalidResponse.Code != http.StatusForbidden || authAPI.refreshCalls != 1 {
		t.Fatalf("CSRF failure must stop refresh: status=%d", invalidResponse.Code)
	}

	me := authenticatedRequest(t, http.MethodGet, "/api/v1/auth/me", nil)
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusOK || !strings.Contains(meResponse.Body.String(), testUserID) {
		t.Fatalf("current-user endpoint failed: status=%d body=%s", meResponse.Code, meResponse.Body.String())
	}

	logout := authenticatedRequest(t, http.MethodPost, "/api/v1/auth/logout", nil)
	logout.Header.Set("Origin", "https://app.onerole.test")
	logout.Header.Set("X-CSRF-Token", "signed-csrf")
	logout.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "signed-csrf"})
	logout.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "rotated-refresh"})
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent || authAPI.logoutID != "44444444-4444-4444-8444-444444444444" {
		t.Fatalf("logout did not revoke the authenticated session: status=%d session=%q", logoutResponse.Code, authAPI.logoutID)
	}
	cleared := map[string]bool{}
	for _, cookie := range logoutResponse.Result().Cookies() {
		if cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
		if cookie.Name == csrfCookieName && cookie.MaxAge < 0 && cookie.Path != csrfCookiePath {
			t.Fatalf("logout must clear the root-scoped CSRF cookie: %#v", cookie)
		}
	}
	if !cleared[refreshCookieName] || !cleared[csrfCookieName] {
		t.Fatalf("logout must clear the refresh and CSRF cookies: %#v", logoutResponse.Result().Cookies())
	}
}

func newTestHTTPHandler(t *testing.T) (http.Handler, *applicationAPIFake, *authAPIFake) {
	t.Helper()
	if testHTTPPrivateKey == nil {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		testHTTPPrivateKey = privateKey
	}
	now := time.Now().UTC()
	jwtVerifier, err := middleware.NewJWTVerifier("https://auth.onerole.test", "onerole-api", map[string]*rsa.PublicKey{"key-1": &testHTTPPrivateKey.PublicKey}, httpSessionStoreFake{
		session: service.AuthSession{ID: "44444444-4444-4444-8444-444444444444", UserID: testUserID, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	authAPI := &authAPIFake{
		start: service.OAuthStartResult{
			AuthorizationURL:     "https://accounts.google.com/o/oauth2/v2/auth?state=state",
			BrowserBindingSecret: "browser-binding", ExpiresAt: now.Add(5 * time.Minute),
		},
		complete: service.OAuthCallbackResult{
			RefreshToken: "new-refresh", RefreshExpiresAt: now.Add(time.Hour), CSRFToken: "signed-csrf",
			RedirectURL: "https://app.onerole.test/auth/complete",
		},
		refresh: service.RefreshResult{AccessToken: "access-token", ExpiresIn: 600, RefreshToken: "rotated-refresh", RefreshExpiresAt: now.Add(time.Hour)},
		current: service.CurrentUser{UserID: testUserID, DisplayName: "One User"}, csrfValid: true,
	}
	appAPI := &applicationAPIFake{}
	handler, err := NewHTTPHandler(HTTPDependencies{
		Auth: authAPI, Applications: appAPI, JWT: jwtVerifier,
		Memberships: httpMembershipReaderFake{membership: service.TenantMembership{
			TenantID: testTenantID, UserID: testUserID, Role: "member", Status: "active",
		}},
		AuthConfig:     AuthHTTPConfig{SecureCookies: true, FrontendRedirectURL: "https://app.onerole.test/auth/complete"},
		AllowedOrigins: []string{"https://app.onerole.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, appAPI, authAPI
}

func authenticatedRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	return newRequestWithTestKey(t, method, target, body)
}

var testHTTPPrivateKey *rsa.PrivateKey

func newRequestWithTestKey(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	if testHTTPPrivateKey == nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		testHTTPPrivateKey = key
	}
	return signedRequestWithKey(t, method, target, body, testHTTPPrivateKey)
}

func signedRequestWithKey(t *testing.T, method, target string, body io.Reader, key *rsa.PrivateKey) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, body)
	now := time.Now().UTC()
	claims := middleware.AccessClaims{
		SessionID: "44444444-4444-4444-8444-444444444444", TokenUse: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "https://auth.onerole.test", Subject: testUserID, Audience: jwt.ClaimStrings{"onerole-api"},
			IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "key-1"
	encoded, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+encoded)
	return request
}

type profileAPIFake struct {
	profile models.CandidateProfile
}

func (p *profileAPIFake) GetProfile(context.Context, service.ActorScope) (*models.CandidateProfile, error) {
	return &p.profile, nil
}

func (p *profileAPIFake) UpdateProfile(_ context.Context, _ service.ActorScope, update models.CandidateProfile) (*models.CandidateProfile, error) {
	p.profile = update
	return &p.profile, nil
}

type autoApplyAPIFake struct {
	result          *service.AutoApplyResult
	batchResult     *service.AutoApplyBatchResult
	recommendations []service.ScoredJob
}

func (a *autoApplyAPIFake) ApplyOneClick(context.Context, service.ActorScope, service.AutoApplyCommand) (*service.AutoApplyResult, error) {
	return a.result, nil
}

func (a *autoApplyAPIFake) ApplyBatchOneClick(context.Context, service.ActorScope, service.AutoApplyBatchCommand) (*service.AutoApplyBatchResult, error) {
	return a.batchResult, nil
}

func (a *autoApplyAPIFake) GetJobRecommendations(context.Context, service.ActorScope, int, int) ([]service.ScoredJob, error) {
	return a.recommendations, nil
}

func (a *autoApplyAPIFake) GetApplicationResume(context.Context, service.ActorScope, string) (*service.ApplicationResumeView, error) {
	return &service.ApplicationResumeView{
		ApplicationID:   "app-test",
		JobID:           "job-test",
		ResumeVersionID: "rv-test",
		Tailored:        true,
		ContentText:     "test resume",
		RenderedHTML:    "<html><body>test resume</body></html>",
	}, nil
}

func (a *autoApplyAPIFake) AssistApplication(context.Context, service.ActorScope, string) (*service.AutoApplyResult, error) {
	return a.result, nil
}

func (a *autoApplyAPIFake) SendNotificationEmail(context.Context, service.EmailPayload) (*service.EmailDeliveryResult, error) {
	return &service.EmailDeliveryResult{Success: true, Provider: "mock", DeliveredAt: time.Now().UTC()}, nil
}

func (a *autoApplyAPIFake) GetMailerStatus() map[string]any {
	return map[string]any{"active_provider": "mock", "is_ready": true}
}

func TestProfileAndAutoApplyAndCatalogRoutes(t *testing.T) {
	now := time.Now().UTC()
	if testHTTPPrivateKey == nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		testHTTPPrivateKey = key
	}
	jwtVerifier, err := middleware.NewJWTVerifier("https://auth.onerole.test", "onerole-api", map[string]*rsa.PublicKey{"key-1": &testHTTPPrivateKey.PublicKey}, httpSessionStoreFake{
		session: service.AuthSession{ID: "44444444-4444-4444-8444-444444444444", UserID: testUserID, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)},
	})
	if err != nil {
		t.Fatal(err)
	}
	authAPI := &authAPIFake{}
	appAPI := &applicationAPIFake{}
	catAPI := &catalogAPIFake{}
	profAPI := &profileAPIFake{
		profile: models.CandidateProfile{
			UserID:    testUserID,
			TenantID:  testTenantID,
			FirstName: "Test",
			LastName:  "User",
			Email:     "onerole@gmail.com",
		},
	}
	autoAPI := &autoApplyAPIFake{
		result: &service.AutoApplyResult{
			ApplicationID: testAppID,
			JobID:         testJobID,
			Company:       "Accenture",
			JobTitle:      "Software Engineer",
			ReceiptID:     "APP-RECEIPT-001",
			Status:        "submitted",
			SubmittedAt:   now,
			AnswersCount:  10,
		},
		batchResult: &service.AutoApplyBatchResult{
			TotalSubmitted: 5,
			TotalFailed:    0,
			DurationMs:     45,
		},
	}

	handler, err := NewHTTPHandler(HTTPDependencies{
		Auth: authAPI, Applications: appAPI, Profile: profAPI, AutoApply: autoAPI,
		JWT: jwtVerifier, Catalog: catAPI,
		Memberships: httpMembershipReaderFake{membership: service.TenantMembership{
			TenantID: testTenantID, UserID: testUserID, Role: "member", Status: "active",
		}},
		AuthConfig:     AuthHTTPConfig{SecureCookies: true, FrontendRedirectURL: "https://app.onerole.test/auth/complete"},
		AllowedOrigins: []string{"https://app.onerole.test"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Candidate Profile GET
	req := authenticatedRequest(t, http.MethodGet, "/api/v1/profile", nil)
	req.Header.Set("X-Tenant-ID", testTenantID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/profile expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Candidate Profile PATCH
	patchBody := `{"first_name":"Jane","years_of_experience":8}`
	req = authenticatedRequest(t, http.MethodPatch, "/api/v1/profile", strings.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/v1/profile expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. 1-Click AutoApply POST
	applyBody := `{"job_id":"` + testJobID + `"}`
	req = authenticatedRequest(t, http.MethodPost, "/api/v1/applications/auto-apply", strings.NewReader(applyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/applications/auto-apply expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3b. 1-Click AutoApply POST with UI submission_mode payload
	uiPayload := `{"job_id":"` + testJobID + `","async":true,"tailor_resume":true,"submission_mode":"submit_when_ready"}`
	req = authenticatedRequest(t, http.MethodPost, "/api/v1/applications/auto-apply", strings.NewReader(uiPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/applications/auto-apply with submission_mode expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Batch 1-Click AutoApply POST
	batchBody := `{"job_ids":["` + testJobID + `"]}`
	req = authenticatedRequest(t, http.MethodPost, "/api/v1/applications/auto-apply/batch", strings.NewReader(batchBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant-ID", testTenantID)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/applications/auto-apply/batch expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Recent Jobs GET
	req = authenticatedRequest(t, http.MethodGet, "/api/v1/jobs/recent", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/jobs/recent expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Sources GET
	req = authenticatedRequest(t, http.MethodGet, "/api/v1/sources", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sources expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 7. Source Health GET
	req = authenticatedRequest(t, http.MethodGet, "/api/v1/sources/src-123/health", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sources/{id}/health expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRouterSwaggerRoutes(t *testing.T) {
	handler, _, _ := newTestHTTPHandler(t)

	swaggerPaths := []string{
		"/docs",
		"/swagger",
		"/swagger/",
	}

	for _, path := range swaggerPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d", path, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s expected text/html, got %s", path, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
			t.Errorf("GET %s missing SwaggerUIBundle in body", path)
		}
	}

	yamlPaths := []string{
		"/openapi.yaml",
		"/api/v1/openapi.yaml",
	}

	for _, path := range yamlPaths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d", path, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "yaml") {
			t.Errorf("GET %s expected yaml content type, got %s", path, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Body.String(), "openapi: 3.") {
			t.Errorf("GET %s missing openapi version in body", path)
		}
	}
}

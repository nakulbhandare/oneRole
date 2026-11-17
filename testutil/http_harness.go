// Package testutil provides isolated in-memory dependencies for HTTP contract tests.
// Its records exist only inside tests and are never wired into the application runtime.
package testutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oneRole/controller"
	"github.com/oneRole/middleware"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

const (
	TenantID  = "11111111-1111-4111-8111-111111111111"
	UserID    = "22222222-2222-4222-8222-222222222222"
	JobID     = "33333333-3333-4333-8333-333333333333"
	AppID     = "44444444-4444-4444-8444-444444444444"
	ResumeID  = "55555555-5555-4555-8555-555555555555"
	ConsentID = "66666666-6666-4666-8666-666666666666"
	BatchID   = "77777777-7777-4777-8777-777777777777"
	SessionID = "88888888-8888-4888-8888-888888888888"
)

const (
	issuer   = "https://auth.onerole.test"
	audience = "onerole-api"
	keyID    = "test-key"
)

type HTTPHarness struct {
	Handler      http.Handler
	PrivateKey   *rsa.PrivateKey
	Catalog      *CatalogMemory
	Applications *ApplicationMemory
}

func NewHTTPHarness(t *testing.T) *HTTPHarness {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	sessionStore := sessionMemory{session: service.AuthSession{
		ID: SessionID, UserID: UserID, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour),
	}}
	verifier, err := middleware.NewJWTVerifier(issuer, audience, map[string]*rsa.PublicKey{keyID: &privateKey.PublicKey}, sessionStore)
	if err != nil {
		t.Fatal(err)
	}
	catalogStore := &CatalogMemory{Job: sampleJob(now)}
	catalog, err := service.NewCatalogService(catalogStore, catalogStore, catalogStore, catalogStore, catalogStore)
	if err != nil {
		t.Fatal(err)
	}
	applicationStore := &ApplicationMemory{JobID: JobID}
	applications, err := service.NewApplicationService(applicationStore, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := controller.NewHTTPHandler(controller.HTTPDependencies{
		Auth: authMemory{}, Applications: applications, JWT: verifier,
		Memberships:    membershipMemory{},
		AllowedOrigins: []string{"http://localhost:8081"},
		AuthConfig:     controller.AuthHTTPConfig{SecureCookies: false, FrontendRedirectURL: "http://localhost:8081/auth/complete"},
		Catalog:        catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &HTTPHarness{Handler: handler, PrivateKey: privateKey, Catalog: catalogStore, Applications: applicationStore}
}

func (h *HTTPHarness) SignedRequest(t *testing.T, method, target string, body []byte) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	claims := middleware.AccessClaims{
		SessionID: SessionID,
		TokenUse:  "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: UserID, Audience: jwt.ClaimStrings{audience},
			IssuedAt: jwt.NewNumericDate(now.Add(-time.Minute)), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	encoded, err := token.SignedString(h.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+encoded)
	return request
}

// CatalogMemory is an isolated test double for the catalog persistence ports.
type CatalogMemory struct {
	Job                 models.Job
	SearchRequest       models.JobSearchRequest
	SearchCalls         int
	GetJobCalls         int
	LastJobID           string
	CompanyQuery        string
	CompanyLimit        int
	LocationQuery       string
	LocationCountry     string
	LocationLimit       int
	FilterMetadataCalls int
}

func (c *CatalogMemory) SearchJobs(_ context.Context, request models.JobSearchRequest) (models.JobSearchResponse, error) {
	c.SearchCalls++
	c.SearchRequest = request
	return models.JobSearchResponse{Items: []models.Job{c.Job}, AppliedFilters: request, Facets: models.FacetSet{
		"workplace": {Options: []models.FacetOption{{Value: "remote", Label: "Remote", Count: 1}}, UnknownCount: 2},
	}}, nil
}
func (c *CatalogMemory) GetJob(_ context.Context, id string) (models.Job, error) {
	c.GetJobCalls++
	c.LastJobID = id
	if id != c.Job.ID {
		return models.Job{}, service.ErrCatalogItemNotFound
	}
	return c.Job, nil
}
func (c *CatalogMemory) SearchCompanies(_ context.Context, query string, _ *string, limit int) (service.CompanySearchResponse, error) {
	c.CompanyQuery, c.CompanyLimit = query, limit
	return service.CompanySearchResponse{Items: []service.CompanySuggestion{{ID: "99999999-9999-4999-8999-999999999999", Name: "Test Employer"}}}, nil
}
func (c *CatalogMemory) SuggestLocations(_ context.Context, query, country string, limit int) (service.LocationSuggestions, error) {
	c.LocationQuery, c.LocationCountry, c.LocationLimit = query, country, limit
	city, region := "Seattle", "Washington"
	return service.LocationSuggestions{Items: []service.LocationSuggestion{{ID: "normalized:seattle", Label: "Seattle, Washington, US", City: &city, Region: &region, CountryCode: "US", MatchingJobCount: 4}}}, nil
}
func (c *CatalogMemory) GetFilterMetadata(context.Context) (service.FilterMetadata, error) {
	c.FilterMetadataCalls++
	return service.FilterMetadata{Facets: models.FacetSet{
		"company": {Options: []models.FacetOption{{Value: "99999999-9999-4999-8999-999999999999", Label: "Test Employer", Count: 1}}, UnknownCount: 3},
	}}, nil
}

// ApplicationMemory is an isolated transactional-port stand-in. Tests can
// inspect the scope and command that reached the service boundary.
type ApplicationMemory struct {
	JobID        string
	Scope        service.ActorScope
	Command      service.CreateApplicationCommand
	BatchCommand service.CreateBatchCommand
	CreateCalls  int
	SubmitCalls  int
	BatchCalls   int
	ReadCalls    int
	BatchReads   int
	Idempotency  service.IdempotencyIdentity
}

func (a *ApplicationMemory) AcceptApplication(_ context.Context, scope service.ActorScope, command service.CreateApplicationCommand, idem service.IdempotencyIdentity, now time.Time) (service.WorkflowAcceptance, error) {
	a.Scope, a.Command, a.Idempotency = scope, command, idem
	a.CreateCalls++
	return service.WorkflowAcceptance{WorkflowID: AppID, Status: "preparing", AcceptedAt: now}, nil
}
func (a *ApplicationMemory) AcceptSubmission(_ context.Context, scope service.ActorScope, _ string, idem service.IdempotencyIdentity, now time.Time) (service.WorkflowAcceptance, error) {
	a.Scope, a.Idempotency = scope, idem
	a.SubmitCalls++
	return service.WorkflowAcceptance{WorkflowID: AppID, Status: "queued", AcceptedAt: now}, nil
}
func (a *ApplicationMemory) AcceptBatch(_ context.Context, scope service.ActorScope, command service.CreateBatchCommand, idem service.IdempotencyIdentity, now time.Time) (service.WorkflowAcceptance, error) {
	a.Scope, a.BatchCommand, a.Idempotency = scope, command, idem
	a.BatchCalls++
	return service.WorkflowAcceptance{WorkflowID: BatchID, Status: "accepted", AcceptedAt: now}, nil
}
func (a *ApplicationMemory) GetApplication(_ context.Context, scope service.ActorScope, id string) (models.Application, error) {
	a.Scope = scope
	a.ReadCalls++
	if id != AppID {
		return models.Application{}, service.ErrApplicationNotFound
	}
	return models.Application{
		ID: AppID, JobID: a.JobID, Status: models.PublicApplicationPreparing,
		Capability: models.CapabilityAssistedSupported, UpdatedAt: time.Now().UTC(),
	}, nil
}
func (a *ApplicationMemory) GetBatch(_ context.Context, scope service.ActorScope, id string) (service.BatchView, error) {
	a.Scope = scope
	a.BatchReads++
	if id != BatchID {
		return service.BatchView{}, service.ErrApplicationNotFound
	}
	return service.BatchView{ID: BatchID, Total: 2, Counts: map[string]int{"accepted": 2}, CreatedAt: time.Now().UTC()}, nil
}
func (a *ApplicationMemory) ListApplications(_ context.Context, scope service.ActorScope, _ *string, _ string, _ int) ([]models.Application, *string, error) {
	a.Scope = scope
	return []models.Application{{
		ID: AppID, JobID: a.JobID, Status: models.PublicApplicationPreparing,
		Capability: models.CapabilityAssistedSupported, UpdatedAt: time.Now().UTC(),
	}}, nil, nil
}
func (a *ApplicationMemory) GetApplicationTimeline(_ context.Context, scope service.ActorScope, id string) ([]models.ApplicationEvent, error) {
	a.Scope = scope
	return []models.ApplicationEvent{{
		ID: "evt-test-1", TenantID: scope.TenantID, ApplicationID: id,
		EventType: "application.created", ToStatus: "preparing", OccurredAt: time.Now().UTC(),
	}}, nil
}
func (a *ApplicationMemory) GetApplicationReceipt(_ context.Context, scope service.ActorScope, id string) (*models.ApplicationReceipt, error) {
	a.Scope = scope
	return &models.ApplicationReceipt{
		ApplicationID: id, ReceiptID: "RCPT-TEST-1", Status: "submitted", SubmittedAt: time.Now().UTC(),
	}, nil
}

type sessionMemory struct{ session service.AuthSession }

func (sessionMemory) Create(context.Context, service.AuthSession) error { return nil }
func (sessionMemory) Rotate(context.Context, service.RefreshRotation) (service.AuthSession, error) {
	return service.AuthSession{}, service.ErrAuthSessionUnavailable
}
func (s sessionMemory) FindActive(_ context.Context, id string, now time.Time) (service.AuthSession, error) {
	if id != s.session.ID || !s.session.IdleExpiresAt.After(now) || !s.session.AbsoluteExpiresAt.After(now) {
		return service.AuthSession{}, service.ErrAuthSessionUnavailable
	}
	return s.session, nil
}
func (sessionMemory) Revoke(context.Context, string, time.Time, string) error { return nil }

type membershipMemory struct{}

func (membershipMemory) ListActive(context.Context, string) ([]service.TenantMembership, error) {
	return []service.TenantMembership{{TenantID: TenantID, UserID: UserID, Role: "member", Status: "active"}}, nil
}
func (membershipMemory) FindActive(_ context.Context, userID, tenantID string) (service.TenantMembership, error) {
	if userID != UserID || tenantID != TenantID {
		return service.TenantMembership{}, service.ErrTenantMembershipNotFound
	}
	return service.TenantMembership{TenantID: TenantID, UserID: UserID, Role: "member", Status: "active"}, nil
}

type authMemory struct{}

func (authMemory) Providers() []service.OAuthProviderStatus {
	return []service.OAuthProviderStatus{
		{ID: service.OAuthProviderGoogle, Label: "Continue with Google", Enabled: false},
		{ID: service.OAuthProviderGitHub, Label: "Continue with GitHub", Enabled: false},
	}
}
func (authMemory) StartOAuth(context.Context, service.OAuthProvider, time.Time) (service.OAuthStartResult, error) {
	return service.OAuthStartResult{}, service.ErrOAuthProviderUnavailable
}
func (authMemory) CompleteOAuth(context.Context, service.OAuthProvider, string, string, string, string, time.Time) (service.OAuthCallbackResult, error) {
	return service.OAuthCallbackResult{}, service.ErrOAuthProviderUnavailable
}
func (authMemory) Refresh(context.Context, string, time.Time) (service.RefreshResult, error) {
	return service.RefreshResult{}, service.ErrAuthSessionUnavailable
}
func (authMemory) Logout(context.Context, string, time.Time) error { return nil }
func (authMemory) CurrentUser(context.Context, string) (service.CurrentUser, error) {
	return service.CurrentUser{UserID: UserID, DisplayName: "Test User", Memberships: []service.TenantMembership{{TenantID: TenantID, UserID: UserID, Role: "member", Status: "active"}}}, nil
}
func (authMemory) ValidCSRF(origin, cookie, header string) bool {
	return origin == "http://localhost:8081" && cookie != "" && cookie == header
}
func (authMemory) DevLogin(_ context.Context, email, name string, _ time.Time) (service.DevLoginResult, error) {
	if email == "" {
		email = "onerole@gmail.com"
	}
	if name == "" {
		name = "OneRole Candidate"
	}
	return service.DevLoginResult{
		AccessToken:  "test-access-token",
		TokenType:    "Bearer",
		ExpiresIn:    600,
		UserID:       UserID,
		Email:        email,
		DisplayName:  name,
		TenantID:     TenantID,
		RefreshToken: "test-refresh-token",
		CSRFToken:    "test-csrf-token",
	}, nil
}

func sampleJob(now time.Time) models.Job {
	companyID := "99999999-9999-4999-8999-999999999999"
	country := "US"
	jobType := models.JobSenior
	return models.Job{
		ID: JobID, CompanyID: &companyID, Company: "Test Employer", Title: "Platform Engineer",
		Description: "Build and maintain platform systems.", Location: "Remote, United States", CountryCode: &country,
		WorkplaceModes: []models.WorkplaceMode{models.WorkplaceRemote}, RoleIDs: []string{"platform_engineering"},
		JobType: &jobType, EmploymentTypes: []models.EmploymentType{models.EmploymentFullTime},
		DegreeLevels: []models.DegreeLevel{models.DegreeBachelor}, VisaSponsorship: models.VisaUnknown,
		FilterProvenance: map[string]models.FilterProvenance{}, PublicationTimeQuality: models.PublicationUnknown,
		FirstSeenAt: now.Add(-time.Hour), LastSeenAt: now, ApplicationCapability: models.CapabilityAssistedSupported,
		SourceURL: "https://jobs.example.test/platform-engineer",
	}
}

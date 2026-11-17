package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/oneRole/models"
	"github.com/oneRole/service"
	"github.com/oneRole/testutil"
)

func TestProviderListingShowsOnlyConfiguredSignInOptions(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	response := httptest.NewRecorder()
	harness.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("provider list status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Providers []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	if len(result.Providers) != 2 {
		t.Fatalf("provider list should expose the two supported providers: %#v", result.Providers)
	}
	for _, provider := range result.Providers {
		if provider.Enabled {
			t.Fatalf("unconfigured provider %q must not be presented as available", provider.ID)
		}
	}
}

func TestBrowserRequestsUseConfiguredExactOrigin(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	request.Header.Set("Origin", "http://localhost:8081")
	response := httptest.NewRecorder()
	harness.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8081" || response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("configured browser origin was not authorized: status=%d headers=%v", response.Code, response.Header())
	}

	untrusted := httptest.NewRequest(http.MethodGet, "/api/v1/auth/providers", nil)
	untrusted.Header.Set("Origin", "https://attacker.example")
	untrustedResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(untrustedResponse, untrusted)
	if untrustedResponse.Code != http.StatusForbidden || untrustedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unconfigured origin must not be reflected: status=%d headers=%v", untrustedResponse.Code, untrustedResponse.Header())
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/applications", nil)
	preflight.Header.Set("Origin", "http://localhost:8081")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflight.Header.Set("Access-Control-Request-Headers", "authorization,content-type,idempotency-key,x-tenant-id,x-csrf-token")
	preflightResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent || preflightResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8081" {
		t.Fatalf("browser preflight for documented application headers failed: status=%d headers=%v", preflightResponse.Code, preflightResponse.Header())
	}
}

func TestCSRFBootstrapRouteReturnsNoStoreTokenForSameSiteFrontend(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf", nil)
	request.Header.Set("Origin", "http://localhost:8081")
	request.AddCookie(&http.Cookie{Name: "onerole_csrf", Value: strings.Repeat("s", 64)})
	response := httptest.NewRecorder()
	harness.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("CSRF bootstrap response must be successful and uncached: status=%d headers=%v", response.Code, response.Header())
	}
	var result struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.CSRFToken != strings.Repeat("s", 64) {
		t.Fatalf("CSRF bootstrap did not return the signed cookie token: %#v err=%v", result, err)
	}
}

func TestCatalogSearchContractAcrossRouterJWTAndService(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)
	body := `{"query":"platform engineer","exclude_terms":["junior","staffing agency"],"date":{"preset":"last_60_days","include_unknown_publication_time":true},"include_unknown_location":true,"include_unknown_company":true,"include_unknown_degree":true,"include_unknown_workplace":true,"include_unknown_role":true,"include_unknown_job_type":true,"include_unknown_employment_type":true,"locations":[{"location_id":"normalized:seattle","radius_km":25}],"workplace_modes":["remote","hybrid"],"company_ids":["99999999-9999-4999-8999-999999999999"],"excluded_company_ids":["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"],"degree_levels":["bachelor","master"],"degree_requirement":"preferred","max_experience_years":8,"include_unknown_experience":true,"visa_sponsorship":"yes","role_ids":["platform_engineering"],"job_types":["senior"],"employment_types":["full_time"],"sort":"published_desc","limit":10}`
	request := harness.SignedRequest(t, http.MethodPost, "/api/v1/jobs/search", []byte(body))
	request.Header.Set("Content-Type", "application/json")
	// Catalog search is authenticated but global, so it intentionally has no tenant header.
	response := httptest.NewRecorder()
	harness.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", response.Code, response.Body.String())
	}

	var result models.JobSearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != testutil.JobID || result.Items[0].Title != "Platform Engineer" {
		t.Fatalf("search did not return the isolated store record: %#v", result.Items)
	}
	if result.AppliedFilters.Query != "platform engineer" || len(result.AppliedFilters.WorkplaceModes) != 2 || result.AppliedFilters.WorkplaceModes[0] != models.WorkplaceRemote ||
		len(result.AppliedFilters.ExcludeTerms) != 2 || len(result.AppliedFilters.Locations) != 1 || result.AppliedFilters.Locations[0].RadiusKM == nil || *result.AppliedFilters.Locations[0].RadiusKM != 25 ||
		len(result.AppliedFilters.CompanyIDs) != 1 || len(result.AppliedFilters.ExcludedCompanyIDs) != 1 || result.AppliedFilters.DegreeRequirement != models.DegreePreferred ||
		result.AppliedFilters.MaxExperienceYears == nil || *result.AppliedFilters.MaxExperienceYears != 8 ||
		result.AppliedFilters.VisaSponsorship != models.VisaYes || len(result.AppliedFilters.RoleIDs) != 1 || len(result.AppliedFilters.JobTypes) != 1 ||
		len(result.AppliedFilters.EmploymentTypes) != 1 || !result.AppliedFilters.IncludeUnknownLocation ||
		!result.AppliedFilters.IncludeUnknownCompany || !result.AppliedFilters.IncludeUnknownDegree ||
		!result.AppliedFilters.IncludeUnknownWorkplace || !result.AppliedFilters.IncludeUnknownRole ||
		!result.AppliedFilters.IncludeUnknownJobType || !result.AppliedFilters.IncludeUnknownEmploymentType ||
		!result.AppliedFilters.IncludeUnknownExperience || !result.AppliedFilters.Date.IncludeUnknownPublicationTime {
		t.Fatalf("normalized API filters did not reach the response: %#v", result.AppliedFilters)
	}
	if harness.Catalog.SearchCalls != 1 || harness.Catalog.SearchRequest.Sort != models.SortPublishedDesc || harness.Catalog.SearchRequest.Limit != 10 {
		t.Fatalf("router/service/store contract mismatch: %#v", harness.Catalog)
	}

	// The linkable GET route keeps repeated multi-select values and the q alias.
	queryRequest := harness.SignedRequest(t, http.MethodGet, "/api/v1/jobs/search?q=platform&workplace=remote&workplace=hybrid", nil)
	queryResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(queryResponse, queryRequest)
	if queryResponse.Code != http.StatusOK || harness.Catalog.SearchCalls != 2 {
		t.Fatalf("GET search route: status=%d body=%s", queryResponse.Code, queryResponse.Body.String())
	}
	if harness.Catalog.SearchRequest.Query != "platform" || len(harness.Catalog.SearchRequest.WorkplaceModes) != 2 {
		t.Fatalf("repeated query filters were not preserved: %#v", harness.Catalog.SearchRequest)
	}

	// Body-supplied tenant identity is rejected instead of becoming a trusted scope.
	injected := harness.SignedRequest(t, http.MethodPost, "/api/v1/jobs/search", []byte(`{"query":"platform","tenant_id":"`+testutil.TenantID+`"}`))
	injected.Header.Set("Content-Type", "application/json")
	injectedResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(injectedResponse, injected)
	if injectedResponse.Code != http.StatusBadRequest || harness.Catalog.SearchCalls != 2 {
		t.Fatalf("unknown tenant field must be rejected before catalog access: status=%d calls=%d", injectedResponse.Code, harness.Catalog.SearchCalls)
	}

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/search", strings.NewReader(body))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized || harness.Catalog.SearchCalls != 2 {
		t.Fatalf("unauthenticated catalog request must not reach store: status=%d calls=%d", unauthorizedResponse.Code, harness.Catalog.SearchCalls)
	}
}

func TestCatalogSuggestionAndFacetRoutesUseAuthenticatedCatalogPorts(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)

	companyRequest := harness.SignedRequest(t, http.MethodGet, "/api/v1/companies/search?q=Acme&limit=8", nil)
	companyResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(companyResponse, companyRequest)
	if companyResponse.Code != http.StatusOK || !strings.Contains(companyResponse.Body.String(), `"id":"99999999-9999-4999-8999-999999999999"`) ||
		harness.Catalog.CompanyQuery != "Acme" || harness.Catalog.CompanyLimit != 8 {
		t.Fatalf("company suggestion route contract mismatch: status=%d body=%s catalog=%#v", companyResponse.Code, companyResponse.Body.String(), harness.Catalog)
	}

	locationRequest := harness.SignedRequest(t, http.MethodGet, "/api/v1/locations/suggest?q=Seattle&country_code=US&limit=6", nil)
	locationResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(locationResponse, locationRequest)
	if locationResponse.Code != http.StatusOK || !strings.Contains(locationResponse.Body.String(), `"matching_job_count":4`) ||
		harness.Catalog.LocationQuery != "Seattle" || harness.Catalog.LocationCountry != "US" || harness.Catalog.LocationLimit != 6 {
		t.Fatalf("location suggestion route contract mismatch: status=%d body=%s catalog=%#v", locationResponse.Code, locationResponse.Body.String(), harness.Catalog)
	}

	metadataRequest := harness.SignedRequest(t, http.MethodGet, "/api/v1/jobs/filters", nil)
	metadataResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(metadataResponse, metadataRequest)
	if metadataResponse.Code != http.StatusOK || !strings.Contains(metadataResponse.Body.String(), `"unknown_count":3`) || harness.Catalog.FilterMetadataCalls != 1 {
		t.Fatalf("filter metadata route mismatch: status=%d body=%s catalog=%#v", metadataResponse.Code, metadataResponse.Body.String(), harness.Catalog)
	}

	contextRequest := harness.SignedRequest(t, http.MethodPost, "/api/v1/jobs/filters", []byte(`{"query":"platform","workplace_modes":["remote"],"include_unknown_workplace":true}`))
	contextRequest.Header.Set("Content-Type", "application/json")
	contextResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(contextResponse, contextRequest)
	if contextResponse.Code != http.StatusOK || !strings.Contains(contextResponse.Body.String(), `"unknown_count":2`) ||
		harness.Catalog.SearchCalls != 1 || harness.Catalog.SearchRequest.Query != "platform" || !harness.Catalog.SearchRequest.IncludeUnknownWorkplace {
		t.Fatalf("contextual filter facet route mismatch: status=%d body=%s catalog=%#v", contextResponse.Code, contextResponse.Body.String(), harness.Catalog)
	}
}

func TestApplicationWorkflowContractUsesCurrentTenantAndHonestAcceptance(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)
	body := `{"job_id":"` + testutil.JobID + `","resume_version_id":"` + testutil.ResumeID + `","consent_id":"` + testutil.ConsentID + `","mode":"review_before_submit"}`

	missingTenant := harness.SignedRequest(t, http.MethodPost, "/api/v1/applications", []byte(body))
	missingTenant.Header.Set("Content-Type", "application/json")
	missingTenant.Header.Set("Idempotency-Key", "contract-create-key-001")
	missingTenantResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(missingTenantResponse, missingTenant)
	if missingTenantResponse.Code != http.StatusBadRequest || harness.Applications.CreateCalls != 0 {
		t.Fatalf("application creation without X-Tenant-ID must be rejected: status=%d calls=%d", missingTenantResponse.Code, harness.Applications.CreateCalls)
	}

	create := harness.SignedRequest(t, http.MethodPost, "/api/v1/applications", []byte(body))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Idempotency-Key", "contract-create-key-002")
	create.Header.Set("X-Tenant-ID", testutil.TenantID)
	created := httptest.NewRecorder()
	harness.Handler.ServeHTTP(created, create)
	if created.Code != http.StatusAccepted || harness.Applications.CreateCalls != 1 {
		t.Fatalf("application create status=%d body=%s", created.Code, created.Body.String())
	}

	var accepted struct {
		WorkflowID string `json:"workflow_id"`
		Status     string `json:"status"`
		StatusURL  string `json:"status_url"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted response: %v", err)
	}
	if accepted.WorkflowID != testutil.AppID || accepted.Status != "preparing" || accepted.StatusURL != "/api/v1/applications/"+testutil.AppID {
		t.Fatalf("unexpected durable acceptance response: %#v", accepted)
	}
	if accepted.Message != "Accepted by OneRole; employer submission is not yet confirmed." {
		t.Fatalf("acceptance must not claim employer success: %q", accepted.Message)
	}
	if harness.Applications.Scope != (service.ActorScope{TenantID: testutil.TenantID, UserID: testutil.UserID}) {
		t.Fatalf("actor scope must come from the JWT and current membership: %#v", harness.Applications.Scope)
	}
	if harness.Applications.Command.JobID != testutil.JobID || harness.Applications.Idempotency.KeyHash == ([32]byte{}) || harness.Applications.Idempotency.RequestHash == ([32]byte{}) {
		t.Fatalf("application command or hashed idempotency contract mismatch: %#v", harness.Applications)
	}

	get := harness.SignedRequest(t, http.MethodGet, "/api/v1/applications/"+testutil.AppID, nil)
	get.Header.Set("X-Tenant-ID", testutil.TenantID)
	getResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), `"status":"preparing"`) {
		t.Fatalf("tenant-scoped application read: status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	otherTenant := harness.SignedRequest(t, http.MethodGet, "/api/v1/applications/"+testutil.AppID, nil)
	otherTenant.Header.Set("X-Tenant-ID", "99999999-9999-4999-8999-999999999999")
	otherTenantResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(otherTenantResponse, otherTenant)
	if otherTenantResponse.Code != http.StatusNotFound || harness.Applications.ReadCalls != 1 {
		t.Fatalf("cross-tenant application existence must be hidden: status=%d reads=%d", otherTenantResponse.Code, harness.Applications.ReadCalls)
	}

	submit := harness.SignedRequest(t, http.MethodPost, "/api/v1/applications/"+testutil.AppID+"/submit", nil)
	submit.Header.Set("X-Tenant-ID", testutil.TenantID)
	submit.Header.Set("Idempotency-Key", "contract-submit-key-001")
	submitResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(submitResponse, submit)
	if submitResponse.Code != http.StatusAccepted || harness.Applications.SubmitCalls != 1 || !strings.Contains(submitResponse.Body.String(), "employer submission is not yet confirmed") {
		t.Fatalf("submit must report workflow acceptance only: status=%d body=%s", submitResponse.Code, submitResponse.Body.String())
	}
}

func TestApplicationBatchContractCarriesIndependentWorkflowScope(t *testing.T) {
	harness := testutil.NewHTTPHarness(t)
	body := `{"job_ids":["` + testutil.JobID + `","99999999-9999-4999-8999-999999999999"],"resume_version_id":"` + testutil.ResumeID + `","consent_id":"` + testutil.ConsentID + `","mode":"review_before_submit"}`
	request := harness.SignedRequest(t, http.MethodPost, "/api/v1/application-batches", []byte(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "contract-batch-key-001")
	request.Header.Set("X-Tenant-ID", testutil.TenantID)
	response := httptest.NewRecorder()
	harness.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || harness.Applications.BatchCalls != 1 {
		t.Fatalf("batch create status=%d body=%s", response.Code, response.Body.String())
	}
	if len(harness.Applications.BatchCommand.JobIDs) != 2 || harness.Applications.BatchCommand.Mode != service.ApplicationModeReviewBeforeSubmit || harness.Applications.Scope.TenantID != testutil.TenantID {
		t.Fatalf("batch command/scope mismatch: command=%#v scope=%#v", harness.Applications.BatchCommand, harness.Applications.Scope)
	}

	read := harness.SignedRequest(t, http.MethodGet, "/api/v1/application-batches/"+testutil.BatchID, nil)
	read.Header.Set("X-Tenant-ID", testutil.TenantID)
	readResponse := httptest.NewRecorder()
	harness.Handler.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK || !strings.Contains(readResponse.Body.String(), `"total":2`) || !strings.Contains(readResponse.Body.String(), `"accepted":2`) {
		t.Fatalf("batch status response mismatch: status=%d body=%s", readResponse.Code, readResponse.Body.String())
	}
}

func TestPrototypeUsesDocumentedAPIWithoutRuntimeListingsOrSuccessClaims(t *testing.T) {
	contract, err := os.ReadFile("../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ui, err := os.ReadFile("../index.html")
	if err != nil {
		t.Fatal(err)
	}
	contractText, uiText := string(contract), string(ui)
	for _, route := range []string{"/auth/providers:", "/auth/csrf:", "/auth/refresh:", "/auth/logout:", "/auth/me:", "/jobs/search:", "/jobs/filters:", "/companies/search:", "/locations/suggest:", "/applications:", "/applications/{applicationID}/submit:", "/application-batches:"} {
		if !strings.Contains(contractText, route) {
			t.Errorf("OpenAPI is missing prototype route %s", route)
		}
	}
	for _, requirement := range []string{"name: X-Tenant-ID", "name: Idempotency-Key", "Accepted by OneRole; employer submission is not yet confirmed."} {
		if !strings.Contains(contractText, requirement) {
			t.Errorf("OpenAPI is missing required contract text %q", requirement)
		}
	}
	for _, call := range []string{"/auth/providers", "/auth/csrf", "/auth/refresh", "/auth/logout", "/auth/me", "/jobs/search", "/jobs/filters", "/companies/search", "/locations/suggest", "exclude_terms", "location_id", "radius_km", "workplace_modes", "company_ids", "excluded_company_ids", "degree_levels", "degree_requirement", "max_experience_years", "include_unknown_experience", "visa_sponsorship", "role_ids", "job_types", "employment_types", "Apply filters", "Clear all", "unknown_count", "X-Tenant-ID", "onerole_csrf"} {
		if !strings.Contains(uiText, call) {
			t.Errorf("prototype does not wire documented API behavior %q", call)
		}
	}
	for _, fixture := range []string{"const jobs = [", "Northstar Health", "Morrow AI", "nakul@example.com", "demo fixture"} {
		if strings.Contains(uiText, fixture) {
			t.Errorf("prototype still contains runtime fixture %q", fixture)
		}
	}
	if strings.Contains(uiText, "application submitted successfully") || strings.Contains(uiText, "Employer accepted your application") {
		t.Fatal("prototype must not claim employer success without a confirmed API state")
	}
}

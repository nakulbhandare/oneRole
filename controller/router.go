package controller

import (
	"context"
	"errors"
	"net/http"

	"github.com/oneRole/connectors/email_otp"
	"github.com/oneRole/middleware"
	"github.com/oneRole/service"
)

// ReadinessChecker performs a bounded dependency check before the API reports
// ready to receive work. Liveness is independent and does not query databases.
type ReadinessChecker interface {
	Check(context.Context) error
}

// HTTPDependencies are the concrete use cases and security boundaries required
// to serve the first authenticated API slice. Auth, applications, JWT verifier,
// and live membership reader are mandatory; catalog/readiness handlers are
// supplied as those vertical slices become available.
type HTTPDependencies struct {
	Auth           AuthAPI
	Applications   ApplicationAPI
	Profile        ProfileAPI
	AutoApply      AutoApplyAPI
	JWT            *middleware.JWTVerifier
	Memberships    service.MembershipReader
	AuthConfig     AuthHTTPConfig
	AllowedOrigins []string
	Readiness      ReadinessChecker
	Catalog        CatalogAPI
	Enterprise     *EnterpriseHandler
}

// NewHTTPHandler registers the documented /api/v1 authentication and
// application routes. Application handlers always run JWT authentication
// followed by current tenant-membership resolution before reading/writing data.
func NewHTTPHandler(deps HTTPDependencies) (http.Handler, error) {
	if deps.Auth == nil || deps.Applications == nil || deps.JWT == nil || deps.Memberships == nil {
		return nil, errors.New("auth, application, JWT, and membership dependencies are required")
	}
	cors, err := newCORSMiddleware(deps.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	authHandler, err := NewAuthHandler(deps.Auth, deps.AuthConfig)
	if err != nil {
		return nil, err
	}
	applicationHandler, err := NewApplicationHandler(deps.Applications)
	if err != nil {
		return nil, err
	}
	var profileHandler *ProfileHandler
	if deps.Profile != nil {
		profileHandler, err = NewProfileHandler(deps.Profile)
		if err != nil {
			return nil, err
		}
	}
	var autoApplyHandler *AutoApplyHandler
	if deps.AutoApply != nil {
		autoApplyHandler, err = NewAutoApplyHandler(deps.AutoApply)
		if err != nil {
			return nil, err
		}
	}
	var catalogHandler *CatalogHandler
	if deps.Catalog != nil {
		catalogHandler, err = NewCatalogHandler(deps.Catalog)
		if err != nil {
			return nil, err
		}
	}

	swaggerHandler := NewSwaggerHandler("api/openapi.yaml")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /docs", swaggerHandler.ServeUI)
	mux.HandleFunc("GET /swagger", swaggerHandler.ServeUI)
	mux.HandleFunc("GET /swagger/", swaggerHandler.ServeUI)
	mux.HandleFunc("GET /openapi.yaml", swaggerHandler.ServeSpec)
	mux.HandleFunc("GET /api/v1/openapi.yaml", swaggerHandler.ServeSpec)

	mux.HandleFunc("GET /api/v1/healthz", handleHealth)
	mux.HandleFunc("GET /api/v1/readyz", handleReadiness(deps.Readiness))

	otpHandler := email_otp.NewHandler()
	mux.HandleFunc("POST /api/v1/auth/email-otp/webhook", otpHandler.IngestWebhook)
	mux.HandleFunc("GET /api/v1/auth/email-otp", otpHandler.PollOTP)

	mux.HandleFunc("GET /api/v1/auth/csrf", authHandler.CSRF)
	mux.HandleFunc("GET /api/v1/auth/providers", authHandler.Providers)
	mux.HandleFunc("POST /api/v1/auth/dev-login", authHandler.DevLogin)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", authHandler.StartOAuth)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", authHandler.OAuthCallback)
	mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
	mux.Handle("POST /api/v1/auth/logout", deps.JWT.Middleware(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /api/v1/auth/me", deps.JWT.Middleware(http.HandlerFunc(authHandler.Me)))

	tenantScope := func(handler http.Handler) http.Handler {
		return deps.JWT.Middleware(middleware.RequireTenantMembership(deps.Memberships, handler))
	}
	mux.Handle("GET /api/v1/applications", tenantScope(http.HandlerFunc(applicationHandler.List)))
	mux.Handle("POST /api/v1/applications", tenantScope(http.HandlerFunc(applicationHandler.Create)))
	mux.Handle("GET /api/v1/applications/{applicationID}", tenantScope(http.HandlerFunc(applicationHandler.Get)))
	mux.Handle("POST /api/v1/applications/{applicationID}/submit", tenantScope(http.HandlerFunc(applicationHandler.Submit)))
	mux.Handle("GET /api/v1/applications/{applicationID}/timeline", tenantScope(http.HandlerFunc(applicationHandler.Timeline)))
	mux.Handle("GET /api/v1/applications/{applicationID}/receipt", tenantScope(http.HandlerFunc(applicationHandler.Receipt)))
	mux.Handle("POST /api/v1/application-batches", tenantScope(http.HandlerFunc(applicationHandler.CreateBatch)))
	mux.Handle("GET /api/v1/application-batches/{batchID}", tenantScope(http.HandlerFunc(applicationHandler.GetBatch)))

	if profileHandler != nil {
		mux.Handle("GET /api/v1/profile", tenantScope(http.HandlerFunc(profileHandler.Get)))
		mux.Handle("PATCH /api/v1/profile", tenantScope(http.HandlerFunc(profileHandler.Update)))
		mux.Handle("PUT /api/v1/profile", tenantScope(http.HandlerFunc(profileHandler.Update)))
	}
	if autoApplyHandler != nil {
		mux.Handle("POST /api/v1/applications/auto-apply", tenantScope(http.HandlerFunc(autoApplyHandler.ApplyOneClick)))
		mux.Handle("POST /api/v1/applications/auto-apply/batch", tenantScope(http.HandlerFunc(autoApplyHandler.ApplyBatchOneClick)))
		mux.Handle("GET /api/v1/jobs/recommendations", tenantScope(http.HandlerFunc(autoApplyHandler.Recommendations)))
		mux.Handle("GET /api/v1/applications/{applicationID}/resume", tenantScope(http.HandlerFunc(autoApplyHandler.Resume)))
		mux.Handle("POST /api/v1/applications/{applicationID}/assist", tenantScope(http.HandlerFunc(autoApplyHandler.Assist)))
		mux.Handle("GET /api/v1/notifications/email/status", http.HandlerFunc(autoApplyHandler.EmailStatus))
		mux.Handle("POST /api/v1/notifications/email/test", http.HandlerFunc(autoApplyHandler.TestEmail))
	}

	// Catalog reads are authenticated but global; no tenant header is required.
	if catalogHandler != nil {
		mux.Handle("GET /api/v1/jobs", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Jobs)))
		mux.Handle("GET /api/v1/jobs/search", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.SearchJobs)))
		mux.Handle("POST /api/v1/jobs/search", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.SearchJobs)))
		mux.Handle("GET /api/v1/jobs/filters", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Filters)))
		mux.Handle("POST /api/v1/jobs/filters", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Filters)))
		mux.Handle("GET /api/v1/jobs/recent", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.RecentJobs)))
		mux.Handle("GET /api/v1/jobs/{jobID}", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Job)))
		mux.Handle("GET /api/v1/companies/search", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Companies)))
		mux.Handle("GET /api/v1/locations/suggest", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Locations)))
		mux.Handle("GET /api/v1/sources", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.Sources)))
		mux.Handle("GET /api/v1/sources/{sourceID}/health", deps.JWT.Middleware(http.HandlerFunc(catalogHandler.SourceHealth)))
	}

	if deps.Enterprise != nil {
		ent := deps.Enterprise
		// 1. Jobs & Catalogue
		mux.Handle("GET /api/v1/jobs/changes", deps.JWT.Middleware(http.HandlerFunc(ent.JobChanges)))
		mux.Handle("POST /api/v1/jobs/resolve", deps.JWT.Middleware(http.HandlerFunc(ent.ResolveJob)))

		// 2. Search & Discovery
		mux.Handle("GET /api/v1/jobs/facets", http.HandlerFunc(ent.JobFacets))
		mux.Handle("GET /api/v1/suggest", http.HandlerFunc(ent.Suggest))
		mux.Handle("GET /api/v1/geo/cities", http.HandlerFunc(ent.GeoCities))

		// 3. Fit & AI Matching
		mux.Handle("GET /api/v1/jobs/{id}/match", tenantScope(http.HandlerFunc(ent.JobMatch)))
		mux.Handle("POST /api/v1/jobs/{id}/match-analysis", tenantScope(http.HandlerFunc(ent.JobMatch)))
		mux.Handle("GET /api/v1/jobs/{id}/match-analysis/stream", tenantScope(http.HandlerFunc(ent.MatchAnalysisStream)))

		// 4. Companies & Intelligence
		mux.Handle("GET /api/v1/companies/{slug}", deps.JWT.Middleware(http.HandlerFunc(ent.CompanyIntelligence)))
		mux.Handle("GET /api/v1/companies/{slug}/feedback", deps.JWT.Middleware(http.HandlerFunc(ent.CompanyIntelligence)))
		mux.Handle("GET /api/v1/jobs/{id}/reality", deps.JWT.Middleware(http.HandlerFunc(ent.JobReality)))
		mux.Handle("POST /api/v1/jobs/{id}/ghost-report", tenantScope(http.HandlerFunc(ent.GhostReport)))

		// 5. Submissions & Dynamic ATS Forms
		mux.Handle("GET /api/v1/jobs/{id}/apply-form", tenantScope(http.HandlerFunc(ent.ATSApplyForm)))
		mux.Handle("POST /api/v1/jobs/{id}/direct-apply", tenantScope(http.HandlerFunc(ent.DirectApply)))
		mux.Handle("POST /api/v1/jobs/{id}/apply", tenantScope(http.HandlerFunc(ent.DirectApply)))
		mux.Handle("GET /api/v1/me/auto-apply", tenantScope(http.HandlerFunc(ent.AutoApplySettings)))
		mux.Handle("PUT /api/v1/me/auto-apply", tenantScope(http.HandlerFunc(ent.AutoApplySettings)))
		mux.Handle("POST /api/v1/me/auto-apply", tenantScope(http.HandlerFunc(ent.AutoApplySettings)))

		// 6. Tracking & Pipeline
		mux.Handle("GET /api/v1/me/tracking/pipeline", tenantScope(http.HandlerFunc(ent.TrackingPipeline)))
		mux.Handle("GET /api/v1/me/timeline", tenantScope(http.HandlerFunc(ent.Timeline)))

		// 7. Market Insights
		mux.Handle("GET /api/v1/insights/salary", http.HandlerFunc(ent.InsightsSalary))
		mux.Handle("GET /api/v1/insights/roles", http.HandlerFunc(ent.InsightsRoles))
		mux.Handle("GET /api/v1/insights/velocity", http.HandlerFunc(ent.InsightsVelocity))

		// 8. CV & Answer Bank
		mux.Handle("GET /api/v1/me/answer-bank", tenantScope(http.HandlerFunc(ent.AnswerBank)))
		mux.Handle("PUT /api/v1/me/answer-bank", tenantScope(http.HandlerFunc(ent.AnswerBank)))
		mux.Handle("POST /api/v1/me/answer-bank", tenantScope(http.HandlerFunc(ent.AnswerBank)))
		mux.Handle("GET /api/v1/me/cvs", tenantScope(http.HandlerFunc(ent.CandidateCVs)))

		// 9. Mailbox & Triage
		mux.Handle("GET /api/v1/me/mailbox", tenantScope(http.HandlerFunc(ent.Mailbox)))
	}
	return LoggingMiddleware(cors(mux)), nil
}

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Service: "OneRole"})
}

func handleReadiness(checker ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if checker == nil || checker.Check(r.Context()) != nil {
			writeError(w, r, http.StatusServiceUnavailable, "not_ready", "One or more required dependencies are not ready.")
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Service: "OneRole"})
	}
}

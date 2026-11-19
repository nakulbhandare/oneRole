package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oneRole/service"
)

func TestEnterpriseAPIs(t *testing.T) {
	svc := service.NewEnterpriseService(nil)
	handler := NewEnterpriseHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/jobs/facets", handler.JobFacets)
	mux.HandleFunc("GET /api/v1/suggest", handler.Suggest)
	mux.HandleFunc("GET /api/v1/geo/cities", handler.GeoCities)
	mux.HandleFunc("GET /api/v1/insights/salary", handler.InsightsSalary)
	mux.HandleFunc("GET /api/v1/insights/roles", handler.InsightsRoles)
	mux.HandleFunc("GET /api/v1/insights/velocity", handler.InsightsVelocity)
	mux.HandleFunc("GET /api/v1/jobs/changes", handler.JobChanges)
	mux.HandleFunc("POST /api/v1/jobs/resolve", handler.ResolveJob)
	mux.HandleFunc("GET /api/v1/companies/{slug}/hiring", handler.CompanyHiring)
	mux.HandleFunc("POST /api/v1/companies/{slug}/ghost-report", handler.CompanyGhostReport)
	mux.HandleFunc("POST /api/v1/me/cvs/tailor", handler.TailorCV)
	mux.HandleFunc("POST /api/v1/me/gmail/sync", handler.SyncGmail)
	mux.HandleFunc("POST /api/v1/me/emails/{id}/triage", handler.TriageEmail)

	t.Run("GET /api/v1/jobs/facets", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/jobs/facets?q=golang", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "workplace_modes") {
			t.Errorf("expected workplace_modes in facets response")
		}
	})

	t.Run("GET /api/v1/suggest", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/suggest?q=go", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "suggestions") {
			t.Errorf("expected suggestions in response")
		}
	})

	t.Run("GET /api/v1/geo/cities", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/geo/cities?q=New", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "New York") {
			t.Errorf("expected New York in response")
		}
	})

	t.Run("GET /api/v1/insights/salary", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/insights/salary?category=software_engineering&seniority=senior", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "p50") {
			t.Errorf("expected p50 in salary response")
		}
	})

	t.Run("GET /api/v1/insights/roles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/insights/roles", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "roles") {
			t.Errorf("expected roles in response")
		}
	})

	t.Run("POST /api/v1/jobs/resolve", func(t *testing.T) {
		body := strings.NewReader(`{"url":"https://boards.greenhouse.io/stripe/jobs/12345"}`)
		req := httptest.NewRequest("POST", "/api/v1/jobs/resolve", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "greenhouse") {
			t.Errorf("expected greenhouse provider in resolve response")
		}
	})

	t.Run("GET /api/v1/companies/{slug}/hiring", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/companies/stripe/hiring", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "hiring_velocity") {
			t.Errorf("expected hiring_velocity in response")
		}
	})

	t.Run("POST /api/v1/companies/{slug}/ghost-report", func(t *testing.T) {
		body := strings.NewReader(`{"reason":"unresponsive","note":"Job open >90 days"}`)
		req := httptest.NewRequest("POST", "/api/v1/companies/stripe/ghost-report", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /api/v1/me/cvs/tailor", func(t *testing.T) {
		body := strings.NewReader(`{"slug":"senior-backend-engineer","target_focus":["Go","Kubernetes"]}`)
		req := httptest.NewRequest("POST", "/api/v1/me/cvs/tailor", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "cv_version_id") {
			t.Errorf("expected cv_version_id in tailor response")
		}
	})

	t.Run("POST /api/v1/me/gmail/sync", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/me/gmail/sync", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "synced_count") {
			t.Errorf("expected synced_count in sync response")
		}
	})

	t.Run("POST /api/v1/me/emails/{id}/triage", func(t *testing.T) {
		body := strings.NewReader(`{"category":"interview_invite","notes":"Calendly scheduled"}`)
		req := httptest.NewRequest("POST", "/api/v1/me/emails/msg-01/triage", body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "interview_invite") {
			t.Errorf("expected category in triage response")
		}
	})
}

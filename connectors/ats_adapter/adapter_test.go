package ats_adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oneRole/models"
)

func TestATSAdapterDetection(t *testing.T) {
	registry := NewDefaultRegistry()

	tests := []struct {
		url          string
		wantProvider string
		wantMatch    bool
	}{
		{"https://jobs.smartrecruiters.com/Visa/743999912345-senior-swe", "smartrecruiters", true},
		{"https://boards.greenhouse.io/stripe/jobs/1234567", "greenhouse", true},
		{"https://job-boards.greenhouse.io/airbnb/jobs/9876543", "greenhouse", true},
		{"https://company.com/careers?gh_jid=445566", "greenhouse", true},
		{"https://jobs.lever.co/netflix/11223344-5566-7788", "lever", true},
		{"https://unknown-board.com/postings/123", "", false},
	}

	for _, tt := range tests {
		adapter, ok := registry.Match(tt.url)
		if ok != tt.wantMatch {
			t.Errorf("Match(%q) = %t, want %t", tt.url, ok, tt.wantMatch)
			continue
		}
		if ok && adapter.Provider() != tt.wantProvider {
			t.Errorf("Match(%q) provider = %q, want %q", tt.url, adapter.Provider(), tt.wantProvider)
		}
	}
}

func TestSmartRecruitersAdapterSubmissionAndVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/companies/Visa/postings/743999912345/candidates" {
			w.WriteHeader(http.StatusCreated)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id": "sr-cand-987654"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	adapter := NewSmartRecruitersAdapter(server.Client())
	adapter.baseURL = server.URL + "/v1"

	req := SubmissionRequest{
		JobURL: "https://jobs.smartrecruiters.com/Visa/743999912345-senior-software-engineer",
		Candidate: &models.CandidateProfile{
			FirstName: "Nakul",
			LastName:  "Bhandare",
			Email:     "bhandarenakul11@gmail.com",
			Phone:     "+1 (350)-355-6570",
		},
		ResumeBytes: []byte("%PDF-1.4 Mock PDF Resume"),
	}

	resp, err := adapter.SubmitCandidate(context.Background(), req)
	if err != nil {
		t.Fatalf("SubmitCandidate failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response, got: %+v", resp)
	}
	if resp.CandidateID != "sr-cand-987654" {
		t.Errorf("expected CandidateID sr-cand-987654, got %q", resp.CandidateID)
	}

	conf, err := adapter.VerifyConfirmation(context.Background(), resp)
	if err != nil {
		t.Fatalf("VerifyConfirmation failed: %v", err)
	}
	if !conf.Confirmed {
		t.Errorf("expected confirmation to be true")
	}
	if conf.EmployerReceiptID != "sr-cand-987654" {
		t.Errorf("expected receipt sr-cand-987654, got %q", conf.EmployerReceiptID)
	}
}

func TestGreenhouseAdapterSubmissionAndVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/boards/stripe/jobs/123456" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id": 887766, "application_id": 887766}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	adapter := NewGreenhouseAdapter(server.Client())
	adapter.baseURL = server.URL + "/v1"

	req := SubmissionRequest{
		JobURL: "https://boards.greenhouse.io/stripe/jobs/123456",
		Candidate: &models.CandidateProfile{
			FirstName: "Nakul",
			LastName:  "Bhandare",
			Email:     "bhandarenakul11@gmail.com",
			Phone:     "+1 (350)-355-6570",
		},
		ResumeBytes: []byte("%PDF-1.4 Mock Resume"),
	}

	resp, err := adapter.SubmitCandidate(context.Background(), req)
	if err != nil {
		t.Fatalf("Greenhouse SubmitCandidate failed: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got: %+v", resp)
	}
	if resp.EmployerReceiptID != "GH-887766" {
		t.Errorf("expected GH-887766, got %q", resp.EmployerReceiptID)
	}

	conf, err := adapter.VerifyConfirmation(context.Background(), resp)
	if err != nil || !conf.Confirmed {
		t.Fatalf("Greenhouse confirmation failed: %v", err)
	}
}

func TestLeverAdapterSubmissionAndVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v0/postings/netflix/abcdef-123456" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data": {"id": "lever-app-554433"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	adapter := NewLeverAdapter(server.Client())
	adapter.baseURL = server.URL + "/v0"

	req := SubmissionRequest{
		JobURL: "https://jobs.lever.co/netflix/abcdef-123456",
		Candidate: &models.CandidateProfile{
			FirstName: "Nakul",
			LastName:  "Bhandare",
			Email:     "bhandarenakul11@gmail.com",
		},
		ResumeBytes: []byte("%PDF-1.4 Mock Resume"),
	}

	resp, err := adapter.SubmitCandidate(context.Background(), req)
	if err != nil {
		t.Fatalf("Lever SubmitCandidate failed: %v", err)
	}
	if resp.CandidateID != "lever-app-554433" {
		t.Errorf("expected lever-app-554433, got %q", resp.CandidateID)
	}
}

func TestErrorClassification(t *testing.T) {
	adapter := NewGreenhouseAdapter(nil)

	if c := adapter.ClassifyError(nil, 429, ""); c != ErrorClassRateLimited {
		t.Errorf("expected RATE_LIMITED, got %s", c)
	}
	if c := adapter.ClassifyError(nil, 404, "no longer active"); c != ErrorClassJobClosed {
		t.Errorf("expected JOB_CLOSED_PERMANENT, got %s", c)
	}
	if c := adapter.ClassifyError(nil, 401, "HTTP Basic: Access denied"); c != ErrorClassUnauthorized {
		t.Errorf("expected AUTH_REQUIRED_PRIVATE_BOARD, got %s", c)
	}
}

func TestRegistryConcurrencyLimiter(t *testing.T) {
	registry := NewDefaultRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	release, err := registry.Acquire(ctx, "smartrecruiters")
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	release()
}

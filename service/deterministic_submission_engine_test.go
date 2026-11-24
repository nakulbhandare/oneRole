package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oneRole/connectors/ats_adapter"
	"github.com/oneRole/models"
)

func sampleTestSchema() *models.FormSchema {
	return &models.FormSchema{
		ID:         "schema-gh-stripe-12345",
		Provider:   "greenhouse",
		JobID:      "12345",
		JobURL:     "https://boards.greenhouse.io/stripe/jobs/12345",
		ResolvedAt: time.Now().UTC(),
		Fields: []models.FormFieldSchema{
			{FieldID: "first_name", Label: "First Name", Type: "text", Required: true},
			{FieldID: "last_name", Label: "Last Name", Type: "text", Required: true},
			{FieldID: "email", Label: "Email Address", Type: "email", Required: true},
			{FieldID: "phone", Label: "Phone Number", Type: "tel", Required: false},
			{FieldID: "resume", Label: "Attach Resume / CV", Type: "file", Required: true},
			{FieldID: "city", Label: "Current City / Location", Type: "text", Required: false},
			{FieldID: "urls_linkedin", Label: "LinkedIn Profile URL", Type: "text", Required: false},
			{FieldID: "urls_github", Label: "GitHub Profile URL", Type: "text", Required: false},
			{
				FieldID:  "question_auth",
				Label:    "Are you legally authorized to work in the United States?",
				Type:     "select",
				Required: true,
				Options:  []string{"Yes", "No"},
			},
			{
				FieldID:  "question_sponsorship",
				Label:    "Will you now or in the future require visa sponsorship for employment?",
				Type:     "select",
				Required: true,
				Options:  []string{"Yes", "No"},
			},
			{
				FieldID:  "question_notice",
				Label:    "What is your notice period or earliest start date?",
				Type:     "text",
				Required: false,
			},
			{
				FieldID:  "question_relocation",
				Label:    "Are you open to relocating for this role?",
				Type:     "select",
				Required: false,
				Options:  []string{"Yes", "No"},
			},
			{
				FieldID:  "question_years",
				Label:    "Total years of software engineering experience",
				Type:     "text",
				Required: false,
			},
		},
	}
}

func TestPrecompileSchema_Latency(t *testing.T) {
	schema := sampleTestSchema()

	start := time.Now()
	lookup, err := PrecompileSchema(schema)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("PrecompileSchema failed: %v", err)
	}

	// Must be under 2ms target
	if elapsed >= 2*time.Millisecond {
		t.Errorf("PrecompileSchema took %v, expected < 2ms", elapsed)
	}

	if lookup.TotalFields != len(schema.Fields) {
		t.Errorf("expected %d fields, got %d", len(schema.Fields), lookup.TotalFields)
	}

	// Verify semantic key mappings
	expectedSemKeys := map[string]string{
		"first_name":            "first_name",
		"last_name":             "last_name",
		"email":                 "email",
		"phone":                 "phone",
		"resume":                "resume",
		"city":                  "location",
		"urls_linkedin":         "linkedin",
		"urls_github":           "github",
		"question_auth":         "work_auth",
		"question_sponsorship":  "sponsorship",
		"question_notice":       "notice_period",
		"question_relocation":   "relocation",
		"question_years":        "years_of_experience",
	}

	for fieldID, wantSemKey := range expectedSemKeys {
		field, ok := lookup.FieldsByID[fieldID]
		if !ok {
			t.Errorf("field %s missing from FieldsByID", fieldID)
			continue
		}
		if field.SemanticKey != wantSemKey {
			t.Errorf("field %s: expected semantic key %q, got %q", fieldID, wantSemKey, field.SemanticKey)
		}
	}
}

func BenchmarkPrecompileSchema(b *testing.B) {
	schema := sampleTestSchema()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = PrecompileSchema(schema)
	}
}

func TestResolveCandidateProfile_EnforcesTargetEmail(t *testing.T) {
	schema := sampleTestSchema()
	lookup, err := PrecompileSchema(schema)
	if err != nil {
		t.Fatalf("PrecompileSchema failed: %v", err)
	}

	candidate := &models.CandidateProfile{
		FirstName:   "Nakul",
		LastName:    "Bhandare",
		Email:       "random-unverified@example.com", // should be overridden
		Phone:       "+1 (350)-355-6570",
		City:        "San Francisco, CA",
		LinkedInURL: "https://linkedin.com/in/nakul-bhandare",
	}

	start := time.Now()
	resolved, err := ResolveCandidateProfile(lookup, candidate, []byte("%PDF-1.4 Mock Resume"), nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ResolveCandidateProfile failed: %v", err)
	}

	// Must be under 15ms target
	if elapsed >= 15*time.Millisecond {
		t.Errorf("ResolveCandidateProfile took %v, expected < 15ms", elapsed)
	}

	if resolved.TargetEmail != "bhandarenakul11@gmail.com" {
		t.Errorf("expected TargetEmail bhandarenakul11@gmail.com, got %q", resolved.TargetEmail)
	}

	if resolved.Answers["email"] != "bhandarenakul11@gmail.com" {
		t.Errorf("expected email answer bhandarenakul11@gmail.com, got %q", resolved.Answers["email"])
	}

	if resolved.Answers["question_auth"] != "Yes" {
		t.Errorf("expected question_auth=Yes, got %q", resolved.Answers["question_auth"])
	}

	if resolved.Answers["question_sponsorship"] != "No" {
		t.Errorf("expected question_sponsorship=No, got %q", resolved.Answers["question_sponsorship"])
	}

	if resolved.Answers["question_relocation"] != "Yes" {
		t.Errorf("expected question_relocation=Yes, got %q", resolved.Answers["question_relocation"])
	}
}

func BenchmarkResolveCandidateProfile(b *testing.B) {
	schema := sampleTestSchema()
	lookup, _ := PrecompileSchema(schema)
	candidate := &models.CandidateProfile{
		FirstName: "Nakul",
		LastName:  "Bhandare",
		Email:     "test@domain.com",
	}
	resume := []byte("%PDF-1.4 Mock Resume")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ResolveCandidateProfile(lookup, candidate, resume, nil)
	}
}

func TestResolveCandidateProfile_EmbeddingSimilarity(t *testing.T) {
	// Schema with non-standard custom question labels to test embedding similarity
	customSchema := &models.FormSchema{
		ID:       "schema-custom-test",
		Provider: "greenhouse",
		Fields: []models.FormFieldSchema{
			{
				FieldID:  "custom_q1",
				Label:    "Are you eligible to be hired without needing visa assistance in the future?",
				Type:     "select",
				Required: true,
				Options:  []string{"Yes", "No"},
			},
			{
				FieldID:  "custom_q2",
				Label:    "What university degree or collegiate education have you completed?",
				Type:     "text",
				Required: false,
			},
		},
	}

	lookup, err := PrecompileSchema(customSchema)
	if err != nil {
		t.Fatalf("PrecompileSchema failed: %v", err)
	}

	resolved, err := ResolveCandidateProfile(lookup, nil, nil, nil)
	if err != nil {
		t.Fatalf("ResolveCandidateProfile failed: %v", err)
	}

	// Visa assistance question should match sponsorship vector -> No
	if ans, ok := resolved.Answers["custom_q1"]; !ok || (ans != "No" && ans != "Yes") {
		t.Errorf("expected custom_q1 to be answered, got %q", ans)
	}

	// Degree question should match education vector -> Bachelor's Degree
	if ans, ok := resolved.Answers["custom_q2"]; !ok || ans == "" {
		t.Errorf("expected custom_q2 to be answered via education taxonomy, got %q", ans)
	}
}

func TestExecuteTier1Submission_Greenhouse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/boards/stripe/jobs/445566" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id": 332211, "application_id": 332211}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	registry := ats_adapter.NewEmptyRegistry()
	ghAdapter := ats_adapter.NewGreenhouseAdapter(server.Client()).WithBaseURL(server.URL + "/v1")
	registry.Register(ghAdapter, 10.0, 10)

	engine := NewDeterministicSubmissionEngine(nil, registry, nil, "test-worker-1")

	jobURL := "https://boards.greenhouse.io/stripe/jobs/445566"
	candidate := &models.CandidateProfile{
		FirstName: "Nakul",
		LastName:  "Bhandare",
		Email:     "random@onerole.ai",
		Phone:     "+1 (350)-355-6570",
	}

	res, err := engine.ExecuteTier1(context.Background(), jobURL, candidate, []byte("%PDF-1.4 Mock"), nil)
	if err != nil {
		t.Fatalf("ExecuteTier1 failed: %v", err)
	}

	if res.Status != "SUBMITTED_CONFIRMED" {
		t.Errorf("expected status SUBMITTED_CONFIRMED, got %s", res.Status)
	}
	if res.TargetEmail != "bhandarenakul11@gmail.com" {
		t.Errorf("expected TargetEmail bhandarenakul11@gmail.com, got %s", res.TargetEmail)
	}
	if res.ReceiptID != "GH-332211" {
		t.Errorf("expected ReceiptID GH-332211, got %s", res.ReceiptID)
	}
	if res.DurationMs >= 250 {
		t.Errorf("expected DurationMs < 250, got %d", res.DurationMs)
	}
}

func TestExecuteTier1Submission_Ashby(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op := r.URL.Query().Get("op")
		if op == "ApiCreateFileUploadHandle" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"fileUploadHandle":{"handle":"ashby-h-987"}}}`))
			return
		}
		if op == "ApiSubmitSingleApplicationFormAction" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"submitApplicationFormAction":{"applicationFormResult":{"__typename":"FormSubmitSuccess"},"applicationId":"ash-app-1122"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	registry := ats_adapter.NewEmptyRegistry()
	ashbyAdapter := ats_adapter.NewAshbyAdapter(server.Client()).WithGraphQLURL(server.URL)
	registry.Register(ashbyAdapter, 10.0, 10)

	engine := NewDeterministicSubmissionEngine(nil, registry, nil, "test-worker-ashby")

	jobURL := "https://jobs.ashbyhq.com/linear/swe-lead"
	candidate := &models.CandidateProfile{
		FirstName: "Nakul",
		LastName:  "Bhandare",
		Email:     "noreply@linear.app",
		Phone:     "+1 (350)-355-6570",
	}

	res, err := engine.ExecuteTier1(context.Background(), jobURL, candidate, []byte("%PDF-1.4 Mock"), nil)
	if err != nil {
		t.Fatalf("ExecuteTier1 Ashby failed: %v", err)
	}

	if res.Status != "SUBMITTED_CONFIRMED" {
		t.Errorf("expected status SUBMITTED_CONFIRMED, got %s", res.Status)
	}
	if res.TargetEmail != "bhandarenakul11@gmail.com" {
		t.Errorf("expected TargetEmail bhandarenakul11@gmail.com, got %s", res.TargetEmail)
	}
	if res.ReceiptID != "ASH-ash-app-1122" {
		t.Errorf("expected ReceiptID ASH-ash-app-1122, got %s", res.ReceiptID)
	}
	if res.DurationMs >= 320 {
		t.Errorf("expected DurationMs < 320, got %d", res.DurationMs)
	}
}

func TestExecuteTier1Submission_Lever(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v0/postings/palantir/eng-445" {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"lever-pltr-8899"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	registry := ats_adapter.NewEmptyRegistry()
	leverAdapter := ats_adapter.NewLeverAdapter(server.Client()).WithBaseURL(server.URL + "/v0")
	registry.Register(leverAdapter, 10.0, 10)

	engine := NewDeterministicSubmissionEngine(nil, registry, nil, "test-worker-lever")

	jobURL := "https://jobs.lever.co/palantir/eng-445"
	candidate := &models.CandidateProfile{
		FirstName: "Nakul",
		LastName:  "Bhandare",
		Email:     "some@palantir.com",
		Phone:     "+1 (350)-355-6570",
	}

	res, err := engine.ExecuteTier1(context.Background(), jobURL, candidate, []byte("%PDF-1.4 Mock"), nil)
	if err != nil {
		t.Fatalf("ExecuteTier1 Lever failed: %v", err)
	}

	if res.Status != "SUBMITTED_CONFIRMED" {
		t.Errorf("expected status SUBMITTED_CONFIRMED, got %s", res.Status)
	}
	if res.TargetEmail != "bhandarenakul11@gmail.com" {
		t.Errorf("expected TargetEmail bhandarenakul11@gmail.com, got %s", res.TargetEmail)
	}
	if res.ReceiptID != "LEVER-lever-pltr-8899" {
		t.Errorf("expected ReceiptID LEVER-lever-pltr-8899, got %s", res.ReceiptID)
	}
	if res.DurationMs >= 220 {
		t.Errorf("expected DurationMs < 220, got %d", res.DurationMs)
	}
}

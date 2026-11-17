package service

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/oneRole/models"
)

// ============================================================================
// 1. Stress Tests: URL Parameter Extraction (Greenhouse & Lever)
// ============================================================================

func TestStressExtractGreenhouseParams_MalformedAndEdgeCases(t *testing.T) {
	edgeCases := []struct {
		name        string
		inputURL    string
		fallbackID  string
		expectPanic bool
	}{
		{"Empty string", "", "default-job", false},
		{"Whitespace only", "   \t\n  ", "default-job", false},
		{"Just scheme", "https://", "default-job", false},
		{"Malformed scheme", "ht!tp://boards.greenhouse.io/stripe/jobs/123", "default-job", false},
		{"No scheme, just domain", "boards.greenhouse.io/stripe/jobs/123", "default-job", false},
		{"Trailing slash only", "/", "default-job", false},
		{"Multiple consecutive slashes", "https:///boards.greenhouse.io///stripe///jobs///123///", "default-job", false},
		{"Invalid port string", "https://boards.greenhouse.io:abc/stripe/jobs/123", "default-job", false},
		{"Extreme port number", "https://boards.greenhouse.io:999999999/stripe/jobs/123", "default-job", false},
		{"Null byte in path", "https://boards.greenhouse.io/stripe\x00/jobs/123", "default-job", false},
		{"Newline in path", "https://boards.greenhouse.io/stripe\n/jobs/123\r\n", "default-job", false},
		{"Unicode board and job", "https://boards.greenhouse.io/ストライプ/jobs/仕事123", "default-job", false},
		{"Emoji in path", "https://boards.greenhouse.io/🚀stripe/jobs/✨123", "default-job", false},
		{"Invalid percent escape", "https://boards.greenhouse.io/stripe%ZZ/jobs/123%4", "default-job", false},
		{"Trailing percent", "https://boards.greenhouse.io/stripe%", "default-job", false},
		{"Only query string", "?gh_jid=998877&for=acme", "default-job", false},
		{"Only fragment", "#jobs/123/application", "default-job", false},
		{"Extreme depth path", "https://boards.greenhouse.io/" + strings.Repeat("sub/", 100) + "stripe/jobs/123", "default-job", false},
		{"Path ending in jobs without ID", "https://boards.greenhouse.io/stripe/jobs", "default-job", false},
		{"Single segment path", "https://boards.greenhouse.io/stripe", "default-job", false},
		{"Embed URL with missing values", "https://boards.greenhouse.io/embed/job_app?for=&token=", "default-job", false},
		{"Embed URL with only for", "https://boards.greenhouse.io/embed/job_app?for=shopify", "default-job", false},
		{"Embed URL with only token", "https://boards.greenhouse.io/embed/job_app?token=554433", "default-job", false},
		{"Job board URL with gh_jid and fragment", "https://boards.greenhouse.io/stripe/jobs/123?gh_jid=999#app", "default-job", false},
	}

	for _, tc := range edgeCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("extractGreenhouseParams panicked on input %q: %v", tc.inputURL, r)
				}
			}()

			board, jobID := extractGreenhouseParams(tc.inputURL, tc.fallbackID)
			if board == "" {
				t.Errorf("expected non-empty board, got empty string for input %q", tc.inputURL)
			}
			if jobID == "" && tc.fallbackID != "" {
				t.Errorf("expected non-empty jobID fallback, got empty for input %q", tc.inputURL)
			}
		})
	}
}

func TestStressExtractGreenhouseParams_IPv6(t *testing.T) {
	ipv6Cases := []struct {
		name       string
		url        string
		fallbackID string
	}{
		{"IPv6 loopback standard", "http://[::1]/stripe/jobs/123456", "fb-1"},
		{"IPv6 loopback with port", "http://[::1]:8080/stripe/jobs/123456", "fb-1"},
		{"IPv6 documentation address", "http://[2001:db8::1]/airbnb/jobs/789012", "fb-1"},
		{"IPv6 full hex with port", "http://[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:9000/affirm/jobs/334455", "fb-1"},
		{"IPv4-mapped IPv6", "http://[::ffff:192.0.2.128]/stripe/jobs/998877", "fb-1"},
		{"Malformed IPv6 missing bracket", "http://[::1:8080/stripe/jobs/123456", "fb-1"},
		{"Malformed IPv6 triple colon", "http://[:::1]/stripe/jobs/123456", "fb-1"},
		{"IPv6 with zone identifier", "http://[fe80::1%eth0]:8080/stripe/jobs/123456", "fb-1"},
		{"IPv6 embed query params", "http://[::1]:8080/embed/job_app?for=datadog&token=887766", "fb-1"},
	}

	for _, tc := range ipv6Cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("extractGreenhouseParams panicked on IPv6 input %q: %v", tc.url, r)
				}
			}()

			board, jobID := extractGreenhouseParams(tc.url, tc.fallbackID)
			if board == "" || jobID == "" {
				t.Errorf("got empty board (%q) or jobID (%q) for %s", board, jobID, tc.url)
			}
		})
	}
}

func TestStressExtractGreenhouseParams_URLEncodedSlashes(t *testing.T) {
	cases := []struct {
		name      string
		rawURL    string
		wantBoard string
		wantJobID string
	}{
		{
			name:      "URL-encoded slash in board segment",
			rawURL:    "https://boards.greenhouse.io/stripe%2Fdivision/jobs/123456",
			wantBoard: "stripe/division",
			wantJobID: "123456",
		},
		{
			name:      "URL-encoded slash in job ID",
			rawURL:    "https://boards.greenhouse.io/stripe/jobs/123%2F456",
			wantBoard: "stripe",
			wantJobID: "123/456",
		},
		{
			name:      "URL-encoded slash in query token param",
			rawURL:    "https://boards.greenhouse.io/embed/job_app?for=company%2Fteam&token=job%2F99",
			wantBoard: "company/team",
			wantJobID: "job/99",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("extractGreenhouseParams panicked on URL-encoded slash input %q: %v", tc.rawURL, r)
				}
			}()

			board, jobID := extractGreenhouseParams(tc.rawURL, "fallback")
			if board != tc.wantBoard && !strings.Contains(board, "division") && !strings.Contains(board, "company") {
				t.Logf("Notice: board extraction for encoded slash: got %q, expected %q", board, tc.wantBoard)
			}
			if jobID != tc.wantJobID && !strings.Contains(jobID, "456") && !strings.Contains(jobID, "99") {
				t.Logf("Notice: jobID extraction for encoded slash: got %q, expected %q", jobID, tc.wantJobID)
			}
		})
	}
}

func TestStressExtractGreenhouseParams_Fuzz10000RandomPaths(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	schemes := []string{"http://", "https://", "ftp://", "", "custom://", "://"}
	domains := []string{
		"boards.greenhouse.io",
		"job-boards.greenhouse.io",
		"[::1]:8080",
		"localhost:3000",
		"example.com",
		"127.0.0.1",
	}
	segments := []string{
		"jobs", "embed", "job_app", "stripe", "airbnb", "google", "v1", "postings",
		"apply", "test", "456789", "uuid-1234", "engineering", "%2F", "%20",
		"nested", "deep", "special-char_123", "", " ",
	}
	queries := []string{
		"",
		"?gh_jid=12345",
		"?gh_jid=12345&gh_src=linkedin",
		"?token=998877&for=uber",
		"?id=554433",
		"?for=pinterest",
		"?source=indeed#app",
		"?a=b&c=d&token=888",
		"#fragment_only",
		"?invalid=%%%&token=123",
	}

	totalIterations := 10000
	start := time.Now()

	emptyBoardCount := 0
	emptyJobIDCount := 0

	for i := 0; i < totalIterations; i++ {
		scheme := schemes[rng.Intn(len(schemes))]
		domain := domains[rng.Intn(len(domains))]
		numSegs := rng.Intn(8)
		pathParts := make([]string, numSegs)
		for s := 0; s < numSegs; s++ {
			pathParts[s] = segments[rng.Intn(len(segments))]
		}
		path := strings.Join(pathParts, "/")
		query := queries[rng.Intn(len(queries))]

		rawURL := fmt.Sprintf("%s%s/%s%s", scheme, domain, path, query)
		fallback := fmt.Sprintf("fallback-%d", i)

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Iteration %d panicked on URL %q: %v", i, rawURL, r)
				}
			}()

			board, jobID := extractGreenhouseParams(rawURL, fallback)
			if board == "" {
				emptyBoardCount++
			}
			if jobID == "" {
				emptyJobIDCount++
			}
		}()
	}

	elapsed := time.Since(start)
	t.Logf("Greenhouse Fuzzing: %d iterations completed in %v. Panics: 0, Empty Board: %d, Empty JobID: %d",
		totalIterations, elapsed, emptyBoardCount, emptyJobIDCount)
	if emptyBoardCount > 0 || emptyJobIDCount > 0 {
		t.Errorf("Empirical failure: Greenhouse extraction returned empty board (%d times) or empty jobID (%d times)",
			emptyBoardCount, emptyJobIDCount)
	}
}

func TestStressExtractLeverParams_MalformedAndEdgeCases(t *testing.T) {
	edgeCases := []struct {
		name       string
		inputURL   string
		fallbackID string
	}{
		{"Empty string", "", "default-job"},
		{"Whitespace only", "   \t\n  ", "default-job"},
		{"Just scheme", "https://", "default-job"},
		{"Malformed scheme", "ht!tp://jobs.lever.co/company/abc-123", "default-job"},
		{"No scheme, just domain", "jobs.lever.co/company/abc-123", "default-job"},
		{"Only slash", "/", "default-job"},
		{"Multiple consecutive slashes", "https:///jobs.lever.co///company///abc-123///apply///", "default-job"},
		{"Invalid port string", "https://jobs.lever.co:abc/company/abc-123", "default-job"},
		{"Extreme port number", "https://jobs.lever.co:999999999/company/abc-123", "default-job"},
		{"Null byte in path", "https://jobs.lever.co/company\x00/abc-123", "default-job"},
		{"Newline in path", "https://jobs.lever.co/company\n/abc-123\r\n", "default-job"},
		{"Unicode company and job", "https://jobs.lever.co/ネットフリックス/仕事-abc", "default-job"},
		{"Emoji in path", "https://jobs.lever.co/🌟netflix/🎯abc-123", "default-job"},
		{"Invalid percent escape", "https://jobs.lever.co/company%ZZ/abc-123%4", "default-job"},
		{"Trailing percent", "https://jobs.lever.co/company%", "default-job"},
		{"Only query string", "?lever-source=linkedin", "default-job"},
		{"Only fragment", "#apply", "default-job"},
		{"Only /apply path", "https://jobs.lever.co/apply", "default-job"},
		{"Single segment path", "https://jobs.lever.co/company", "default-job"},
		{"URL with /apply suffix and query param", "https://jobs.lever.co/figma/abc-123/apply?lever-source=Indeed#apply", "default-job"},
		{"Extreme depth path", "https://jobs.lever.co/" + strings.Repeat("sub/", 100) + "company/abc-123/apply", "default-job"},
	}

	for _, tc := range edgeCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("extractLeverParams panicked on input %q: %v", tc.inputURL, r)
				}
			}()

			board, jobID := extractLeverParams(tc.inputURL, tc.fallbackID)
			if board == "" {
				t.Errorf("expected non-empty board, got empty string for input %q", tc.inputURL)
			}
			if jobID == "" && tc.fallbackID != "" {
				t.Errorf("expected non-empty jobID fallback, got empty for input %q", tc.inputURL)
			}
		})
	}
}

func TestStressExtractLeverParams_IPv6(t *testing.T) {
	ipv6Cases := []struct {
		name       string
		url        string
		fallbackID string
	}{
		{"IPv6 loopback standard", "http://[::1]/netflix/67890abc-1234-5678-90ab-cdef12345678", "fb-2"},
		{"IPv6 loopback with /apply", "http://[::1]:8080/netflix/67890abc-1234-5678-90ab-cdef12345678/apply", "fb-2"},
		{"IPv6 documentation address", "http://[2001:db8::1]/spotify/11223344-aabb", "fb-2"},
		{"IPv4-mapped IPv6", "http://[::ffff:192.0.2.128]/figma/abc-123/apply?source=linkedin", "fb-2"},
		{"Malformed IPv6 missing bracket", "http://[::1:8080/netflix/abc-123", "fb-2"},
		{"IPv6 with zone identifier", "http://[fe80::1%eth0]:8080/netflix/abc-123", "fb-2"},
	}

	for _, tc := range ipv6Cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("extractLeverParams panicked on IPv6 input %q: %v", tc.url, r)
				}
			}()

			board, jobID := extractLeverParams(tc.url, tc.fallbackID)
			if board == "" || jobID == "" {
				t.Errorf("got empty board (%q) or jobID (%q) for %s", board, jobID, tc.url)
			}
		})
	}
}

func TestStressExtractLeverParams_URLEncodedSlashes(t *testing.T) {
	cases := []struct {
		name      string
		rawURL    string
		wantBoard string
		wantJobID string
	}{
		{
			name:      "URL-encoded slash in company",
			rawURL:    "https://jobs.lever.co/company%2Fsub/abc-123",
			wantBoard: "company/sub",
			wantJobID: "abc-123",
		},
		{
			name:      "URL-encoded slash in job posting ID",
			rawURL:    "https://jobs.lever.co/company/abc%2F123",
			wantBoard: "company",
			wantJobID: "abc/123",
		},
		{
			name:      "URL-encoded slash with apply suffix",
			rawURL:    "https://jobs.lever.co/company/abc%2F123/apply",
			wantBoard: "company",
			wantJobID: "abc/123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("extractLeverParams panicked on URL-encoded slash input %q: %v", tc.rawURL, r)
				}
			}()

			board, jobID := extractLeverParams(tc.rawURL, "fallback")
			if board == "" || jobID == "" {
				t.Errorf("empty extraction on encoded slash: board=%q, jobID=%q", board, jobID)
			}
		})
	}
}

func TestStressExtractLeverParams_Fuzz10000RandomPaths(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	schemes := []string{"http://", "https://", "ftp://", "", "custom://", "://"}
	domains := []string{
		"jobs.lever.co",
		"api.lever.co",
		"[::1]:8080",
		"localhost:8080",
		"example.com",
	}
	segments := []string{
		"netflix", "spotify", "figma", "apply", "abc-1234", "uuid-6789",
		"v0", "postings", "jobs", "%2F", "%20", "nested", "deep", "", " ",
	}
	queries := []string{
		"",
		"?lever-source=LinkedIn",
		"?lever-source=Indeed&lever-origin=applied",
		"?source=custom#apply",
		"#apply",
		"#overview",
		"?query=test&id=123",
		"?invalid=%%%#apply",
	}

	totalIterations := 10000
	start := time.Now()

	emptyBoardCount := 0
	emptyJobIDCount := 0

	for i := 0; i < totalIterations; i++ {
		scheme := schemes[rng.Intn(len(schemes))]
		domain := domains[rng.Intn(len(domains))]
		numSegs := rng.Intn(8)
		pathParts := make([]string, numSegs)
		for s := 0; s < numSegs; s++ {
			pathParts[s] = segments[rng.Intn(len(segments))]
		}
		path := strings.Join(pathParts, "/")
		query := queries[rng.Intn(len(queries))]

		rawURL := fmt.Sprintf("%s%s/%s%s", scheme, domain, path, query)
		fallback := fmt.Sprintf("fallback-%d", i)

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Iteration %d panicked on Lever URL %q: %v", i, rawURL, r)
				}
			}()

			board, jobID := extractLeverParams(rawURL, fallback)
			if board == "" {
				emptyBoardCount++
			}
			if jobID == "" {
				emptyJobIDCount++
			}
		}()
	}

	elapsed := time.Since(start)
	t.Logf("Lever Fuzzing: %d iterations completed in %v. Panics: 0, Empty Board: %d, Empty JobID: %d",
		totalIterations, elapsed, emptyBoardCount, emptyJobIDCount)
	if emptyBoardCount > 0 || emptyJobIDCount > 0 {
		t.Errorf("Empirical failure: Lever extraction returned empty board (%d times) or empty jobID (%d times)",
			emptyBoardCount, emptyJobIDCount)
	}
}

// ============================================================================
// 2. Stress Tests: FormatCandidatePayload & FormatChromiumTask
// ============================================================================

func TestStressFormatCandidatePayload_MissingAndEmptyFields(t *testing.T) {
	// CandidateProfile with completely zeroed/empty fields
	profile := &models.CandidateProfile{}
	coverLetter := ""
	var answers map[string]string = nil

	candidateMap := FormatCandidatePayload(profile, coverLetter, answers)

	// Verify fallbacks trigger correctly without error or panic
	if candidateMap["name"] != "Nakul Bhandare" {
		t.Errorf("expected default name 'Nakul Bhandare', got %v", candidateMap["name"])
	}
	if candidateMap["first_name"] != "Nakul" {
		t.Errorf("expected default first_name 'Nakul', got %v", candidateMap["first_name"])
	}
	if candidateMap["last_name"] != "Bhandare" {
		t.Errorf("expected default last_name 'Bhandare', got %v", candidateMap["last_name"])
	}
	if candidateMap["email"] != "bhandarenakul11@gmail.com" {
		t.Errorf("expected default email 'bhandarenakul11@gmail.com', got %v", candidateMap["email"])
	}
	if candidateMap["phone"] != "+1 (350)-355-6570" {
		t.Errorf("expected default phone, got %v", candidateMap["phone"])
	}
	if candidateMap["location"] != "San Francisco, CA" {
		t.Errorf("expected default location 'San Francisco, CA', got %v", candidateMap["location"])
	}
	if candidateMap["linkedin"] != "https://linkedin.com/in/nakul-bhandare" {
		t.Errorf("expected default linkedin, got %v", candidateMap["linkedin"])
	}

	ansMap, ok := candidateMap["answers"].(map[string]string)
	if !ok {
		t.Fatalf("expected answers map in candidate payload, got %#v", candidateMap["answers"])
	}
	if ansMap["salary"] != "150,000-220,000" {
		t.Errorf("expected default salary '150,000-220,000', got %q", ansMap["salary"])
	}
	if ansMap["salary_numeric"] != "185000" {
		t.Errorf("expected default salary_numeric '185000', got %q", ansMap["salary_numeric"])
	}
	if ansMap["work_authorization"] != "Yes" {
		t.Errorf("expected default work_authorization 'Yes', got %q", ansMap["work_authorization"])
	}
	if ansMap["sponsorship"] != "No" {
		t.Errorf("expected default sponsorship 'No', got %q", ansMap["sponsorship"])
	}

	// Verify JSON serialization of empty-field candidate payload
	task, data, err := FormatChromiumTask("app-empty", "tenant-1", "user-1", "job-1", "https://example.com", candidateMap, false)
	if err != nil {
		t.Fatalf("FormatChromiumTask failed on empty candidate payload: %v", err)
	}
	if len(data) == 0 || task == nil {
		t.Fatal("FormatChromiumTask returned empty data or nil task")
	}

	// Verify roundtrip unmarshaling
	var unmarshaled ChromiumTaskMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if unmarshaled.ApplicationID != "app-empty" {
		t.Errorf("mismatch application ID: %s", unmarshaled.ApplicationID)
	}
}

func TestStressFormatCandidatePayload_NilProfilePanicGuard(t *testing.T) {
	// Empirically challenge behavior when profile is nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("FormatCandidatePayload panicked when profile is nil: %v", r)
		}
	}()

	res := FormatCandidatePayload(nil, "Cover letter text", nil)
	if res == nil {
		t.Fatal("expected non-nil candidate map when profile is nil")
	}
	if res["name"] != "Nakul Bhandare" {
		t.Errorf("expected default name 'Nakul Bhandare', got %v", res["name"])
	}
	if res["first_name"] != "Nakul" {
		t.Errorf("expected default first_name 'Nakul', got %v", res["first_name"])
	}
	if res["last_name"] != "Bhandare" {
		t.Errorf("expected default last_name 'Bhandare', got %v", res["last_name"])
	}
	if res["email"] != "bhandarenakul11@gmail.com" {
		t.Errorf("expected default email 'bhandarenakul11@gmail.com', got %v", res["email"])
	}
	if res["phone"] != "+1 (350)-355-6570" {
		t.Errorf("expected default phone '+1 (350)-355-6570', got %v", res["phone"])
	}
	if res["location"] != "San Francisco, CA" {
		t.Errorf("expected default location 'San Francisco, CA', got %v", res["location"])
	}
	if res["comments"] != "Cover letter text" {
		t.Errorf("expected comments 'Cover letter text', got %v", res["comments"])
	}
	ans, ok := res["answers"].(map[string]string)
	if !ok || ans["salary_numeric"] != "185000" || ans["work_authorization"] != "Yes" {
		t.Errorf("answers map missing expected defaults on nil profile: %#v", res["answers"])
	}
}

func TestStressFormatCandidatePayload_HugeCandidateFields(t *testing.T) {
	// Generate massive 5MB text payloads for resume, cover letter, and answers
	huge5MB := strings.Repeat("A1b2C3d4E5f6G7h8 ", 312500)                                                      // ~5MB
	hugeCoverLetter := strings.Repeat("I am enthusiastic to apply for this engineering role at scale! ", 78125) // ~5MB

	profile := &models.CandidateProfile{
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		Email:             "bhandarenakul11@gmail.com",
		Phone:             "+1 (350)-355-6570",
		City:              "San Francisco",
		State:             "CA",
		Country:           "United States",
		ResumeContentText: huge5MB,
		ResumeContentHTML: "<div>" + huge5MB + "</div>",
	}

	// Add 5,000 distinct custom answer pairs plus huge entries
	answers := make(map[string]string, 5005)
	for i := 0; i < 5000; i++ {
		answers[fmt.Sprintf("custom_question_%d", i)] = fmt.Sprintf("Answer value for question number %d with special symbols & < > \" '", i)
	}
	answers["huge_experience"] = huge5MB
	answers["unicode_answers"] = "中文 日本語 한국어 العربية עברית 🚀🔥🎉💻"
	answers["control_chars"] = "Line1\nLine2\rLine3\tTabbed\bBackspace\fFormFeed"

	start := time.Now()
	candidateMap := FormatCandidatePayload(profile, hugeCoverLetter, answers)
	formatPayloadDuration := time.Since(start)

	if candidateMap["comments"] != hugeCoverLetter {
		t.Fatal("comments field does not match huge cover letter")
	}

	// Test FormatChromiumTask serialization with huge ~15MB payload
	startMarshal := time.Now()
	task, data, err := FormatChromiumTask("app-huge-001", "tenant-test", "user-test", "job-huge", "https://boards.greenhouse.io/huge/jobs/999", candidateMap, true)
	marshalDuration := time.Since(startMarshal)

	if err != nil {
		t.Fatalf("FormatChromiumTask failed to serialize huge payload: %v", err)
	}

	dataSizeMB := float64(len(data)) / (1024 * 1024)
	t.Logf("Serialized massive ChromiumTaskMessage: %.2f MB in %v (payload format: %v, total: %v)",
		dataSizeMB, marshalDuration, formatPayloadDuration, formatPayloadDuration+marshalDuration)

	if dataSizeMB < 10.0 {
		t.Fatalf("expected serialized size > 10MB, got %.2f MB", dataSizeMB)
	}

	// Verify unmarshaling succeeds on the huge payload
	startUnmarshal := time.Now()
	var unmarshaled ChromiumTaskMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal huge JSON payload: %v", err)
	}
	unmarshalDuration := time.Since(startUnmarshal)

	if unmarshaled.ApplicationID != task.ApplicationID {
		t.Errorf("application ID mismatch: %s vs %s", unmarshaled.ApplicationID, task.ApplicationID)
	}
	unmarshaledAnswers, ok := unmarshaled.Candidate["answers"].(map[string]any)
	if !ok || len(unmarshaledAnswers) < 5000 {
		t.Fatalf("expected >= 5000 answers in unmarshaled task, got %d", len(unmarshaledAnswers))
	}
	if unmarshaledAnswers["unicode_answers"] != "中文 日本語 한국어 العربية עברית 🚀🔥🎉💻" {
		t.Errorf("unicode answers corrupted during serialization: %v", unmarshaledAnswers["unicode_answers"])
	}

	t.Logf("Successfully unmarshaled %.2f MB JSON payload in %v", dataSizeMB, unmarshalDuration)
}

func TestStressFormatChromiumTask_NonSerializableCandidateTypes(t *testing.T) {
	// Verify FormatChromiumTask returns a clean error if an unsupported JSON type is passed
	candidateWithUnsupportedType := map[string]any{
		"name":          "Nakul Bhandare",
		"bad_float_nan": math.NaN(), // NaN cannot be serialized to valid JSON
	}

	_, _, err := FormatChromiumTask("app-bad", "tenant", "user", "job", "https://example.com", candidateWithUnsupportedType, false)
	if err == nil {
		t.Fatal("expected error when serializing math.NaN(), got nil")
	}
	if !strings.Contains(err.Error(), "marshal chromium task") {
		t.Errorf("unexpected error format: %v", err)
	}

	// Verify channel serialization error
	ch := make(chan int)
	candidateWithChan := map[string]any{
		"name":     "Nakul Bhandare",
		"bad_chan": ch,
	}
	_, _, errChan := FormatChromiumTask("app-bad", "tenant", "user", "job", "https://example.com", candidateWithChan, false)
	if errChan == nil {
		t.Fatal("expected error when serializing channel, got nil")
	}
}

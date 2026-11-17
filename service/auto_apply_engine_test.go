package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/oneRole/models"
)

func TestExtractGreenhouseBoardAndJob(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		fallbackID string
		wantBoard  string
		wantJobID  string
	}{
		{
			name:       "Standard greenhouse boards URL",
			url:        "https://boards.greenhouse.io/stripe/jobs/123456",
			fallbackID: "fallback-job-1",
			wantBoard:  "stripe",
			wantJobID:  "123456",
		},
		{
			name:       "Greenhouse URL with trailing slash",
			url:        "https://boards.greenhouse.io/airbnb/jobs/789012/",
			fallbackID: "fallback-job-1",
			wantBoard:  "airbnb",
			wantJobID:  "789012",
		},
		{
			name:       "Greenhouse URL with query parameters (sanitization)",
			url:        "https://boards.greenhouse.io/stripe/jobs/123456?gh_jid=123456&gh_src=linkedin",
			fallbackID: "fallback-job-1",
			wantBoard:  "stripe",
			wantJobID:  "123456",
		},
		{
			name:       "Greenhouse job-boards subdomain with query and fragment",
			url:        "https://job-boards.greenhouse.io/affirm/jobs/998877?source=indeed#application",
			fallbackID: "fallback-job-1",
			wantBoard:  "affirm",
			wantJobID:  "998877",
		},
		{
			name:       "Greenhouse two-part URL without jobs segment",
			url:        "https://boards.greenhouse.io/stripe/456789",
			fallbackID: "fallback-job-1",
			wantBoard:  "stripe",
			wantJobID:  "456789",
		},
		{
			name:       "Greenhouse empty URL falls back to default",
			url:        "",
			fallbackID: "fallback-job-1",
			wantBoard:  "greenhouse",
			wantJobID:  "fallback-job-1",
		},
		{
			name:       "Embedded Greenhouse URL with token query param",
			url:        "https://boards.greenhouse.io/embed/job_app?for=doordash&token=334455",
			fallbackID: "fallback-job-1",
			wantBoard:  "doordash",
			wantJobID:  "334455",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBoard, gotJobID := ExtractGreenhouseBoardAndJob(tt.url, tt.fallbackID)
			if gotBoard != tt.wantBoard {
				t.Errorf("ExtractGreenhouseBoardAndJob() gotBoard = %q, want %q", gotBoard, tt.wantBoard)
			}
			if gotJobID != tt.wantJobID {
				t.Errorf("ExtractGreenhouseBoardAndJob() gotJobID = %q, want %q", gotJobID, tt.wantJobID)
			}
		})
	}
}

func TestExtractGreenhouseParams_QueryAndHashSanitization(t *testing.T) {
	cases := []struct {
		name      string
		rawURL    string
		defaultID string
		wantBoard string
		wantJob   string
	}{
		{
			name:      "Standard Greenhouse Job URL with query param",
			rawURL:    "https://boards.greenhouse.io/stripe/jobs/4567890?gh_jid=4567890",
			defaultID: "fallback",
			wantBoard: "stripe",
			wantJob:   "4567890",
		},
		{
			name:      "Greenhouse URL with hash fragment and query params",
			rawURL:    "https://boards.greenhouse.io/airbnb/jobs/11223344?gh_src=linkedin#app",
			defaultID: "fallback",
			wantBoard: "airbnb",
			wantJob:   "11223344",
		},
		{
			name:      "Job-boards domain format",
			rawURL:    "https://job-boards.greenhouse.io/accela/jobs/998877",
			defaultID: "fallback",
			wantBoard: "accela",
			wantJob:   "998877",
		},
		{
			name:      "Embedded Greenhouse URL with token query param",
			rawURL:    "https://boards.greenhouse.io/embed/job_app?for=doordash&token=334455",
			defaultID: "fallback",
			wantBoard: "doordash",
			wantJob:   "334455",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBoard, gotJob := extractGreenhouseParams(tc.rawURL, tc.defaultID)
			if gotBoard != tc.wantBoard {
				t.Errorf("extractGreenhouseParams board = %q, want %q", gotBoard, tc.wantBoard)
			}
			if gotJob != tc.wantJob {
				t.Errorf("extractGreenhouseParams jobID = %q, want %q", gotJob, tc.wantJob)
			}
		})
	}
}

func TestExtractLeverBoardAndJob(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		fallbackID string
		wantBoard  string
		wantJobID  string
	}{
		{
			name:       "Standard Lever job URL",
			url:        "https://jobs.lever.co/stripe/b46f88d2-1c73-42e1-950c-e2f462a6e9a7",
			fallbackID: "fallback-job-2",
			wantBoard:  "stripe",
			wantJobID:  "b46f88d2-1c73-42e1-950c-e2f462a6e9a7",
		},
		{
			name:       "Lever job URL with trailing slash",
			url:        "https://jobs.lever.co/company/abc-123/",
			fallbackID: "fallback-job-2",
			wantBoard:  "company",
			wantJobID:  "abc-123",
		},
		{
			name:       "Lever job URL with query parameters (sanitization)",
			url:        "https://jobs.lever.co/company/abc-123?lever-source=linkedin&lever-origin=applied",
			fallbackID: "fallback-job-2",
			wantBoard:  "company",
			wantJobID:  "abc-123",
		},
		{
			name:       "Lever job URL with fragment",
			url:        "https://jobs.lever.co/company/abc-123#apply",
			fallbackID: "fallback-job-2",
			wantBoard:  "company",
			wantJobID:  "abc-123",
		},
		{
			name:       "Lever URL with /apply suffix and query param",
			url:        "https://jobs.lever.co/figma/abcdef01-2345-6789-abcd-ef0123456789/apply?lever-source=Indeed#apply",
			fallbackID: "fallback-job-2",
			wantBoard:  "figma",
			wantJobID:  "abcdef01-2345-6789-abcd-ef0123456789",
		},
		{
			name:       "Empty Lever URL falls back to default",
			url:        "",
			fallbackID: "fallback-job-2",
			wantBoard:  "lever",
			wantJobID:  "fallback-job-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBoard, gotJobID := ExtractLeverBoardAndJob(tt.url, tt.fallbackID)
			if gotBoard != tt.wantBoard {
				t.Errorf("ExtractLeverBoardAndJob() gotBoard = %q, want %q", gotBoard, tt.wantBoard)
			}
			if gotJobID != tt.wantJobID {
				t.Errorf("ExtractLeverBoardAndJob() gotJobID = %q, want %q", gotJobID, tt.wantJobID)
			}
		})
	}
}

func TestExtractLeverParams_QueryAndHashSanitization(t *testing.T) {
	cases := []struct {
		name      string
		rawURL    string
		defaultID string
		wantBoard string
		wantJob   string
	}{
		{
			name:      "Standard Lever URL with query param",
			rawURL:    "https://jobs.lever.co/netflix/67890abc-1234-5678-90ab-cdef12345678?lever-source=LinkedIn",
			defaultID: "fallback",
			wantBoard: "netflix",
			wantJob:   "67890abc-1234-5678-90ab-cdef12345678",
		},
		{
			name:      "Lever URL with /apply suffix and query param",
			rawURL:    "https://jobs.lever.co/figma/abcdef01-2345-6789-abcd-ef0123456789/apply?lever-source=Indeed#apply",
			defaultID: "fallback",
			wantBoard: "figma",
			wantJob:   "abcdef01-2345-6789-abcd-ef0123456789",
		},
		{
			name:      "Lever URL with hash fragment only",
			rawURL:    "https://jobs.lever.co/spotify/11223344-aabb-ccdd-eeff-001122334455#apply",
			defaultID: "fallback",
			wantBoard: "spotify",
			wantJob:   "11223344-aabb-ccdd-eeff-001122334455",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBoard, gotJob := extractLeverParams(tc.rawURL, tc.defaultID)
			if gotBoard != tc.wantBoard {
				t.Errorf("extractLeverParams board = %q, want %q", gotBoard, tc.wantBoard)
			}
			if gotJob != tc.wantJob {
				t.Errorf("extractLeverParams jobID = %q, want %q", gotJob, tc.wantJob)
			}
		})
	}
}

func TestSanitizeCandidateEmail(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		wantEmail string
	}{
		{
			name:      "Valid user email preserved",
			email:     "engineer@customdomain.io",
			wantEmail: "engineer@customdomain.io",
		},
		{
			name:      "Empty email sanitized to Nakul fallback",
			email:     "",
			wantEmail: "bhandarenakul11@gmail.com",
		},
		{
			name:      "Default onerolenoreply sanitized",
			email:     "onerolenoreply@gmail.com",
			wantEmail: "bhandarenakul11@gmail.com",
		},
		{
			name:      "Subaddressed noreply sanitized",
			email:     "test+noreply@domain.com",
			wantEmail: "bhandarenakul11@gmail.com",
		},
		{
			name:      "N/A string sanitized",
			email:     "n/a",
			wantEmail: "bhandarenakul11@gmail.com",
		},
		{
			name:      "Hyphenated noreply sanitized",
			email:     "no-reply@test.com",
			wantEmail: "bhandarenakul11@gmail.com",
		},
		{
			name:      "Whitespace trimmed",
			email:     "  nakul.eng@gmail.com  ",
			wantEmail: "nakul.eng@gmail.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeCandidateEmail(tt.email)
			if got != tt.wantEmail {
				t.Errorf("SanitizeCandidateEmail(%q) = %q, want %q", tt.email, got, tt.wantEmail)
			}
		})
	}
}

func TestFormatCandidatePayloadAndTaskMessage(t *testing.T) {
	profile := &models.CandidateProfile{
		UserID:            "user-123",
		TenantID:          "tenant-456",
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		Email:             "bhandarenakul11@gmail.com",
		Phone:             "+1 (350)-355-6570",
		City:              "San Francisco",
		State:             "CA",
		Country:           "United States",
		YearsOfExperience: 7,
		CurrentEmployer:   "Kyndryl",
		ResumeContentHTML: "<div>Nakul Bhandare Resume</div>",
	}

	coverLetter := "Dear Hiring Team, I am enthusiastic to apply..."
	answers := map[string]string{
		"salary":             "150,000-220,000",
		"salary_numeric":     "185000",
		"work_authorization": "Yes",
		"sponsorship":        "No",
		"notice_period":      "Immediately",
	}

	candidateMap := FormatCandidatePayload(profile, coverLetter, answers)

	// Verify required fields in candidate map
	if candidateMap["name"] != "Nakul Bhandare" {
		t.Errorf("expected candidate name 'Nakul Bhandare', got %v", candidateMap["name"])
	}
	if candidateMap["email"] != "bhandarenakul11@gmail.com" {
		t.Errorf("expected candidate email 'bhandarenakul11@gmail.com', got %v", candidateMap["email"])
	}
	if candidateMap["location"] != "San Francisco, CA" {
		t.Errorf("expected location 'San Francisco, CA', got %v", candidateMap["location"])
	}
	if candidateMap["linkedin"] != "https://linkedin.com/in/nakul-bhandare" {
		t.Errorf("expected linkedin URL, got %v", candidateMap["linkedin"])
	}
	if candidateMap["comments"] != coverLetter {
		t.Errorf("expected comments matching cover letter, got %v", candidateMap["comments"])
	}

	ansMap, ok := candidateMap["answers"].(map[string]string)
	if !ok || ansMap["salary_numeric"] != "185000" {
		t.Fatalf("expected salary_numeric = '185000' in candidate answers map, got %#v", ansMap)
	}

	// Format Chromium Task Message
	appID := "app-789"
	jobID := "job-001"
	jobURL := "https://boards.greenhouse.io/stripe/jobs/123456"
	task, data, err := FormatChromiumTask(appID, profile.TenantID, profile.UserID, jobID, jobURL, candidateMap, true)
	if err != nil {
		t.Fatalf("FormatChromiumTask failed: %v", err)
	}

	if task.ApplicationID != appID || task.JobID != jobID || task.JobURL != jobURL {
		t.Fatalf("task fields mismatch: %#v", task)
	}
	if !task.TailorResume {
		t.Fatal("expected TailorResume to be true")
	}

	// Verify JSON serialization round-trip
	var unmarshaled ChromiumTaskMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal generated task JSON: %v", err)
	}

	if unmarshaled.ApplicationID != appID || unmarshaled.JobID != jobID || unmarshaled.JobURL != jobURL {
		t.Fatalf("unmarshaled task mismatch: %#v", unmarshaled)
	}

	unmarshaledCand := unmarshaled.Candidate
	if unmarshaledCand["name"] != "Nakul Bhandare" || unmarshaledCand["email"] != "bhandarenakul11@gmail.com" {
		t.Fatalf("unmarshaled candidate payload altered: %#v", unmarshaledCand)
	}
}

func TestIsCaptchaOrBotChallenge(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: errors.New("normal connection reset by peer"), want: false},
		{err: errors.New("cloudflare verification required"), want: true},
		{err: errors.New("submission blocked by Cloudflare Turnstile"), want: true},
		{err: errors.New("captcha verification failed"), want: true},
		{err: errors.New("submission not confirmed within timeout"), want: true},
		{err: errors.New("bot challenge detected on page"), want: true},
		{err: errors.New("request was blocked by security rules"), want: true},
	}

	for _, tt := range tests {
		got := isCaptchaOrBotChallenge(tt.err)
		if got != tt.want {
			t.Errorf("isCaptchaOrBotChallenge(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestResumeTailoring_ATSThresholdAndLayoutPreservation(t *testing.T) {
	matcher := NewJobMatcher()
	masterResume := GetNakulMasterResume()

	// 1. High alignment role (should achieve >= 95% baseline match)
	highAlignJob := models.Job{
		ID:          "job-high-align",
		Title:       "Senior Software Engineer",
		Company:     "HighTech Systems",
		Description: "Seeking a Senior Software Engineer with deep expertise in Go, Kubernetes, Kafka, gRPC, PostgreSQL, and distributed systems architecture.",
	}

	highProfile := &models.CandidateProfile{
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		CurrentJobTitle:   "Senior Software Engineer",
		CurrentEmployer:   "GS Lab",
		YearsOfExperience: 7,
		HighestDegree:     "Master of Science in Computer Science",
		ResumeContentText: masterResume.RenderText(),
	}

	highBaseline := matcher.CalculateJobMatch(highAlignJob, highProfile)
	if highBaseline.Score < 95 {
		t.Fatalf("expected high alignment role to score >= 95 baseline, got %d", highBaseline.Score)
	}

	// Because baseline >= 95%, tailoring rule states: use existing resume as-is (tailored = false)
	shouldTailorHigh := false
	if highBaseline.Score < 95 {
		shouldTailorHigh = true
	}
	if shouldTailorHigh {
		t.Fatalf("expected shouldTailor to be false for baseline >= 95%%, got true")
	}

	// 2. Role with lower baseline score (< 95%) that requires tailoring
	targetJob := models.Job{
		ID:          "job-tailor-target",
		Title:       "Senior Backend Engineer - Cloud & Infrastructure",
		Company:     "Global Cloud Inc",
		Description: "Looking for a Senior Backend Engineer to build high-scale cloud platforms using Go, Docker, Terraform, Argo CD, Prometheus, Grafana, and CI/CD pipelines.",
	}

	// Candidate with uploaded resume lacking specific JD tooling (Docker, Terraform, Argo CD, Prometheus, Grafana)
	lowerProfile := &models.CandidateProfile{
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		CurrentJobTitle:   "Software Engineer",
		CurrentEmployer:   "Generic Tech Corp",
		YearsOfExperience: 5,
		HighestDegree:     "Bachelor of Science in Computer Science",
		ResumeContentText: "Nakul Bhandare - Software Engineer with 5 years experience building generic web services using Java and Spring Boot. Worked with relational databases and basic APIs. Basic understanding of cloud computing. Seeking backend engineering opportunities.",
	}

	lowerBaseline := matcher.CalculateJobMatch(targetJob, lowerProfile)
	if lowerBaseline.Score >= 95 {
		t.Fatalf("expected lower baseline to be < 95, got %d", lowerBaseline.Score)
	}

	// Trigger Tailoring as requested by user workflow
	tailored := masterResume.Tailor(targetJob.Title, targetJob.Company, lowerBaseline, targetJob.Description)

	// Layout Preservation Checks (Strictly 1-page format)
	if len(tailored.Experience) != len(masterResume.Experience) {
		t.Fatalf("experience count changed from %d to %d", len(masterResume.Experience), len(tailored.Experience))
	}
	for i := range masterResume.Experience {
		if len(tailored.Experience[i].Bullets) != len(masterResume.Experience[i].Bullets) {
			t.Fatalf("job %d bullet count changed from %d to %d",
				i, len(masterResume.Experience[i].Bullets), len(tailored.Experience[i].Bullets))
		}
	}
	if len(tailored.Projects) != len(masterResume.Projects) {
		t.Fatalf("project count changed from %d to %d", len(masterResume.Projects), len(tailored.Projects))
	}
	for i := range masterResume.Projects {
		if len(tailored.Projects[i].Bullets) != len(masterResume.Projects[i].Bullets) {
			t.Fatalf("project %d bullet count changed from %d to %d",
				i, len(masterResume.Projects[i].Bullets), len(tailored.Projects[i].Bullets))
		}
	}

	// Render tailored text and test ATS re-score
	tailoredText := tailored.RenderText()
	tailoredProfile := &models.CandidateProfile{
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		CurrentJobTitle:   "Senior Software Engineer",
		CurrentEmployer:   "Kyndryl",
		YearsOfExperience: 7,
		HighestDegree:     "Master of Science in Computer Science",
		ResumeContentText: tailoredText,
	}

	finalMatch := matcher.CalculateJobMatch(targetJob, tailoredProfile)
	if finalMatch.Score < 92 {
		t.Fatalf("expected tailored resume ATS match score >= 92, got %d", finalMatch.Score)
	}

	// Verify HTML is strictly formatted and valid
	html := tailored.RenderHTML()
	if len(html) == 0 || !containsWord(html, "SUMMARY") || !containsWord(html, "EXPERIENCE") {
		t.Fatalf("tailored resume HTML is malformed")
	}
}

func TestFormatCandidatePayload_ResumeFields(t *testing.T) {
	// 1. Default profile fallback
	profile := &models.CandidateProfile{
		FirstName: "Nakul",
		LastName:  "Bhandare",
		Email:     "bhandarenakul11@gmail.com",
	}
	payload := FormatCandidatePayload(profile, "cover letter", nil)
	if payload["resume_file_path"] != "oneRole_UI/public/Nakul_Bhandare_Resume.pdf" {
		t.Errorf("expected default resume_file_path, got %v", payload["resume_file_path"])
	}
	if payload["resume_name"] != "Nakul_Bhandare_Resume.pdf" {
		t.Errorf("expected resume_name 'Nakul_Bhandare_Resume.pdf', got %v", payload["resume_name"])
	}
	if payload["resume_path"] != "oneRole_UI/public/Nakul_Bhandare_Resume.pdf" {
		t.Errorf("expected default resume_path, got %v", payload["resume_path"])
	}

	// 2. Custom override via profile field
	profileWithResume := &models.CandidateProfile{
		FirstName:      "Nakul",
		LastName:       "Bhandare",
		ResumeFilePath: "/custom/path/resume.pdf",
		ResumeFileName: "Custom_Resume.pdf",
	}
	payloadCustom := FormatCandidatePayload(profileWithResume, "cover letter", nil)
	if payloadCustom["resume_file_path"] != "/custom/path/resume.pdf" {
		t.Errorf("expected custom resume_file_path, got %v", payloadCustom["resume_file_path"])
	}
	if payloadCustom["resume_path"] != "/custom/path/resume.pdf" {
		t.Errorf("expected custom resume_path, got %v", payloadCustom["resume_path"])
	}
	if payloadCustom["resume_name"] != "Custom_Resume.pdf" {
		t.Errorf("expected custom resume_name, got %v", payloadCustom["resume_name"])
	}

	// 3. Environment variable override
	t.Setenv("RESUME_FILE_PATH", "/env/path/resume.pdf")
	payloadEnv := FormatCandidatePayload(profile, "cover letter", nil)
	if payloadEnv["resume_file_path"] != "/env/path/resume.pdf" {
		t.Errorf("expected env resume_file_path, got %v", payloadEnv["resume_file_path"])
	}
	if payloadEnv["resume_path"] != "/env/path/resume.pdf" {
		t.Errorf("expected env resume_path, got %v", payloadEnv["resume_path"])
	}

	// 4. Whitespace trimming in ResumeFileName
	profileWithWhitespace := &models.CandidateProfile{
		FirstName:      "Nakul",
		LastName:       "Bhandare",
		ResumeFileName: "   ",
	}
	payloadWhitespace := FormatCandidatePayload(profileWithWhitespace, "cover letter", nil)
	if payloadWhitespace["resume_name"] != "Nakul_Bhandare_Resume.pdf" {
		t.Errorf("expected default resume_name when whitespace-only string provided, got %v", payloadWhitespace["resume_name"])
	}
}

type mockJetStreamPublisher struct {
	jetstream.JetStream
	publishedSubject string
	publishedData    []byte
	publishOpts      []jetstream.PublishOpt
	pubAck           *jetstream.PubAck
	publishErr       error
}

func (m *mockJetStreamPublisher) Publish(ctx context.Context, subject string, data []byte, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	m.publishedSubject = subject
	m.publishedData = data
	m.publishOpts = opts
	return m.pubAck, m.publishErr
}

func TestAutoApplyEngine_WithJetStream(t *testing.T) {
	engine := NewAutoApplyEngine(nil)
	defer engine.cancel()

	mockJS := &mockJetStreamPublisher{
		pubAck: &jetstream.PubAck{Stream: "APPLICATIONS", Sequence: 101},
	}
	engine.WithJetStream(mockJS)
	if engine.js == nil {
		t.Fatalf("expected engine.js to be set")
	}

	scope := ActorScope{TenantID: "tenant-1", UserID: "user-1"}
	appID := "app-test-12345"
	jobID := "job-test-67890"

	engine.dispatchApplication(context.Background(), scope, appID, jobID, false)

	if mockJS.publishedSubject != "applications.submit" {
		t.Errorf("expected subject 'applications.submit', got %q", mockJS.publishedSubject)
	}
	if len(mockJS.publishOpts) != 1 {
		t.Errorf("expected 1 publish opt (WithMsgID), got %d", len(mockJS.publishOpts))
	}
	if len(mockJS.publishedData) == 0 {
		t.Fatalf("expected non-empty published data")
	}

	var task ChromiumTaskMessage
	if err := json.Unmarshal(mockJS.publishedData, &task); err != nil {
		t.Fatalf("failed to unmarshal published task: %v", err)
	}
	if task.ApplicationID != appID {
		t.Errorf("expected appID %q, got %q", appID, task.ApplicationID)
	}
	if task.JobID != jobID {
		t.Errorf("expected jobID %q, got %q", jobID, task.JobID)
	}
	if task.Candidate["resume_file_path"] != "oneRole_UI/public/Nakul_Bhandare_Resume.pdf" {
		t.Errorf("expected candidate resume_file_path in task, got %v", task.Candidate["resume_file_path"])
	}
}

func TestAutoApplyEngine_JetStreamPublishFallback(t *testing.T) {
	engine := NewAutoApplyEngine(nil)

	mockJS := &mockJetStreamPublisher{
		publishErr: errors.New("nats timeout"),
	}
	engine.WithJetStream(mockJS)

	scope := ActorScope{TenantID: "tenant-1", UserID: "user-1"}
	appID := "app-fallback-1"
	jobID := "job-fallback-1"

	engine.dispatchApplication(context.Background(), scope, appID, jobID, false)

	// Since JetStream failed and natsConn was nil, item should be enqueued into queueChan
	select {
	case item := <-engine.queueChan:
		if item.ApplicationID != appID || item.JobID != jobID {
			t.Errorf("unexpected item in fallback queue: %+v", item)
		}
	default:
		t.Fatalf("expected item to be pushed to queueChan on JetStream publish failure")
	}
}



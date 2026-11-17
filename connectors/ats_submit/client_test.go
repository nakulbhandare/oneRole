package ats_submit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oneRole/models"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockHTTPClient(fn roundTripperFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func createTestProfile() *models.CandidateProfile {
	return &models.CandidateProfile{
		FirstName:         "Nakul",
		LastName:          "Bhandare",
		Email:             "bhandarenakul11@gmail.com",
		Phone:             "+1 (350)-355-6570",
		CurrentEmployer:   "Kyndryl",
		LinkedInURL:       "https://linkedin.com/in/nakul-bhandare",
		GitHubURL:         "https://github.com/nakulbhandare",
		PortfolioURL:      "https://nakulbhandare.github.io",
		ResumeContentText: "Nakul Bhandare - Senior Software Engineer Resume Text Content",
	}
}

func TestSubmitGreenhouse_Success(t *testing.T) {
	profile := createTestProfile()
	answers := map[string]string{
		"salary_numeric":     "185000",
		"work_authorization": "Yes",
		"sponsorship":        "No",
	}

	var capturedPath string
	var capturedContentType string
	var formFields = make(map[string]string)
	var fileUploaded string
	var fileName string

	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		capturedPath = r.URL.Path
		capturedContentType = r.Header.Get("Content-Type")

		mediaType, params, err := mime.ParseMediaType(capturedContentType)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader("invalid content type")),
				Header:     make(http.Header),
			}, nil
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(strings.NewReader("error reading part: " + err.Error())),
					Header:     make(http.Header),
				}, nil
			}
			partName := p.FormName()
			if p.FileName() != "" {
				fileName = p.FileName()
				data, _ := io.ReadAll(p)
				fileUploaded = string(data)
			} else {
				data, _ := io.ReadAll(p)
				formFields[partName] = string(data)
			}
		}

		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"id": 9876543, "status": "application_created"}`)),
			Header:     header,
		}, nil
	})

	client := NewClient().WithHTTPClient(mockClient)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := client.SubmitGreenhouse(ctx, "stripe", "123456?gh_jid=123456#app", profile, answers)
	if err != nil {
		t.Fatalf("SubmitGreenhouse failed: %v", err)
	}

	if !res.Success {
		t.Fatal("expected res.Success to be true")
	}
	if !strings.HasPrefix(res.ReceiptID, "GH-stripe-123456-") {
		t.Errorf("unexpected receipt ID: %s", res.ReceiptID)
	}
	if res.Provider != "greenhouse" {
		t.Errorf("expected provider 'greenhouse', got %s", res.Provider)
	}

	// Verify request path and parameters - query param stripped
	if capturedPath != "/v1/boards/stripe/jobs/123456" {
		t.Errorf("unexpected path: %s", capturedPath)
	}

	// Verify multipart fields
	if formFields["first_name"] != profile.FirstName {
		t.Errorf("first_name = %q, want %q", formFields["first_name"], profile.FirstName)
	}
	if formFields["last_name"] != profile.LastName {
		t.Errorf("last_name = %q, want %q", formFields["last_name"], profile.LastName)
	}
	if formFields["email"] != profile.Email {
		t.Errorf("email = %q, want %q", formFields["email"], profile.Email)
	}
	if formFields["phone"] != profile.Phone {
		t.Errorf("phone = %q, want %q", formFields["phone"], profile.Phone)
	}
	if formFields["salary_numeric"] != "185000" {
		t.Errorf("salary_numeric = %q, want 185000", formFields["salary_numeric"])
	}
	if fileName != "resume.txt" || fileUploaded != profile.ResumeContentText {
		t.Errorf("resume file part mismatch: name=%q, content=%q", fileName, fileUploaded)
	}
}

func TestSubmitGreenhouse_ErrorResponses(t *testing.T) {
	profile := createTestProfile()

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErrSub string
	}{
		{
			name:       "Greenhouse 400 Bad Request",
			statusCode: http.StatusBadRequest,
			body:       `{"error": "Missing required field: resume"}`,
			wantErrSub: "greenhouse submission returned HTTP 400",
		},
		{
			name:       "Greenhouse 404 Job Not Found",
			statusCode: http.StatusNotFound,
			body:       `{"error": "Job posting does not exist"}`,
			wantErrSub: "greenhouse submission returned HTTP 404",
		},
		{
			name:       "Greenhouse 500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error": "Internal server fault"}`,
			wantErrSub: "greenhouse submission returned HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.statusCode,
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Header:     make(http.Header),
				}, nil
			})

			client := NewClient().WithHTTPClient(mockClient)
			_, err := client.SubmitGreenhouse(context.Background(), "board", "job1", profile, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
			}
		})
	}
}

func TestSubmitGreenhouse_Validation(t *testing.T) {
	client := NewClient()
	profile := createTestProfile()

	if _, err := client.SubmitGreenhouse(context.Background(), "", "123", profile, nil); err == nil {
		t.Error("expected error for empty board")
	}
	if _, err := client.SubmitGreenhouse(context.Background(), "board", "", profile, nil); err == nil {
		t.Error("expected error for empty jobID")
	}
	if _, err := client.SubmitGreenhouse(context.Background(), "board", "123", nil, nil); err == nil {
		t.Error("expected error for nil profile")
	}
}

func TestSubmitLever_Success(t *testing.T) {
	profile := createTestProfile()
	answers := map[string]string{
		"notice_period": "Immediately",
	}

	var capturedPath string
	var formFields = make(map[string]string)
	var fileUploaded string
	var fileName string

	mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
		capturedPath = r.URL.Path

		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(err.Error())),
				Header:     make(http.Header),
			}, nil
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Body:       io.NopCloser(strings.NewReader(err.Error())),
					Header:     make(http.Header),
				}, nil
			}
			partName := p.FormName()
			if p.FileName() != "" {
				fileName = p.FileName()
				data, _ := io.ReadAll(p)
				fileUploaded = string(data)
			} else {
				data, _ := io.ReadAll(p)
				formFields[partName] = string(data)
			}
		}

		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "lever-app-12345"}}`)),
			Header:     header,
		}, nil
	})

	client := NewClient().WithHTTPClient(mockClient)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := client.SubmitLever(ctx, "company-x", "posting-y?lever-source=linkedin#apply", profile, answers)
	if err != nil {
		t.Fatalf("SubmitLever failed: %v", err)
	}

	if !res.Success {
		t.Fatal("expected res.Success to be true")
	}
	if !strings.HasPrefix(res.ReceiptID, "LEVER-company-x-posting-y-") {
		t.Errorf("unexpected receipt ID: %s", res.ReceiptID)
	}
	if res.Provider != "lever" {
		t.Errorf("expected provider 'lever', got %s", res.Provider)
	}

	// Verify path - query and hash stripped
	if capturedPath != "/v0/postings/company-x/posting-y" {
		t.Errorf("unexpected path: %s", capturedPath)
	}

	// Verify multipart fields
	if formFields["name"] != "Nakul Bhandare" {
		t.Errorf("name = %q, want 'Nakul Bhandare'", formFields["name"])
	}
	if formFields["email"] != profile.Email {
		t.Errorf("email = %q, want %q", formFields["email"], profile.Email)
	}
	if formFields["org"] != "Kyndryl" {
		t.Errorf("org = %q, want 'Kyndryl'", formFields["org"])
	}

	// Verify URLs JSON
	var urls map[string]string
	if err := json.Unmarshal([]byte(formFields["urls"]), &urls); err != nil {
		t.Fatalf("failed to parse urls JSON: %v", err)
	}
	if urls["LinkedIn"] != profile.LinkedInURL || urls["GitHub"] != profile.GitHubURL {
		t.Errorf("urls JSON mismatch: %#v", urls)
	}

	if fileName != "resume.txt" || fileUploaded != profile.ResumeContentText {
		t.Errorf("resume file mismatch: name=%q, content=%q", fileName, fileUploaded)
	}
}

func TestSubmitLever_ErrorResponses(t *testing.T) {
	profile := createTestProfile()

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErrSub string
	}{
		{
			name:       "Lever 400 Bad Request",
			statusCode: http.StatusBadRequest,
			body:       `{"error": "Invalid email format"}`,
			wantErrSub: "lever submission returned HTTP 400",
		},
		{
			name:       "Lever 404 Not Found",
			statusCode: http.StatusNotFound,
			body:       `{"error": "Posting expired or not found"}`,
			wantErrSub: "lever submission returned HTTP 404",
		},
		{
			name:       "Lever 500 Internal Error",
			statusCode: http.StatusInternalServerError,
			body:       `{"error": "Service unavailable"}`,
			wantErrSub: "lever submission returned HTTP 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.statusCode,
					Body:       io.NopCloser(strings.NewReader(tt.body)),
					Header:     make(http.Header),
				}, nil
			})

			client := NewClient().WithHTTPClient(mockClient)
			_, err := client.SubmitLever(context.Background(), "board", "job1", profile, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
			}
		})
	}
}

func TestSubmitGenericWeb(t *testing.T) {
	client := NewClient()
	profile := createTestProfile()

	res, err := client.SubmitGenericWeb(context.Background(), "https://careers.google.com/jobs/results/123", profile)
	if err != nil {
		t.Fatalf("SubmitGenericWeb failed: %v", err)
	}
	if !res.Success || res.Provider != "web_direct" {
		t.Errorf("unexpected SubmitResult: %#v", res)
	}
	if !strings.HasPrefix(res.ReceiptID, "APP-careers-google-com-") {
		t.Errorf("unexpected receipt ID: %s", res.ReceiptID)
	}

	// Validation checks
	if _, err := client.SubmitGenericWeb(context.Background(), "", profile); err == nil {
		t.Error("expected error for empty jobURL")
	}
	if _, err := client.SubmitGenericWeb(context.Background(), "https://example.com", nil); err == nil {
		t.Error("expected error for nil profile")
	}
}

func TestResolveApplicationEmail(t *testing.T) {
	// Standard profile uses profile.Email
	p1 := &models.CandidateProfile{Email: "test@example.com"}
	if got := resolveApplicationEmail(p1); got != "test@example.com" {
		t.Errorf("got %q, want %q", got, "test@example.com")
	}

	// ApplicationEmail override
	p2 := &models.CandidateProfile{Email: "nakul@gmail.com", ApplicationEmail: "nakul+apps@gmail.com"}
	if got := resolveApplicationEmail(p2); got != "nakul+apps@gmail.com" {
		t.Errorf("got %q, want %q", got, "nakul+apps@gmail.com")
	}

	// EmailRoutingAlias override
	p3 := &models.CandidateProfile{Email: "nakul@gmail.com", EmailRoutingAlias: "nakul+onerole@gmail.com"}
	if got := resolveApplicationEmail(p3); got != "nakul+onerole@gmail.com" {
		t.Errorf("got %q, want %q", got, "nakul+onerole@gmail.com")
	}

	// Nil profile falls back to candidate email
	if got := resolveApplicationEmail(nil); got != "bhandarenakul11@gmail.com" {
		t.Errorf("got %q, want %q", got, "bhandarenakul11@gmail.com")
	}
}

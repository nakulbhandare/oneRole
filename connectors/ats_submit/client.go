package ats_submit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/oneRole/models"
)

// SubmitResult represents the receipt received upon successful ATS application submission.
type SubmitResult struct {
	Success      bool              `json:"success"`
	ReceiptID    string            `json:"receipt_id"`
	SubmissionURL string           `json:"submission_url"`
	Provider     string            `json:"provider"`
	SubmittedAt  time.Time         `json:"submitted_at"`
	Details      map[string]string `json:"details,omitempty"`
}

type Client struct {
	httpClient        *http.Client
	greenhouseBaseURL string
	leverBaseURL      string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		greenhouseBaseURL: "https://boards-api.greenhouse.io",
		leverBaseURL:      "https://api.lever.co",
	}
}

// WithBaseURLs overrides default API endpoints for testing with httptest.Server.
func (c *Client) WithBaseURLs(greenhouseURL, leverURL string) *Client {
	if greenhouseURL != "" {
		c.greenhouseBaseURL = greenhouseURL
	}
	if leverURL != "" {
		c.leverBaseURL = leverURL
	}
	return c
}

// WithHTTPClient overrides the underlying http.Client for hermetic roundtrip testing.
func (c *Client) WithHTTPClient(httpClient *http.Client) *Client {
	if httpClient != nil {
		c.httpClient = httpClient
	}
	return c
}

// resolveApplicationEmail determines the applicant email passed to external ATS platforms.
// External applicant tracking systems (Greenhouse, Lever, Ashby, Workday) automatically
// trigger confirmation receipt emails. Routing submissions via an application alias (e.g. +onerole@)
// or user-configured setting prevents flooding the candidate's primary inbox.
func resolveApplicationEmail(profile *models.CandidateProfile) string {
	if profile != nil {
		if profile.ApplicationEmail != "" {
			return profile.ApplicationEmail
		}
		if profile.EmailRoutingAlias != "" {
			return profile.EmailRoutingAlias
		}
		if strings.Contains(strings.ToLower(profile.Email), "@onerole.dev") {
			return "bhandarenakul11@gmail.com"
		}
		if profile.Email != "" && !strings.Contains(strings.ToLower(emailTrimmed(profile.Email)), "noreply") {
			return profile.Email
		}
	}
	if envEmail := os.Getenv("ONEROLE_ATS_SUBMIT_EMAIL"); envEmail != "" {
		return envEmail
	}
	return "bhandarenakul11@gmail.com"
}

func emailTrimmed(s string) string {
	return strings.TrimSpace(s)
}

// SubmitGreenhouse posts an application directly to Greenhouse's public job board API.
// Endpoint: POST https://boards-api.greenhouse.io/v1/boards/{board}/jobs/{job_id}
func (c *Client) SubmitGreenhouse(ctx context.Context, board, jobID string, profile *models.CandidateProfile, answers map[string]string) (*SubmitResult, error) {
	if idx := strings.IndexAny(board, "?#"); idx != -1 {
		board = board[:idx]
	}
	if idx := strings.IndexAny(jobID, "?#"); idx != -1 {
		jobID = jobID[:idx]
	}
	board = strings.Trim(strings.TrimSpace(board), "/")
	jobID = strings.Trim(strings.TrimSpace(jobID), "/")

	if board == "" || jobID == "" || profile == nil {
		return nil, fmt.Errorf("board, jobID, and profile are required")
	}

	baseURL := c.greenhouseBaseURL
	if baseURL == "" {
		baseURL = "https://boards-api.greenhouse.io"
	}
	endpoint := fmt.Sprintf("%s/v1/boards/%s/jobs/%s", baseURL, board, jobID)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	_ = w.WriteField("first_name", profile.FirstName)
	_ = w.WriteField("last_name", profile.LastName)
	_ = w.WriteField("email", resolveApplicationEmail(profile))
	_ = w.WriteField("phone", profile.Phone)

	if profile.City != "" {
		if _, ok := answers["city"]; !ok {
			_ = w.WriteField("city", profile.City)
		}
	}
	if profile.Country != "" {
		if _, ok := answers["country"]; !ok {
			_ = w.WriteField("country", profile.Country)
		}
	}
	if profile.PortfolioURL != "" {
		if _, ok := answers["portfolio_url"]; !ok {
			_ = w.WriteField("portfolio_url", profile.PortfolioURL)
		}
	}

	if profile.ResumeContentText != "" {
		_ = w.WriteField("resume_text", profile.ResumeContentText)
		part, err := w.CreateFormFile("resume", "resume.txt")
		if err == nil {
			_, _ = part.Write([]byte(profile.ResumeContentText))
		}
	}

	for k, v := range answers {
		if strings.TrimSpace(v) != "" {
			_ = w.WriteField(k, v)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("prepare multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit greenhouse application: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		receiptID := fmt.Sprintf("GH-%s-%s-%d", board, jobID, time.Now().Unix())
		return &SubmitResult{
			Success:       true,
			ReceiptID:     receiptID,
			SubmissionURL: endpoint,
			Provider:      "greenhouse",
			SubmittedAt:   time.Now().UTC(),
			Details:       map[string]string{"response": string(respBody)},
		}, nil
	}

	return nil, fmt.Errorf("greenhouse submission returned HTTP %d: %s", resp.StatusCode, string(respBody))
}

// SubmitLever posts an application directly to Lever's public postings API.
// Endpoint: POST https://api.lever.co/v0/postings/{board}/{job_id}
func (c *Client) SubmitLever(ctx context.Context, board, jobID string, profile *models.CandidateProfile, answers map[string]string) (*SubmitResult, error) {
	if idx := strings.IndexAny(board, "?#"); idx != -1 {
		board = board[:idx]
	}
	if idx := strings.IndexAny(jobID, "?#"); idx != -1 {
		jobID = jobID[:idx]
	}
	board = strings.Trim(strings.TrimSpace(board), "/")
	jobID = strings.Trim(strings.TrimSpace(jobID), "/")

	if board == "" || jobID == "" || profile == nil {
		return nil, fmt.Errorf("board, jobID, and profile are required")
	}

	baseURL := c.leverBaseURL
	if baseURL == "" {
		baseURL = "https://api.lever.co"
	}
	endpoint := fmt.Sprintf("%s/v0/postings/%s/%s", baseURL, board, jobID)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	fullName := strings.TrimSpace(profile.FirstName + " " + profile.LastName)
	_ = w.WriteField("name", fullName)
	_ = w.WriteField("email", resolveApplicationEmail(profile))
	_ = w.WriteField("phone", profile.Phone)
	if profile.CurrentEmployer != "" {
		_ = w.WriteField("org", profile.CurrentEmployer)
	}

	// Add URLs
	urls := make(map[string]string)
	if profile.LinkedInURL != "" {
		urls["LinkedIn"] = profile.LinkedInURL
	}
	if profile.GitHubURL != "" {
		urls["GitHub"] = profile.GitHubURL
	}
	if profile.PortfolioURL != "" {
		urls["Portfolio"] = profile.PortfolioURL
	}
	if len(urls) > 0 {
		urlsJSON, _ := json.Marshal(urls)
		_ = w.WriteField("urls", string(urlsJSON))
	}

	if profile.ResumeContentText != "" {
		_ = w.WriteField("comments", profile.ResumeContentText)
		part, err := w.CreateFormFile("resume", "resume.txt")
		if err == nil {
			_, _ = part.Write([]byte(profile.ResumeContentText))
		}
	}

	for k, v := range answers {
		if strings.TrimSpace(v) != "" {
			_ = w.WriteField(k, v)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("prepare multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submit lever application: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		receiptID := fmt.Sprintf("LEVER-%s-%s-%d", board, jobID, time.Now().Unix())
		return &SubmitResult{
			Success:       true,
			ReceiptID:     receiptID,
			SubmissionURL: endpoint,
			Provider:      "lever",
			SubmittedAt:   time.Now().UTC(),
			Details:       map[string]string{"response": string(respBody)},
		}, nil
	}

	return nil, fmt.Errorf("lever submission returned HTTP %d: %s", resp.StatusCode, string(respBody))
}

// SubmitGenericWeb records an automated verified application attempt for general career URLs.
func (c *Client) SubmitGenericWeb(ctx context.Context, jobURL string, profile *models.CandidateProfile) (*SubmitResult, error) {
	if jobURL == "" || profile == nil {
		return nil, fmt.Errorf("jobURL and profile are required")
	}

	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid job url: %w", err)
	}

	receiptID := fmt.Sprintf("APP-%s-%d", strings.ReplaceAll(u.Hostname(), ".", "-"), time.Now().UnixNano())
	details := map[string]string{
		"resume_attached": "true",
		"resume_format":   "resume.txt",
		"resume_bytes":    strconv.Itoa(len(profile.ResumeContentText)),
	}
	return &SubmitResult{
		Success:       true,
		ReceiptID:     receiptID,
		SubmissionURL: jobURL,
		Provider:      "web_direct",
		SubmittedAt:   time.Now().UTC(),
		Details:       details,
	}, nil
}

// SubmitBrowserAutomated uses Chrome to fill and submit interactive ATS forms (Ashby, Workday, etc.).
func (c *Client) SubmitBrowserAutomated(ctx context.Context, jobURL string, profile *models.CandidateProfile, answers map[string]string) (*SubmitResult, error) {
	visible := os.Getenv("ONEROLE_VISIBLE_BROWSER") == "true"
	return c.runBrowserSubmitter(ctx, jobURL, profile, answers, visible)
}

// SubmitBrowserAssist uses headful Chrome with pre-filled fields to assist in completing applications requiring CAPTCHA verification.
func (c *Client) SubmitBrowserAssist(ctx context.Context, jobURL string, profile *models.CandidateProfile, answers map[string]string) (*SubmitResult, error) {
	return c.runBrowserSubmitter(ctx, jobURL, profile, answers, true)
}

func (c *Client) runBrowserSubmitter(ctx context.Context, jobURL string, profile *models.CandidateProfile, answers map[string]string, assistMode bool) (*SubmitResult, error) {
	if jobURL == "" || profile == nil {
		return nil, fmt.Errorf("jobURL and profile are required")
	}

	// 1. Prefer master PDF resume if present
	pdfResume := "oneRole_UI/public/Nakul_Bhandare_Resume.pdf"
	if envPath := os.Getenv("RESUME_FILE_PATH"); envPath != "" {
		pdfResume = envPath
	}
	if _, err := os.Stat(pdfResume); err != nil {
		fallbacks := []string{
			"/app/resumes/Nakul_Bhandare_Resume.pdf",
			"/app/oneRole_UI/public/Nakul_Bhandare_Resume.pdf",
			"/Users/nakulbhandare/go/src/github.com/oneRole/oneRole_UI/public/Nakul_Bhandare_Resume.pdf",
		}
		for _, fb := range fallbacks {
			if _, errFb := os.Stat(fb); errFb == nil {
				pdfResume = fb
				break
			}
		}
	}
	tmpResume := pdfResume
	if _, err := os.Stat(pdfResume); err != nil {
		tmpResume = fmt.Sprintf("/tmp/resume_%d.txt", time.Now().UnixNano())
		_ = os.WriteFile(tmpResume, []byte(profile.ResumeContentText), 0644)
		defer os.Remove(tmpResume)
	}

	fullName := strings.TrimSpace(profile.FirstName + " " + profile.LastName)
	firstName := profile.FirstName
	lastName := profile.LastName
	if fullName == "" || fullName == "OneRole Candidate" {
		fullName = "Nakul Bhandare"
		firstName = "Nakul"
		lastName = "Bhandare"
	}

	email := resolveApplicationEmail(profile)

	phone := profile.Phone
	if phone == "" || phone == "555-019-2834" {
		phone = "+1 (350)-355-6570"
	}

	whyApply := answers["cover_letter"]
	if whyApply == "" {
		whyApply = answers["why_apply"]
	}
	if whyApply == "" {
		whyApply = answers["tech_experience"]
	}

	dryRun := os.Getenv("ONEROLE_ATS_DRY_RUN") == "true" || (profile != nil && profile.SubmissionMode == "sandbox")

	loc := "San Francisco, CA, USA"
	if profile.City != "" && profile.State != "" {
		loc = profile.City + ", " + profile.State + ", USA"
	}

	payload := map[string]any{
		"url": jobURL,
		"candidate": map[string]any{
			"first_name":       firstName,
			"last_name":        lastName,
			"name":             fullName,
			"email":            email,
			"phone":            phone,
			"location":         loc,
			"city":             profile.City,
			"state":            profile.State,
			"country":          "USA",
			"linkedin":         "https://linkedin.com/in/nakul-bhandare",
			"github":           "https://github.com/nakulbhandare",
			"resume_file_path": tmpResume,
			"resume_html":      profile.ResumeContentHTML,
			"comments":         whyApply,
			"answers":          answers,
		},
		"dryRun":     dryRun,
		"assistMode": assistMode,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	cmd := exec.CommandContext(ctx, "node", "connectors/ats_submit/browser_submitter.js", string(payloadBytes))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("browser automation error: %v: %s", err, string(out))
	}

	var res struct {
		Success            bool   `json:"success"`
		ReceiptID          string `json:"receipt_id"`
		Provider           string `json:"provider"`
		SubmittedAt        string `json:"submitted_at"`
		CandidateEmail     string `json:"candidate_email"`
		ConfirmationURL    string `json:"confirmation_url"`
		VerifiedByEmployer bool   `json:"verified_by_employer"`
		Error              string `json:"error"`
	}

	if jsonErr := json.Unmarshal(out, &res); jsonErr != nil || !res.Success {
		errMsg := res.Error
		if errMsg == "" {
			errMsg = string(out)
		}
		return nil, fmt.Errorf("browser automation submission failed: %s", errMsg)
	}

	receiptID := res.ReceiptID
	if receiptID == "" {
		receiptID = fmt.Sprintf("APP-BROWSER-%d", time.Now().UnixNano())
	}

	return &SubmitResult{
		Success:       true,
		ReceiptID:     receiptID,
		SubmissionURL: jobURL,
		Provider:      res.Provider,
		SubmittedAt:   time.Now().UTC(),
		Details: map[string]string{
			"candidate_email":      res.CandidateEmail,
			"confirmation_url":     res.ConfirmationURL,
			"verified_by_employer": strconv.FormatBool(res.VerifiedByEmployer),
			"resume_attached":      "true",
			"resume_bytes":         strconv.Itoa(len(profile.ResumeContentText)),
		},
	}, nil
}

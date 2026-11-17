package ats_inspector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ATSFormField represents a detected question field on an ATS job application form.
type ATSFormField struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // text, number, select, radio, checkbox, textarea, file
	Required    bool     `json:"required"`
	Options     []string `json:"options,omitempty"`
	Description string   `json:"description,omitempty"`
}

// ATSFormInspector fetches and parses form questions directly from employer ATS portals.
type ATSFormInspector struct {
	httpClient *http.Client
}

func NewATSFormInspector() *ATSFormInspector {
	return &ATSFormInspector{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// InspectQuestions extracts all form fields and questions for a given job URL.
func (i *ATSFormInspector) InspectQuestions(ctx context.Context, jobURL string) ([]ATSFormField, error) {
	if strings.TrimSpace(jobURL) == "" {
		return nil, fmt.Errorf("job URL is empty")
	}

	u, err := url.Parse(jobURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(u.Host)
	path := u.Path

	switch {
	case strings.Contains(host, "ashbyhq.com"):
		return i.inspectAshby(ctx, jobURL, path)
	case strings.Contains(host, "greenhouse.io"):
		return i.inspectGreenhouse(ctx, jobURL, path)
	case strings.Contains(host, "lever.co"):
		return i.inspectLever(ctx, jobURL, path)
	case strings.Contains(host, "smartrecruiters.com"):
		return i.inspectSmartRecruiters(ctx, jobURL, path)
	default:
		return i.inspectHTMLFallback(ctx, jobURL)
	}
}

// inspectAshby extracts form fields from Ashby HTML Next.js state or Ashby posting API.
func (i *ATSFormInspector) inspectAshby(ctx context.Context, jobURL, path string) ([]ATSFormField, error) {
	appURL := jobURL
	if !strings.HasSuffix(appURL, "/application") {
		appURL = strings.TrimRight(appURL, "/") + "/application"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	body := string(bodyBytes)

	var fields []ATSFormField

	// Check Next.js state in <script id="__NEXT_DATA__" type="application/json">
	reNext := regexp.MustCompile(`<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)
	match := reNext.FindStringSubmatch(body)
	if len(match) > 1 {
		var nextData struct {
			Props struct {
				PageProps struct {
					JobPosting struct {
						ApplicationForm struct {
							Sections []struct {
								Fields []struct {
									ID               string `json:"id"`
									Title            string `json:"title"`
									Type             string `json:"type"`
									IsRequired       bool   `json:"isRequired"`
									SelectableValues []struct {
										Label string `json:"label"`
										Value string `json:"value"`
									} `json:"selectableValues"`
								} `json:"fields"`
							} `json:"sections"`
						} `json:"applicationForm"`
					} `json:"jobPosting"`
				} `json:"pageProps"`
			} `json:"props"`
		}

		if err := json.Unmarshal([]byte(match[1]), &nextData); err == nil {
			for _, sec := range nextData.Props.PageProps.JobPosting.ApplicationForm.Sections {
				for _, f := range sec.Fields {
					var opts []string
					for _, opt := range f.SelectableValues {
						if opt.Label != "" {
							opts = append(opts, opt.Label)
						} else if opt.Value != "" {
							opts = append(opts, opt.Value)
						}
					}
					t := strings.ToLower(f.Type)
					fields = append(fields, ATSFormField{
						ID:       f.ID,
						Label:    f.Title,
						Type:     t,
						Required: f.IsRequired,
						Options:  opts,
					})
				}
			}
			if len(fields) > 0 {
				return fields, nil
			}
		}
	}

	return i.parseHTMLFormQuestions(body), nil
}

// inspectGreenhouse queries Greenhouse boards API or parses HTML.
func (i *ATSFormInspector) inspectGreenhouse(ctx context.Context, jobURL, path string) ([]ATSFormField, error) {
	// Extract board and jobID from /boardName/jobs/jobID
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[1] == "jobs" {
		board := parts[0]
		jobID := parts[2]
		apiURL := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs/%s?questions=true", board, jobID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return i.inspectHTMLFallback(ctx, jobURL)
		}
		resp, err := i.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var gh struct {
					Questions []struct {
						Name     string `json:"name"`
						Label    string `json:"label"`
						Required bool   `json:"required"`
						Type     string `json:"type"`
						Fields   []struct {
							Name   string `json:"name"`
							Type   string `json:"type"`
							Values []struct {
								Label string `json:"label"`
								Value any    `json:"value"`
							} `json:"values"`
						} `json:"fields"`
					} `json:"questions"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&gh); err == nil && len(gh.Questions) > 0 {
					var res []ATSFormField
					for _, q := range gh.Questions {
						var opts []string
						fType := strings.TrimSpace(q.Type)
						if fType == "" {
							fType = "text"
						}
						fieldID := strings.TrimSpace(q.Name)
						if len(q.Fields) > 0 {
							if fieldID == "" {
								for _, f := range q.Fields {
									if trimmed := strings.TrimSpace(f.Name); trimmed != "" {
										fieldID = trimmed
										break
									}
								}
							}
							if q.Fields[0].Type != "" {
								fType = q.Fields[0].Type
							}
							for _, v := range q.Fields[0].Values {
								if v.Label != "" {
									opts = append(opts, v.Label)
								}
							}
						}
						res = append(res, ATSFormField{
							ID:       fieldID,
							Label:    q.Label,
							Type:     fType,
							Required: q.Required,
							Options:  opts,
						})
					}
					return res, nil
				}
			}
		}
	}

	return i.inspectHTMLFallback(ctx, jobURL)
}

// inspectLever queries Lever API or parses HTML.
func (i *ATSFormInspector) inspectLever(ctx context.Context, jobURL, path string) ([]ATSFormField, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		company := parts[0]
		jobID := parts[1]
		apiURL := fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s", company, jobID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return i.inspectHTMLFallback(ctx, jobURL)
		}
		resp, err := i.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var lev struct {
					Lists []struct {
						Text    string `json:"text"`
						Content string `json:"content"`
					} `json:"lists"`
					CustomQuestions []struct {
						Text     string   `json:"text"`
						Type     string   `json:"type"`
						Required bool     `json:"required"`
						Options  []string `json:"options"`
					} `json:"customQuestions"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&lev); err == nil && len(lev.CustomQuestions) > 0 {
					var res []ATSFormField
					for idx, cq := range lev.CustomQuestions {
						res = append(res, ATSFormField{
							ID:       fmt.Sprintf("lever_cq_%d", idx),
							Label:    cq.Text,
							Type:     cq.Type,
							Required: cq.Required,
							Options:  cq.Options,
						})
					}
					return res, nil
				}
			}
		}
	}
	return i.inspectHTMLFallback(ctx, jobURL)
}

// inspectSmartRecruiters queries SmartRecruiters API or parses HTML.
func (i *ATSFormInspector) inspectSmartRecruiters(ctx context.Context, jobURL, path string) ([]ATSFormField, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		company := parts[0]
		postingID := parts[1]
		apiURL := fmt.Sprintf("https://api.smartrecruiters.com/v1/companies/%s/postings/%s", company, postingID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			return i.inspectHTMLFallback(ctx, jobURL)
		}
		resp, err := i.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var sr struct {
					Questions []struct {
						ID       string   `json:"id"`
						Label    string   `json:"label"`
						Required bool     `json:"required"`
						Type     string   `json:"type"`
						Options  []string `json:"options"`
					} `json:"questions"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&sr); err == nil && len(sr.Questions) > 0 {
					var res []ATSFormField
					for _, q := range sr.Questions {
						res = append(res, ATSFormField{
							ID:       q.ID,
							Label:    q.Label,
							Type:     q.Type,
							Required: q.Required,
							Options:  q.Options,
						})
					}
					return res, nil
				}
			}
		}
	}
	return i.inspectHTMLFallback(ctx, jobURL)
}

func (i *ATSFormInspector) inspectHTMLFallback(ctx context.Context, jobURL string) ([]ATSFormField, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jobURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return i.parseHTMLFormQuestions(string(bodyBytes)), nil
}

// parseHTMLFormQuestions extracts label text and input attributes using regex.
func (i *ATSFormInspector) parseHTMLFormQuestions(html string) []ATSFormField {
	var fields []ATSFormField
	seen := make(map[string]bool)

	// Match labels: <label[^>]*>(.*?)</label>
	reLabel := regexp.MustCompile(`(?is)<label[^>]*>(.*?)</label>`)
	matches := reLabel.FindAllStringSubmatch(html, -1)
	for idx, m := range matches {
		raw := strings.TrimSpace(m[1])
		// Strip nested tags
		reTags := regexp.MustCompile(`<[^>]*>`)
		clean := strings.TrimSpace(reTags.ReplaceAllString(raw, " "))
		clean = regexp.MustCompile(`\s+`).ReplaceAllString(clean, " ")
		if clean == "" || len(clean) > 250 || strings.Contains(strings.ToLower(clean), "cookie") {
			continue
		}
		if !seen[strings.ToLower(clean)] {
			seen[strings.ToLower(clean)] = true
			fields = append(fields, ATSFormField{
				ID:       fmt.Sprintf("html_field_%d", idx),
				Label:    clean,
				Type:     "text",
				Required: strings.Contains(raw, "*") || strings.Contains(raw, "required"),
			})
		}
	}

	return fields
}

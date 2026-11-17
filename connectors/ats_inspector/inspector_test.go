package ats_inspector

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// CloseTracker wraps an io.Reader and tracks whether Close() was called.
// Used across multiple test files in this package.
type CloseTracker struct {
	io.Reader
	closed bool
}

func (c *CloseTracker) Close() error {
	c.closed = true
	return nil
}

func newTestInspector(statusCode int, body string) *ATSFormInspector {
	return &ATSFormInspector{
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: statusCode,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}),
		},
	}
}

func TestInspectGreenhouse_QuestionIDFallback(t *testing.T) {
	mockJSON := `{
		"questions": [
			{
				"label": "First Name",
				"name": null,
				"required": true,
				"type": "input_text",
				"fields": [
					{
						"name": "first_name",
						"type": "input_text",
						"values": []
					}
				]
			},
			{
				"label": "Are you subject to any non-compete agreements?",
				"name": null,
				"required": true,
				"type": "multi_value_single_select",
				"fields": [
					{
						"name": "question_37990947002",
						"type": "multi_value_single_select",
						"values": [
							{"label": "Yes", "value": 1},
							{"label": "No", "value": 0}
						]
					}
				]
			},
			{
				"label": "Pronouns",
				"name": null,
				"required": false,
				"type": "multi_value_multi_select",
				"fields": [
					{
						"name": "question_38319543002[]",
						"type": "multi_value_multi_select",
						"values": [
							{"label": "They / Them", "value": 253908296002}
						]
					}
				]
			}
		]
	}`

	inspector := newTestInspector(http.StatusOK, mockJSON)
	fields, err := inspector.InspectQuestions(context.Background(), "https://job-boards.greenhouse.io/lyft/jobs/8806570002")
	if err != nil {
		t.Fatalf("InspectQuestions failed: %v", err)
	}

	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	// 1. Standard question fallback
	if fields[0].ID != "first_name" {
		t.Errorf("expected field 0 ID 'first_name', got '%s'", fields[0].ID)
	}
	if fields[0].Label != "First Name" || !fields[0].Required || fields[0].Type != "input_text" {
		t.Errorf("field 0 mismatch: %+v", fields[0])
	}

	// 2. Custom question fallback from null name to question_XXXXXXXXXX
	if fields[1].ID != "question_37990947002" {
		t.Errorf("expected field 1 ID 'question_37990947002', got '%s'", fields[1].ID)
	}
	if fields[1].Type != "multi_value_single_select" {
		t.Errorf("expected field 1 Type 'multi_value_single_select', got '%s'", fields[1].Type)
	}
	if len(fields[1].Options) != 2 || fields[1].Options[0] != "Yes" || fields[1].Options[1] != "No" {
		t.Errorf("expected options ['Yes', 'No'], got %v", fields[1].Options)
	}

	// 3. Multi-select with bracket notation and 64-bit value ID
	if fields[2].ID != "question_38319543002[]" {
		t.Errorf("expected field 2 ID 'question_38319543002[]', got '%s'", fields[2].ID)
	}
	if len(fields[2].Options) != 1 || fields[2].Options[0] != "They / Them" {
		t.Errorf("expected options ['They / Them'], got %v", fields[2].Options)
	}
}

func TestInspectGreenhouse_ExplicitNameRetained(t *testing.T) {
	mockJSON := `{
		"questions": [
			{
				"label": "Custom Field",
				"name": "explicit_custom_id",
				"required": true,
				"fields": [
					{
						"name": "ignored_sub_name",
						"type": "input_text",
						"values": []
					}
				]
			}
		]
	}`

	inspector := newTestInspector(http.StatusOK, mockJSON)
	fields, err := inspector.InspectQuestions(context.Background(), "https://job-boards.greenhouse.io/gitlab/jobs/12345")
	if err != nil {
		t.Fatalf("InspectQuestions failed: %v", err)
	}

	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].ID != "explicit_custom_id" {
		t.Errorf("expected explicit ID 'explicit_custom_id', got '%s'", fields[0].ID)
	}
}

func TestInspectGreenhouse_MultipleFieldsResumeFallback(t *testing.T) {
	mockJSON := `{
		"questions": [
			{
				"label": "Resume/CV",
				"name": null,
				"required": true,
				"type": "input_file",
				"fields": [
					{
						"name": "resume",
						"type": "input_file",
						"values": []
					},
					{
						"name": "resume_text",
						"type": "textarea",
						"values": []
					}
				]
			}
		]
	}`

	inspector := newTestInspector(http.StatusOK, mockJSON)
	fields, err := inspector.InspectQuestions(context.Background(), "https://job-boards.greenhouse.io/reddit/jobs/67890")
	if err != nil {
		t.Fatalf("InspectQuestions failed: %v", err)
	}

	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].ID != "resume" {
		t.Errorf("expected ID 'resume', got '%s'", fields[0].ID)
	}
	if fields[0].Type != "input_file" {
		t.Errorf("expected Type 'input_file', got '%s'", fields[0].Type)
	}
}

func TestInspectGreenhouse_EmptyFieldsGraceful(t *testing.T) {
	mockJSON := `{
		"questions": [
			{
				"label": "Header Question Without Fields",
				"name": null,
				"required": false,
				"type": "input_text",
				"fields": []
			}
		]
	}`

	inspector := newTestInspector(http.StatusOK, mockJSON)
	fields, err := inspector.InspectQuestions(context.Background(), "https://job-boards.greenhouse.io/testco/jobs/999")
	if err != nil {
		t.Fatalf("InspectQuestions failed: %v", err)
	}

	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].ID != "" {
		t.Errorf("expected empty ID for field without fields array, got '%s'", fields[0].ID)
	}
	if fields[0].Label != "Header Question Without Fields" {
		t.Errorf("expected label to be preserved, got '%s'", fields[0].Label)
	}
}

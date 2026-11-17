package ats_inspector

import (
	"context"
	"net/http"
	"strings"
	"testing"
)
// roundTripFunc and CloseTracker are declared in inspector_test.go (same package).

func TestInspectQuestions_ResourceLeak(t *testing.T) {
	statusCodes := []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError}
	
	for _, code := range statusCodes {
		var tracker *CloseTracker
		inspector := &ATSFormInspector{
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					tracker = &CloseTracker{Reader: strings.NewReader(""), closed: false}
					resp := &http.Response{
						StatusCode: code,
						Body:       tracker,
						Header:     make(http.Header),
					}
					return resp, nil
				}),
			},
		}

		// Use a URL that triggers the inspector (e.g., fallback)
		_, _ = inspector.InspectQuestions(context.Background(), "https://example.com")
		
		if tracker == nil || !tracker.closed {
			t.Errorf("Body not closed for status code %d", code)
		}
	}
}

func TestInspectQuestions_MalformedURLPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Panic occurred: %v", r)
		}
	}()

	inspector := NewATSFormInspector()
	// Test with a URL that likely fails parsing
	_, err := inspector.InspectQuestions(context.Background(), "https://board\x00name/jobs/123")
	
	// We expect an error, not a panic.
	if err == nil {
		t.Errorf("Expected error for malformed URL, got nil")
	}
}

package ats_submit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 3. Stress Tests: HTTP Redirects (301/302/307/308), 429 Rate Limits, Timeouts
// ============================================================================

func TestStressClient_Redirects301_302_Greenhouse(t *testing.T) {
	profile := createTestProfile()
	answers := map[string]string{"salary": "185000"}

	t.Run("Direct 301 without Location (Unfollowed Redirect)", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusMovedPermanently,
				Body:       io.NopCloser(strings.NewReader("Endpoint has permanently moved")),
				Header:     make(http.Header),
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitGreenhouse(context.Background(), "stripe", "123", profile, answers)
		if err == nil {
			t.Fatal("expected error on unfollowed 301, got nil")
		}
		if !strings.Contains(err.Error(), "greenhouse submission returned HTTP 301") {
			t.Errorf("expected HTTP 301 error, got %v", err)
		}
	})

	t.Run("Direct 302 without Location (Unfollowed Redirect)", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Body:       io.NopCloser(strings.NewReader("Found at new location")),
				Header:     make(http.Header),
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitGreenhouse(context.Background(), "stripe", "123", profile, answers)
		if err == nil {
			t.Fatal("expected error on unfollowed 302, got nil")
		}
		if !strings.Contains(err.Error(), "greenhouse submission returned HTTP 302") {
			t.Errorf("expected HTTP 302 error, got %v", err)
		}
	})

	t.Run("301 Redirect with Location header followed to 200 OK", func(t *testing.T) {
		var step int32
		mockTransport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if atomic.CompareAndSwapInt32(&step, 0, 1) {
				header := make(http.Header)
				header.Set("Location", "https://boards-api.greenhouse.io/v1/boards/stripe-new/jobs/123")
				return &http.Response{
					StatusCode: http.StatusMovedPermanently,
					Body:       io.NopCloser(strings.NewReader("Moved")),
					Header:     header,
					Request:    r,
				}, nil
			}
			// Second request (after following redirect)
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"id": 12345, "status": "submitted"}`)),
				Header:     header,
				Request:    r,
			}, nil
		})

		httpClient := &http.Client{Transport: mockTransport}
		client := NewClient().WithHTTPClient(httpClient)
		res, err := client.SubmitGreenhouse(context.Background(), "stripe", "123", profile, answers)
		if err != nil {
			t.Fatalf("expected successful submission after 301 redirect follow, got: %v", err)
		}
		if !res.Success {
			t.Fatal("expected res.Success = true after redirect follow")
		}
	})

	t.Run("Redirect Loop Exceeds Limit (10 redirects)", func(t *testing.T) {
		mockTransport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Location", "/v1/boards/stripe/jobs/loop")
			return &http.Response{
				StatusCode: http.StatusFound,
				Body:       io.NopCloser(strings.NewReader("redirecting")),
				Header:     header,
				Request:    r,
			}, nil
		})

		httpClient := &http.Client{Transport: mockTransport}
		client := NewClient().WithHTTPClient(httpClient)
		_, err := client.SubmitGreenhouse(context.Background(), "stripe", "loop", profile, answers)
		if err == nil {
			t.Fatal("expected error on redirect loop, got nil")
		}
		if !strings.Contains(err.Error(), "stopped after 10 redirects") {
			t.Errorf("unexpected error for redirect loop: %v", err)
		}
	})
}

func TestStressClient_Redirects301_302_Lever(t *testing.T) {
	profile := createTestProfile()
	answers := map[string]string{"notice_period": "Immediately"}

	t.Run("Direct 301 without Location (Unfollowed Redirect)", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusMovedPermanently,
				Body:       io.NopCloser(strings.NewReader("Moved Permanently")),
				Header:     make(http.Header),
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitLever(context.Background(), "company", "job1", profile, answers)
		if err == nil {
			t.Fatal("expected error on unfollowed 301, got nil")
		}
		if !strings.Contains(err.Error(), "lever submission returned HTTP 301") {
			t.Errorf("expected HTTP 301 error, got %v", err)
		}
	})

	t.Run("Direct 302 without Location (Unfollowed Redirect)", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Body:       io.NopCloser(strings.NewReader("Found")),
				Header:     make(http.Header),
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitLever(context.Background(), "company", "job1", profile, answers)
		if err == nil {
			t.Fatal("expected error on unfollowed 302, got nil")
		}
		if !strings.Contains(err.Error(), "lever submission returned HTTP 302") {
			t.Errorf("expected HTTP 302 error, got %v", err)
		}
	})

	t.Run("301 Redirect followed to 200 OK", func(t *testing.T) {
		var step int32
		mockTransport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if atomic.CompareAndSwapInt32(&step, 0, 1) {
				header := make(http.Header)
				header.Set("Location", "https://api.lever.co/v0/postings/company-new/job1")
				return &http.Response{
					StatusCode: http.StatusMovedPermanently,
					Body:       io.NopCloser(strings.NewReader("Moved")),
					Header:     header,
					Request:    r,
				}, nil
			}
			header := make(http.Header)
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`{"data": {"id": "lever-999"}}`)),
				Header:     header,
				Request:    r,
			}, nil
		})

		httpClient := &http.Client{Transport: mockTransport}
		client := NewClient().WithHTTPClient(httpClient)
		res, err := client.SubmitLever(context.Background(), "company", "job1", profile, answers)
		if err != nil {
			t.Fatalf("expected successful Lever submission after 301 redirect follow, got: %v", err)
		}
		if !res.Success {
			t.Fatal("expected res.Success = true after redirect follow")
		}
	})

	t.Run("Redirect Loop Exceeds Limit (10 redirects)", func(t *testing.T) {
		mockTransport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Location", "/v0/postings/company/loop")
			return &http.Response{
				StatusCode: http.StatusFound,
				Body:       io.NopCloser(strings.NewReader("redirecting")),
				Header:     header,
				Request:    r,
			}, nil
		})

		httpClient := &http.Client{Transport: mockTransport}
		client := NewClient().WithHTTPClient(httpClient)
		_, err := client.SubmitLever(context.Background(), "company", "loop", profile, answers)
		if err == nil {
			t.Fatal("expected error on redirect loop, got nil")
		}
		if !strings.Contains(err.Error(), "stopped after 10 redirects") {
			t.Errorf("unexpected error for redirect loop: %v", err)
		}
	})
}

func TestStressClient_RateLimit429(t *testing.T) {
	profile := createTestProfile()

	t.Run("Greenhouse 429 with Retry-After and JSON body", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Retry-After", "60")
			header.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(`{"error": "Too Many Requests", "retry_after": 60}`)),
				Header:     header,
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitGreenhouse(context.Background(), "stripe", "123", profile, nil)
		if err == nil {
			t.Fatal("expected error on 429, got nil")
		}
		if !strings.Contains(err.Error(), "greenhouse submission returned HTTP 429") {
			t.Errorf("expected HTTP 429 in error, got %v", err)
		}
		if !strings.Contains(err.Error(), "Too Many Requests") {
			t.Errorf("expected response body snippet in error, got %v", err)
		}
	})

	t.Run("Lever 429 with plain text body", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Retry-After", "120")
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader("Rate limit exceeded. Please wait 120 seconds.")),
				Header:     header,
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitLever(context.Background(), "company", "job1", profile, nil)
		if err == nil {
			t.Fatal("expected error on 429, got nil")
		}
		if !strings.Contains(err.Error(), "lever submission returned HTTP 429") {
			t.Errorf("expected HTTP 429 in error, got %v", err)
		}
		if !strings.Contains(err.Error(), "Rate limit exceeded") {
			t.Errorf("expected response body snippet in error, got %v", err)
		}
	})

	t.Run("Greenhouse 429 with massive response body truncated by LimitReader", func(t *testing.T) {
		// Response body larger than 64KB (1MB)
		massiveBody := strings.Repeat("RateLimitExceeded-DataBlock-1234567890 ", 26214) // ~1MB
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(massiveBody)),
				Header:     make(http.Header),
			}, nil
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitGreenhouse(context.Background(), "stripe", "123", profile, nil)
		if err == nil {
			t.Fatal("expected error on 429, got nil")
		}
		if !strings.Contains(err.Error(), "greenhouse submission returned HTTP 429") {
			t.Errorf("expected HTTP 429 in error, got %v", err)
		}
		// Confirm body in error does not exceed 64KB cap (65536 bytes)
		errMsg := err.Error()
		if len(errMsg) > 70000 {
			t.Errorf("error message exceeds 64KB safety limit, got %d bytes", len(errMsg))
		}
	})
}

func TestStressClient_ConnectionTimeouts(t *testing.T) {
	profile := createTestProfile()

	t.Run("Context already canceled before Greenhouse request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // canceled immediately

		client := NewClient()
		_, err := client.SubmitGreenhouse(ctx, "stripe", "123", profile, nil)
		if err == nil {
			t.Fatal("expected error on canceled context, got nil")
		}
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected context canceled error, got %v", err)
		}
	})

	t.Run("Context deadline exceeded during Greenhouse server response delay", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			// Simulate slow server that exceeds client context deadline
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(500 * time.Millisecond):
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"success": true}`)),
					Header:     make(http.Header),
				}, nil
			}
		})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitGreenhouse(ctx, "stripe", "123", profile, nil)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Errorf("expected deadline exceeded, got %v", err)
		}
	})

	t.Run("Context already canceled before Lever request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		client := NewClient()
		_, err := client.SubmitLever(ctx, "company", "job1", profile, nil)
		if err == nil {
			t.Fatal("expected error on canceled context, got nil")
		}
		if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected context canceled error, got %v", err)
		}
	})

	t.Run("Context deadline exceeded during Lever request", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(500 * time.Millisecond):
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"data": {}}`)),
					Header:     make(http.Header),
				}, nil
			}
		})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitLever(ctx, "company", "job1", profile, nil)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Errorf("expected deadline exceeded, got %v", err)
		}
	})

	t.Run("Connection closed abruptly (unexpected EOF)", func(t *testing.T) {
		mockClient := newMockHTTPClient(func(r *http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		})

		client := NewClient().WithHTTPClient(mockClient)
		_, err := client.SubmitGreenhouse(context.Background(), "stripe", "123", profile, nil)
		if err == nil {
			t.Fatal("expected error on unexpected EOF, got nil")
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) && !strings.Contains(err.Error(), "unexpected EOF") {
			t.Errorf("expected unexpected EOF error, got %v", err)
		}
	})
}

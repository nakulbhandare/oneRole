package controller

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok response"))
		case "/warn":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad request"))
		case "/err":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
		}
	})

	loggedHandler := LoggingMiddleware(dummyHandler)

	// 1. Success request
	req1 := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req1.Header.Set("X-Tenant-ID", "tenant-123")
	rec1 := httptest.NewRecorder()
	loggedHandler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	if rec1.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header to be populated")
	}

	// 2. Client warning request (4xx)
	req2 := httptest.NewRequest(http.MethodPost, "/warn", nil)
	rec2 := httptest.NewRecorder()
	loggedHandler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec2.Code)
	}

	// 3. Server error request (5xx)
	req3 := httptest.NewRequest(http.MethodDelete, "/err", nil)
	rec3 := httptest.NewRecorder()
	loggedHandler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec3.Code)
	}

	logs := buf.String()
	if !strings.Contains(logs, "HTTP API Request") {
		t.Errorf("expected logs to contain 'HTTP API Request', got: %s", logs)
	}
	if !strings.Contains(logs, "HTTP API Client Warning") {
		t.Errorf("expected logs to contain 'HTTP API Client Warning', got: %s", logs)
	}
	if !strings.Contains(logs, "HTTP API Error") {
		t.Errorf("expected logs to contain 'HTTP API Error', got: %s", logs)
	}
	if !strings.Contains(logs, "tenant_id=tenant-123") {
		t.Errorf("expected logs to contain tenant_id=tenant-123, got: %s", logs)
	}
}

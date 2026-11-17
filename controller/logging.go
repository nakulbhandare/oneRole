package controller

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

// LoggingMiddleware logs every incoming HTTP request with method, path, status, duration, and metadata.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if reqID == "" {
			reqID = newRequestID()
			r.Header.Set("X-Request-ID", reqID)
		}
		w.Header().Set("X-Request-ID", reqID)

		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		tenantID := r.Header.Get("X-Tenant-ID")

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration_ms", duration.Milliseconds(),
			"duration", duration.String(),
			"bytes", rec.bytesWritten,
			"ip", r.RemoteAddr,
			"request_id", reqID,
		}
		if tenantID != "" {
			attrs = append(attrs, "tenant_id", tenantID)
		}

		if rec.statusCode >= 500 {
			slog.Error("HTTP API Error", attrs...)
		} else if rec.statusCode >= 400 {
			slog.Warn("HTTP API Client Warning", attrs...)
		} else {
			slog.Info("HTTP API Request", attrs...)
		}
	})
}

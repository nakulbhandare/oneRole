package controller

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var errInvalidCORSConfig = errors.New("invalid exact-origin CORS configuration")

const (
	corsAllowedMethods = "GET, POST, OPTIONS"
	corsAllowedHeaders = "Authorization, Content-Type, Idempotency-Key, X-Tenant-ID, X-CSRF-Token, X-Request-ID"
)

var corsAllowedHeaderNames = map[string]struct{}{
	"authorization":   {},
	"content-type":    {},
	"idempotency-key": {},
	"x-tenant-id":     {},
	"x-csrf-token":    {},
	"x-request-id":    {},
}

var corsAllowedMethodNames = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodOptions: {},
}

// newCORSMiddleware accepts only exact configured HTTP origins. Credentials
// are enabled for those origins only; wildcard and reflected origins are
// never used. A preflight succeeds only for documented methods and headers.
func newCORSMiddleware(allowedOrigins []string) (func(http.Handler) http.Handler, error) {
	if len(allowedOrigins) == 0 {
		return nil, errInvalidCORSConfig
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, raw := range allowedOrigins {
		origin, ok := normalizeCORSOrigin(raw)
		if !ok {
			return nil, errInvalidCORSConfig
		}
		allowed[origin] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, errInvalidCORSConfig
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			addVary(w.Header(), "Origin")
			originHeader := r.Header.Get("Origin")
			if originHeader == "" {
				if r.Method == http.MethodOptions {
					writeCORSForbidden(w)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			origin, ok := normalizeCORSOrigin(originHeader)
			if !ok || originHeader != origin {
				writeCORSForbidden(w)
				return
			}
			if _, ok := allowed[origin]; !ok {
				writeCORSForbidden(w)
				return
			}

			if r.Method == http.MethodOptions {
				addVary(w.Header(), "Access-Control-Request-Method")
				addVary(w.Header(), "Access-Control-Request-Headers")
				if !validPreflightMethod(r.Header.Get("Access-Control-Request-Method")) ||
					!validPreflightHeaders(r.Header.Get("Access-Control-Request-Headers")) {
					writeCORSForbidden(w)
					return
				}
				setCORSResponseHeaders(w.Header(), origin)
				w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if _, ok := corsAllowedMethodNames[r.Method]; !ok {
				writeCORSForbidden(w)
				return
			}
			setCORSResponseHeaders(w.Header(), origin)
			next.ServeHTTP(w, r)
		})
	}, nil
}

func normalizeCORSOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" || strings.Contains(parsed.Host, "*") {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", false
	}
	port := parsed.Port()
	if port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", false
		}
		port = strconv.Itoa(portNumber)
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	var host string
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	} else {
		host = hostname
	}
	return scheme + "://" + host, true
}

func validPreflightMethod(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	_, ok := corsAllowedMethodNames[strings.ToUpper(raw)]
	return ok
}

func validPreflightHeaders(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	for _, header := range strings.Split(raw, ",") {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" {
			return false
		}
		if _, ok := corsAllowedHeaderNames[header]; !ok {
			return false
		}
	}
	return true
}

func setCORSResponseHeaders(header http.Header, origin string) {
	header.Set("Access-Control-Allow-Origin", origin)
	header.Set("Access-Control-Allow-Credentials", "true")
}

func writeCORSForbidden(w http.ResponseWriter) {
	http.Error(w, "CORS request is not allowed", http.StatusForbidden)
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, token := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(token), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

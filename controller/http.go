package controller

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const maxJSONRequestBytes = 1 << 20

var errInvalidJSON = errors.New("invalid JSON request")

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code      string           `json:"code"`
	Message   string           `json:"message"`
	RequestID string           `json:"request_id"`
	Details   []map[string]any `json:"details,omitempty"`
}

func writeErrorDetails(w http.ResponseWriter, r *http.Request, status int, code, message string, details []map[string]any) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" || len(requestID) > 128 {
		requestID = newRequestID()
	}
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message, RequestID: requestID, Details: details}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" || len(requestID) > 128 {
		requestID = newRequestID()
	}
	writeJSON(w, status, errorEnvelope{Error: apiError{Code: code, Message: message, RequestID: requestID}})
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errInvalidJSON
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errInvalidJSON
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxJSONRequestBytes+1))
	if err != nil || len(data) > maxJSONRequestBytes {
		return errInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errInvalidJSON
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errInvalidJSON
	}
	return nil
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

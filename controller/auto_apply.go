package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oneRole/service"
)

type AutoApplyAPI interface {
	ApplyOneClick(context.Context, service.ActorScope, service.AutoApplyCommand) (*service.AutoApplyResult, error)
	ApplyBatchOneClick(context.Context, service.ActorScope, service.AutoApplyBatchCommand) (*service.AutoApplyBatchResult, error)
	GetJobRecommendations(context.Context, service.ActorScope, int, int) ([]service.ScoredJob, error)
	GetApplicationResume(context.Context, service.ActorScope, string) (*service.ApplicationResumeView, error)
	AssistApplication(context.Context, service.ActorScope, string) (*service.AutoApplyResult, error)
	CompleteEdgeApplication(context.Context, service.ActorScope, string, string, string) (*service.AutoApplyResult, error)
	SendNotificationEmail(context.Context, service.EmailPayload) (*service.EmailDeliveryResult, error)
	GetMailerStatus() map[string]any
}

type AutoApplyHandler struct {
	api AutoApplyAPI
}

func NewAutoApplyHandler(api AutoApplyAPI) (*AutoApplyHandler, error) {
	if api == nil {
		return nil, errors.New("auto-apply API is required")
	}
	return &AutoApplyHandler{api: api}, nil
}

func (h *AutoApplyHandler) ApplyOneClick(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	var cmd service.AutoApplyCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application request is invalid.")
		return
	}
	result, err := h.api.ApplyOneClick(r.Context(), scope, cmd)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "auto_apply_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AutoApplyHandler) ApplyBatchOneClick(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	var cmd service.AutoApplyBatchCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The batch auto-apply request is invalid.")
		return
	}
	if len(cmd.JobIDs) == 0 {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "At least one job_id is required.")
		return
	}
	result, err := h.api.ApplyBatchOneClick(r.Context(), scope, cmd)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "auto_apply_batch_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AutoApplyHandler) Recommendations(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	minScore := 50
	if s := r.URL.Query().Get("min_score"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			minScore = v
		}
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	results, err := h.api.GetJobRecommendations(r.Context(), scope, minScore, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "recommendations_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": len(results),
		"items": results,
	})
}

func (h *AutoApplyHandler) Resume(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	appID := r.PathValue("applicationID")
	if appID == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Application ID is required.")
		return
	}
	view, err := h.api.GetApplicationResume(r.Context(), scope, appID)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "not_found", err.Error())
		return
	}

	format := r.URL.Query().Get("format")
	accept := r.Header.Get("Accept")
	if format == "html" || (format == "" && strings.Contains(accept, "text/html")) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(view.RenderedHTML))
		return
	}
	if format == "cover" || format == "cover_letter" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(view.CoverLetter))
		return
	}
	if format == "text" || format == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(view.ContentText))
		return
	}

	writeJSON(w, http.StatusOK, view)
}

func (h *AutoApplyHandler) Assist(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	appID := r.PathValue("applicationID")
	if appID == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Application ID is required.")
		return
	}
	result, err := h.api.AssistApplication(r.Context(), scope, appID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "assist_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AutoApplyHandler) EdgeComplete(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	appID := r.PathValue("applicationID")
	if appID == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Application ID is required.")
		return
	}
	var req struct {
		ReceiptID       string `json:"receipt_id"`
		ConfirmationURL string `json:"confirmation_url"`
	}
	_ = decodeJSON(r, &req)
	result, err := h.api.CompleteEdgeApplication(r.Context(), scope, appID, req.ReceiptID, req.ConfirmationURL)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "complete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AutoApplyHandler) EmailStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.api.GetMailerStatus())
}

func (h *AutoApplyHandler) TestEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Recipient string `json:"recipient"`
	}
	_ = decodeJSON(r, &req)
	recipient := strings.TrimSpace(req.Recipient)
	if recipient == "" {
		recipient = "bhandarenakul11@gmail.com"
	}

	now := time.Now().UTC()
	textBody, htmlBody := service.GenerateConfirmationEmailContent("Principal Distributed Systems Engineer", "Stripe", "app-test-"+strconv.FormatInt(now.Unix(), 10), "rcpt-test-verified", now)

	result, err := h.api.SendNotificationEmail(r.Context(), service.EmailPayload{
		Recipient:     recipient,
		Subject:       "Test Notification: Application Submitted at Stripe",
		TextBody:      textBody,
		HTMLBody:      htmlBody,
		ApplicationID: "app-test",
		ReceiptID:     "rcpt-test-verified",
		CompanyName:   "Stripe",
		JobTitle:      "Principal Distributed Systems Engineer",
	})

	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   err.Error(),
			"result":  result,
			"status":  h.api.GetMailerStatus(),
			"hint":    "To receive real emails in bhandarenakul11@gmail.com, configure SMTP_HOST=smtp.gmail.com, SMTP_PORT=587, SMTP_USER, SMTP_PASS (16-char App Password) in your .env",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
		"status":  h.api.GetMailerStatus(),
	})
}

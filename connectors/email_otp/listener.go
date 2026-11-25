package email_otp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Handler exposes HTTP endpoints for email webhook ingestion and browser worker polling.
type Handler struct {
	registry *Registry
}

func NewHandler() *Handler {
	return &Handler{
		registry: GetRegistry(),
	}
}

// IngestWebhook accepts incoming emails from mail servers (SendGrid, Mailgun, AWS SES, or IMAP poller).
// POST /api/v1/auth/email-otp/webhook
func (h *Handler) IngestWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Recipient string `json:"recipient"`
		Sender    string `json:"sender"`
		Subject   string `json:"subject"`
		Body      string `json:"body"`
		Text      string `json:"text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	body := req.Body
	if body == "" {
		body = req.Text
	}

	senderDomain := req.Sender
	if atIdx := strings.LastIndex(req.Sender, "@"); atIdx != -1 {
		senderDomain = req.Sender[atIdx+1:]
	}

	payload := h.registry.DeliverIncoming(req.Recipient, senderDomain, req.Subject, body)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"code":     payload.Code,
		"link":     payload.MagicLink,
		"received": payload.ReceivedAt,
	})
}

// PollOTP allows headless browser workers to poll or wait for an incoming OTP.
// GET /api/v1/auth/email-otp?email={email}&domain={domain}&timeout_sec=30
func (h *Handler) PollOTP(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	domain := r.URL.Query().Get("domain")
	company := r.URL.Query().Get("company")
	if email == "" {
		http.Error(w, "email query param required", http.StatusBadRequest)
		return
	}
	if domain == "" {
		domain = "*"
	}

	timeout := 25 * time.Second
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	payload, err := h.registry.WaitForOTP(ctx, email, domain, company)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestTimeout)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "verification code not received within timeout",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"email":      payload.Email,
		"domain":     payload.Domain,
		"code":       payload.Code,
		"magic_link": payload.MagicLink,
	})
}

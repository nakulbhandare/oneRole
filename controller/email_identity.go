package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/oneRole/internal/emailgateway"
	"github.com/oneRole/internal/emailidentity"
	"github.com/oneRole/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EmailIdentityHandler struct {
	identitySvc *emailidentity.Service
	gateway     *emailgateway.Gateway
	db          *mongo.Database
}

func NewEmailIdentityHandler(identitySvc *emailidentity.Service, gw *emailgateway.Gateway, db *mongo.Database) *EmailIdentityHandler {
	return &EmailIdentityHandler{
		identitySvc: identitySvc,
		gateway:     gw,
		db:          db,
	}
}

// GetIdentity returns the current user's dedicated OneRole application email.
// GET /api/v1/email-identity
func (h *EmailIdentityHandler) GetIdentity(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}

	ident, err := h.identitySvc.GetOrCreateIdentity(r.Context(), scope.UserID, scope.TenantID, "Nakul", "Bhandare", "bhandarenakul11@gmail.com")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "identity_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ident)
}

// UpdateForwarding updates the user's personal forwarding address.
// PUT /api/v1/email-identity/forwarding
func (h *EmailIdentityHandler) UpdateForwarding(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}

	var req struct {
		PersonalEmail string `json:"personal_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Invalid JSON payload.")
		return
	}

	updated, err := h.identitySvc.UpdatePersonalForwarding(r.Context(), scope.UserID, scope.TenantID, req.PersonalEmail)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "update_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// InboundWebhook handles incoming employer emails from inbound mail servers.
// POST /api/v1/inbound-email/webhook
func (h *EmailIdentityHandler) InboundWebhook(w http.ResponseWriter, r *http.Request) {
	var payload emailgateway.InboundWebhookPayload

	// Support both JSON body and multipart form data from email services
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err == nil {
			payload.To = r.FormValue("to")
			payload.From = r.FormValue("from")
			payload.Subject = r.FormValue("subject")
			payload.Text = r.FormValue("text")
			payload.HTML = r.FormValue("html")
			payload.MessageID = r.FormValue("Message-Id")
			if payload.MessageID == "" {
				payload.MessageID = r.FormValue("message_id")
			}
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid_payload", "Could not parse email webhook JSON.")
			return
		}
	}

	if payload.To == "" {
		writeError(w, r, http.StatusBadRequest, "missing_recipient", "Recipient 'to' field is required.")
		return
	}

	res, err := h.gateway.ProcessInbound(r.Context(), payload)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "processing_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"email_id":       res.ID,
		"category":       res.Category,
		"contains_otp":   res.ContainsOTP,
		"receipt_id":     res.VerifiedReceiptID,
		"forward_status": res.ForwardStatus,
		"forwarded_to":   res.ForwardedTo,
	})
}

// ListMessages returns recent employer emails received for the candidate.
// GET /api/v1/email-identity/messages
func (h *EmailIdentityHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}

	if h.db == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"items": []interface{}{}})
		return
	}

	opts := options.Find().SetSort(bson.D{{Key: "received_at", Value: -1}}).SetLimit(50)
	cur, err := h.db.Collection("email_messages").Find(r.Context(), bson.M{"user_id": scope.UserID}, opts)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_error", err.Error())
		return
	}
	defer cur.Close(r.Context())

	var items []models.EmailMessage
	if err := cur.All(r.Context(), &items); err != nil {
		writeError(w, r, http.StatusInternalServerError, "decode_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
		"count": len(items),
	})
}

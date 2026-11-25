package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oneRole/middleware"
	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

// ApplicationAPI is the HTTP-facing application workflow service.
type ApplicationAPI interface {
	CreateApplication(context.Context, service.ActorScope, service.CreateApplicationCommand, string) (service.WorkflowAcceptance, error)
	SubmitApplication(context.Context, service.ActorScope, string, string) (service.WorkflowAcceptance, error)
	CreateBatch(context.Context, service.ActorScope, service.CreateBatchCommand, string) (service.WorkflowAcceptance, error)
	GetApplication(context.Context, service.ActorScope, string) (models.Application, error)
	GetBatch(context.Context, service.ActorScope, string) (service.BatchView, error)
	ListApplications(context.Context, service.ActorScope, *string, string, int) (service.ApplicationListPage, error)
	ClearApplications(context.Context, service.ActorScope) error
	GetApplicationTimeline(context.Context, service.ActorScope, string) ([]models.ApplicationEvent, error)
	GetApplicationReceipt(context.Context, service.ActorScope, string) (*models.ApplicationReceipt, error)
}

type ApplicationHandler struct {
	api ApplicationAPI
}

func NewApplicationHandler(api ApplicationAPI) (*ApplicationHandler, error) {
	if api == nil {
		return nil, errors.New("application API is required")
	}
	return &ApplicationHandler{api: api}, nil
}

func (h *ApplicationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var command service.CreateApplicationCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application request is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	accepted, err := h.api.CreateApplication(r.Context(), scope, command, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeAccepted(w, r, accepted, "/api/v1/applications/"+accepted.WorkflowID)
}

func (h *ApplicationHandler) Get(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationID")
	if !isUUID(applicationID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application ID is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	application, err := h.api.GetApplication(r.Context(), scope, applicationID)
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	if !isUUID(application.ID) || !isUUID(application.JobID) || application.UpdatedAt.IsZero() || !publicStatus(application.Status) {
		writeError(w, r, http.StatusServiceUnavailable, "application_unavailable", "The application could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, application)
}

func (h *ApplicationHandler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	q := r.URL.Query()
	var statusFilter *string
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		statusFilter = &s
	}
	limit := 25
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	cursor := strings.TrimSpace(q.Get("cursor"))

	page, err := h.api.ListApplications(r.Context(), scope, statusFilter, cursor, limit)
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *ApplicationHandler) Clear(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	if err := h.api.ClearApplications(r.Context(), scope); err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *ApplicationHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationID")
	if !isUUID(applicationID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application ID is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	events, err := h.api.GetApplicationTimeline(r.Context(), scope, applicationID)
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"application_id": applicationID,
		"events":         events,
	})
}

func (h *ApplicationHandler) Receipt(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationID")
	if !isUUID(applicationID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application ID is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	receipt, err := h.api.GetApplicationReceipt(r.Context(), scope, applicationID)
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (h *ApplicationHandler) Submit(w http.ResponseWriter, r *http.Request) {
	applicationID := r.PathValue("applicationID")
	if !isUUID(applicationID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application ID is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	accepted, err := h.api.SubmitApplication(r.Context(), scope, applicationID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeAccepted(w, r, accepted, "/api/v1/applications/"+applicationID)
}

func (h *ApplicationHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var command service.CreateBatchCommand
	if err := decodeJSON(r, &command); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The batch request is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	accepted, err := h.api.CreateBatch(r.Context(), scope, command, r.Header.Get("Idempotency-Key"))
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeAccepted(w, r, accepted, "/api/v1/application-batches/"+accepted.WorkflowID)
}

func (h *ApplicationHandler) GetBatch(w http.ResponseWriter, r *http.Request) {
	batchID := r.PathValue("batchID")
	if !isUUID(batchID) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The batch ID is invalid.")
		return
	}
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	batch, err := h.api.GetBatch(r.Context(), scope, batchID)
	if err != nil {
		h.writeApplicationError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, batchResponse{
		ID: batch.ID, Total: batch.Total, Counts: batch.Counts, CreatedAt: batch.CreatedAt,
	})
}

func (h *ApplicationHandler) writeApplicationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrApplicationRequest):
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The application request is invalid.")
	case errors.Is(err, service.ErrIdempotencyConflict), errors.Is(err, service.ErrApplicationDuplicate), errors.Is(err, service.ErrApplicationStateConflict):
		writeError(w, r, http.StatusConflict, "conflict", "The application workflow conflicts with current state.")
	case errors.Is(err, service.ErrApplicationNotFound):
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
	case errors.Is(err, service.ErrConsentInvalid), errors.Is(err, service.ErrApplicationResourceInvalid), errors.Is(err, service.ErrApplicationPolicy):
		writeError(w, r, http.StatusUnprocessableEntity, "policy_rejected", "The application cannot be accepted under the current consent or policy.")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "application_unavailable", "The application service could not complete the request.")
	}
}

type workflowAcceptedResponse struct {
	WorkflowID string    `json:"workflow_id"`
	Status     string    `json:"status"`
	StatusURL  string    `json:"status_url"`
	AcceptedAt time.Time `json:"accepted_at"`
	Message    string    `json:"message"`
}

type batchResponse struct {
	ID        string         `json:"id"`
	Total     int            `json:"total"`
	Counts    map[string]int `json:"counts"`
	CreatedAt time.Time      `json:"created_at"`
}

func writeAccepted(w http.ResponseWriter, r *http.Request, accepted service.WorkflowAcceptance, statusURL string) {
	if accepted.Validate() != nil || !strings.HasPrefix(statusURL, "/api/v1/") {
		writeError(w, r, http.StatusServiceUnavailable, "application_unavailable", "The application workflow could not be accepted.")
		return
	}
	w.Header().Set("Location", statusURL)
	writeJSON(w, http.StatusAccepted, workflowAcceptedResponse{
		WorkflowID: accepted.WorkflowID,
		Status:     accepted.Status,
		StatusURL:  statusURL,
		AcceptedAt: accepted.AcceptedAt,
		Message:    "Accepted by OneRole; employer submission is not yet confirmed.",
	})
}

func actorScope(r *http.Request) (service.ActorScope, bool) {
	principal, principalOK := middleware.PrincipalFromContext(r.Context())
	membership, membershipOK := middleware.TenantMembershipFromContext(r.Context())
	if !principalOK || !membershipOK || principal.UserID == "" || membership.UserID != principal.UserID || membership.Status != "active" || membership.TenantID == "" {
		return service.ActorScope{}, false
	}
	return service.ActorScope{TenantID: membership.TenantID, UserID: principal.UserID}, true
}

func publicStatus(status models.PublicApplicationStatus) bool {
	switch status {
	case models.PublicApplicationPreparing, models.PublicApplicationNeedsUserInput,
		models.PublicApplicationReadyForReview, models.PublicApplicationApproved,
		models.PublicApplicationQueued, models.PublicApplicationSubmitting,
		models.PublicApplicationInFlight,
		models.PublicApplicationSubmitted, models.PublicApplicationSubmissionUnconfirmed,
		models.PublicApplicationRequiresManualAction, models.PublicApplicationFailedRetryable,
		models.PublicApplicationFailedPermanent, models.PublicApplicationDuplicate,
		models.PublicApplicationExpired, models.PublicApplicationSkipped,
		models.PublicApplicationRejectedByUser:
		return true
	default:
		return false
	}
}

func isUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for i, ch := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
			return false
		}
	}
	return true
}

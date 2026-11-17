package controller

import (
	"context"
	"errors"
	"net/http"

	"github.com/oneRole/models"
	"github.com/oneRole/service"
)

type ProfileAPI interface {
	GetProfile(context.Context, service.ActorScope) (*models.CandidateProfile, error)
	UpdateProfile(context.Context, service.ActorScope, models.CandidateProfile) (*models.CandidateProfile, error)
}

type ProfileHandler struct {
	api ProfileAPI
}

func NewProfileHandler(api ProfileAPI) (*ProfileHandler, error) {
	if api == nil {
		return nil, errors.New("profile API is required")
	}
	return &ProfileHandler{api: api}, nil
}

func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	profile, err := h.api.GetProfile(r.Context(), scope)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "profile_error", "Failed to retrieve profile.")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	scope, ok := actorScope(r)
	if !ok {
		writeError(w, r, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	var update models.CandidateProfile
	if err := decodeJSON(r, &update); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "The profile request is invalid.")
		return
	}
	profile, err := h.api.UpdateProfile(r.Context(), scope, update)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "profile_error", "Failed to update profile.")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

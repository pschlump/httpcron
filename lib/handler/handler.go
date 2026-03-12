// Package handler implements the HTTP handlers for the HTTPCron API.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/pschlump/httpcron/lib/repository"
)

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	repo            *repository.Repository
	registrationKey string
	log             *slog.Logger
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(repo *repository.Repository, registrationKey string, log *slog.Logger) *Handler {
	return &Handler{
		repo:            repo,
		registrationKey: registrationKey,
		log:             log,
	}
}

// --- helpers ----------------------------------------------------------------

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- POST /api/v1/self-register ---------------------------------------------

type selfRegisterRequest struct {
	HostName        string `json:"host_name"`
	RegistrationKey string `json:"registration_key"`
	HostURL         string `json:"host_url"`
}

type selfRegisterResponse struct {
	UserID        string `json:"user_id"`
	PerUserAPIKey string `json:"per_user_api_key"`
}

// SelfRegister validates the registration_key and creates a new user.
func (h *Handler) SelfRegister(w http.ResponseWriter, r *http.Request) {
	var req selfRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.HostName == "" || req.RegistrationKey == "" || req.HostURL == "" {
		writeError(w, http.StatusBadRequest, "host_name, registration_key, and host_url are required")
		return
	}
	if req.RegistrationKey != h.registrationKey {
		writeError(w, http.StatusUnauthorized, "invalid registration_key")
		return
	}

	user, err := h.repo.CreateUser(r.Context(), req.HostName, req.HostURL)
	if err != nil {
		h.log.Error("create user", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, selfRegisterResponse{
		UserID:        user.UserID,
		PerUserAPIKey: user.PerUserAPIKey,
	})
}

// --- POST /api/v1/create-timed-event ----------------------------------------

type createTimedEventRequest struct {
	EventName    string `json:"event_name"`
	PerUserAPIKey string `json:"per_user_api_key"`
	CronSpec     string `json:"cron_spec"`
	HumanSpec    string `json:"human_spec"`
	BodyTemplate string `json:"body_template"`
}

// CreateTimedEvent creates a new scheduled event for the authenticated user.
func (h *Handler) CreateTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req createTimedEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.EventName == "" || req.PerUserAPIKey == "" {
		writeError(w, http.StatusBadRequest, "event_name and per_user_api_key are required")
		return
	}
	if req.CronSpec == "" && req.HumanSpec == "" {
		writeError(w, http.StatusBadRequest, "cron_spec or human_spec is required")
		return
	}

	user, err := h.repo.GetUserByAPIKey(r.Context(), req.PerUserAPIKey)
	if err != nil {
		h.log.Error("get user by api key", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid per_user_api_key")
		return
	}

	event, err := h.repo.CreateEvent(r.Context(), user.UserID, req.EventName, req.CronSpec, req.HumanSpec, req.BodyTemplate)
	if err != nil {
		h.log.Error("create event", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"event_id": event.EventID})
}

// --- POST /api/v1/update-timed-event ----------------------------------------

type updateTimedEventRequest struct {
	EventID      string  `json:"event_id"`
	PerUserAPIKey string  `json:"per_user_api_key"`
	EventName    *string `json:"event_name"`
	CronSpec     *string `json:"cron_spec"`
	HumanSpec    *string `json:"human_spec"`
	BodyTemplate *string `json:"body_template"`
}

// UpdateTimedEvent updates the specified fields of an existing event.
func (h *Handler) UpdateTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req updateTimedEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.EventID == "" || req.PerUserAPIKey == "" {
		writeError(w, http.StatusBadRequest, "event_id and per_user_api_key are required")
		return
	}

	user, err := h.repo.GetUserByAPIKey(r.Context(), req.PerUserAPIKey)
	if err != nil {
		h.log.Error("get user by api key", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid per_user_api_key")
		return
	}

	params := repository.UpdateEventParams{
		EventName:    req.EventName,
		CronSpec:     req.CronSpec,
		HumanSpec:    req.HumanSpec,
		BodyTemplate: req.BodyTemplate,
	}
	if err := h.repo.UpdateEvent(r.Context(), req.EventID, user.UserID, params); err != nil {
		if err.Error() == "event not found" {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		h.log.Error("update event", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /api/v1/delete-timed-event ----------------------------------------

type deleteTimedEventRequest struct {
	EventID string `json:"event_id"`
}

// DeleteTimedEvent removes the specified event.
func (h *Handler) DeleteTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req deleteTimedEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.EventID == "" {
		writeError(w, http.StatusBadRequest, "event_id is required")
		return
	}

	if err := h.repo.DeleteEvent(r.Context(), req.EventID); err != nil {
		if err.Error() == "event not found" {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		h.log.Error("delete event", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /api/v1/list-timed-event ------------------------------------------

type perUserAPIKeyRequest struct {
	PerUserAPIKey string `json:"per_user_api_key"`
}

type eventListResponse struct {
	Status string                 `json:"status"`
	Data   []repository.UserEvent `json:"data"`
}

// ListTimedEvent returns all events for the authenticated user.
func (h *Handler) ListTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req perUserAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.PerUserAPIKey == "" {
		writeError(w, http.StatusBadRequest, "per_user_api_key is required")
		return
	}

	user, err := h.repo.GetUserByAPIKey(r.Context(), req.PerUserAPIKey)
	if err != nil {
		h.log.Error("get user by api key", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid per_user_api_key")
		return
	}

	events, err := h.repo.ListEvents(r.Context(), user.UserID)
	if err != nil {
		h.log.Error("list events", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if events == nil {
		events = []repository.UserEvent{}
	}
	writeJSON(w, http.StatusOK, eventListResponse{Status: "success", Data: events})
}

// --- POST /api/v1/search-timed-event ----------------------------------------

type searchTimedEventRequest struct {
	PerUserAPIKey string `json:"per_user_api_key"`
	EventName     string `json:"event_name"`
}

// SearchTimedEvent returns events whose name partially matches the given string.
func (h *Handler) SearchTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req searchTimedEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.PerUserAPIKey == "" {
		writeError(w, http.StatusBadRequest, "per_user_api_key is required")
		return
	}

	user, err := h.repo.GetUserByAPIKey(r.Context(), req.PerUserAPIKey)
	if err != nil {
		h.log.Error("get user by api key", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "invalid per_user_api_key")
		return
	}

	events, err := h.repo.SearchEvents(r.Context(), user.UserID, req.EventName)
	if err != nil {
		h.log.Error("search events", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if events == nil {
		events = []repository.UserEvent{}
	}
	writeJSON(w, http.StatusOK, eventListResponse{Status: "success", Data: events})
}

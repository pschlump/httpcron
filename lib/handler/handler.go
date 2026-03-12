// Package handler implements the HTTP handlers for the HTTPCron API.
package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/pschlump/englishtocron"
	"github.com/pschlump/httpcron/lib/config"
	"github.com/pschlump/httpcron/lib/repository"
	"github.com/pschlump/httpcron/lib/scheduler"
	"github.com/pschlump/httpcron/lib/utils"
)

// humanToCron converts an English schedule description to a cron spec.
// Returns an error if the description cannot be parsed.
func humanToCron(humanSpec string) (string, error) {
	return englishtocron.StrCronSyntax(humanSpec)
}

// Handler holds dependencies for all HTTP handlers.
type Handler struct {
	repo            *repository.Repository
	registrationKey string
	log             *slog.Logger
	sched           *scheduler.Scheduler
	cfg             *config.Config
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(repo *repository.Repository, registrationKey string, log *slog.Logger, sched *scheduler.Scheduler, cfg *config.Config) *Handler {
	return &Handler{
		repo:            repo,
		registrationKey: registrationKey,
		log:             log,
		sched:           sched,
		cfg:             cfg,
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
	HostName        string `json:"host_name" validate:"required"`
	RegistrationKey string `json:"registration_key" validate:"required"`
	HostURL         string `json:"host_url" validate:"required,url"`
}

type selfRegisterResponse struct {
	UserID        string `json:"user_id"`
	PerUserAPIKey string `json:"per_user_api_key"`
}

// SelfRegister validates the registration_key and creates a new user.
func (h *Handler) SelfRegister(w http.ResponseWriter, r *http.Request) {
	var req selfRegisterRequest
	if err := utils.JsonBody(w, r, h.cfg, &req); err != nil {
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
	EventName     string `json:"event_name" validate:"required"`
	PerUserAPIKey string `json:"per_user_api_key" validate:"required"`
	CronSpec      string `json:"cron_spec"`
	HumanSpec     string `json:"human_spec"`
	BodyTemplate  string `json:"body_template"`
	URL           string `json:"url" validate:"required,url"`
	HTTPMethod    string `json:"http_method" default:"POST"`
}

// CreateTimedEvent creates a new scheduled event for the authenticated user.
func (h *Handler) CreateTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req createTimedEventRequest
	if err := utils.JsonBody(w, r, h.cfg, &req); err != nil {
		return
	}
	if req.CronSpec == "" && req.HumanSpec == "" {
		writeError(w, http.StatusBadRequest, "cron_spec or human_spec is required")
		return
	}
	if req.HumanSpec != "" && req.CronSpec == "" {
		spec, err := humanToCron(req.HumanSpec)
		if err != nil {
			writeError(w, http.StatusBadRequest, "cannot parse human_spec: "+err.Error())
			return
		}
		req.CronSpec = spec
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

	event, err := h.repo.CreateEvent(r.Context(), user.UserID, req.EventName, req.CronSpec, req.HumanSpec, req.BodyTemplate, req.URL, req.HTTPMethod)
	if err != nil {
		h.log.Error("create event", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Add the event to the running scheduler (if available).
	if h.sched != nil {
		if err := h.sched.AddEvent(r.Context(), *event); err != nil {
			h.log.Error("add event to scheduler", "event_id", event.EventID, "err", err)
			// Event was created in DB but not added to scheduler; log it but don't fail the request.
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"event_id": event.EventID})
}

// --- POST /api/v1/update-timed-event ----------------------------------------

type updateTimedEventRequest struct {
	EventID       string  `json:"event_id" validate:"required"`
	PerUserAPIKey string  `json:"per_user_api_key" validate:"required"`
	EventName     *string `json:"event_name"`
	CronSpec      *string `json:"cron_spec"`
	HumanSpec     *string `json:"human_spec"`
	BodyTemplate  *string `json:"body_template"`
	URL           *string `json:"url" validate:"url"`
	HTTPMethod    *string `json:"http_method"`
}

// UpdateTimedEvent updates the specified fields of an existing event.
func (h *Handler) UpdateTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req updateTimedEventRequest
	if err := utils.JsonBody(w, r, h.cfg, &req); err != nil {
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

	// If human_spec is provided without an explicit cron_spec, derive cron_spec from it.
	if req.HumanSpec != nil && *req.HumanSpec != "" && req.CronSpec == nil {
		spec, err := humanToCron(*req.HumanSpec)
		if err != nil {
			writeError(w, http.StatusBadRequest, "cannot parse human_spec: "+err.Error())
			return
		}
		req.CronSpec = &spec
	}

	params := repository.UpdateEventParams{
		EventName:    req.EventName,
		CronSpec:     req.CronSpec,
		HumanSpec:    req.HumanSpec,
		BodyTemplate: req.BodyTemplate,
		URL:          req.URL,
		HTTPMethod:   req.HTTPMethod,
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

	// Get the updated event from the database to pass to the scheduler (if available).
	if h.sched != nil {
		events, err := h.repo.ListEvents(r.Context(), user.UserID)
		if err != nil {
			h.log.Error("list events after update", "err", err)
		} else {
			for _, ev := range events {
				if ev.EventID == req.EventID {
					// Update the event in the running scheduler.
					if err := h.sched.UpdateEvent(r.Context(), ev); err != nil {
						h.log.Error("update event in scheduler", "event_id", ev.EventID, "err", err)
						// Event was updated in DB but not in scheduler; log it but don't fail the request.
					}
					break
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /api/v1/delete-timed-event ----------------------------------------

type deleteTimedEventRequest struct {
	EventID string `json:"event_id" validate:"required"`
}

// DeleteTimedEvent removes the specified event.
func (h *Handler) DeleteTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req deleteTimedEventRequest
	if err := utils.JsonBody(w, r, h.cfg, &req); err != nil {
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

	// Remove the event from the running scheduler (if available).
	if h.sched != nil {
		if err := h.sched.DeleteEvent(r.Context(), req.EventID); err != nil {
			h.log.Error("delete event from scheduler", "event_id", req.EventID, "err", err)
			// Event was deleted from DB but not from scheduler; log it but don't fail the request.
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /api/v1/list-timed-event ------------------------------------------

type perUserAPIKeyRequest struct {
	PerUserAPIKey string `json:"per_user_api_key" validate:"required"`
}

type eventListResponse struct {
	Status string                 `json:"status"`
	Data   []repository.UserEvent `json:"data"`
}

// ListTimedEvent returns all events for the authenticated user.
func (h *Handler) ListTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req perUserAPIKeyRequest
	if err := utils.JsonBody(w, r, h.cfg, &req); err != nil {
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
	PerUserAPIKey string `json:"per_user_api_key" validate:"required"`
	EventName     string `json:"event_name"`
}

// SearchTimedEvent returns events whose name partially matches the given string.
func (h *Handler) SearchTimedEvent(w http.ResponseWriter, r *http.Request) {
	var req searchTimedEventRequest
	if err := utils.JsonBody(w, r, h.cfg, &req); err != nil {
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

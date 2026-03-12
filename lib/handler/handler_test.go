// Package handler tests the HTTP handlers for the HTTPCron API.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pschlump/httpcron/lib/repository"
)

const testRegKey = "test-registration-key-abc123"

// newTestHandler creates a handler with a test repository and logger.
func newTestHandler(t *testing.T) (*Handler, func()) {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	h := NewHandler(repo, testRegKey, log, nil)

	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
	}

	return h, cleanup
}

// TestNewHandler verifies that a new handler can be created.
func TestNewHandler(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()
	os.Remove(dbPath)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	h := NewHandler(repo, testRegKey, log, nil)

	if h == nil {
		t.Fatal("NewHandler returned nil")
	}
	if h.repo != repo {
		t.Error("repo not set correctly")
	}
	if h.registrationKey != testRegKey {
		t.Error("registrationKey not set correctly")
	}
	if h.log != log {
		t.Error("log not set correctly")
	}
	if h.sched != nil {
		t.Error("scheduler should be nil for tests")
	}
}

// TestSelfRegister_Success verifies successful user registration.
func TestSelfRegister_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]string{
		"host_name":        "test-host",
		"registration_key": testRegKey,
		"host_url":         "http://localhost:8080",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/self-register", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.SelfRegister(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody selfRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if respBody.UserID == "" {
		t.Error("user_id is empty")
	}
	if respBody.PerUserAPIKey == "" {
		t.Error("per_user_api_key is empty")
	}
	if len(respBody.PerUserAPIKey) < 4 || respBody.PerUserAPIKey[:4] != "uak-" {
		t.Errorf("per_user_api_key should start with 'uak-', got %q", respBody.PerUserAPIKey)
	}
}

// TestSelfRegister_WrongKey verifies that wrong registration key returns 401.
func TestSelfRegister_WrongKey(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]string{
		"host_name":        "test-host",
		"registration_key": "wrong-key",
		"host_url":         "http://localhost:8080",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/self-register", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.SelfRegister(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestSelfRegister_MissingFields verifies that missing required fields returns 400.
func TestSelfRegister_MissingFields(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	tests := []struct {
		name       string
		body       map[string]string
		wantStatus int
	}{
		{
			name: "missing host_name",
			body: map[string]string{
				"registration_key": testRegKey,
				"host_url":         "http://localhost:8080",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing registration_key",
			body: map[string]string{
				"host_name": "test-host",
				"host_url":  "http://localhost:8080",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing host_url",
			body: map[string]string{
				"host_name":        "test-host",
				"registration_key": testRegKey,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty body",
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, _ := json.Marshal(tt.body)

			req := httptest.NewRequest("POST", "/api/v1/self-register", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.SelfRegister(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

// TestSelfRegister_InvalidJSON verifies that invalid JSON returns 400.
func TestSelfRegister_InvalidJSON(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/api/v1/self-register", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.SelfRegister(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

// TestCreateTimedEvent_Success verifies successful event creation with cron spec.
func TestCreateTimedEvent_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// First, register a user.
	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	body := map[string]interface{}{
		"event_name":       "test-event",
		"per_user_api_key": user.PerUserAPIKey,
		"cron_spec":        "0 0 * * *",
		"url":              "http://example.com",
		"http_method":      "POST",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/create-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	eventID := respBody["event_id"]
	if eventID == "" {
		t.Error("event_id is empty")
	}
	if len(eventID) < 5 || eventID[:5] != "evid-" {
		t.Errorf("event_id should start with 'evid-', got %q", eventID)
	}
}

// TestCreateTimedEvent_HumanSpec verifies successful event creation with human spec.
func TestCreateTimedEvent_HumanSpec(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	// First, register a user.
	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	body := map[string]interface{}{
		"event_name":       "test-event",
		"per_user_api_key": user.PerUserAPIKey,
		"human_spec":       "every day",
		"url":              "http://example.com",
		"http_method":      "GET",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/create-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	eventID := respBody["event_id"]
	if eventID == "" {
		t.Error("event_id is empty")
	}
}

// TestCreateTimedEvent_InvalidAPIKey verifies that invalid API key returns 401.
func TestCreateTimedEvent_InvalidAPIKey(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]interface{}{
		"event_name":       "test-event",
		"per_user_api_key": "invalid-key",
		"cron_spec":        "0 0 * * *",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/create-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestCreateTimedEvent_MissingRequiredFields verifies validation of required fields.
func TestCreateTimedEvent_MissingRequiredFields(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name: "missing event_name",
			body: map[string]interface{}{
				"per_user_api_key": user.PerUserAPIKey,
				"cron_spec":        "0 0 * * *",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing per_user_api_key",
			body: map[string]interface{}{
				"event_name": "test-event",
				"cron_spec":  "0 0 * * *",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing both cron_spec and human_spec",
			body: map[string]interface{}{
				"event_name":       "test-event",
				"per_user_api_key": user.PerUserAPIKey,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, _ := json.Marshal(tt.body)

			req := httptest.NewRequest("POST", "/api/v1/create-timed-event", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			h.CreateTimedEvent(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

// TestCreateTimedEvent_InvalidHumanSpec verifies that invalid human spec returns error.
func TestCreateTimedEvent_InvalidHumanSpec(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	body := map[string]interface{}{
		"event_name":       "test-event",
		"per_user_api_key": user.PerUserAPIKey,
		"human_spec":       "invalid schedule spec",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/create-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	var respBody map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.Contains(respBody["error"], "cannot parse human_spec") {
		t.Errorf("error message should mention human_spec parsing, got %q", respBody["error"])
	}
}

// TestUpdateTimedEvent_Success verifies successful event update.
func TestUpdateTimedEvent_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := h.repo.CreateEvent(context.Background(), user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	newName := "updated-event"
	body := map[string]interface{}{
		"event_id":         event.EventID,
		"per_user_api_key": user.PerUserAPIKey,
		"event_name":       newName,
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/update-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify the event was updated.
	events, err := h.repo.ListEvents(context.Background(), user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName != newName {
		t.Errorf("event_name = %q, want %q", events[0].EventName, newName)
	}
}

// TestUpdateTimedEvent_NotFound verifies that updating non-existent event returns 404.
func TestUpdateTimedEvent_NotFound(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	body := map[string]interface{}{
		"event_id":         "evid-nonexistent",
		"per_user_api_key": user.PerUserAPIKey,
		"event_name":       "updated",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/update-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

// TestUpdateTimedEvent_WithHumanSpec verifies update with human spec conversion.
func TestUpdateTimedEvent_WithHumanSpec(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := h.repo.CreateEvent(context.Background(), user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	body := map[string]interface{}{
		"event_id":         event.EventID,
		"per_user_api_key": user.PerUserAPIKey,
		"human_spec":       "every hour",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/update-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify the event was updated.
	events, err := h.repo.ListEvents(context.Background(), user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if events[0].HumanSpec != "every hour" {
		t.Errorf("human_spec = %q, want %q", events[0].HumanSpec, "every hour")
	}
}

// TestUpdateTimedEvent_InvalidAPIKey verifies that invalid API key returns 401.
func TestUpdateTimedEvent_InvalidAPIKey(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]interface{}{
		"event_id":         "evid-test",
		"per_user_api_key": "invalid-key",
		"event_name":       "updated",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/update-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.UpdateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestDeleteTimedEvent_Success verifies successful event deletion.
func TestDeleteTimedEvent_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := h.repo.CreateEvent(context.Background(), user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	body := map[string]string{
		"event_id": event.EventID,
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/delete-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.DeleteTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify the event was deleted.
	events, err := h.repo.ListEvents(context.Background(), user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after deletion, got %d", len(events))
	}
}

// TestDeleteTimedEvent_NotFound verifies that deleting non-existent event returns 404.
func TestDeleteTimedEvent_NotFound(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]string{
		"event_id": "evid-nonexistent",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/delete-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.DeleteTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", resp.StatusCode)
	}
}

// TestDeleteTimedEvent_MissingEventID verifies that missing event_id returns 400.
func TestDeleteTimedEvent_MissingEventID(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]string{}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/delete-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.DeleteTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
}

// TestListTimedEvent_Success verifies successful event listing.
func TestListTimedEvent_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create some events.
	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "event-1", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "event-2", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	body := map[string]string{
		"per_user_api_key": user.PerUserAPIKey,
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/list-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ListTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody eventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if respBody.Status != "success" {
		t.Errorf("status = %q, want %q", respBody.Status, "success")
	}
	if len(respBody.Data) != 2 {
		t.Errorf("expected 2 events, got %d", len(respBody.Data))
	}
}

// TestListTimedEvent_Empty verifies listing when user has no events.
func TestListTimedEvent_Empty(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	body := map[string]string{
		"per_user_api_key": user.PerUserAPIKey,
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/list-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ListTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody eventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if respBody.Status != "success" {
		t.Errorf("status = %q, want %q", respBody.Status, "success")
	}
	if len(respBody.Data) != 0 {
		t.Errorf("expected 0 events, got %d", len(respBody.Data))
	}
}

// TestListTimedEvent_InvalidAPIKey verifies that invalid API key returns 401.
func TestListTimedEvent_InvalidAPIKey(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	body := map[string]string{
		"per_user_api_key": "invalid-key",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/list-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ListTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", resp.StatusCode)
	}
}

// TestSearchTimedEvent_Success verifies successful event search.
func TestSearchTimedEvent_Success(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create some events.
	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "daily-backup", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "hourly-ping", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "weekly-report", "0 0 * * 0", "weekly", "{}", "http://example.com/3", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	body := map[string]string{
		"per_user_api_key": user.PerUserAPIKey,
		"event_name":       "backup",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/search-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.SearchTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody eventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if respBody.Status != "success" {
		t.Errorf("status = %q, want %q", respBody.Status, "success")
	}
	if len(respBody.Data) != 1 {
		t.Errorf("expected 1 event matching 'backup', got %d", len(respBody.Data))
	}
	if respBody.Data[0].EventName != "daily-backup" {
		t.Errorf("event_name = %q, want 'daily-backup'", respBody.Data[0].EventName)
	}
}

// TestSearchTimedEvent_NoMatch verifies search with no matching events.
func TestSearchTimedEvent_NoMatch(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	body := map[string]string{
		"per_user_api_key": user.PerUserAPIKey,
		"event_name":       "nonexistent",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/search-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.SearchTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody eventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(respBody.Data) != 0 {
		t.Errorf("expected 0 events, got %d", len(respBody.Data))
	}
}

// TestSearchTimedEvent_EmptySearch verifies search with empty string returns all events.
func TestSearchTimedEvent_EmptySearch(t *testing.T) {
	h, cleanup := newTestHandler(t)
	defer cleanup()

	user, err := h.repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "event-1", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = h.repo.CreateEvent(context.Background(), user.UserID, "event-2", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	body := map[string]string{
		"per_user_api_key": user.PerUserAPIKey,
		"event_name":       "",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/search-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.SearchTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var respBody eventListResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(respBody.Data) != 2 {
		t.Errorf("expected 2 events with empty search, got %d", len(respBody.Data))
	}
}

// TestSchedulerIntegration verifies that handlers work with nil scheduler (graceful degradation).
func TestSchedulerIntegration(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create handler with nil scheduler (simulating test environment).
	h := NewHandler(repo, testRegKey, log, nil)

	user, err := repo.CreateUser(context.Background(), "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create an event via handler - should succeed even without scheduler.
	body := map[string]interface{}{
		"event_name":       "test-event",
		"per_user_api_key": user.PerUserAPIKey,
		"cron_spec":        "0 0 * * *",
		"url":              "http://example.com",
		"http_method":      "POST",
	}
	reqBody, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/create-timed-event", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.CreateTimedEvent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify the event was created in the database.
	events, err := repo.ListEvents(context.Background(), user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event in database, got %d", len(events))
	}
	if events[0].EventName != "test-event" {
		t.Errorf("event_name = %q, want 'test-event'", events[0].EventName)
	}
}

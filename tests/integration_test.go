package tests

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	schedpkg "github.com/pschlump/httpcron/lib/scheduler"

	"github.com/pschlump/httpcron/lib/handler"
	"github.com/pschlump/httpcron/lib/repository"
)

const testRegKey = "test-registration-key-abc123"

// newTestServer creates an in-memory test server backed by a temp SQLite file.
func newTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	repo, err := repository.NewRepository(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Note: Scheduler and cfg are nil for tests; the handler will skip scheduler operations when not started.
	h := handler.NewHandler(repo, testRegKey, log, (*schedpkg.Scheduler)(nil), nil)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))

	r.Post("/api/v1/self-register", h.SelfRegister)
	r.Post("/api/v1/create-timed-event", h.CreateTimedEvent)
	r.Post("/api/v1/update-timed-event", h.UpdateTimedEvent)
	r.Post("/api/v1/delete-timed-event", h.DeleteTimedEvent)
	r.Post("/api/v1/list-timed-event", h.ListTimedEvent)
	r.Post("/api/v1/search-timed-event", h.SearchTimedEvent)

	ts := httptest.NewServer(r)
	return ts, func() {
		ts.Close()
		repo.Close()
	}
}

// postJSON sends a POST with a JSON body and decodes the JSON response.
func postJSON(t *testing.T, ts *httptest.Server, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return resp, result
}

// registerUser is a test helper that registers a user and returns its API key.
func registerUser(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	_, body := postJSON(t, ts, "/api/v1/self-register", map[string]string{
		"host_name":        "test-host",
		"registration_key": testRegKey,
		"host_url":         "http://localhost:9090",
	})
	key, _ := body["per_user_api_key"].(string)
	if key == "" {
		t.Fatal("registerUser: no per_user_api_key in response")
	}
	return key
}

// ---- /api/v1/self-register -------------------------------------------------

func TestSelfRegister_Success(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	resp, body := postJSON(t, ts, "/api/v1/self-register", map[string]string{
		"host_name":        "my-host",
		"registration_key": testRegKey,
		"host_url":         "http://localhost:9090",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if _, ok := body["user_id"]; !ok {
		t.Error("missing user_id")
	}
	apiKey, _ := body["per_user_api_key"].(string)
	if apiKey == "" {
		t.Error("missing per_user_api_key")
	}
	if len(apiKey) < 4 || apiKey[:4] != "uak-" {
		t.Errorf("per_user_api_key should start with 'uak-', got %q", apiKey)
	}
}

func TestSelfRegister_WrongKey(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	resp, _ := postJSON(t, ts, "/api/v1/self-register", map[string]string{
		"host_name":        "my-host",
		"registration_key": "wrong-key",
		"host_url":         "http://localhost:9090",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestSelfRegister_MissingFields(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	resp, _ := postJSON(t, ts, "/api/v1/self-register", map[string]string{
		"host_name": "only-name",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", resp.StatusCode)
	}
}

// ---- /api/v1/create-timed-event --------------------------------------------

func TestCreateTimedEvent_CronSpec(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	resp, body := postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name":       "backup",
		"per_user_api_key": apiKey,
		"cron_spec":        "*/5 * * * *",
	})

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	eventID, _ := body["event_id"].(string)
	if eventID == "" {
		t.Error("missing event_id")
	}
	if len(eventID) < 5 || eventID[:5] != "evid-" {
		t.Errorf("event_id should start with 'evid-', got %q", eventID)
	}
}

func TestCreateTimedEvent_HumanSpec(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	resp, _ := postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name":       "heartbeat",
		"per_user_api_key": apiKey,
		"human_spec":       "@every 1m",
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestCreateTimedEvent_InvalidAPIKey(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	resp, _ := postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name":       "backup",
		"per_user_api_key": "bad-key",
		"cron_spec":        "*/5 * * * *",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestCreateTimedEvent_MissingSchedule(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	resp, _ := postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name":       "backup",
		"per_user_api_key": apiKey,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// ---- /api/v1/update-timed-event --------------------------------------------

func TestUpdateTimedEvent_Success(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	_, cb := postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name":       "original",
		"per_user_api_key": apiKey,
		"cron_spec":        "@daily",
	})
	eventID := cb["event_id"].(string)

	newName := "renamed"
	resp, body := postJSON(t, ts, "/api/v1/update-timed-event", map[string]any{
		"event_id":         eventID,
		"per_user_api_key": apiKey,
		"event_name":       newName,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

func TestUpdateTimedEvent_NotFound(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	resp, _ := postJSON(t, ts, "/api/v1/update-timed-event", map[string]any{
		"event_id":         "evid-doesnotexist",
		"per_user_api_key": apiKey,
		"event_name":       "new-name",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// ---- /api/v1/delete-timed-event --------------------------------------------

func TestDeleteTimedEvent_Success(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	_, cb := postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name":       "to-delete",
		"per_user_api_key": apiKey,
		"cron_spec":        "@hourly",
	})
	eventID := cb["event_id"].(string)

	resp, body := postJSON(t, ts, "/api/v1/delete-timed-event", map[string]string{
		"event_id": eventID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body["status"])
	}
}

func TestDeleteTimedEvent_NotFound(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	resp, _ := postJSON(t, ts, "/api/v1/delete-timed-event", map[string]string{
		"event_id": "evid-ghost",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// ---- /api/v1/list-timed-event ----------------------------------------------

func TestListTimedEvent_Success(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)

	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "event-alpha", "per_user_api_key": apiKey, "cron_spec": "@daily",
	})
	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "event-beta", "per_user_api_key": apiKey, "cron_spec": "@hourly",
	})

	resp, body := postJSON(t, ts, "/api/v1/list-timed-event", map[string]string{
		"per_user_api_key": apiKey,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "success" {
		t.Errorf("expected status success, got %v", body["status"])
	}
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatal("data is not an array")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 events, got %d", len(data))
	}
}

func TestListTimedEvent_Empty(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	resp, body := postJSON(t, ts, "/api/v1/list-timed-event", map[string]string{
		"per_user_api_key": apiKey,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatal("data is not an array")
	}
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

// ---- /api/v1/search-timed-event --------------------------------------------

func TestSearchTimedEvent_PartialMatch(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "daily-backup", "per_user_api_key": apiKey, "cron_spec": "@daily",
	})
	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "hourly-ping", "per_user_api_key": apiKey, "cron_spec": "@hourly",
	})

	resp, body := postJSON(t, ts, "/api/v1/search-timed-event", map[string]string{
		"per_user_api_key": apiKey,
		"event_name":       "backup",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 result, got %d", len(data))
	}
}

func TestSearchTimedEvent_NoMatch(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "daily-backup", "per_user_api_key": apiKey, "cron_spec": "@daily",
	})

	resp, body := postJSON(t, ts, "/api/v1/search-timed-event", map[string]string{
		"per_user_api_key": apiKey,
		"event_name":       "zzznomatch",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data, _ := body["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected 0 results, got %d", len(data))
	}
}

func TestSearchTimedEvent_AllWhenEmpty(t *testing.T) {
	ts, cleanup := newTestServer(t)
	defer cleanup()

	apiKey := registerUser(t, ts)
	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "alpha", "per_user_api_key": apiKey, "cron_spec": "@daily",
	})
	postJSON(t, ts, "/api/v1/create-timed-event", map[string]string{
		"event_name": "beta", "per_user_api_key": apiKey, "cron_spec": "@hourly",
	})

	// Empty event_name should match all
	resp, body := postJSON(t, ts, "/api/v1/search-timed-event", map[string]string{
		"per_user_api_key": apiKey,
		"event_name":       "",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data, _ := body["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 results, got %d", len(data))
	}
}

// Package scheduler tests the cron-scheduled HTTP job scheduler.
package scheduler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pschlump/httpcron/lib/config"
	"github.com/pschlump/httpcron/lib/repository"
)

// newTestConfig creates a test config with SQLite database.
func newTestConfig(dbPath string) *config.Config {
	cfg := &config.Config{}
	if err := config.SetDefaults(cfg); err != nil {
		panic(err)
	}
	cfg.Server.DbKind = "sqlite"
	cfg.Server.DbPath = dbPath
	return cfg
}

// TestNewScheduler verifies that a new scheduler can be created.
func TestNewScheduler(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	if sched == nil {
		t.Fatal("New returned nil")
	}
	if sched.repo != repo {
		t.Error("repo not set correctly")
	}
	if sched.log != log {
		t.Error("log not set correctly")
	}
	if sched.client == nil {
		t.Error("client not initialized")
	}
	if sched.events == nil {
		t.Error("events map not initialized")
	}
	if sched.opChan == nil {
		t.Error("opChan not initialized")
	}
	if sched.started {
		t.Error("scheduler should not be started initially")
	}
}

// TestNormalizeCronSpec tests the cron spec normalization function.
func TestNormalizeCronSpec(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "6-field spec",
			input:    "0 0 * * *",
			expected: "0 0 * * *",
		},
		{
			name:     "6-field spec with seconds",
			input:    "0 0 0 * * *",
			expected: "0 0 0 * * *",
		},
		{
			name:     "7-field spec (with year)",
			input:    "0 0 0 * * * 2024",
			expected: "0 0 0 * * *",
		},
		{
			name:     "Quartz-style with ?",
			input:    "0 0 0 ? * *",
			expected: "0 0 0 * * *",
		},
		{
			name:     "7-field with ? and year",
			input:    "0 0 0 ? * * 2024",
			expected: "0 0 0 * * *",
		},
		{
			name:     "every 5 minutes",
			input:    "*/5 * * * *",
			expected: "*/5 * * * *",
		},
		{
			name:     "every 5 minutes with seconds",
			input:    "0 */5 * * * *",
			expected: "0 */5 * * * *",
		},
		{
			name:     "empty spec",
			input:    "",
			expected: "",
		},
		{
			name:     "spec with extra spaces",
			input:    "0  0  *  *  *",
			expected: "0 0 * * *",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeCronSpec(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCronSpec(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestAddEventNotStarted verifies that AddEvent returns an error when the scheduler hasn't started.
func TestAddEventNotStarted(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx := context.Background()
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.AddEvent(ctx, ev)
	if err == nil {
		t.Error("expected error when adding event to unstarted scheduler")
	}
	if err.Error() != "scheduler not started" {
		t.Errorf("error = %q, want %q", err.Error(), "scheduler not started")
	}
}

// TestUpdateEventNotStarted verifies that UpdateEvent returns an error when the scheduler hasn't started.
func TestUpdateEventNotStarted(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx := context.Background()
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.UpdateEvent(ctx, ev)
	if err == nil {
		t.Error("expected error when updating event in unstarted scheduler")
	}
	if err.Error() != "scheduler not started" {
		t.Errorf("error = %q, want %q", err.Error(), "scheduler not started")
	}
}

// TestDeleteEventNotStarted verifies that DeleteEvent returns an error when the scheduler hasn't started.
func TestDeleteEventNotStarted(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx := context.Background()

	err = sched.DeleteEvent(ctx, "evid-test")
	if err == nil {
		t.Error("expected error when deleting event from unstarted scheduler")
	}
	if err.Error() != "scheduler not started" {
		t.Errorf("error = %q, want %q", err.Error(), "scheduler not started")
	}
}

// TestStartAndAddEvent verifies that events can be added after the scheduler starts.
func TestStartAndAddEvent(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start.
	time.Sleep(100 * time.Millisecond)

	if !sched.started {
		t.Error("scheduler not started after calling Start")
	}

	// Add an event.
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.AddEvent(ctx, ev)
	if err != nil {
		t.Fatalf("AddEvent failed: %v", err)
	}

	// Verify the event is in the scheduler's events map.
	sched.eventsMu.RLock()
	_, exists := sched.events[ev.EventID]
	sched.eventsMu.RUnlock()

	if !exists {
		t.Error("event not found in scheduler's events map")
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestUpdateEvent verifies that an existing event can be updated.
func TestUpdateEvent(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start.
	time.Sleep(100 * time.Millisecond)

	// Add an event.
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.AddEvent(ctx, ev)
	if err != nil {
		t.Fatalf("AddEvent failed: %v", err)
	}

	// Update the event.
	ev.CronSpec = "0 */5 * * * *" // 6-field spec with seconds
	ev.URL = "http://updated.com"

	err = sched.UpdateEvent(ctx, ev)
	if err != nil {
		t.Fatalf("UpdateEvent failed: %v", err)
	}

	// Verify the event is still in the scheduler's events map.
	sched.eventsMu.RLock()
	scheduled, exists := sched.events[ev.EventID]
	sched.eventsMu.RUnlock()

	if !exists {
		t.Error("event not found in scheduler's events map after update")
	}

	if scheduled.Event.URL != "http://updated.com" {
		t.Errorf("URL = %q, want %q", scheduled.Event.URL, "http://updated.com")
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestUpdateEventNotFound verifies that updating a non-existent event returns an error.
func TestUpdateEventNotFound(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start.
	time.Sleep(100 * time.Millisecond)

	// Try to update a non-existent event.
	ev := repository.UserEvent{
		EventID:    "evid-nonexistent",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.UpdateEvent(ctx, ev)
	if err == nil {
		t.Error("expected error when updating non-existent event")
	}
	if err.Error() != "event not found in scheduler" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found in scheduler")
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestDeleteEvent verifies that an existing event can be deleted.
func TestDeleteEvent(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start.
	time.Sleep(100 * time.Millisecond)

	// Add an event.
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.AddEvent(ctx, ev)
	if err != nil {
		t.Fatalf("AddEvent failed: %v", err)
	}

	// Verify the event is in the scheduler's events map.
	sched.eventsMu.RLock()
	_, exists := sched.events[ev.EventID]
	sched.eventsMu.RUnlock()

	if !exists {
		t.Error("event not found in scheduler's events map")
	}

	// Delete the event.
	err = sched.DeleteEvent(ctx, ev.EventID)
	if err != nil {
		t.Fatalf("DeleteEvent failed: %v", err)
	}

	// Verify the event is no longer in the scheduler's events map.
	sched.eventsMu.RLock()
	_, exists = sched.events[ev.EventID]
	sched.eventsMu.RUnlock()

	if exists {
		t.Error("event still in scheduler's events map after deletion")
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestDeleteEventNotFound verifies that deleting a non-existent event returns an error.
func TestDeleteEventNotFound(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start.
	time.Sleep(100 * time.Millisecond)

	// Try to delete a non-existent event.
	err = sched.DeleteEvent(ctx, "evid-nonexistent")
	if err == nil {
		t.Error("expected error when deleting non-existent event")
	}
	if err.Error() != "event not found in scheduler" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found in scheduler")
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestAddEventInvalidCronSpec verifies that adding an event with an invalid cron spec returns an error.
func TestAddEventInvalidCronSpec(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start.
	time.Sleep(100 * time.Millisecond)

	// Try to add an event with an invalid cron spec.
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "invalid cron spec",
		URL:        "http://example.com",
		HTTPMethod: "POST",
	}

	err = sched.AddEvent(ctx, ev)
	if err == nil {
		t.Error("expected error when adding event with invalid cron spec")
	}

	// Try to add an event with empty cron spec.
	ev.CronSpec = ""
	err = sched.AddEvent(ctx, ev)
	if err == nil {
		t.Error("expected error when adding event with empty cron spec")
	}
	if err.Error() != "invalid cron_spec" {
		t.Errorf("error = %q, want %q", err.Error(), "invalid cron_spec")
	}

	// Try to add an event with empty URL.
	ev.CronSpec = "0 0 * * *"
	ev.URL = ""
	err = sched.AddEvent(ctx, ev)
	if err == nil {
		t.Error("expected error when adding event with empty URL")
	}
	if err.Error() != "url is required" {
		t.Errorf("error = %q, want %q", err.Error(), "url is required")
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestRunJob verifies that runJob executes HTTP requests correctly.
func TestRunJob(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	// Create a test HTTP server.
	var receivedMethod atomic.Value
	var receivedBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod.Store(r.Method)
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		receivedBody.Store(string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create an event with a body template.
	ev := repository.UserEvent{
		EventID:      "evid-test",
		UserID:       "user-123",
		EventName:    "test-event",
		CronSpec:     "0 0 * * *",
		HumanSpec:    "daily",
		URL:          server.URL,
		HTTPMethod:   "POST",
		BodyTemplate: `{"event_id": "{{.EventID}}", "user_id": "{{.UserID}}"}`,
	}

	// Run the job.
	sched.runJob(ev)

	// Verify the request was made correctly.
	method := receivedMethod.Load()
	if method == nil {
		t.Fatal("no request was made")
	}
	if method != "POST" {
		t.Errorf("method = %q, want %q", method, "POST")
	}

	body := receivedBody.Load()
	if body == nil {
		t.Fatal("no body was received")
	}
	bodyStr := body.(string)
	if !strings.Contains(bodyStr, `"event_id": "evid-test"`) {
		t.Errorf("body does not contain event_id: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"user_id": "user-123"`) {
		t.Errorf("body does not contain user_id: %s", bodyStr)
	}
}

// TestRunJobGETMethod verifies that GET requests don't include a body.
func TestRunJobGETMethod(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	// Create a test HTTP server.
	var receivedMethod atomic.Value
	var hasBody atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod.Store(r.Method)
		hasBody.Store(r.ContentLength > 0)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create an event with GET method.
	ev := repository.UserEvent{
		EventID:      "evid-test",
		CronSpec:     "0 0 * * *",
		URL:          server.URL,
		HTTPMethod:   "GET",
		BodyTemplate: `{"test": "value"}`,
	}

	// Run the job.
	sched.runJob(ev)

	// Verify the request was made correctly.
	method := receivedMethod.Load()
	if method != "GET" {
		t.Errorf("method = %q, want %q", method, "GET")
	}

	if hasBody.Load() {
		t.Error("GET request should not have a body")
	}
}

// TestRunJobDefaultMethod verifies that empty HTTPMethod defaults to POST.
func TestRunJobDefaultMethod(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	// Create a test HTTP server.
	var receivedMethod atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod.Store(r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create an event with empty HTTPMethod (should default to POST).
	ev := repository.UserEvent{
		EventID:    "evid-test",
		CronSpec:   "0 0 0 * * *", // 6-field spec with seconds
		URL:        server.URL,
		HTTPMethod: "",
	}

	// Run the job.
	sched.runJob(ev)

	// Verify the method defaulted to POST.
	method := receivedMethod.Load()
	if method != "POST" {
		t.Errorf("method = %q, want %q (default)", method, "POST")
	}
}

// TestRunJobInvalidTemplate verifies that runJob handles invalid templates gracefully.
func TestRunJobInvalidTemplate(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	// Create an event with an invalid template.
	ev := repository.UserEvent{
		EventID:      "evid-test",
		CronSpec:     "0 0 * * *",
		URL:          "http://example.com",
		HTTPMethod:   "POST",
		BodyTemplate: "{{.UndefinedField}",
	}

	// Run the job - should not panic.
	sched.runJob(ev)
	// If we got here, the test passes (no panic)
}

// TestStartWithRepositoryEvents verifies that Start loads events from the repository.
func TestStartWithRepositoryEvents(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	// Create a user and an event in the repository.
	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 0 * * *", "daily", "{}", "http://example.com", "POST") // 6-field spec
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start and load events.
	time.Sleep(100 * time.Millisecond)

	// Verify the event was loaded.
	sched.eventsMu.RLock()
	eventsCount := len(sched.events)
	sched.eventsMu.RUnlock()

	if eventsCount != 1 {
		t.Errorf("expected 1 event in scheduler, got %d", eventsCount)
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestStartSkipEventsWithNoCronSpec verifies that events without cron_spec are skipped.
func TestStartSkipEventsWithNoCronSpec(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	// Create a user and an event with empty cron_spec in the repository.
	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = repo.CreateEvent(ctx, user.UserID, "test-event", "", "", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start and load events.
	time.Sleep(100 * time.Millisecond)

	// Verify the event was not loaded (due to empty cron_spec).
	sched.eventsMu.RLock()
	eventsCount := len(sched.events)
	sched.eventsMu.RUnlock()

	if eventsCount != 0 {
		t.Errorf("expected 0 events in scheduler (event should be skipped), got %d", eventsCount)
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

// TestStartSkipEventsWithNoURL verifies that events without URL are skipped.
func TestStartSkipEventsWithNoURL(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	repo, err := repository.NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		repo.Close()
		os.Remove(dbPath)
	}()

	// Create a user and an event with empty URL in the repository.
	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	_, err = repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	sched := New(repo, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduler in a goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- sched.Start(ctx)
	}()

	// Wait a bit for the scheduler to start and load events.
	time.Sleep(100 * time.Millisecond)

	// Verify the event was not loaded (due to empty URL).
	sched.eventsMu.RLock()
	eventsCount := len(sched.events)
	sched.eventsMu.RUnlock()

	if eventsCount != 0 {
		t.Errorf("expected 0 events in scheduler (event should be skipped), got %d", eventsCount)
	}

	// Stop the scheduler.
	cancel()
	<-errChan
}

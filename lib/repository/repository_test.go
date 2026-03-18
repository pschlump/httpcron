// Package repository tests the SQLite-backed persistence layer.
package repository

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/pschlump/httpcron/lib/config"
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

// TestNewRepository verifies that a new repository can be created and closed.
func TestNewRepository(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	cfg := newTestConfig(dbPath)
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	if repo == nil {
		t.Fatal("NewRepository returned nil")
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	// Verify the database file was created.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
	// Clean up the database file.
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("failed to remove database file: %v", err)
	}
}

// TestCreateUser verifies that a new user can be created with valid fields.
func TestCreateUser(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	cfg := newTestConfig(dbPath)
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.UserID == "" {
		t.Error("UserID is empty")
	}
	if user.HostName != "test-host" {
		t.Errorf("HostName = %q, want %q", user.HostName, "test-host")
	}
	if user.HostURL != "http://localhost:8080" {
		t.Errorf("HostURL = %q, want %q", user.HostURL, "http://localhost:8080")
	}
	if user.PerUserAPIKey == "" {
		t.Error("PerUserAPIKey is empty")
	}
	if len(user.PerUserAPIKey) < 4 || user.PerUserAPIKey[:4] != "uak-" {
		t.Errorf("PerUserAPIKey should start with 'uak-', got %q", user.PerUserAPIKey)
	}
	// Note: CreatedAt is set by the database but not returned in the struct after creation.
	// To get the actual CreatedAt, we would need to query the user back from the database.
}

// TestGetUserByAPIKey verifies that a user can be retrieved by their API key.
func TestGetUserByAPIKey(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Test with non-existent user.
	user, err := repo.GetUserByAPIKey(ctx, "non-existent-key")
	if err != nil {
		t.Fatalf("GetUserByAPIKey with non-existent key failed: %v", err)
	}
	if user != nil {
		t.Error("expected nil user for non-existent key")
	}

	// Create a user and retrieve it.
	created, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	retrieved, err := repo.GetUserByAPIKey(ctx, created.PerUserAPIKey)
	if err != nil {
		t.Fatalf("GetUserByAPIKey failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetUserByAPIKey returned nil for existing key")
	}

	if retrieved.UserID != created.UserID {
		t.Errorf("UserID = %q, want %q", retrieved.UserID, created.UserID)
	}
	if retrieved.HostName != created.HostName {
		t.Errorf("HostName = %q, want %q", retrieved.HostName, created.HostName)
	}
	if retrieved.HostURL != created.HostURL {
		t.Errorf("HostURL = %q, want %q", retrieved.HostURL, created.HostURL)
	}
	if retrieved.PerUserAPIKey != created.PerUserAPIKey {
		t.Errorf("PerUserAPIKey = %q, want %q", retrieved.PerUserAPIKey, created.PerUserAPIKey)
	}
}

// TestCreateEvent verifies that a new event can be created with valid fields.
func TestCreateEvent(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	if event.EventID == "" {
		t.Error("EventID is empty")
	}
	if len(event.EventID) < 5 || event.EventID[:5] != "evid-" {
		t.Errorf("EventID should start with 'evid-', got %q", event.EventID)
	}
	if event.UserID != user.UserID {
		t.Errorf("UserID = %q, want %q", event.UserID, user.UserID)
	}
	if event.EventName != "test-event" {
		t.Errorf("EventName = %q, want %q", event.EventName, "test-event")
	}
	if event.CronSpec != "0 0 * * *" {
		t.Errorf("CronSpec = %q, want %q", event.CronSpec, "0 0 * * *")
	}
	if event.HumanSpec != "daily" {
		t.Errorf("HumanSpec = %q, want %q", event.HumanSpec, "daily")
	}
	if event.BodyTemplate != "{}" {
		t.Errorf("BodyTemplate = %q, want %q", event.BodyTemplate, "{}")
	}
	if event.URL != "http://example.com" {
		t.Errorf("URL = %q, want %q", event.URL, "http://example.com")
	}
	if event.HTTPMethod != "POST" {
		t.Errorf("HTTPMethod = %q, want %q", event.HTTPMethod, "POST")
	}
	// Note: CreatedAt and UpdatedAt are set by the database but not returned in the struct after creation.
	// To get the actual timestamps, we would need to query the event back from the database.
}

// TestUpdateEvent verifies that an event can be updated with various field combinations.
func TestUpdateEvent(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Test updating EventName.
	newName := "updated-event"
	params := UpdateEventParams{EventName: &newName}
	if err := repo.UpdateEvent(ctx, event.EventID, user.UserID, params); err != nil {
		t.Fatalf("UpdateEvent failed: %v", err)
	}

	events, err := repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName != newName {
		t.Errorf("EventName = %q, want %q", events[0].EventName, newName)
	}

	// Test updating multiple fields.
	newURL := "http://updated.com"
	newMethod := "GET"
	params = UpdateEventParams{
		CronSpec:   &[]string{"*/5 * * * *"}[0],
		URL:        &newURL,
		HTTPMethod: &newMethod,
	}
	if err := repo.UpdateEvent(ctx, event.EventID, user.UserID, params); err != nil {
		t.Fatalf("UpdateEvent with multiple fields failed: %v", err)
	}

	events, err = repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if events[0].CronSpec != "*/5 * * * *" {
		t.Errorf("CronSpec = %q, want %q", events[0].CronSpec, "*/5 * * * *")
	}
	if events[0].URL != newURL {
		t.Errorf("URL = %q, want %q", events[0].URL, newURL)
	}
	if events[0].HTTPMethod != newMethod {
		t.Errorf("HTTPMethod = %q, want %q", events[0].HTTPMethod, newMethod)
	}
}

// TestUpdateEventNotFound verifies that updating a non-existent event returns an error.
func TestUpdateEventNotFound(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	newName := "updated-event"
	params := UpdateEventParams{EventName: &newName}
	err = repo.UpdateEvent(ctx, "evid-nonexistent", user.UserID, params)
	if err == nil {
		t.Error("expected error for non-existent event")
	}
	if err.Error() != "event not found" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found")
	}

	// Test with wrong user ID (event exists but belongs to different user).
	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	otherUser, err := repo.CreateUser(ctx, "other-host", "http://localhost:8081")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = repo.UpdateEvent(ctx, event.EventID, otherUser.UserID, params)
	if err == nil {
		t.Error("expected error when updating event with wrong user ID")
	}
	if err.Error() != "event not found" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found")
	}
}

// TestDeleteEvent verifies that an event can be deleted.
func TestDeleteEvent(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Verify the event exists.
	events, err := repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event before deletion, got %d", len(events))
	}

	// Delete the event.
	if err := repo.DeleteEvent(ctx, event.EventID); err != nil {
		t.Fatalf("DeleteEvent failed: %v", err)
	}

	// Verify the event is gone.
	events, err = repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after deletion, got %d", len(events))
	}
}

// TestDeleteEventNotFound verifies that deleting a non-existent event returns an error.
func TestDeleteEventNotFound(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	err = repo.DeleteEvent(ctx, "evid-nonexistent")
	if err == nil {
		t.Error("expected error for non-existent event")
	}
	if err.Error() != "event not found" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found")
	}
}

// TestListEvents verifies that events can be listed for a user.
func TestListEvents(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Create two users.
	user1, err := repo.CreateUser(ctx, "test-host-1", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	user2, err := repo.CreateUser(ctx, "test-host-2", "http://localhost:8081")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create events for user1.
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-1", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-2", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Create an event for user2.
	_, err = repo.CreateEvent(ctx, user2.UserID, "event-3", "0 0 * * *", "daily", "{}", "http://example.com/3", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// List events for user1.
	events1, err := repo.ListEvents(ctx, user1.UserID)
	if err != nil {
		t.Fatalf("ListEvents for user1 failed: %v", err)
	}
	if len(events1) != 2 {
		t.Fatalf("expected 2 events for user1, got %d", len(events1))
	}

	// List events for user2.
	events2, err := repo.ListEvents(ctx, user2.UserID)
	if err != nil {
		t.Fatalf("ListEvents for user2 failed: %v", err)
	}
	if len(events2) != 1 {
		t.Fatalf("expected 1 event for user2, got %d", len(events2))
	}

	// Verify events are returned in descending order by created_at (newest first).
	if events1[0].CreatedAt.Before(events1[1].CreatedAt) {
		t.Error("events are not in descending order by created_at")
	}
}

// TestSearchEvents verifies that events can be searched by name.
func TestSearchEvents(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create events with different names.
	_, err = repo.CreateEvent(ctx, user.UserID, "daily-backup", "0 0 * * *", "daily", "{}", "http://example.com/backup", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user.UserID, "hourly-ping", "*/5 * * * *", "every 5 min", "{}", "http://example.com/ping", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user.UserID, "weekly-report", "0 0 * * 0", "weekly", "{}", "http://example.com/report", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Search for "backup".
	events, err := repo.SearchEvents(ctx, user.UserID, "backup")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event matching 'backup', got %d", len(events))
	}
	if events[0].EventName != "daily-backup" {
		t.Errorf("EventName = %q, want 'daily-backup'", events[0].EventName)
	}

	// Search for "ping".
	events, err = repo.SearchEvents(ctx, user.UserID, "ping")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event matching 'ping', got %d", len(events))
	}
	if events[0].EventName != "hourly-ping" {
		t.Errorf("EventName = %q, want 'hourly-ping'", events[0].EventName)
	}

	// Search with partial match.
	events, err = repo.SearchEvents(ctx, user.UserID, "ly")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	// "daily-backup", "hourly-ping", and "weekly-report" all contain "ly"
	if len(events) != 3 {
		t.Fatalf("expected 3 events matching 'ly', got %d", len(events))
	}

	// Search with no matches.
	events, err = repo.SearchEvents(ctx, user.UserID, "nonexistent")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events matching 'nonexistent', got %d", len(events))
	}

	// Search with empty string (should match all).
	events, err = repo.SearchEvents(ctx, user.UserID, "")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events matching empty string, got %d", len(events))
	}
}

// TestListAllEvents verifies that all events across all users can be listed.
func TestListAllEvents(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	defer func() {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove database file: %v", err)
		}
	}()

	repo, err := NewRepository(newTestConfig(dbPath))
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Create two users.
	user1, err := repo.CreateUser(ctx, "test-host-1", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	user2, err := repo.CreateUser(ctx, "test-host-2", "http://localhost:8081")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create events for both users.
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-1", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-2", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user2.UserID, "event-3", "0 0 * * *", "daily", "{}", "http://example.com/3", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// List all events.
	allEvents, err := repo.ListAllEvents(ctx)
	if err != nil {
		t.Fatalf("ListAllEvents failed: %v", err)
	}
	if len(allEvents) != 3 {
		t.Fatalf("expected 3 events total, got %d", len(allEvents))
	}

	// Verify we have events from both users.
	userIDs := make(map[string]bool)
	for _, ev := range allEvents {
		userIDs[ev.UserID] = true
	}
	if !userIDs[user1.UserID] {
		t.Error("missing events from user1")
	}
	if !userIDs[user2.UserID] {
		t.Error("missing events from user2")
	}
}

// ============================================================================
// PostgreSQL Tests
// These tests run the same validations as the SQLite tests but against
// a PostgreSQL database using the configuration in test-config.json.
// ============================================================================

// newPostgresTestConfig loads the test config for PostgreSQL.
func newPostgresTestConfig() *config.Config {
	cfg, err := config.FromFile("test-config.json")
	if err != nil {
		panic(fmt.Sprintf("Failed to load test-config.json: %v", err))
	}
	// Ensure db_kind is set to postgres
	cfg.Server.DbKind = "postgres"
	return cfg
}

// cleanupPostgresTestData cleans up test data from PostgreSQL.
func cleanupPostgresTestData(repo Repository) error {
	ctx := context.Background()

	// Get all events and delete them
	events, err := repo.ListAllEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to list events for cleanup: %w", err)
	}

	for _, ev := range events {
		if err := repo.DeleteEvent(ctx, ev.EventID); err != nil {
			return fmt.Errorf("failed to delete event %s: %w", ev.EventID, err)
		}
	}

	return nil
}

// TestPostgres_NewRepository verifies that a new PostgreSQL repository can be created and closed.
func TestPostgres_NewRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	if repo == nil {
		t.Fatal("NewRepository returned nil")
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// TestPostgres_CreateUser verifies that a new user can be created with valid fields in PostgreSQL.
func TestPostgres_CreateUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.UserID == "" {
		t.Error("UserID is empty")
	}
	if user.HostName != "test-host" {
		t.Errorf("HostName = %q, want %q", user.HostName, "test-host")
	}
	if user.HostURL != "http://localhost:8080" {
		t.Errorf("HostURL = %q, want %q", user.HostURL, "http://localhost:8080")
	}
	if user.PerUserAPIKey == "" {
		t.Error("PerUserAPIKey is empty")
	}
	if len(user.PerUserAPIKey) < 4 || user.PerUserAPIKey[:4] != "uak-" {
		t.Errorf("PerUserAPIKey should start with 'uak-', got %q", user.PerUserAPIKey)
	}
}

// TestPostgres_GetUserByAPIKey verifies that a user can be retrieved by their API key in PostgreSQL.
func TestPostgres_GetUserByAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()

	// Test with non-existent user.
	user, err := repo.GetUserByAPIKey(ctx, "non-existent-key")
	if err != nil {
		t.Fatalf("GetUserByAPIKey with non-existent key failed: %v", err)
	}
	if user != nil {
		t.Error("expected nil user for non-existent key")
	}

	// Create a user and retrieve it.
	created, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	retrieved, err := repo.GetUserByAPIKey(ctx, created.PerUserAPIKey)
	if err != nil {
		t.Fatalf("GetUserByAPIKey failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetUserByAPIKey returned nil for existing key")
	}

	if retrieved.UserID != created.UserID {
		t.Errorf("UserID = %q, want %q", retrieved.UserID, created.UserID)
	}
	if retrieved.HostName != created.HostName {
		t.Errorf("HostName = %q, want %q", retrieved.HostName, created.HostName)
	}
	if retrieved.HostURL != created.HostURL {
		t.Errorf("HostURL = %q, want %q", retrieved.HostURL, created.HostURL)
	}
	if retrieved.PerUserAPIKey != created.PerUserAPIKey {
		t.Errorf("PerUserAPIKey = %q, want %q", retrieved.PerUserAPIKey, created.PerUserAPIKey)
	}
}

// TestPostgres_CreateEvent verifies that a new event can be created with valid fields in PostgreSQL.
func TestPostgres_CreateEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	if event.EventID == "" {
		t.Error("EventID is empty")
	}
	if len(event.EventID) < 5 || event.EventID[:5] != "evid-" {
		t.Errorf("EventID should start with 'evid-', got %q", event.EventID)
	}
	if event.UserID != user.UserID {
		t.Errorf("UserID = %q, want %q", event.UserID, user.UserID)
	}
	if event.EventName != "test-event" {
		t.Errorf("EventName = %q, want %q", event.EventName, "test-event")
	}
	if event.CronSpec != "0 0 * * *" {
		t.Errorf("CronSpec = %q, want %q", event.CronSpec, "0 0 * * *")
	}
	if event.HumanSpec != "daily" {
		t.Errorf("HumanSpec = %q, want %q", event.HumanSpec, "daily")
	}
	if event.BodyTemplate != "{}" {
		t.Errorf("BodyTemplate = %q, want %q", event.BodyTemplate, "{}")
	}
	if event.URL != "http://example.com" {
		t.Errorf("URL = %q, want %q", event.URL, "http://example.com")
	}
	if event.HTTPMethod != "POST" {
		t.Errorf("HTTPMethod = %q, want %q", event.HTTPMethod, "POST")
	}
}

// TestPostgres_UpdateEvent verifies that an event can be updated with various field combinations in PostgreSQL.
func TestPostgres_UpdateEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Test updating EventName.
	newName := "updated-event"
	params := UpdateEventParams{EventName: &newName}
	if err := repo.UpdateEvent(ctx, event.EventID, user.UserID, params); err != nil {
		t.Fatalf("UpdateEvent failed: %v", err)
	}

	events, err := repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName != newName {
		t.Errorf("EventName = %q, want %q", events[0].EventName, newName)
	}

	// Test updating multiple fields.
	newURL := "http://updated.com"
	newMethod := "GET"
	params = UpdateEventParams{
		CronSpec:   &[]string{"*/5 * * * *"}[0],
		URL:        &newURL,
		HTTPMethod: &newMethod,
	}
	if err := repo.UpdateEvent(ctx, event.EventID, user.UserID, params); err != nil {
		t.Fatalf("UpdateEvent with multiple fields failed: %v", err)
	}

	events, err = repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if events[0].CronSpec != "*/5 * * * *" {
		t.Errorf("CronSpec = %q, want %q", events[0].CronSpec, "*/5 * * * *")
	}
	if events[0].URL != newURL {
		t.Errorf("URL = %q, want %q", events[0].URL, newURL)
	}
	if events[0].HTTPMethod != newMethod {
		t.Errorf("HTTPMethod = %q, want %q", events[0].HTTPMethod, newMethod)
	}
}

// TestPostgres_UpdateEventNotFound verifies that updating a non-existent event returns an error in PostgreSQL.
func TestPostgres_UpdateEventNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	newName := "updated-event"
	params := UpdateEventParams{EventName: &newName}
	err = repo.UpdateEvent(ctx, "evid-nonexistent", user.UserID, params)
	if err == nil {
		t.Error("expected error for non-existent event")
	}
	if err.Error() != "event not found" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found")
	}

	// Test with wrong user ID (event exists but belongs to different user).
	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	otherUser, err := repo.CreateUser(ctx, "other-host", "http://localhost:8081")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = repo.UpdateEvent(ctx, event.EventID, otherUser.UserID, params)
	if err == nil {
		t.Error("expected error when updating event with wrong user ID")
	}
	if err.Error() != "event not found" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found")
	}
}

// TestPostgres_DeleteEvent verifies that an event can be deleted in PostgreSQL.
func TestPostgres_DeleteEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	event, err := repo.CreateEvent(ctx, user.UserID, "test-event", "0 0 * * *", "daily", "{}", "http://example.com", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Verify the event exists.
	events, err := repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event before deletion, got %d", len(events))
	}

	// Delete the event.
	if err := repo.DeleteEvent(ctx, event.EventID); err != nil {
		t.Fatalf("DeleteEvent failed: %v", err)
	}

	// Verify the event is gone.
	events, err = repo.ListEvents(ctx, user.UserID)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after deletion, got %d", len(events))
	}
}

// TestPostgres_DeleteEventNotFound verifies that deleting a non-existent event returns an error in PostgreSQL.
func TestPostgres_DeleteEventNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()

	err = repo.DeleteEvent(ctx, "evid-nonexistent")
	if err == nil {
		t.Error("expected error for non-existent event")
	}
	if err.Error() != "event not found" {
		t.Errorf("error = %q, want %q", err.Error(), "event not found")
	}
}

// TestPostgres_ListEvents verifies that events can be listed for a user in PostgreSQL.
func TestPostgres_ListEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()

	// Create two users.
	user1, err := repo.CreateUser(ctx, "test-host-1", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	user2, err := repo.CreateUser(ctx, "test-host-2", "http://localhost:8081")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create events for user1.
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-1", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-2", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Create an event for user2.
	_, err = repo.CreateEvent(ctx, user2.UserID, "event-3", "0 0 * * *", "daily", "{}", "http://example.com/3", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// List events for user1.
	events1, err := repo.ListEvents(ctx, user1.UserID)
	if err != nil {
		t.Fatalf("ListEvents for user1 failed: %v", err)
	}
	if len(events1) != 2 {
		t.Fatalf("expected 2 events for user1, got %d", len(events1))
	}

	// List events for user2.
	events2, err := repo.ListEvents(ctx, user2.UserID)
	if err != nil {
		t.Fatalf("ListEvents for user2 failed: %v", err)
	}
	if len(events2) != 1 {
		t.Fatalf("expected 1 event for user2, got %d", len(events2))
	}

	// Verify events are returned in descending order by created_at (newest first).
	if events1[0].CreatedAt.Before(events1[1].CreatedAt) {
		t.Error("events are not in descending order by created_at")
	}
}

// TestPostgres_SearchEvents verifies that events can be searched by name in PostgreSQL.
func TestPostgres_SearchEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()
	user, err := repo.CreateUser(ctx, "test-host", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create events with different names.
	_, err = repo.CreateEvent(ctx, user.UserID, "daily-backup", "0 0 * * *", "daily", "{}", "http://example.com/backup", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user.UserID, "hourly-ping", "*/5 * * * *", "every 5 min", "{}", "http://example.com/ping", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user.UserID, "weekly-report", "0 0 * * 0", "weekly", "{}", "http://example.com/report", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// Search for "backup" (case-insensitive in PostgreSQL).
	events, err := repo.SearchEvents(ctx, user.UserID, "backup")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event matching 'backup', got %d", len(events))
	}
	if events[0].EventName != "daily-backup" {
		t.Errorf("EventName = %q, want 'daily-backup'", events[0].EventName)
	}

	// Search for "PING" (case-insensitive in PostgreSQL).
	events, err = repo.SearchEvents(ctx, user.UserID, "PING")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event matching 'PING', got %d", len(events))
	}
	if events[0].EventName != "hourly-ping" {
		t.Errorf("EventName = %q, want 'hourly-ping'", events[0].EventName)
	}

	// Search with partial match.
	events, err = repo.SearchEvents(ctx, user.UserID, "ly")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	// "daily-backup", "hourly-ping", and "weekly-report" all contain "ly"
	if len(events) != 3 {
		t.Fatalf("expected 3 events matching 'ly', got %d", len(events))
	}

	// Search with no matches.
	events, err = repo.SearchEvents(ctx, user.UserID, "nonexistent")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events matching 'nonexistent', got %d", len(events))
	}

	// Search with empty string (should match all).
	events, err = repo.SearchEvents(ctx, user.UserID, "")
	if err != nil {
		t.Fatalf("SearchEvents failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events matching empty string, got %d", len(events))
	}
}

// TestPostgres_ListAllEvents verifies that all events across all users can be listed in PostgreSQL.
func TestPostgres_ListAllEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	cfg := newPostgresTestConfig()
	repo, err := NewRepository(cfg)
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() {
		cleanupPostgresTestData(repo)
		repo.Close()
	}()

	ctx := context.Background()

	// Create two users.
	user1, err := repo.CreateUser(ctx, "test-host-1", "http://localhost:8080")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	user2, err := repo.CreateUser(ctx, "test-host-2", "http://localhost:8081")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create events for both users.
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-1", "0 0 * * *", "daily", "{}", "http://example.com/1", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user1.UserID, "event-2", "*/5 * * * *", "every 5 min", "{}", "http://example.com/2", "GET")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}
	_, err = repo.CreateEvent(ctx, user2.UserID, "event-3", "0 0 * * *", "daily", "{}", "http://example.com/3", "POST")
	if err != nil {
		t.Fatalf("CreateEvent failed: %v", err)
	}

	// List all events.
	allEvents, err := repo.ListAllEvents(ctx)
	if err != nil {
		t.Fatalf("ListAllEvents failed: %v", err)
	}
	if len(allEvents) != 3 {
		t.Fatalf("expected 3 events total, got %d", len(allEvents))
	}

	// Verify we have events from both users.
	userIDs := make(map[string]bool)
	for _, ev := range allEvents {
		userIDs[ev.UserID] = true
	}
	if !userIDs[user1.UserID] {
		t.Error("missing events from user1")
	}
	if !userIDs[user2.UserID] {
		t.Error("missing events from user2")
	}
}

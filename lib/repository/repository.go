// Package repository provides database-backed persistence with support for SQLite and PostgreSQL.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/pschlump/httpcron/lib/config"
)

// RegisteredUser represents a row in the registered_users table.
type RegisteredUser struct {
	UserID        string    `json:"user_id"`
	HostName      string    `json:"host_name"`
	HostURL       string    `json:"host_url"`
	PerUserAPIKey string    `json:"per_user_api_key"`
	CreatedAt     time.Time `json:"created_at"`
}

// UserEvent represents a row in the user_events table.
type UserEvent struct {
	EventID      string    `json:"event_id"`
	UserID       string    `json:"user_id"`
	EventName    string    `json:"event_name"`
	CronSpec     string    `json:"cron_spec,omitempty"`
	HumanSpec    string    `json:"human_spec,omitempty"`
	BodyTemplate string    `json:"body_template,omitempty"`
	URL          string    `json:"url,omitempty"`
	HTTPMethod   string    `json:"http_method,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UpdateEventParams holds optional fields for updating a UserEvent.
type UpdateEventParams struct {
	EventName    *string
	CronSpec     *string
	HumanSpec    *string
	BodyTemplate *string
	URL          *string
	HTTPMethod   *string
}

// Repository defines the interface for database operations.
type Repository interface {
	// Close closes the underlying database connection.
	Close() error

	// CreateUser inserts a new registered user and returns it with generated IDs.
	CreateUser(ctx context.Context, hostName, hostURL string) (*RegisteredUser, error)

	// GetUserByAPIKey returns the user with the given per_user_api_key, or nil if not found.
	GetUserByAPIKey(ctx context.Context, apiKey string) (*RegisteredUser, error)

	// CreateEvent inserts a new user event and returns it with a generated event_id.
	CreateEvent(ctx context.Context, userID, eventName, cronSpec, humanSpec, bodyTemplate, url, httpMethod string) (*UserEvent, error)

	// UpdateEvent applies the non-nil fields in params to the event identified by eventID + userID.
	// Returns an error if the event does not exist or does not belong to the user.
	UpdateEvent(ctx context.Context, eventID, userID string, params UpdateEventParams) error

	// DeleteEvent removes the event with the given eventID.
	DeleteEvent(ctx context.Context, eventID string) error

	// ListEvents returns all events belonging to userID, newest first.
	ListEvents(ctx context.Context, userID string) ([]UserEvent, error)

	// SearchEvents returns events for userID whose name contains eventName (case-insensitive).
	SearchEvents(ctx context.Context, userID, eventName string) ([]UserEvent, error)

	// ListAllEvents returns every event across all users. Used by the cron scheduler on startup.
	ListAllEvents(ctx context.Context) ([]UserEvent, error)
}

// NewRepository creates a repository implementation based on the database configuration.
func NewRepository(cfg *config.Config) (Repository, error) {
	switch cfg.Server.DbKind {
	case "sqlite":
		return NewSQLiteRepository(cfg.Server.DbPath)
	case "postgres":
		return NewPostgresRepository(cfg)
	default:
		return nil, fmt.Errorf("unsupported database kind: %s (supported: sqlite, postgres)", cfg.Server.DbKind)
	}
}

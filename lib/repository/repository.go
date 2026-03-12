// Package repository provides a SQLite-backed persistence layer.
package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
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
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UpdateEventParams holds optional fields for updating a UserEvent.
type UpdateEventParams struct {
	EventName    *string
	CronSpec     *string
	HumanSpec    *string
	BodyTemplate *string
}

// Repository wraps a SQLite database connection.
type Repository struct {
	db *sql.DB
}

// NewRepository opens (or creates) the SQLite database at dbPath and runs migrations.
func NewRepository(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	r := &Repository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return r, nil
}

// Close closes the underlying database connection.
func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS registered_users (
		user_id          TEXT PRIMARY KEY,
		host_name        TEXT NOT NULL,
		host_url         TEXT NOT NULL,
		per_user_api_key TEXT NOT NULL UNIQUE,
		created_at       DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS user_events (
		event_id      TEXT PRIMARY KEY,
		user_id       TEXT NOT NULL REFERENCES registered_users(user_id),
		event_name    TEXT NOT NULL,
		cron_spec     TEXT NOT NULL DEFAULT '',
		human_spec    TEXT NOT NULL DEFAULT '',
		body_template TEXT NOT NULL DEFAULT '',
		created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := r.db.Exec(schema)
	return err
}

// newUUID generates a random RFC-4122 v4 UUID string.
func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// CreateUser inserts a new registered user and returns it with generated IDs.
func (r *Repository) CreateUser(ctx context.Context, hostName, hostURL string) (*RegisteredUser, error) {
	user := &RegisteredUser{
		UserID:        newUUID(),
		HostName:      hostName,
		HostURL:       hostURL,
		PerUserAPIKey: "uak-" + newUUID(),
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO registered_users (user_id, host_name, host_url, per_user_api_key) VALUES (?, ?, ?, ?)`,
		user.UserID, user.HostName, user.HostURL, user.PerUserAPIKey,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByAPIKey returns the user with the given per_user_api_key, or nil if not found.
func (r *Repository) GetUserByAPIKey(ctx context.Context, apiKey string) (*RegisteredUser, error) {
	user := &RegisteredUser{}
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, host_name, host_url, per_user_api_key, created_at
		 FROM registered_users WHERE per_user_api_key = ?`,
		apiKey,
	).Scan(&user.UserID, &user.HostName, &user.HostURL, &user.PerUserAPIKey, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// CreateEvent inserts a new user event and returns it with a generated event_id.
func (r *Repository) CreateEvent(ctx context.Context, userID, eventName, cronSpec, humanSpec, bodyTemplate string) (*UserEvent, error) {
	event := &UserEvent{
		EventID:      "evid-" + newUUID(),
		UserID:       userID,
		EventName:    eventName,
		CronSpec:     cronSpec,
		HumanSpec:    humanSpec,
		BodyTemplate: bodyTemplate,
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_events (event_id, user_id, event_name, cron_spec, human_spec, body_template)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		event.EventID, event.UserID, event.EventName, event.CronSpec, event.HumanSpec, event.BodyTemplate,
	)
	if err != nil {
		return nil, err
	}
	return event, nil
}

// UpdateEvent applies the non-nil fields in params to the event identified by eventID + userID.
// Returns an error if the event does not exist or does not belong to the user.
func (r *Repository) UpdateEvent(ctx context.Context, eventID, userID string, params UpdateEventParams) error {
	setClauses := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []interface{}{}

	if params.EventName != nil {
		setClauses = append(setClauses, "event_name = ?")
		args = append(args, *params.EventName)
	}
	if params.CronSpec != nil {
		setClauses = append(setClauses, "cron_spec = ?")
		args = append(args, *params.CronSpec)
	}
	if params.HumanSpec != nil {
		setClauses = append(setClauses, "human_spec = ?")
		args = append(args, *params.HumanSpec)
	}
	if params.BodyTemplate != nil {
		setClauses = append(setClauses, "body_template = ?")
		args = append(args, *params.BodyTemplate)
	}

	args = append(args, eventID, userID)
	query := fmt.Sprintf(
		"UPDATE user_events SET %s WHERE event_id = ? AND user_id = ?",
		strings.Join(setClauses, ", "),
	)

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("event not found")
	}
	return nil
}

// DeleteEvent removes the event with the given eventID.
func (r *Repository) DeleteEvent(ctx context.Context, eventID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM user_events WHERE event_id = ?`, eventID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("event not found")
	}
	return nil
}

func scanEvents(rows *sql.Rows) ([]UserEvent, error) {
	var events []UserEvent
	for rows.Next() {
		var e UserEvent
		if err := rows.Scan(&e.EventID, &e.UserID, &e.EventName, &e.CronSpec, &e.HumanSpec, &e.BodyTemplate, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

const eventSelectCols = `event_id, user_id, event_name, cron_spec, human_spec, body_template, created_at, updated_at`

// ListEvents returns all events belonging to userID, newest first.
func (r *Repository) ListEvents(ctx context.Context, userID string) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventSelectCols+` FROM user_events WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// SearchEvents returns events for userID whose name contains eventName (case-insensitive).
func (r *Repository) SearchEvents(ctx context.Context, userID, eventName string) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventSelectCols+` FROM user_events WHERE user_id = ? AND event_name LIKE ? ORDER BY created_at DESC`,
		userID, "%"+eventName+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

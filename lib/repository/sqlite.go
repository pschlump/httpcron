// Package repository provides SQLite-backed persistence.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// sqliteRepository implements the Repository interface for SQLite.
type sqliteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens (or creates) the SQLite database at dbPath and runs migrations.
func NewSQLiteRepository(dbPath string) (Repository, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	r := &sqliteRepository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return r, nil
}

// Close closes the underlying database connection.
func (r *sqliteRepository) Close() error {
	return r.db.Close()
}

func (r *sqliteRepository) migrate() error {
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
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	// SQLite has no ADD COLUMN IF NOT EXISTS; try each and ignore duplicate-column errors.
	for _, stmt := range []string{
		`ALTER TABLE user_events ADD COLUMN url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE user_events ADD COLUMN http_method TEXT NOT NULL DEFAULT 'POST'`,
	} {
		if _, err := r.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

// newUUID generates a random RFC-4122 v4 UUID string.
func newUUID() string {
	return uuid.New().String()
}

// CreateUser inserts a new registered user and returns it with generated IDs.
func (r *sqliteRepository) CreateUser(ctx context.Context, hostName, hostURL string) (*RegisteredUser, error) {
	now := time.Now()
	user := &RegisteredUser{
		UserID:        newUUID(),
		HostName:      hostName,
		HostURL:       hostURL,
		PerUserAPIKey: "uak-" + newUUID(),
		CreatedAt:     now,
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
func (r *sqliteRepository) GetUserByAPIKey(ctx context.Context, apiKey string) (*RegisteredUser, error) {
	user := &RegisteredUser{}
	var createdAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, host_name, host_url, per_user_api_key, created_at
		 FROM registered_users WHERE per_user_api_key = ?`,
		apiKey,
	).Scan(&user.UserID, &user.HostName, &user.HostURL, &user.PerUserAPIKey, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if createdAt.Valid {
		user.CreatedAt = createdAt.Time
	}
	return user, nil
}

// CreateEvent inserts a new user event and returns it with a generated event_id.
func (r *sqliteRepository) CreateEvent(ctx context.Context, userID, eventName, cronSpec, humanSpec, bodyTemplate, url, httpMethod string) (*UserEvent, error) {
	now := time.Now()
	event := &UserEvent{
		EventID:      "evid-" + newUUID(),
		UserID:       userID,
		EventName:    eventName,
		CronSpec:     cronSpec,
		HumanSpec:    humanSpec,
		BodyTemplate: bodyTemplate,
		URL:          url,
		HTTPMethod:   httpMethod,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_events (event_id, user_id, event_name, cron_spec, human_spec, body_template, url, http_method)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.EventID, event.UserID, event.EventName, event.CronSpec, event.HumanSpec, event.BodyTemplate, event.URL, event.HTTPMethod,
	)
	if err != nil {
		return nil, err
	}
	return event, nil
}

// UpdateEvent applies the non-nil fields in params to the event identified by eventID + userID.
// Returns an error if the event does not exist or does not belong to the user.
func (r *sqliteRepository) UpdateEvent(ctx context.Context, eventID, userID string, params UpdateEventParams) error {
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
	if params.URL != nil {
		setClauses = append(setClauses, "url = ?")
		args = append(args, *params.URL)
	}
	if params.HTTPMethod != nil {
		setClauses = append(setClauses, "http_method = ?")
		args = append(args, *params.HTTPMethod)
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
func (r *sqliteRepository) DeleteEvent(ctx context.Context, eventID string) error {
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

func scanSQLiteEvents(rows *sql.Rows) ([]UserEvent, error) {
	var events []UserEvent
	for rows.Next() {
		var e UserEvent
		var createdAt, updatedAt sql.NullTime
		if err := rows.Scan(&e.EventID, &e.UserID, &e.EventName, &e.CronSpec, &e.HumanSpec, &e.BodyTemplate, &e.URL, &e.HTTPMethod, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if createdAt.Valid {
			e.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			e.UpdatedAt = updatedAt.Time
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

const eventSelectCols = `event_id, user_id, event_name, cron_spec, human_spec, body_template, url, http_method, created_at, updated_at`

// ListEvents returns all events belonging to userID, newest first.
func (r *sqliteRepository) ListEvents(ctx context.Context, userID string) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventSelectCols+` FROM user_events WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteEvents(rows)
}

// SearchEvents returns events for userID whose name contains eventName (case-insensitive).
func (r *sqliteRepository) SearchEvents(ctx context.Context, userID, eventName string) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventSelectCols+` FROM user_events WHERE user_id = ? AND event_name LIKE ? ORDER BY created_at DESC`,
		userID, "%"+eventName+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteEvents(rows)
}

// ListAllEvents returns every event across all users. Used by the cron scheduler on startup.
func (r *sqliteRepository) ListAllEvents(ctx context.Context) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventSelectCols+` FROM user_events ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteEvents(rows)
}

// Package repository provides PostgreSQL-backed persistence.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"github.com/pschlump/httpcron/lib/config"
)

// postgresRepository implements the Repository interface for PostgreSQL.
type postgresRepository struct {
	db *sql.DB
}

// NewPostgresRepository creates a new PostgreSQL repository connection based on the config.
func NewPostgresRepository(cfg *config.Config) (Repository, error) {
	var dsn string
	if cfg.Server.DbPgAuthUseURL == "yes" || cfg.Server.DbPgAuthUseURL == "true" {
		// Use URL-based connection
		dsn = cfg.Server.DbPgAuthURL
	} else {
		// Build connection string from individual components
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			cfg.Server.DbPgAuthHost,
			cfg.Server.DbPgAuthPort,
			cfg.Server.DbPgAuthUsername,
			cfg.Server.DbPgAuthPassword,
			cfg.Server.DbPgAuthDatabase,
		)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	r := &postgresRepository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return r, nil
}

// Close closes the underlying database connection.
func (r *postgresRepository) Close() error {
	return r.db.Close()
}

func (r *postgresRepository) migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS registered_users (
		user_id          TEXT PRIMARY KEY,
		host_name        TEXT NOT NULL,
		host_url         TEXT NOT NULL,
		per_user_api_key TEXT NOT NULL UNIQUE,
		created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS user_events (
		event_id      TEXT PRIMARY KEY,
		user_id       TEXT NOT NULL REFERENCES registered_users(user_id),
		event_name    TEXT NOT NULL,
		cron_spec     TEXT NOT NULL DEFAULT '',
		human_spec    TEXT NOT NULL DEFAULT '',
		body_template TEXT NOT NULL DEFAULT '',
		url           TEXT NOT NULL DEFAULT '',
		http_method   TEXT NOT NULL DEFAULT 'POST',
		created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	/*
		// PostgreSQL supports ADD COLUMN IF NOT EXISTS
		for _, stmt := range []string{
			`ALTER TABLE user_events ADD COLUMN IF NOT EXISTS url TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE user_events ADD COLUMN IF NOT EXISTS http_method TEXT NOT NULL DEFAULT 'POST'`,
		} {
			if _, err := r.db.Exec(stmt); err != nil {
				return err
			}
		}
	*/
	return nil
}

// CreateUser inserts a new registered user and returns it with generated IDs.
func (r *postgresRepository) CreateUser(ctx context.Context, hostName, hostURL string) (*RegisteredUser, error) {
	now := time.Now()
	user := &RegisteredUser{
		UserID:        newUUID(),
		HostName:      hostName,
		HostURL:       hostURL,
		PerUserAPIKey: "uak-" + newUUID(),
		CreatedAt:     now,
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO registered_users (user_id, host_name, host_url, per_user_api_key) VALUES ($1, $2, $3, $4)`,
		user.UserID, user.HostName, user.HostURL, user.PerUserAPIKey,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByAPIKey returns the user with the given per_user_api_key, or nil if not found.
func (r *postgresRepository) GetUserByAPIKey(ctx context.Context, apiKey string) (*RegisteredUser, error) {
	user := &RegisteredUser{}
	var createdAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id, host_name, host_url, per_user_api_key, created_at
		 FROM registered_users WHERE per_user_api_key = $1`,
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
func (r *postgresRepository) CreateEvent(ctx context.Context, userID, eventName, cronSpec, humanSpec, bodyTemplate, url, httpMethod string) (*UserEvent, error) {
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
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		event.EventID, event.UserID, event.EventName, event.CronSpec, event.HumanSpec, event.BodyTemplate, event.URL, event.HTTPMethod,
	)
	if err != nil {
		return nil, err
	}
	return event, nil
}

// UpdateEvent applies the non-nil fields in params to the event identified by eventID + userID.
// Returns an error if the event does not exist or does not belong to the user.
func (r *postgresRepository) UpdateEvent(ctx context.Context, eventID, userID string, params UpdateEventParams) error {
	setClauses := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []interface{}{}
	argPos := 1

	if params.EventName != nil {
		setClauses = append(setClauses, fmt.Sprintf("event_name = $%d", argPos))
		args = append(args, *params.EventName)
		argPos++
	}
	if params.CronSpec != nil {
		setClauses = append(setClauses, fmt.Sprintf("cron_spec = $%d", argPos))
		args = append(args, *params.CronSpec)
		argPos++
	}
	if params.HumanSpec != nil {
		setClauses = append(setClauses, fmt.Sprintf("human_spec = $%d", argPos))
		args = append(args, *params.HumanSpec)
		argPos++
	}
	if params.BodyTemplate != nil {
		setClauses = append(setClauses, fmt.Sprintf("body_template = $%d", argPos))
		args = append(args, *params.BodyTemplate)
		argPos++
	}
	if params.URL != nil {
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", argPos))
		args = append(args, *params.URL)
		argPos++
	}
	if params.HTTPMethod != nil {
		setClauses = append(setClauses, fmt.Sprintf("http_method = $%d", argPos))
		args = append(args, *params.HTTPMethod)
		argPos++
	}

	args = append(args, eventID, userID)
	query := fmt.Sprintf(
		"UPDATE user_events SET %s WHERE event_id = $%d AND user_id = $%d",
		strings.Join(setClauses, ", "),
		argPos,
		argPos+1,
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
func (r *postgresRepository) DeleteEvent(ctx context.Context, eventID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM user_events WHERE event_id = $1`, eventID)
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

func scanPostgresEvents(rows *sql.Rows) ([]UserEvent, error) {
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

const postgresEventSelectCols = `event_id, user_id, event_name, cron_spec, human_spec, body_template, url, http_method, created_at, updated_at`

// ListEvents returns all events belonging to userID, newest first.
func (r *postgresRepository) ListEvents(ctx context.Context, userID string) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+postgresEventSelectCols+` FROM user_events WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostgresEvents(rows)
}

// SearchEvents returns events for userID whose name contains eventName (case-insensitive).
func (r *postgresRepository) SearchEvents(ctx context.Context, userID, eventName string) ([]UserEvent, error) {
	// PostgreSQL uses ILIKE for case-insensitive search
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+postgresEventSelectCols+` FROM user_events WHERE user_id = $1 AND event_name ILIKE $2 ORDER BY created_at DESC`,
		userID, "%"+eventName+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostgresEvents(rows)
}

// ListAllEvents returns every event across all users. Used by the cron scheduler on startup.
func (r *postgresRepository) ListAllEvents(ctx context.Context) ([]UserEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+postgresEventSelectCols+` FROM user_events ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPostgresEvents(rows)
}

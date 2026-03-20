// Command create-admin-user creates an admin user in the admin_user_auth table.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/pschlump/httpcron/lib/config"
	"github.com/pschlump/httpcron/lib/repository"
)

var configPathFlag = flag.String("cfg", "config.json", "path of the config file")
var usernameFlag = flag.String("username", "", "admin username (required)")
var passwordFlag = flag.String("password", "", "admin password (required)")

func main() {
	flag.Parse()

	if *usernameFlag == "" {
		log.Fatal("Error: -username flag is required")
	}
	if *passwordFlag == "" {
		log.Fatal("Error: -password flag is required")
	}

	// Read config
	cfg, err := config.FromFile(*configPathFlag)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	var db *sql.DB
	switch cfg.Server.DbKind {
	case "sqlite":
		db, err = sql.Open("sqlite", cfg.Server.DbPath)
	case "postgres":
		var dsn string
		if cfg.Server.DbPgAuthUseURL == "yes" || cfg.Server.DbPgAuthUseURL == "true" {
			dsn = cfg.Server.DbPgAuthURL
		} else {
			dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
				cfg.Server.DbPgAuthHost,
				cfg.Server.DbPgAuthPort,
				cfg.Server.DbPgAuthUsername,
				cfg.Server.DbPgAuthPassword,
				cfg.Server.DbPgAuthDatabase,
			)
		}
		db, err = sql.Open("postgres", dsn)
	default:
		log.Fatalf("Unsupported database kind: %s", cfg.Server.DbKind)
	}
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Hash the password
	passwordHash, err := repository.HashPasswordBcrypt(*passwordFlag)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	// Generate user ID
	userID := uuid.New().String()

	// Insert admin user
	ctx := context.Background()
	var result sql.Result
	var query string

	switch cfg.Server.DbKind {
	case "sqlite":
		query = `INSERT INTO admin_user_auth (user_id, username, password_hash) VALUES (?, ?, ?)`
	case "postgres":
		query = `INSERT INTO admin_user_auth (user_id, username, password_hash) VALUES ($1, $2, $3)`
	}

	result, err = db.ExecContext(ctx, query, userID, *usernameFlag, passwordHash)
	if err != nil {
		log.Fatalf("Failed to insert admin user: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("Successfully created admin user:\n")
	fmt.Printf("  User ID: %s\n", userID)
	fmt.Printf("  Username: %s\n", *usernameFlag)
	fmt.Printf("  Rows affected: %d\n", rowsAffected)
}

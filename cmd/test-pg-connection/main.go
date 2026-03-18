// Command test-pg-connection tests the PostgreSQL database connection.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"

	_ "github.com/lib/pq"

	"github.com/pschlump/dbgo"
	"github.com/pschlump/httpcron/lib/config"
)

var configPathFlag = flag.String("cfg", "test-config.json", "path of the config file")

func main() {
	flag.Parse()

	// Read config
	cfg, err := config.FromFile(*configPathFlag)
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	fmt.Printf("Testing PostgreSQL connection...\n")
	fmt.Printf("Config: DbKind=%s, DbPgAuthUseURL=%s\n", cfg.Server.DbKind, cfg.Server.DbPgAuthUseURL)

	var dsn string
	if cfg.Server.DbPgAuthUseURL == "yes" || cfg.Server.DbPgAuthUseURL == "true" {
		// Use URL-based connection
		dsn = cfg.Server.DbPgAuthURL
		fmt.Printf("Using URL connection: %s\n", dsn)
	} else {
		// Build connection string from individual components
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			cfg.Server.DbPgAuthHost,
			cfg.Server.DbPgAuthPort,
			cfg.Server.DbPgAuthUsername,
			cfg.Server.DbPgAuthPassword,
			cfg.Server.DbPgAuthDatabase,
		)
		fmt.Printf("Using individual parameters:\n")
		fmt.Printf("  host=%s\n", cfg.Server.DbPgAuthHost)
		fmt.Printf("  port=%s\n", cfg.Server.DbPgAuthPort)
		fmt.Printf("  user=%s\n", cfg.Server.DbPgAuthUsername)
		fmt.Printf("  database=%s\n", cfg.Server.DbPgAuthDatabase)
	}

	if false {
		ioutil.WriteFile("test-config-validate.json", []byte(dbgo.SVarI(cfg)), 0644)
	}

	// Test connection
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test the connection with a ping
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("✓ Successfully connected to PostgreSQL!")

	// Run a simple query to verify database access
	var version string
	err = db.QueryRow("SELECT version()").Scan(&version)
	if err != nil {
		log.Fatalf("Failed to query database version: %v", err)
	}

	fmt.Printf("✓ Database version: %.50s...\n", version)

	// Check if the httpcron database exists and is accessible
	var dbName string
	err = db.QueryRow("SELECT current_database()").Scan(&dbName)
	if err != nil {
		log.Fatalf("Failed to get current database: %v", err)
	}
	fmt.Printf("✓ Connected to database: %s\n", dbName)

	// Check if we can create a test table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS connection_test (
			id SERIAL PRIMARY KEY,
			test_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Fatalf("Failed to create test table: %v", err)
	}
	fmt.Println("✓ Successfully created test table")

	// Insert a test row
	result, err := db.Exec("INSERT INTO connection_test DEFAULT VALUES")
	if err != nil {
		log.Fatalf("Failed to insert test row: %v", err)
	}
	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("✓ Successfully inserted test row (affected: %d)\n", rowsAffected)

	// Clean up the test table
	_, err = db.Exec("DROP TABLE IF EXISTS connection_test")
	if err != nil {
		log.Fatalf("Failed to drop test table: %v", err)
	}
	fmt.Println("✓ Successfully cleaned up test table")

	fmt.Println("\n✓ All PostgreSQL connection tests passed!")
}

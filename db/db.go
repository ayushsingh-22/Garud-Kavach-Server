package db

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func Init() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	var err error
	DB, err = sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("failed to open database connection: %v", err)
	}

	DB.SetMaxOpenConns(8)
	DB.SetMaxIdleConns(4)
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err := DB.Ping(); err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	// Phase-2 fix: Ensure notifications.type CHECK constraint allows 'sos' value
	_, _ = DB.Exec(`
		ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;
		ALTER TABLE notifications ADD CONSTRAINT notifications_type_check
			CHECK (type IN ('info', 'warning', 'success', 'error', 'sos'));
	`)

	log.Println("Database connection successful.")
}

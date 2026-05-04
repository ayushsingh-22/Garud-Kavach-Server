package main

import (
	"fmt"
	"log"
	"os"

	"server/db"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	db.Init()

	// Check table existence
	tables := []string{"guard_locations", "incidents"}
	for _, t := range tables {
		var exists bool
		_ = db.DB.QueryRow(
			`SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname='public' AND tablename=$1)`, t,
		).Scan(&exists)
		fmt.Printf("Table %-20s exists=%v\n", t, exists)
	}

	// Apply migration 007
	sql, err := os.ReadFile("migrations/007_guard_tracking_schema.sql")
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}
	if _, err := db.DB.Exec(string(sql)); err != nil {
		log.Fatalf("apply migration: %v", err)
	}
	fmt.Println("\nMigration 007 applied (or already up to date).")

	// Verify
	for _, t := range tables {
		var exists bool
		_ = db.DB.QueryRow(
			`SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname='public' AND tablename=$1)`, t,
		).Scan(&exists)
		fmt.Printf("After: Table %-20s exists=%v\n", t, exists)
	}
}

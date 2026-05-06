package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("No .env file found, reading DATABASE_URL from environment")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	sqlBytes, err := os.ReadFile("../../migrations/010_fix_overtime_formula.sql")
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}

	res, err := db.Exec(string(sqlBytes))
	if err != nil {
		log.Fatalf("exec migration: %v", err)
	}

	rows, _ := res.RowsAffected()
	fmt.Printf("Migration 010 applied. Rows affected (payroll update): %d\n", rows)

	// Verify: count shifts with overtime
	var otShifts int
	db.QueryRow(`SELECT COUNT(*) FROM shifts WHERE overtime_hours > 0 AND deleted_at IS NULL`).Scan(&otShifts)
	fmt.Printf("Shifts with overtime:  %d\n", otShifts)

	// Verify: check that no paid_hours > 12
	var badShifts int
	db.QueryRow(`SELECT COUNT(*) FROM shifts WHERE paid_hours > 12 AND deleted_at IS NULL`).Scan(&badShifts)
	fmt.Printf("Shifts with paid_hours > 12 (should be 0): %d\n", badShifts)

	// Sample: show a few OT shifts for verification
	sampleRows, err := db.Query(`
		SELECT id, actual_hours, overtime_hours, paid_hours
		FROM shifts
		WHERE overtime_hours > 0 AND deleted_at IS NULL
		LIMIT 5
	`)
	if err == nil {
		defer sampleRows.Close()
		fmt.Println("\nSample OT shifts (actual | overtime | paid):")
		for sampleRows.Next() {
			var id, ah, ot, ph float64
			sampleRows.Scan(&id, &ah, &ot, &ph)
			fmt.Printf("  shift %v: actual=%.1f  overtime=%.1f  paid=%.1f\n", id, ah, ot, ph)
		}
	}
}

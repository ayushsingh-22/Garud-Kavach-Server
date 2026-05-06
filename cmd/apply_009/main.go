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

	sql, err := os.ReadFile("migrations/009_overtime_schema.sql")
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}
	if _, err := db.DB.Exec(string(sql)); err != nil {
		log.Fatalf("apply migration: %v", err)
	}
	fmt.Println("Migration 009 applied successfully.")

	// Verify new columns exist
	cols := []string{"overtime_hours", "paid_hours"}
	for _, table := range []string{"shifts", "payroll"} {
		for _, col := range cols {
			var exists bool
			_ = db.DB.QueryRow(
				`SELECT EXISTS (
					SELECT FROM information_schema.columns
					WHERE table_name=$1 AND column_name=$2
				)`, table, col,
			).Scan(&exists)
			fmt.Printf("  %-10s.%-20s exists=%v\n", table, col, exists)
		}
	}

	// Report how many shifts had data fixed
	var capped, updated int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM shifts WHERE actual_hours = 12 AND deleted_at IS NULL`).Scan(&capped)
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM shifts WHERE paid_hours > actual_hours AND deleted_at IS NULL`).Scan(&updated)
	fmt.Printf("\nShifts with overtime pay: %d\n", updated)
	fmt.Printf("Payroll records updated:  ")
	var prCount int
	_ = db.DB.QueryRow(`SELECT COUNT(*) FROM payroll WHERE paid_hours > 0 AND deleted_at IS NULL`).Scan(&prCount)
	fmt.Printf("%d\n", prCount)
	_ = capped
}

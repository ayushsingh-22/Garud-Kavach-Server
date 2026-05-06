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
		log.Println("No .env file, using env var")
	}
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	sqlBytes, err := os.ReadFile("../../migrations/011_payroll_consistency.sql")
	if err != nil {
		log.Fatalf("read migration: %v", err)
	}

	if _, err := db.Exec(string(sqlBytes)); err != nil {
		log.Fatalf("exec migration: %v", err)
	}
	fmt.Println("Migration 011 applied.")

	// Verify
	var mismatch int
	db.QueryRow(`SELECT COUNT(*) FROM payroll WHERE deleted_at IS NULL AND ROUND(paid_hours::numeric,2) != ROUND(total_hours::numeric,2)`).Scan(&mismatch)
	fmt.Printf("paid_hours ≠ total_hours (should be 0): %d\n", mismatch)

	var payMismatch int
	db.QueryRow(`SELECT COUNT(*) FROM payroll WHERE deleted_at IS NULL AND ABS(total_pay - (paid_hours + overtime_hours) * rate_per_hour) > 1`).Scan(&payMismatch)
	fmt.Printf("total_pay formula mismatch (should be 0): %d\n", payMismatch)

	var total int
	db.QueryRow(`SELECT COUNT(*) FROM payroll WHERE deleted_at IS NULL`).Scan(&total)
	fmt.Printf("Total payroll records checked: %d\n", total)

	// Sample OT records
	rows, _ := db.Query(`
		SELECT p.id, g.name, TO_CHAR(p.month,'YYYY-MM'),
		       p.total_hours, p.overtime_hours, p.paid_hours,
		       p.rate_per_hour, p.total_pay
		FROM payroll p LEFT JOIN guards g ON p.guard_id = g.id
		WHERE p.deleted_at IS NULL AND p.overtime_hours > 0
		ORDER BY p.overtime_hours DESC LIMIT 5
	`)
	defer rows.Close()
	fmt.Println("\nSample OT payroll records after fix:")
	fmt.Printf("%-6s %-20s %-8s %-8s %-6s %-8s %-6s %-10s\n",
		"ID", "Guard", "Month", "Total", "OT", "Paid", "Rate", "TotalPay")
	for rows.Next() {
		var id int
		var name, month string
		var total, ot, paid, rate, pay float64
		rows.Scan(&id, &name, &month, &total, &ot, &paid, &rate, &pay)
		n := name
		if len(n) > 18 {
			n = n[:18]
		}
		fmt.Printf("%-6d %-20s %-8s %-8.1f %-6.1f %-8.1f %-6.0f %-10.0f\n",
			id, n, month, total, ot, paid, rate, pay)
	}
}

package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("No .env file, using env var")
	}
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== SHIFT-LEVEL CHECK ===")
	rows, _ := db.Query(`
		SELECT id, actual_hours, overtime_hours, paid_hours,
		       (paid_hours + overtime_hours) AS effective_hrs
		FROM shifts
		WHERE deleted_at IS NULL AND actual_hours IS NOT NULL
		ORDER BY RANDOM() LIMIT 15
	`)
	defer rows.Close()
	fmt.Printf("%-6s %-10s %-12s %-10s %-12s %-20s\n",
		"ID", "actual", "overtime", "paid", "effective", "paid=actual?")
	for rows.Next() {
		var id int
		var actual, ot, paid, eff float64
		rows.Scan(&id, &actual, &ot, &paid, &eff)
		ok := "✓"
		if round2(paid) != round2(actual) {
			ok = "✗ MISMATCH paid≠actual"
		}
		if actual > 12 {
			ok = "✗ actual>12 CAP MISSING"
		}
		fmt.Printf("%-6d %-10.1f %-12.1f %-10.1f %-12.1f %s\n", id, actual, ot, paid, eff, ok)
	}

	fmt.Println("\n=== PAYROLL-LEVEL CHECK (formula: total_pay = (paid+ot)*rate) ===")
	prows, _ := db.Query(`
		SELECT p.id, g.name, TO_CHAR(p.month,'YYYY-MM'),
		       p.total_hours, p.overtime_hours, p.paid_hours,
		       p.rate_per_hour, p.total_pay,
		       (p.paid_hours + p.overtime_hours) * p.rate_per_hour AS expected_pay
		FROM payroll p
		LEFT JOIN guards g ON p.guard_id = g.id
		WHERE p.deleted_at IS NULL AND p.overtime_hours > 0
		ORDER BY p.overtime_hours DESC
		LIMIT 10
	`)
	defer prows.Close()
	fmt.Printf("%-6s %-20s %-8s %-8s %-8s %-8s %-8s %-10s %-10s %-15s\n",
		"ID", "Guard", "Month", "Total", "OT", "Paid", "Rate", "TotalPay", "Expected", "Match?")
	for prows.Next() {
		var id int
		var name, month string
		var total, ot, paid, rate, pay, expected float64
		prows.Scan(&id, &name, &month, &total, &ot, &paid, &rate, &pay, &expected)
		ok := "✓"
		if round2(pay) != round2(expected) {
			ok = fmt.Sprintf("✗ MISMATCH (diff=%.2f)", pay-expected)
		}
		n := name
		if len(n) > 18 {
			n = n[:18]
		}
		fmt.Printf("%-6d %-20s %-8s %-8.1f %-8.1f %-8.1f %-8.0f %-10.0f %-10.0f %s\n",
			id, n, month, total, ot, paid, rate, pay, expected, ok)
	}

	fmt.Println("\n=== PAID_HOURS vs TOTAL_HOURS check ===")
	var mismatch int
	db.QueryRow(`SELECT COUNT(*) FROM payroll WHERE deleted_at IS NULL AND ROUND(paid_hours::numeric,2) != ROUND(total_hours::numeric,2)`).Scan(&mismatch)
	fmt.Printf("Payroll records where paid_hours ≠ total_hours: %d (should be 0 unless caps applied)\n", mismatch)

	var otCount int
	db.QueryRow(`SELECT COUNT(*) FROM payroll WHERE deleted_at IS NULL AND overtime_hours > 0`).Scan(&otCount)
	fmt.Printf("Payroll records with overtime: %d\n", otCount)
}

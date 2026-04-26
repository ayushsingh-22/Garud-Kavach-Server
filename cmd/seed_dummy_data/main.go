package main

import (
	"fmt"
	"log"
	"math/rand"
	"server/db"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		if err := godotenv.Load(); err != nil {
			log.Println("Warning: .env file not loaded, relying on system environment variables")
		}
	}

	db.Init()

	fmt.Println("Starting dummy data seed...")

	seedGuards()
	seedQueries()
	seedAuditLogs()
	seedFinanceAndHR()

	fmt.Println("Seeding completed successfully.")
}

func seedGuards() {
	guards := []struct {
		Name          string
		Phone         string
		Email         string
		LicenseExpiry string
		Status        string
		HourlyRate    float64
	}{
		{"Ramesh Singh", "+91 9876543210", "ramesh@example.com", "2026-10-15", "active", 150},
		{"Suresh Kumar", "+91 9876543211", "suresh@example.com", "2024-11-20", "inactive", 150},
		{"Amit Sharma", "+91 9876543212", "amit@example.com", "2025-05-10", "active", 160},
		{"Vikram Yadav", "+91 9876543213", "vikram@example.com", "2026-01-25", "on_leave", 180},
		{"Rahul Verma", "+91 9876543214", "rahul@example.com", "2024-09-30", "active", 150},
		{"Deepak Rana", "+91 9876543215", "deepak@example.com", "2027-02-14", "active", 170},
		{"Karan Singh", "+91 9876543216", "karan@example.com", "2025-12-01", "active", 160},
		{"Rajesh Gupta", "+91 9876543217", "rajesh@example.com", "2026-08-20", "inactive", 150},
		{"Sanjay Mishra", "+91 9876543218", "sanjay@example.com", "2026-04-11", "active", 190},
		{"Arun Patel", "+91 9876543219", "arun@example.com", "2025-07-22", "active", 150},
	}

	for _, g := range guards {
		var exists int
		err := db.DB.QueryRow("SELECT COUNT(*) FROM guards WHERE phone = $1", g.Phone).Scan(&exists)
		if err != nil || exists > 0 {
			continue
		}

		_, err = db.DB.Exec(
			"INSERT INTO guards (name, phone, email, license_expiry, status, hourly_rate) VALUES ($1, $2, $3, $4, $5, $6)",
			g.Name, g.Phone, g.Email, g.LicenseExpiry, g.Status, g.HourlyRate,
		)
		if err != nil {
			log.Printf("Failed to seed guard %s: %v", g.Name, err)
		}
	}
	fmt.Println("Guards seeded.")
}

func seedQueries() {
	services := []string{"Bouncer", "Security Guard", "Event Security", "Corporate Security"}
	statuses := []string{"Pending", "In Progress", "Resolved", "Rejected"}

	for i := 1; i <= 15; i++ {
		email := fmt.Sprintf("client%d@example.com", i)
		var exists int
		err := db.DB.QueryRow("SELECT COUNT(*) FROM queries WHERE email = $1", email).Scan(&exists)
		if err != nil || exists > 0 {
			continue
		}

		service := services[rand.Intn(len(services))]
		status := statuses[rand.Intn(len(statuses))]
		submittedAt := time.Now().Add(-time.Duration(rand.Intn(720)) * time.Hour) // up to 30 days ago

		_, err = db.DB.Exec(
			`INSERT INTO queries (name, email, phone, service, message, num_guards, status, cost, submitted_at) 
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			fmt.Sprintf("Client %d", i), email, fmt.Sprintf("+91 90000000%02d", i), service,
			"Need security services ASAP", rand.Intn(5)+1, status, float64(rand.Intn(5000)+1000), submittedAt,
		)
		if err != nil {
			log.Printf("Failed to seed query %d: %v", i, err)
		}
	}
	fmt.Println("Queries seeded.")
}

func seedAuditLogs() {
	var adminID int
	err := db.DB.QueryRow("SELECT id FROM users WHERE role = 'superadmin' LIMIT 1").Scan(&adminID)
	if err != nil {
		fmt.Println("No superadmin found for audit logs, skipping audit log seed.")
		return
	}

	actions := []string{"create_guard", "delete_guard", "update_status", "assign_guard"}

	for i := 1; i <= 10; i++ {
		action := actions[rand.Intn(len(actions))]
		target := fmt.Sprintf("system_entity:%d", rand.Intn(100)+1)
		details := fmt.Sprintf(`{"note":"Seeded event %d"}`, i)
		createdAt := time.Now().Add(-time.Duration(rand.Intn(168)) * time.Hour) // up to 7 days ago

		_, err = db.DB.Exec(
			"INSERT INTO audit_logs (user_id, action, target, details, created_at) VALUES ($1, $2, $3, $4, $5)",
			adminID, action, target, details, createdAt,
		)
		if err != nil {
			log.Printf("Failed to seed audit log %d: %v", i, err)
		}
	}
	fmt.Println("Audit logs seeded.")
}

func seedFinanceAndHR() {
	// Seed Invoices
	var queryIDs []int
	rows, err := db.DB.Query("SELECT id FROM queries WHERE status != 'Pending' LIMIT 10")
	if err == nil {
		for rows.Next() {
			var id int
			rows.Scan(&id)
			queryIDs = append(queryIDs, id)
		}
		rows.Close()
	}

	invoiceStatuses := []string{"pending", "paid"}
	for _, qID := range queryIDs {
		status := invoiceStatuses[rand.Intn(len(invoiceStatuses))]
		amount := float64(rand.Intn(10000) + 5000)
		issuedAt := time.Now().Add(-time.Duration(rand.Intn(30*24)) * time.Hour)

		var paidAt interface{}
		if status == "paid" {
			paidTime := issuedAt.Add(time.Duration(rand.Intn(5*24)) * time.Hour)
			paidAt = paidTime
		}

		db.DB.Exec("INSERT INTO invoices (query_id, amount, status, issued_at, paid_at) VALUES ($1, $2, $3, $4, $5)",
			qID, amount, status, issuedAt, paidAt)
	}
	fmt.Println("Invoices seeded.")

	// Seed Expenses
	var superadminID int
	db.DB.QueryRow("SELECT id FROM users WHERE role = 'superadmin' LIMIT 1").Scan(&superadminID)

	categories := []string{"Equipment", "Travel", "Office Supplies", "Marketing"}
	for i := 0; i < 5; i++ {
		cat := categories[rand.Intn(len(categories))]
		amount := float64(rand.Intn(5000) + 500)
		date := time.Now().Add(-time.Duration(rand.Intn(30*24)) * time.Hour).Format("2006-01-02")

		db.DB.Exec("INSERT INTO expenses (category, description, amount, expense_date, added_by) VALUES ($1, $2, $3, $4, $5)",
			cat, "Dummy expense for "+cat, amount, date, superadminID)
	}
	fmt.Println("Expenses seeded.")

	// Seed Shifts & Payroll
	var guardIDs []int
	rows, err = db.DB.Query("SELECT id FROM guards WHERE status = 'active' LIMIT 5")
	if err == nil {
		for rows.Next() {
			var id int
			rows.Scan(&id)
			guardIDs = append(guardIDs, id)
		}
		rows.Close()
	}

	if len(guardIDs) > 0 && len(queryIDs) > 0 {
		for _, gID := range guardIDs {
			for i := 0; i < 3; i++ {
				qID := queryIDs[rand.Intn(len(queryIDs))]
				start := time.Now().Add(-time.Duration(rand.Intn(15*24)) * time.Hour)
				end := start.Add(time.Duration(8) * time.Hour)

				db.DB.Exec("INSERT INTO shifts (guard_id, query_id, start_time, end_time, actual_hours, status) VALUES ($1, $2, $3, $4, $5, $6)",
					gID, qID, start, end, 8.0, "completed")
			}

			// Add leave request
			start := time.Now().Add(time.Duration(rand.Intn(10*24)) * time.Hour).Format("2006-01-02")
			end := time.Now().Add(time.Duration(rand.Intn(10*24)+48) * time.Hour).Format("2006-01-02")
			db.DB.Exec("INSERT INTO leave_requests (guard_id, start_date, end_date, reason, status) VALUES ($1, $2, $3, $4, $5)",
				gID, start, end, "Personal reasons", "pending")
		}
		fmt.Println("Shifts and Leaves seeded.")
	}
}

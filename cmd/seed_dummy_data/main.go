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

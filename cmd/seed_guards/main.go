package main

import (
	"fmt"
	"log"
	"server/db"
	"time"

	"github.com/joho/godotenv"
)

type DummyGuard struct {
	Name          string
	Phone         string
	Email         string
	LicenseNo     string
	LicenseExpiry time.Time
	Status        string
	HourlyRate    float64
}

func main() {
	_ = godotenv.Load()
	db.Init()

	// Clear the table for a clean seed
	if _, err := db.DB.Exec("TRUNCATE TABLE guards RESTART IDENTITY CASCADE"); err != nil {
		log.Fatalf("Failed to truncate guards table: %v", err)
	}
	fmt.Println("Cleared the 'guards' table.")

	guards := []DummyGuard{
		{"John Smith", "123-456-7890", "john.s@test.com", "L12345", time.Now().AddDate(1, 0, 0), "active", 15.50},
		{"Jane Doe", "098-765-4321", "jane.d@test.com", "L67890", time.Now().AddDate(0, 1, 15), "active", 16.00},
		{"Peter Jones", "555-555-5555", "peter.j@test.com", "L54321", time.Now().AddDate(0, 0, 5), "on_leave", 15.00},
	}

	for _, guard := range guards {
		_, err := db.DB.Exec(
			`INSERT INTO guards (name, phone, email, license_no, license_expiry, status, hourly_rate) 
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			guard.Name, guard.Phone, guard.Email, guard.LicenseNo, guard.LicenseExpiry, guard.Status, guard.HourlyRate,
		)
		if err != nil {
			log.Fatalf("Failed to insert guard %s: %v", guard.Name, err)
		}
		fmt.Printf("Successfully seeded guard: %s\n", guard.Name)
	}

	fmt.Println("\nDummy guard seeding complete.")
}

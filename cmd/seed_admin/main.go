package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"server/db"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	db.Init()

	email := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_EMAIL")))
	passwordHash := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_HASH"))

	if email == "" || passwordHash == "" {
		log.Fatal("ADMIN_EMAIL and ADMIN_PASSWORD_HASH are required")
	}

	result, err := db.DB.Exec(
		`INSERT INTO users (email, password, role) VALUES ($1, $2, 'superadmin') ON CONFLICT (email) DO NOTHING`,
		email,
		passwordHash,
	)
	if err != nil {
		log.Fatalf("failed to seed superadmin user: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Fatalf("failed to read seed result: %v", err)
	}

	if rowsAffected == 0 {
		fmt.Printf("Superadmin already exists for email %s\n", email)
		return
	}

	fmt.Printf("Superadmin seeded successfully for email %s\n", email)
}

package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"server/db"
)

type TestUser struct {
	Name  string
	Email string
	Role  string
}

func main() {
	_ = godotenv.Load()
	db.Init()

	// Hash generated from "password123"
	hashedPassword := "$2a$10$zE2cRLdjnH9cJuePEvPfqe8xqHo1w.lO8lwyv08xZyZV1qAjm/TrC"

	users := []TestUser{
		{Name: "Manager User", Email: "manager@test.com", Role: "manager"},
		{Name: "Finance User", Email: "finance@test.com", Role: "finance"},
		{Name: "HR User", Email: "hr@test.com", Role: "hr"},
		{Name: "Customer User", Email: "customer@test.com", Role: "customer"},
	}

	for _, user := range users {
		result, err := db.DB.Exec(
			`INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, $4) ON CONFLICT (email) DO NOTHING`,
			user.Name,
			user.Email,
			hashedPassword,
			user.Role,
		)
		if err != nil {
			log.Fatalf("Failed to insert user %s: %v", user.Email, err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			fmt.Printf("Successfully seeded user: %s\n", user.Email)
		} else {
			fmt.Printf("User already exists: %s\n", user.Email)
		}
	}

	fmt.Println("\nDummy user seeding complete.")
}

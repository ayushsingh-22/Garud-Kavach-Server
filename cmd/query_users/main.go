package main

import (
	"fmt"
	"log"

	"server/db"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	db.Init()

	// Check all users
	rows, err := db.DB.Query("SELECT id, name, email, role, deleted_at IS NULL as active FROM users ORDER BY id")
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	fmt.Println("=== Users Table ===")
	fmt.Printf("%-4s %-20s %-30s %-12s %-6s\n", "ID", "Name", "Email", "Role", "Active")
	fmt.Println("-------------------------------------------------------------------------------------")
	for rows.Next() {
		var id int
		var name, email, role string
		var active bool
		if err := rows.Scan(&id, &name, &email, &role, &active); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-4d %-20s %-30s %-12s %-6v\n", id, name, email, role, active)
	}

	// Check if customers table exists
	var exists bool
	err = db.DB.QueryRow(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'customers')`).Scan(&exists)
	if err != nil {
		log.Fatalf("Check customers table: %v", err)
	}
	fmt.Printf("\n=== customers table exists: %v ===\n", exists)

	if exists {
		crows, err := db.DB.Query("SELECT id, user_id, phone, company, address FROM customers ORDER BY id")
		if err != nil {
			log.Fatalf("Query customers failed: %v", err)
		}
		defer crows.Close()
		fmt.Printf("%-4s %-8s %-15s %-20s %-30s\n", "ID", "UserID", "Phone", "Company", "Address")
		for crows.Next() {
			var id, userID int
			var phone, company, address *string
			if err := crows.Scan(&id, &userID, &phone, &company, &address); err != nil {
				log.Fatal(err)
			}
			p, c, a := "", "", ""
			if phone != nil {
				p = *phone
			}
			if company != nil {
				c = *company
			}
			if address != nil {
				a = *address
			}
			fmt.Printf("%-4d %-8d %-15s %-20s %-30s\n", id, userID, p, c, a)
		}
	}
}

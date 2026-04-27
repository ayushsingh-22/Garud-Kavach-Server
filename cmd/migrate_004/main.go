package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()
	_ = godotenv.Load("../../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	migration, err := os.ReadFile("migrations/004_customer_schema.sql")
	if err != nil {
		log.Fatal(err)
	}

	if _, err := db.Exec(string(migration)); err != nil {
		log.Fatal(err)
	}

	log.Println("Migration 004_customer_schema.sql applied successfully.")
}

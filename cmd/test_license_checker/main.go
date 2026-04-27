package main

import (
	"log"
	"server/db"
	"server/services"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not loaded, relying on system environment variables")
	}

	db.Init()
	services.StartEmailWorker()

	log.Println("Running guard license expiry check NOW...")
	services.CheckExpiringLicenses(db.DB)
	log.Println("Done.")
}

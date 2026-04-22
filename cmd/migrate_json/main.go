package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"server/db"

	"github.com/joho/godotenv"
)

type legacyQuery struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	Email           string  `json:"email"`
	Phone           string  `json:"phone"`
	Service         string  `json:"service"`
	Message         string  `json:"message"`
	SubmittedAt     string  `json:"submitted_at"`
	NumGuards       string  `json:"numGuards"`
	DurationType    string  `json:"durationType"`
	DurationValue   string  `json:"durationValue"`
	CameraRequired  bool    `json:"cameraRequired"`
	VehicleRequired bool    `json:"vehicleRequired"`
	FirstAid        bool    `json:"firstAid"`
	WalkieTalkie    bool    `json:"walkieTalkie"`
	BulletProof     bool    `json:"bulletProof"`
	FireSafety      bool    `json:"fireSafety"`
	Status          string  `json:"status"`
	Cost            float64 `json:"cost"`
}

func parseNumGuards(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 1
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil || parsed <= 0 {
		return 1
	}
	return parsed
}

func parseDurationValue(value string) float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseSubmittedAt(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Now().UTC()
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed.UTC()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/migrate_json <path-to-database.json>")
		os.Exit(1)
	}

	_ = godotenv.Load()
	db.Init()

	jsonPath, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatalf("failed to resolve JSON path: %v", err)
	}

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Fatalf("failed to read JSON file: %v", err)
	}

	var records []legacyQuery
	if err := json.Unmarshal(data, &records); err != nil {
		log.Fatalf("failed to parse JSON file: %v", err)
	}

	tx, err := db.DB.BeginTx(nil, &sql.TxOptions{})
	if err != nil {
		log.Fatalf("failed to start transaction: %v", err)
	}

	inserted := 0
	insertStmt := `
		INSERT INTO queries (
			id,
			name,
			email,
			phone,
			service,
			message,
			num_guards,
			duration_type,
			duration_value,
			camera_required,
			vehicle_required,
			first_aid,
			walkie_talkie,
			bullet_proof,
			fire_safety,
			status,
			cost,
			submitted_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18
		)`

	for _, record := range records {
		numGuards := parseNumGuards(record.NumGuards)
		durationValue := parseDurationValue(record.DurationValue)
		submittedAt := parseSubmittedAt(record.SubmittedAt)
		status := strings.TrimSpace(record.Status)
		if status == "" {
			status = "Pending"
		}

		_, err := tx.Exec(
			insertStmt,
			record.ID,
			record.Name,
			record.Email,
			record.Phone,
			record.Service,
			record.Message,
			numGuards,
			record.DurationType,
			durationValue,
			record.CameraRequired,
			record.VehicleRequired,
			record.FirstAid,
			record.WalkieTalkie,
			record.BulletProof,
			record.FireSafety,
			status,
			record.Cost,
			submittedAt,
		)
		if err != nil {
			_ = tx.Rollback()
			log.Fatalf("failed to insert record id %d: %v", record.ID, err)
		}

		inserted++
	}

	if _, err := tx.Exec(`
		SELECT setval(
			pg_get_serial_sequence('queries', 'id'),
			COALESCE((SELECT MAX(id) FROM queries), 1),
			true
		)`); err != nil {
		_ = tx.Rollback()
		log.Fatalf("failed to update queries id sequence: %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("failed to commit migration: %v", err)
	}

	total := len(records)
	fmt.Printf("Records in JSON: %d\n", total)
	fmt.Printf("Records inserted: %d\n", inserted)

	if inserted != total {
		fmt.Printf("Migration mismatch: inserted=%d expected=%d\n", inserted, total)
		os.Exit(1)
	}

	fmt.Println("Migration completed successfully. Insert count matches source JSON count.")
}

package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"server/db"
	"server/helpers"
	"server/models"
)

func parseNumGuards(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 1
	}

	numGuards, err := strconv.Atoi(trimmed)
	if err != nil || numGuards <= 0 {
		return 1
	}

	return numGuards
}

func parseDurationValue(value string) float64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}

	duration, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}

	return duration
}

func AddQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	var newQuery models.Query

	if err := json.NewDecoder(r.Body).Decode(&newQuery); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid input data"})
		return
	}

	newQuery.SubmittedAt = time.Now().UTC().Format(time.RFC3339)
	if newQuery.Status == "" {
		newQuery.Status = "Pending"
	}

	numGuards := parseNumGuards(newQuery.NumGuards)
	durationValue := parseDurationValue(newQuery.DurationValue)

	var insertedID int
	err := db.DB.QueryRow(
		`INSERT INTO queries (
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
			$15, $16, $17
		)
		RETURNING id`,
		newQuery.Name,
		newQuery.Email,
		newQuery.Phone,
		newQuery.Service,
		newQuery.Message,
		numGuards,
		newQuery.DurationType,
		durationValue,
		newQuery.CameraRequired,
		newQuery.VehicleRequired,
		newQuery.FirstAid,
		newQuery.WalkieTalkie,
		newQuery.BulletProof,
		newQuery.FireSafety,
		newQuery.Status,
		newQuery.Cost,
		newQuery.SubmittedAt,
	).Scan(&insertedID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save query"})
		return
	}

	if err := helpers.WriteAuditLog(db.DB, 0, "create_query", "query:"+strconv.Itoa(insertedID), map[string]interface{}{
		"name":    newQuery.Name,
		"email":   newQuery.Email,
		"service": newQuery.Service,
		"status":  newQuery.Status,
	}); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_query: %v", err)
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Query submitted successfully",
		"id":      insertedID,
	})
}

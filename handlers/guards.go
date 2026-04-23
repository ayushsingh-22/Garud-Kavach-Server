package handlers

import (
	"encoding/json"
	"net/http"
	"server/db"
	"server/helpers"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type Guard struct {
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	Phone         *string    `json:"phone"`
	Email         *string    `json:"email"`
	Address       *string    `json:"address"`
	LicenseNo     *string    `json:"license_no"`
	LicenseExpiry *time.Time `json:"license_expiry"`
	Status        string     `json:"status"`
	HourlyRate    float64    `json:"hourly_rate"`
	PhotoURL      *string    `json:"photo_url"`
}

// GetGuards retrieves all non-deleted guards.
func GetGuards(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query("SELECT id, name, phone, email, address, license_no, license_expiry, status, hourly_rate, photo_url FROM guards WHERE deleted_at IS NULL ORDER BY name")
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve guards"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	guards := []Guard{}
	for rows.Next() {
		var g Guard
		if err := rows.Scan(&g.ID, &g.Name, &g.Phone, &g.Email, &g.Address, &g.LicenseNo, &g.LicenseExpiry, &g.Status, &g.HourlyRate, &g.PhotoURL); err != nil {
			http.Error(w, `{"error":"Failed to scan guard data"}`, http.StatusInternalServerError)
			return
		}
		guards = append(guards, g)
	}
	json.NewEncoder(w).Encode(guards)
}

// CreateGuard adds a new guard to the database.
func CreateGuard(w http.ResponseWriter, r *http.Request) {
	var g Guard
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validation
	if g.Name == "" {
		http.Error(w, `{"error":"Guard name is required"}`, http.StatusBadRequest)
		return
	}
	if g.LicenseExpiry != nil && g.LicenseExpiry.Before(time.Now()) {
		http.Error(w, `{"error":"License expiry date must be in the future"}`, http.StatusBadRequest)
		return
	}
	if g.HourlyRate < 0 {
		http.Error(w, `{"error":"Hourly rate cannot be negative"}`, http.StatusBadRequest)
		return
	}

	err := db.DB.QueryRow(
		"INSERT INTO guards (name, phone, email, address, license_no, license_expiry, status, hourly_rate) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id",
		g.Name, g.Phone, g.Email, g.Address, g.LicenseNo, g.LicenseExpiry, g.Status, g.HourlyRate,
	).Scan(&g.ID)

	if err != nil {
		http.Error(w, `{"error":"Failed to create guard"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	helpers.WriteAuditLog(db.DB, int(userID), "create_guard", "guard:"+strconv.Itoa(g.ID), g)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(g)
}

// SoftDeleteGuard marks a guard as deleted.
func SoftDeleteGuard(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid guard ID"}`, http.StatusBadRequest)
		return
	}

	result, err := db.DB.Exec("UPDATE guards SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		http.Error(w, `{"error":"Failed to delete guard"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Guard not found"}`, http.StatusNotFound)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	helpers.WriteAuditLog(db.DB, int(userID), "delete_guard", "guard:"+strconv.Itoa(id), nil)

	w.WriteHeader(http.StatusNoContent)
}

// Note: Other guard endpoints (update, assign, expiring) will be added in subsequent steps.

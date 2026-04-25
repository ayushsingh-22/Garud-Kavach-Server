package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"server/db"
	"server/helpers"
	"strconv"
	"strings"
)

type UpdateRequest struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

var allowedStatuses = map[string]struct{}{
	"Pending":     {},
	"In Progress": {},
	"Resolved":    {},
	"Rejected":    {},
}

func UpdateQueryStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json") // ✅ always set this early

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Only POST method allowed"})
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	status := strings.TrimSpace(req.Status)
	if _, ok := allowedStatuses[status]; !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid status"})
		return
	}

	if req.ID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid query id"})
		return
	}

	result, err := db.DB.Exec(
		"UPDATE queries SET status = $1 WHERE id = $2 AND deleted_at IS NULL",
		status,
		req.ID,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update query status"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to verify update status"})
		return
	}

	if rowsAffected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Query not found"})
		return
	}

	// Audit Log
	var userID int
	if uid, ok := r.Context().Value(userIDKey).(float64); ok {
		userID = int(uid)
	}
	if err := helpers.WriteAuditLog(db.DB, userID, "status_update", "query:"+strconv.Itoa(req.ID), map[string]string{"new_status": status}); err != nil {
		log.Printf("ERROR: Failed to write audit log for status_update: %v", err)
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

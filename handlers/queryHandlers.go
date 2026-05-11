package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"server/db"
	"server/helpers"
	"server/services"
	"strconv"
	"strings"
)

type UpdateRequest struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
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
	if !helpers.ValidateStatus(status, []string{"Pending", "In Progress", "Resolved", "Rejected"}) {
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

	// ── Remove assigned shifts when status moves AWAY from Resolved ────────
	var guardsRemoved bool
	if status != "Resolved" {
		result2, delErr := db.DB.Exec(
			"DELETE FROM shifts WHERE query_id = $1",
			req.ID,
		)
		if delErr != nil {
			log.Printf("WARNING: failed to remove shifts for query %d on status change: %v", req.ID, delErr)
		} else if n, _ := result2.RowsAffected(); n > 0 {
			guardsRemoved = true
			log.Printf("INFO: removed %d shift(s) for query %d (status changed to %s)", n, req.ID, status)
		}
	}

	// ── Phase-1: Auto-assign guards when status becomes "Resolved" ──────────
	var assignedGuards []assignedGuard
	var assignMsg string
	var autoAssignErr error
	if status == "Resolved" {
		assignedGuards, assignMsg, autoAssignErr = runAutoAssign(req.ID)
		if autoAssignErr != nil {
			log.Printf("WARNING: auto-assign on Resolved failed for query %d: %v", req.ID, autoAssignErr)
		}
	}

	// Phase 6: Notify the query owner about status change and send email
	go func() {
		var queryUserID *int
		var queryEmail, queryName string
		err := db.DB.QueryRow(
			"SELECT user_id, email, name FROM queries WHERE id = $1 AND deleted_at IS NULL",
			req.ID,
		).Scan(&queryUserID, &queryEmail, &queryName)
		if err != nil {
			log.Printf("WARNING: Could not fetch query owner for notification: %v", err)
			return
		}

		msg := fmt.Sprintf("Your request status has been updated to %s", status)

		// Phase-1: If guards were assigned, append guard names to the notification
		if len(assignedGuards) > 0 {
			names := make([]string, len(assignedGuards))
			for i, g := range assignedGuards {
				names[i] = g.GuardName
			}
			msg += fmt.Sprintf(". Assigned guards: %s", strings.Join(names, ", "))
		}

		if queryUserID != nil {
			_ = helpers.CreateNotification(db.DB, *queryUserID, msg, "info")
		}

		if strings.TrimSpace(queryEmail) != "" {
			subject := fmt.Sprintf("Service Request Update — %s", status)
			body := fmt.Sprintf("<h2>Hello %s,</h2><p>%s</p><p>Reference: #%d</p>", queryName, msg, req.ID)
			services.EnqueueEmail(queryEmail, queryName, subject, body)
		}
	}()

	// Phase-1: Return assigned guards info when status is "Resolved"
	response := map[string]interface{}{"status": "success"}
	if guardsRemoved {
		response["guardsRemoved"] = true
	}
	if len(assignedGuards) > 0 {
		response["assignedGuards"] = assignedGuards
		response["assignMessage"] = assignMsg
	} else if status == "Resolved" {
		// No guards assigned — surface the warning to the frontend
		if autoAssignErr != nil {
			response["autoAssignWarning"] = autoAssignErr.Error()
		} else if assignMsg != "" {
			response["autoAssignWarning"] = assignMsg
		}
	}
	json.NewEncoder(w).Encode(response)
}

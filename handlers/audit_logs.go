package handlers

import (
	"encoding/json"
	"net/http"
	"server/db"
	"time"
)

type AuditLog struct {
	ID        int             `json:"id"`
	UserID    *int            `json:"user_id"`
	Action    string          `json:"action"`
	Target    string          `json:"target"`
	Details   json.RawMessage `json:"details"`
	CreatedAt time.Time       `json:"created_at"`
	UserName  *string         `json:"user_name"`
	UserEmail *string         `json:"user_email"`
}

func GetAuditLogs(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT a.id, a.user_id, a.action, a.target, a.details, a.created_at, u.name, u.email
		FROM audit_logs a
		LEFT JOIN users u ON a.user_id = u.id
		ORDER BY a.created_at DESC
		LIMIT 100
	`
	rows, err := db.DB.Query(query)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve audit logs"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var detailsStr string
		if err := rows.Scan(&log.ID, &log.UserID, &log.Action, &log.Target, &detailsStr, &log.CreatedAt, &log.UserName, &log.UserEmail); err != nil {
			http.Error(w, `{"error":"Failed to scan audit log data"}`, http.StatusInternalServerError)
			return
		}
		if detailsStr != "" {
			log.Details = json.RawMessage(detailsStr)
		}
		logs = append(logs, log)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

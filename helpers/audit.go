package helpers

import (
	"database/sql"
	"encoding/json"
	"log"
)

// WriteAuditLog creates a new record in the audit_logs table.
// It should be called after a successful database write operation.
// Errors are logged but do not cause the parent request to fail.
func WriteAuditLog(db *sql.DB, userID int, action string, target string, details interface{}) {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		log.Printf("ERROR: Failed to marshal audit log details for action '%s': %v", action, err)
		return
	}

	_, err = db.Exec(
		"INSERT INTO audit_logs (user_id, action, target, details) VALUES ($1, $2, $3, $4)",
		userID,
		action,
		target,
		detailsJSON,
	)
	if err != nil {
		log.Printf("ERROR: Failed to write to audit log for action '%s': %v", action, err)
	}
}

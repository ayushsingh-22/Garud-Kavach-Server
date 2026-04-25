package helpers

import (
	"database/sql"
	"encoding/json"
)

type auditExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// WriteAuditLog creates a new record in the audit_logs table.
// It should be called after a successful database write operation.
// Errors are logged but do not cause the parent request to fail.
func WriteAuditLog(db auditExecutor, userID int, action string, target string, details interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}

	var uid interface{}
	if userID > 0 {
		uid = userID
	} else {
		uid = nil
	}

	_, err = db.Exec(
		"INSERT INTO audit_logs (user_id, action, target, details) VALUES ($1, $2, $3, $4)",
		uid,
		action,
		target,
		detailsJSON,
	)
	if err != nil {
		return err
	}

	return nil
}

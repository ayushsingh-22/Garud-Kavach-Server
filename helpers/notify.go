package helpers

import (
	"database/sql"
	"log"

	"github.com/lib/pq"
)

// CreateNotification inserts a notification for a specific user.
func CreateNotification(db *sql.DB, userID int, message, notifType string) error {
	if notifType == "" {
		notifType = "info"
	}
	_, err := db.Exec(
		"INSERT INTO notifications (user_id, message, type) VALUES ($1, $2, $3)",
		userID, message, notifType,
	)
	if err != nil {
		log.Printf("ERROR: Failed to create notification for user %d: %v", userID, err)
	}
	return err
}

// NotifyUsersByRole sends a notification to all users with the given role(s).
func NotifyUsersByRole(db *sql.DB, roles []string, message, notifType string) {
	if len(roles) == 0 {
		return
	}

	// Build query for multiple roles
	query := "SELECT id FROM users WHERE deleted_at IS NULL AND role = ANY($1)"
	rows, err := db.Query(query, pq.Array(roles))
	if err != nil {
		log.Printf("ERROR: Failed to query users by roles %v for notification: %v", roles, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var uid int
		if err := rows.Scan(&uid); err != nil {
			log.Printf("ERROR: Failed to scan user id for notification: %v", err)
			continue
		}
		_ = CreateNotification(db, uid, message, notifType)
	}
}

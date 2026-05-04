package handlers

import (
	"encoding/json"
	"net/http"
	"server/db"

	"github.com/lib/pq"
)

type Notification struct {
	ID        int    `json:"id"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

// GetNotifications returns unread notifications for the current user.
func GetNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID, ok := r.Context().Value(userIDKey).(float64)
	if !ok {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	rows, err := db.DB.Query(
		`SELECT id, message, type, read, created_at
		 FROM notifications
		 WHERE user_id = $1 AND read = FALSE
		 ORDER BY created_at DESC
		 LIMIT 50`,
		int(userID),
	)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve notifications"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	notifications := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Message, &n.Type, &n.Read, &n.CreatedAt); err != nil {
			http.Error(w, `{"error":"Failed to scan notification"}`, http.StatusInternalServerError)
			return
		}
		notifications = append(notifications, n)
	}

	json.NewEncoder(w).Encode(notifications)
}

// MarkNotificationsRead marks the given notification IDs as read for the current user.
func MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID, ok := r.Context().Value(userIDKey).(float64)
	if !ok {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	_, err := db.DB.Exec(
		"UPDATE notifications SET read = TRUE WHERE id = ANY($1) AND user_id = $2",
		pq.Array(req.IDs), int(userID),
	)
	if err != nil {
		http.Error(w, `{"error":"Failed to mark notifications as read"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

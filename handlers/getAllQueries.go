package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"server/db"
	"server/models"
)

func GetAllQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rows, err := db.DB.Query(`
		SELECT
			id,
			name,
			email,
			COALESCE(phone, ''),
			COALESCE(service, ''),
			COALESCE(message, ''),
			submitted_at,
			num_guards::text,
			COALESCE(duration_type, ''),
			COALESCE(duration_value::text, '0'),
			camera_required,
			vehicle_required,
			first_aid,
			walkie_talkie,
			bullet_proof,
			fire_safety,
			COALESCE(status, 'Pending'),
			COALESCE(cost, 0)
		FROM queries
		WHERE deleted_at IS NULL
		ORDER BY submitted_at DESC`)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unable to load queries"})
		return
	}
	defer rows.Close()

	queries := make([]models.Query, 0)
	for rows.Next() {
		var query models.Query
		var submittedAt time.Time

		err := rows.Scan(
			&query.ID,
			&query.Name,
			&query.Email,
			&query.Phone,
			&query.Service,
			&query.Message,
			&submittedAt,
			&query.NumGuards,
			&query.DurationType,
			&query.DurationValue,
			&query.CameraRequired,
			&query.VehicleRequired,
			&query.FirstAid,
			&query.WalkieTalkie,
			&query.BulletProof,
			&query.FireSafety,
			&query.Status,
			&query.Cost,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse query rows"})
			return
		}

		query.SubmittedAt = submittedAt.UTC().Format(time.RFC3339)
		queries = append(queries, query)
	}

	if err := rows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Error while reading query rows"})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(queries)
}

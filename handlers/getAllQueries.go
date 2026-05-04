package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"server/db"
	"server/models"
)

// Phase-1: response wrapper that includes assigned guards per query
type assignedGuardInfo struct {
	GuardID   int    `json:"guardId"`
	GuardName string `json:"guardName"`
}

type queryWithGuards struct {
	models.Query
	AssignedGuards []assignedGuardInfo `json:"assignedGuards"`
}

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

	queries := make([]queryWithGuards, 0)
	queryIDs := make([]int, 0)
	for rows.Next() {
		var q queryWithGuards
		var submittedAt time.Time

		err := rows.Scan(
			&q.ID,
			&q.Name,
			&q.Email,
			&q.Phone,
			&q.Service,
			&q.Message,
			&submittedAt,
			&q.NumGuards,
			&q.DurationType,
			&q.DurationValue,
			&q.CameraRequired,
			&q.VehicleRequired,
			&q.FirstAid,
			&q.WalkieTalkie,
			&q.BulletProof,
			&q.FireSafety,
			&q.Status,
			&q.Cost,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse query rows"})
			return
		}

		q.SubmittedAt = submittedAt.UTC().Format(time.RFC3339)
		q.AssignedGuards = []assignedGuardInfo{} // Phase-1: default empty slice (not null in JSON)
		queries = append(queries, q)
		queryIDs = append(queryIDs, q.ID)
	}

	if err := rows.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Error while reading query rows"})
		return
	}

	// ── Phase-1: Fetch assigned guards for all queries in one query ──────────
	if len(queryIDs) > 0 {
		guardRows, gErr := db.DB.Query(`
			SELECT s.query_id, g.id, g.name
			FROM   shifts s
			JOIN   guards g ON g.id = s.guard_id
			WHERE  s.deleted_at IS NULL
			  AND  g.deleted_at IS NULL
			ORDER  BY s.query_id, g.name`)
		if gErr == nil {
			defer guardRows.Close()
			guardMap := make(map[int][]assignedGuardInfo)
			for guardRows.Next() {
				var qid, gid int
				var gname string
				if scanErr := guardRows.Scan(&qid, &gid, &gname); scanErr == nil {
					guardMap[qid] = append(guardMap[qid], assignedGuardInfo{GuardID: gid, GuardName: gname})
				}
			}
			for i := range queries {
				if gs, ok := guardMap[queries[i].ID]; ok {
					queries[i].AssignedGuards = gs
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(queries)
}

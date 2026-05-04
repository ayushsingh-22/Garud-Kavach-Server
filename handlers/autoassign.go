package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"server/db"
	"server/helpers"

	"github.com/gorilla/mux"
)

// ─── Request / response types ─────────────────────────────────────────────────

type autoAssignResponse struct {
	AssignedGuards []assignedGuard `json:"assignedGuards"`
	Message        string          `json:"message"`
}

type assignedGuard struct {
	GuardID   int    `json:"guardId"`
	GuardName string `json:"guardName"`
	ShiftID   int    `json:"shiftId"`
}

// ─── runAutoAssign ─────────────────────────────────────────────────────────────
// Core auto-assign logic, callable from both the HTTP handler and internal
// callers (e.g. UpdateQueryStatus on "Resolved").  Phase-1 change.
//
// Algorithm:
//  1. Load the query (service_date, num_guards, duration, geofence).
//  2. Compute shift window [service_date, service_date + duration].
//  3. Find all active guards with no conflicting shift and no approved leave.
//  4. If the query has a geofence, rank candidates by distance from the last
//     known location to the geofence centre. Otherwise rank alphabetically.
//  5. Create shifts for the top N candidates and send in-app notifications.
//
// Returns (assignedGuards, message, error).
// A nil error with zero assigned guards means no candidates were found.
func runAutoAssign(queryID int) ([]assignedGuard, string, error) {
	// ── Phase-1: skip if guards already assigned (prevent double-assignment) ──
	var existingShifts int
	if err := db.DB.QueryRow(
		"SELECT COUNT(*) FROM shifts WHERE query_id = $1 AND deleted_at IS NULL", queryID,
	).Scan(&existingShifts); err == nil && existingShifts > 0 {
		return nil, "guards already assigned", nil
	}

	// ── 1. Load the query ────────────────────────────────────────────────────
	var (
		numGuards     int
		durationType  sql.NullString
		durationValue sql.NullFloat64
		serviceDate   sql.NullTime
		fenceLat      sql.NullFloat64
		fenceLng      sql.NullFloat64
		fenceRadius   sql.NullInt64
		status        string
	)
	err := db.DB.QueryRow(`
		SELECT num_guards, duration_type, duration_value,
		       service_date, geofence_lat, geofence_lng, geofence_radius_m, status
		FROM   queries
		WHERE  id = $1 AND deleted_at IS NULL`, queryID,
	).Scan(&numGuards, &durationType, &durationValue,
		&serviceDate, &fenceLat, &fenceLng, &fenceRadius, &status)

	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("query not found")
	}
	if err != nil {
		return nil, "", fmt.Errorf("db error: %w", err)
	}
	if status == "Rejected" || status == "completed" || status == "cancelled" {
		return nil, "", fmt.Errorf("query is already %s", status)
	}

	if numGuards <= 0 {
		numGuards = 1
	}

	// ── 2. Compute shift window ───────────────────────────────────────────────
	var startTime time.Time
	if serviceDate.Valid {
		startTime = serviceDate.Time
	} else {
		startTime = time.Now().UTC()
	}

	dval := 8.0
	if durationValue.Valid && durationValue.Float64 > 0 {
		dval = durationValue.Float64
	}
	var shiftDuration time.Duration
	switch durationType.String {
	case "hours":
		shiftDuration = time.Duration(dval * float64(time.Hour))
	case "days":
		shiftDuration = time.Duration(dval * 24 * float64(time.Hour))
	case "weeks":
		shiftDuration = time.Duration(dval * 7 * 24 * float64(time.Hour))
	default:
		shiftDuration = time.Duration(dval * float64(time.Hour))
	}
	endTime := startTime.Add(shiftDuration)

	// ── 3. Find available guards ──────────────────────────────────────────────
	rows, err := db.DB.Query(`
		SELECT g.id, g.name,
		       gl.lat, gl.lng
		FROM   guards g
		LEFT JOIN LATERAL (
		    SELECT lat, lng
		    FROM   guard_locations
		    WHERE  guard_id = g.id
		    ORDER  BY recorded_at DESC
		    LIMIT  1
		) gl ON true
		WHERE  g.status     = 'active'
		  AND  g.deleted_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM shifts s
		      WHERE  s.guard_id    = g.id
		        AND  s.deleted_at  IS NULL
		        AND  s.start_time  < $2
		        AND  s.end_time    > $1
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM leave_requests lr
		      WHERE  lr.guard_id   = g.id
		        AND  lr.status     = 'approved'
		        AND  lr.deleted_at IS NULL
		        AND  lr.start_date < $2
		        AND  lr.end_date   > $1
		  )
		ORDER  BY g.name ASC`,
		startTime, endTime,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query guards: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		guardID   int
		guardName string
		distM     float64
	}
	var candidates []candidate

	for rows.Next() {
		var gid int
		var gname string
		var lat, lng sql.NullFloat64
		if err := rows.Scan(&gid, &gname, &lat, &lng); err != nil {
			continue
		}
		c := candidate{guardID: gid, guardName: gname}
		if fenceLat.Valid && fenceLng.Valid && lat.Valid && lng.Valid {
			c.distM = helpers.HaversineM(lat.Float64, lng.Float64, fenceLat.Float64, fenceLng.Float64)
		}
		candidates = append(candidates, c)
	}
	rows.Close()

	if len(candidates) == 0 {
		return nil, "no available guards for this time window", nil
	}

	// ── 4. Rank candidates ────────────────────────────────────────────────────
	if fenceLat.Valid && fenceLng.Valid {
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].distM < candidates[j].distM
		})
	}
	if len(candidates) > numGuards {
		candidates = candidates[:numGuards]
	}

	// ── 5. Insert shifts and notify ───────────────────────────────────────────
	assigned := make([]assignedGuard, 0, len(candidates))
	for _, c := range candidates {
		var shiftID int
		err := db.DB.QueryRow(`
			INSERT INTO shifts (guard_id, query_id, start_time, end_time)
			VALUES ($1, $2, $3, $4)
			RETURNING id`,
			c.guardID, queryID, startTime, endTime,
		).Scan(&shiftID)
		if err != nil {
			continue
		}
		assigned = append(assigned, assignedGuard{
			GuardID:   c.guardID,
			GuardName: c.guardName,
			ShiftID:   shiftID,
		})

		var userID sql.NullInt64
		if scanErr := db.DB.QueryRow(
			`SELECT u.id FROM users u WHERE u.email = (SELECT email FROM guards WHERE id = $1) AND u.deleted_at IS NULL`,
			c.guardID,
		).Scan(&userID); scanErr == nil && userID.Valid {
			msg := fmt.Sprintf("You have been assigned a new shift from %s to %s.",
				startTime.Format("02 Jan 15:04"), endTime.Format("02 Jan 15:04"))
			_ = helpers.CreateNotification(db.DB, int(userID.Int64), msg, "shift")
		}
	}

	if len(assigned) == 0 {
		return nil, "could not create any shifts", nil
	}

	// Notify managers about the assignment
	helpers.NotifyUsersByRole(db.DB, []string{"superadmin", "manager"},
		fmt.Sprintf("%d guard(s) auto-assigned for query #%d.", len(assigned), queryID),
		"info",
	)

	// Phase-1: also notify HR users about the assignment
	helpers.NotifyUsersByRole(db.DB, []string{"hr"},
		fmt.Sprintf("%d guard(s) assigned to query #%d. Please review shift schedules.", len(assigned), queryID),
		"info",
	)

	return assigned, fmt.Sprintf("Successfully assigned %d of %d required guard(s).", len(assigned), numGuards), nil
}

// ─── AutoAssignGuards ─────────────────────────────────────────────────────────
// POST /api/queries/{id}/auto-assign
// Requires superadmin or manager role.
// Delegates to runAutoAssign for the core logic.
func AutoAssignGuards(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	queryID, err := strconv.Atoi(vars["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid query id"})
		return
	}

	assigned, msg, err := runAutoAssign(queryID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if len(assigned) == 0 {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
		return
	}

	// Auto-set status to Resolved when guards are successfully assigned
	_, _ = db.DB.Exec(`UPDATE queries SET status = 'Resolved' WHERE id = $1 AND deleted_at IS NULL`, queryID)

	_ = json.NewEncoder(w).Encode(autoAssignResponse{
		AssignedGuards: assigned,
		Message:        msg,
	})
}

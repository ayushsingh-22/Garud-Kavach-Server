package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"server/db"
	"server/helpers"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type Shift struct {
	ID            int        `json:"id"`
	GuardID       int        `json:"guard_id"`
	GuardName     *string    `json:"guard_name"`
	QueryID       *int       `json:"query_id"`
	ClientName    *string    `json:"client_name"`
	StartTime     *time.Time `json:"start_time"`
	EndTime       *time.Time `json:"end_time"`
	ActualHours   float64    `json:"actual_hours"`
	OvertimeHours float64    `json:"overtime_hours"`
	PaidHours     float64    `json:"paid_hours"`
	Status        string     `json:"status"`
}

// computeOvertimeAndPaid returns (overtimeHours, paidHours) for a given actualHours.
// Business rules (hard cap at 10 h):
//   - actual ≤  8 h → overtime = 0,                  paid = actual
//   - actual ≤ 10 h → overtime = 2 × (actual − 8),   paid = actual
//   - actual > 10 h → capped to 10 h
//
// total_pay = (paid_hours + overtime_hours) * rate
//
//	e.g. 10 h worked → paid=10, ot=4 → billable=14 → salary = 14 × rate
func computeOvertimeAndPaid(actualHours float64) (overtimeHours, paidHours float64) {
	if actualHours > 10 {
		actualHours = 10
	}
	if actualHours <= 8 {
		return 0, actualHours
	}
	ot := (actualHours - 8) * 2 // 2× multiplier on overtime hours
	return ot, actualHours      // paid_hours = actual (≤10); OT added separately in pay calc
}

type PayrollRecord struct {
	GuardID       int     `json:"guard_id"`
	GuardName     *string `json:"guard_name"`
	Month         string  `json:"month"`
	TotalHours    float64 `json:"total_hours"`
	OvertimeHours float64 `json:"overtime_hours"`
	PaidHours     float64 `json:"paid_hours"`
	RatePerHour   float64 `json:"rate_per_hour"`
	TotalPay      float64 `json:"total_pay"`
	Status        string  `json:"status"`
	ID            *int    `json:"id"`
}

type LeaveRequest struct {
	ID         int       `json:"id"`
	GuardID    int       `json:"guard_id"`
	GuardName  *string   `json:"guard_name"`
	StartDate  string    `json:"start_date"`
	EndDate    string    `json:"end_date"`
	Reason     *string   `json:"reason"`
	Status     string    `json:"status"`
	ReviewedBy *int      `json:"reviewed_by"`
	CreatedAt  time.Time `json:"created_at"`
}

func GetShifts(w http.ResponseWriter, r *http.Request) {
	guardIDFilter := r.URL.Query().Get("guard_id")
	monthFilter := r.URL.Query().Get("month") // YYYY-MM

	query := `
		SELECT s.id, s.guard_id, g.name AS guard_name, s.query_id, q.name AS client_name,
		       s.start_time, s.end_time,
		       COALESCE(s.actual_hours, 0), COALESCE(s.overtime_hours, 0), COALESCE(s.paid_hours, 0),
		       s.status
		FROM shifts s
		LEFT JOIN guards g ON s.guard_id = g.id
		LEFT JOIN queries q ON s.query_id = q.id
		WHERE s.deleted_at IS NULL
	`
	var args []interface{}
	argIdx := 1

	if guardIDFilter != "" {
		query += ` AND s.guard_id = $` + strconv.Itoa(argIdx)
		args = append(args, guardIDFilter)
		argIdx++
	}

	if monthFilter != "" {
		query += ` AND TO_CHAR(s.start_time, 'YYYY-MM') = $` + strconv.Itoa(argIdx)
		args = append(args, monthFilter)
		argIdx++
	}

	query += ` ORDER BY s.start_time DESC`

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve shifts"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var shifts []Shift
	for rows.Next() {
		var s Shift
		if err := rows.Scan(&s.ID, &s.GuardID, &s.GuardName, &s.QueryID, &s.ClientName, &s.StartTime, &s.EndTime, &s.ActualHours, &s.OvertimeHours, &s.PaidHours, &s.Status); err != nil {
			http.Error(w, `{"error":"Failed to scan shift data"}`, http.StatusInternalServerError)
			return
		}
		shifts = append(shifts, s)
	}
	if shifts == nil {
		shifts = []Shift{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(shifts)
}

func CreateShift(w http.ResponseWriter, r *http.Request) {
	var req Shift
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.GuardID <= 0 {
		http.Error(w, `{"error":"Valid guard_id is required"}`, http.StatusBadRequest)
		return
	}
	if req.QueryID == nil || *req.QueryID <= 0 {
		http.Error(w, `{"error":"Valid query_id is required"}`, http.StatusBadRequest)
		return
	}
	if req.StartTime == nil || req.EndTime == nil {
		http.Error(w, `{"error":"start_time and end_time are required"}`, http.StatusBadRequest)
		return
	}
	if !req.EndTime.After(*req.StartTime) {
		http.Error(w, `{"error":"end_time must be after start_time"}`, http.StatusBadRequest)
		return
	}
	if req.EndTime.Sub(*req.StartTime) > 10*time.Hour {
		http.Error(w, `{"error":"Shift duration cannot exceed 10 hours"}`, http.StatusBadRequest)
		return
	}

	err := db.DB.QueryRow(`
		INSERT INTO shifts (guard_id, query_id, start_time, end_time, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, req.GuardID, req.QueryID, req.StartTime, req.EndTime, "scheduled").Scan(&req.ID)

	if err != nil {
		http.Error(w, `{"error":"Failed to create shift"}`, http.StatusInternalServerError)
		return
	}

	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "create_shift", "shift:"+strconv.Itoa(req.ID), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_shift: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req)
}

func UpdateShift(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid shift ID"}`, http.StatusBadRequest)
		return
	}

	var req Shift
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Status != "" && !helpers.ValidateStatus(req.Status, []string{"scheduled", "in_progress", "completed", "cancelled"}) {
		http.Error(w, `{"error":"Invalid shift status"}`, http.StatusBadRequest)
		return
	}
	if req.ActualHours < 0 {
		http.Error(w, `{"error":"Actual hours cannot be negative"}`, http.StatusBadRequest)
		return
	}
	if req.ActualHours > 10 {
		http.Error(w, `{"error":"Shift cannot exceed 10 hours"}`, http.StatusBadRequest)
		return
	}
	if req.StartTime != nil && req.EndTime != nil {
		if !req.EndTime.After(*req.StartTime) {
			http.Error(w, `{"error":"end_time must be after start_time"}`, http.StatusBadRequest)
			return
		}
		if req.EndTime.Sub(*req.StartTime) > 10*time.Hour {
			http.Error(w, `{"error":"Shift duration cannot exceed 10 hours"}`, http.StatusBadRequest)
			return
		}
	}

	req.OvertimeHours, req.PaidHours = computeOvertimeAndPaid(req.ActualHours)

	result, err := db.DB.Exec(`
		UPDATE shifts
		SET start_time = $1, end_time = $2, actual_hours = $3,
		    overtime_hours = $4, paid_hours = $5, status = $6
		WHERE id = $7 AND deleted_at IS NULL
	`, req.StartTime, req.EndTime, req.ActualHours, req.OvertimeHours, req.PaidHours, req.Status, id)

	if err != nil {
		http.Error(w, `{"error":"Failed to update shift"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Shift not found"}`, http.StatusNotFound)
		return
	}

	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "update_shift", "shift:"+strconv.Itoa(id), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for update_shift: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Shift updated successfully"})
}

func GetPayroll(w http.ResponseWriter, r *http.Request) {
	monthFilter := r.URL.Query().Get("month") // YYYY-MM
	if monthFilter == "" {
		http.Error(w, `{"error":"month parameter is required"}`, http.StatusBadRequest)
		return
	}

	// Try to get from payroll table first
	query := `
		SELECT p.id, p.guard_id, g.name, TO_CHAR(p.month, 'YYYY-MM'),
		       p.total_hours, COALESCE(p.overtime_hours,0), COALESCE(p.paid_hours,0),
		       p.rate_per_hour, p.total_pay, p.status
		FROM payroll p
		LEFT JOIN guards g ON p.guard_id = g.id
		WHERE TO_CHAR(p.month, 'YYYY-MM') = $1 AND p.deleted_at IS NULL
	`
	rows, err := db.DB.Query(query, monthFilter)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve payroll"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var payroll []PayrollRecord
	for rows.Next() {
		var p PayrollRecord
		if err := rows.Scan(&p.ID, &p.GuardID, &p.GuardName, &p.Month, &p.TotalHours, &p.OvertimeHours, &p.PaidHours, &p.RatePerHour, &p.TotalPay, &p.Status); err != nil {
			http.Error(w, `{"error":"Failed to scan payroll data"}`, http.StatusInternalServerError)
			return
		}
		payroll = append(payroll, p)
	}

	// If no payroll records for month, calculate from shifts
	if len(payroll) == 0 {
		calcQuery := `
			SELECT s.guard_id, g.name,
			       SUM(COALESCE(s.actual_hours, 0)),
			       SUM(COALESCE(s.overtime_hours, 0)),
			       SUM(COALESCE(s.paid_hours, COALESCE(s.actual_hours, 0))),
			       g.hourly_rate
			FROM shifts s
			JOIN guards g ON s.guard_id = g.id AND g.deleted_at IS NULL
			WHERE TO_CHAR(s.start_time, 'YYYY-MM') = $1
			  AND s.deleted_at IS NULL
			GROUP BY s.guard_id, g.name, g.hourly_rate
		`
		calcRows, err := db.DB.Query(calcQuery, monthFilter)
		if err != nil {
			http.Error(w, `{"error":"Failed to calculate payroll"}`, http.StatusInternalServerError)
			return
		}
		defer calcRows.Close()

		for calcRows.Next() {
			var p PayrollRecord
			p.Month = monthFilter
			p.Status = "pending"
			if err := calcRows.Scan(&p.GuardID, &p.GuardName, &p.TotalHours, &p.OvertimeHours, &p.PaidHours, &p.RatePerHour); err != nil {
				http.Error(w, `{"error":"Failed to scan calculated payroll"}`, http.StatusInternalServerError)
				return
			}
			p.TotalPay = (p.PaidHours + p.OvertimeHours) * p.RatePerHour
			payroll = append(payroll, p)
		}
	}

	if payroll == nil {
		payroll = []PayrollRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payroll)
}

func FinalizePayroll(w http.ResponseWriter, r *http.Request) {
	monthFilter := r.URL.Query().Get("month") // YYYY-MM
	if monthFilter == "" {
		http.Error(w, `{"error":"month parameter is required"}`, http.StatusBadRequest)
		return
	}

	// Check if already finalized
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*) FROM payroll WHERE TO_CHAR(month, 'YYYY-MM') = $1 AND deleted_at IS NULL
	`, monthFilter).Scan(&count)

	if err != nil {
		http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, `{"error":"Payroll already finalized for this month"}`, http.StatusConflict)
		return
	}

	monthDate, err := time.Parse("2006-01-02", monthFilter+"-01")
	if err != nil {
		http.Error(w, `{"error":"Invalid month format"}`, http.StatusBadRequest)
		return
	}

	// total_pay = (paid_hours + overtime_hours) * rate
	// paid_hours = actual_hours (capped at 10); overtime = 2×(actual-8) for shifts > 8h
	query := `
		INSERT INTO payroll (guard_id, month, total_hours, overtime_hours, paid_hours, rate_per_hour, total_pay, status)
		SELECT
		    s.guard_id,
		    $1,
		    SUM(COALESCE(s.actual_hours,   0)),
		    SUM(COALESCE(s.overtime_hours, 0)),
		    SUM(COALESCE(s.paid_hours,     COALESCE(s.actual_hours, 0))),
		    g.hourly_rate,
		    (SUM(COALESCE(s.paid_hours, COALESCE(s.actual_hours, 0))) + SUM(COALESCE(s.overtime_hours, 0))) * g.hourly_rate,
		    'pending'
		FROM shifts s
		JOIN guards g ON s.guard_id = g.id AND g.deleted_at IS NULL
		WHERE TO_CHAR(s.start_time, 'YYYY-MM') = $2
		  AND s.deleted_at IS NULL
		GROUP BY s.guard_id, g.hourly_rate
	`
	result, err := db.DB.Exec(query, monthDate, monthFilter)
	if err != nil {
		log.Printf("ERROR FinalizePayroll: %v", err)
		http.Error(w, `{"error":"Failed to finalize payroll"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()

	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "finalize_payroll", "month:"+monthFilter, map[string]int64{"records_created": rowsAffected}); err != nil {
		log.Printf("ERROR: Failed to write audit log for finalize_payroll: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Payroll finalized", "records_created": rowsAffected})
}

func GetLeaves(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT l.id, l.guard_id, g.name, TO_CHAR(l.start_date, 'YYYY-MM-DD'), TO_CHAR(l.end_date, 'YYYY-MM-DD'), l.reason, l.status, l.reviewed_by, l.created_at
		FROM leave_requests l
		LEFT JOIN guards g ON l.guard_id = g.id
		WHERE l.deleted_at IS NULL
		ORDER BY l.created_at DESC
	`
	rows, err := db.DB.Query(query)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve leave requests"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var leaves []LeaveRequest
	for rows.Next() {
		var l LeaveRequest
		if err := rows.Scan(&l.ID, &l.GuardID, &l.GuardName, &l.StartDate, &l.EndDate, &l.Reason, &l.Status, &l.ReviewedBy, &l.CreatedAt); err != nil {
			http.Error(w, `{"error":"Failed to scan leave data"}`, http.StatusInternalServerError)
			return
		}
		leaves = append(leaves, l)
	}

	if leaves == nil {
		leaves = []LeaveRequest{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(leaves)
}

func UpdateLeaveStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid leave ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if !helpers.ValidateStatus(req.Status, []string{"approved", "rejected", "pending"}) {
		http.Error(w, `{"error":"Invalid status"}`, http.StatusBadRequest)
		return
	}

	currentUserID, _ := r.Context().Value(userIDKey).(float64)

	result, err := db.DB.Exec(`
		UPDATE leave_requests 
		SET status = $1, reviewed_by = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, req.Status, int(currentUserID), id)

	if err != nil {
		http.Error(w, `{"error":"Failed to update leave status"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Leave request not found"}`, http.StatusNotFound)
		return
	}

	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "update_leave", "leave:"+strconv.Itoa(id), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for update_leave: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Leave status updated"})
}

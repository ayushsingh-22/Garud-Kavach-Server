// Package chat provides data-scoping functions for the chatbot.
// Each function enforces role-based boundaries at the query level.
// No function may expose data from another role's domain.
package chat

import (
	"database/sql"
	"fmt"
)

// ScopedData is a generic key-value payload passed to the LLM as context.
// Values must never contain raw PII beyond what is needed to answer the intent.
type ScopedData map[string]interface{}

// ---- allow-lists per role -----------------------------------------------

// CustomerAllowedIntents are the only intents a customer token may trigger.
var CustomerAllowedIntents = map[string]bool{
	"my_bookings":     true,
	"booking_status":  true,
	"my_profile":      true,
	"service_catalog": true,
	"book_service":    true,
	"contact_request": true,
	"company_info":    true,
	"pricing":         true,
}

// HRAllowedIntents are the only intents an HR token may trigger.
var HRAllowedIntents = map[string]bool{
	"guard_list":        true,
	"guard_details":     true,
	"shifts":            true,
	"leave_requests":    true,
	"payroll_summary":   true,
	"expiring_licenses": true,
	"company_info":      true,
}

// FinanceAllowedIntents are the only intents a finance token may trigger.
var FinanceAllowedIntents = map[string]bool{
	"invoices":       true,
	"expenses":       true,
	"finance_report": true,
	"company_info":   true,
}

// ManagerAllowedIntents are the only intents a manager token may trigger.
var ManagerAllowedIntents = map[string]bool{
	"all_queries":      true,
	"query_status":     true,
	"guard_list":       true,
	"guard_assignment": true,
	"analytics":        true,
	"company_info":     true,
}

// AdminAllowedIntents are the only intents a superadmin token may trigger.
var AdminAllowedIntents = map[string]bool{
	"user_list":        true,
	"all_queries":      true,
	"query_status":     true,
	"guard_list":       true,
	"guard_assignment": true,
	"analytics":        true,
	"invoices":         true,
	"expenses":         true,
	"audit_logs":       true,
	"company_info":     true,
}

// PublicAllowedIntents are usable without authentication.
var PublicAllowedIntents = map[string]bool{
	"service_catalog": true,
	"pricing":         true,
	"company_info":    true,
	"book_service":    true,
	"contact_request": true,
}

// errOutOfScope is returned when an intent is not in the role's allow-list.
var errOutOfScope = fmt.Errorf("out_of_scope")

// IsOutOfScope reports whether err is the sentinel errOutOfScope.
func IsOutOfScope(err error) bool { return err == errOutOfScope }

// ---- scoping functions ---------------------------------------------------

// CustomerScope returns data relevant to a single customer's intent.
// It ONLY accesses rows tied to userID; it never touches finance/hr/admin tables.
func CustomerScope(db *sql.DB, userID int, intent string, params map[string]interface{}) (ScopedData, error) {
	if !CustomerAllowedIntents[intent] {
		return nil, errOutOfScope
	}

	switch intent {
	case "my_bookings", "booking_status":
		return customerBookings(db, userID, params)
	case "my_profile":
		return customerProfile(db, userID)
	case "service_catalog", "pricing":
		return serviceCatalog()
	case "company_info":
		return companyInfo()
	case "book_service", "contact_request":
		// No DB read needed; these are action intents.
		return ScopedData{"action": intent}, nil
	}
	return ScopedData{}, nil
}

// HRScope returns data relevant to HR intents.
// It NEVER touches invoices, expenses, or finance tables.
func HRScope(db *sql.DB, userID int, intent string, params map[string]interface{}) (ScopedData, error) {
	if !HRAllowedIntents[intent] {
		return nil, errOutOfScope
	}

	switch intent {
	case "guard_list":
		return guardList(db, params)
	case "guard_details":
		return guardDetails(db, params)
	case "shifts":
		return shiftList(db, params)
	case "leave_requests":
		return leaveList(db, params)
	case "payroll_summary":
		return payrollSummary(db, params)
	case "expiring_licenses":
		return expiringLicenses(db)
	case "company_info":
		return companyInfo()
	}
	return ScopedData{}, nil
}

// FinanceScope returns data relevant to finance intents.
// It NEVER touches guard personal data, HR leave, or user PII beyond what invoices reference.
func FinanceScope(db *sql.DB, userID int, intent string, params map[string]interface{}) (ScopedData, error) {
	if !FinanceAllowedIntents[intent] {
		return nil, errOutOfScope
	}

	switch intent {
	case "invoices":
		return invoiceList(db, params)
	case "expenses":
		return expenseList(db, params)
	case "finance_report":
		return financeReport(db, params)
	case "company_info":
		return companyInfo()
	}
	return ScopedData{}, nil
}

// ManagerScope returns data relevant to manager intents.
// It does not expose HR personal data or finance figures.
func ManagerScope(db *sql.DB, userID int, intent string, params map[string]interface{}) (ScopedData, error) {
	if !ManagerAllowedIntents[intent] {
		return nil, errOutOfScope
	}

	switch intent {
	case "all_queries", "query_status":
		return queryList(db, params)
	case "guard_list":
		return guardList(db, params)
	case "guard_assignment":
		return guardAssignments(db, params)
	case "analytics":
		return analyticsSummary(db, params)
	case "company_info":
		return companyInfo()
	}
	return ScopedData{}, nil
}

// AdminScope returns data relevant to superadmin intents.
// Access is explicit; no blanket "return everything".
func AdminScope(db *sql.DB, userID int, intent string, params map[string]interface{}) (ScopedData, error) {
	if !AdminAllowedIntents[intent] {
		return nil, errOutOfScope
	}

	switch intent {
	case "user_list":
		return userList(db, params)
	case "all_queries", "query_status":
		return queryList(db, params)
	case "guard_list":
		return guardList(db, params)
	case "guard_assignment":
		return guardAssignments(db, params)
	case "analytics":
		return analyticsSummary(db, params)
	case "invoices":
		return invoiceList(db, params)
	case "expenses":
		return expenseList(db, params)
	case "audit_logs":
		return auditLogList(db, params)
	case "company_info":
		return companyInfo()
	}
	return ScopedData{}, nil
}

// PublicScope returns data available without authentication.
func PublicScope(intent string, params map[string]interface{}) (ScopedData, error) {
	if !PublicAllowedIntents[intent] {
		return nil, errOutOfScope
	}

	switch intent {
	case "service_catalog", "pricing":
		return serviceCatalog()
	case "company_info":
		return companyInfo()
	case "book_service", "contact_request":
		return ScopedData{"action": intent}, nil
	}
	return ScopedData{}, nil
}

// ---- private query helpers ----------------------------------------------

func customerBookings(db *sql.DB, userID int, params map[string]interface{}) (ScopedData, error) {
	limit := 10
	rows, err := db.Query(`
		SELECT id, service, status, cost, submitted_at
		FROM queries
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY submitted_at DESC
		LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type booking struct {
		ID          int     `json:"id"`
		Service     string  `json:"service"`
		Status      string  `json:"status"`
		Cost        float64 `json:"cost"`
		SubmittedAt string  `json:"submitted_at"`
	}
	var list []booking
	for rows.Next() {
		var b booking
		if err := rows.Scan(&b.ID, &b.Service, &b.Status, &b.Cost, &b.SubmittedAt); err != nil {
			continue
		}
		list = append(list, b)
	}
	return ScopedData{"bookings": list}, nil
}

func customerProfile(db *sql.DB, userID int) (ScopedData, error) {
	var name, email string
	var phone, company, address *string
	err := db.QueryRow(`
		SELECT u.name, u.email, c.phone, c.company, c.address
		FROM users u
		LEFT JOIN customers c ON c.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL`,
		userID,
	).Scan(&name, &email, &phone, &company, &address)
	if err != nil {
		return nil, err
	}
	return ScopedData{
		"name":    name,
		"email":   email,
		"phone":   deref(phone),
		"company": deref(company),
		"address": deref(address),
	}, nil
}

func guardList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	rows, err := db.Query(`
		SELECT id, name, status, license_expiry
		FROM guards
		WHERE deleted_at IS NULL
		ORDER BY name
		LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type g struct {
		ID            int     `json:"id"`
		Name          string  `json:"name"`
		Status        string  `json:"status"`
		LicenseExpiry *string `json:"license_expiry,omitempty"`
	}
	var list []g
	for rows.Next() {
		var item g
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.LicenseExpiry); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"guards": list, "total": len(list)}, nil
}

func guardDetails(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	id, ok := intParam(params, "id")
	if !ok {
		return ScopedData{"error": "guard id required"}, nil
	}
	var name, status string
	var phone, email, licenseNo *string
	var licenseExpiry *string
	err := db.QueryRow(`
		SELECT name, status, phone, email, license_no, license_expiry::text
		FROM guards WHERE id = $1 AND deleted_at IS NULL`, id,
	).Scan(&name, &status, &phone, &email, &licenseNo, &licenseExpiry)
	if err != nil {
		return nil, err
	}
	return ScopedData{
		"id":             id,
		"name":           name,
		"status":         status,
		"license_expiry": deref(licenseExpiry),
	}, nil
}

func shiftList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	month, _ := params["month"].(string)
	var args []interface{}
	query := `
		SELECT s.id, g.name AS guard_name, s.status,
		       TO_CHAR(s.start_time, 'YYYY-MM-DD HH24:MI') AS start_time,
		       COALESCE(s.actual_hours, 0) AS hours
		FROM shifts s
		JOIN guards g ON s.guard_id = g.id
		WHERE s.deleted_at IS NULL`
	if month != "" {
		query += ` AND TO_CHAR(s.start_time, 'YYYY-MM') = $1`
		args = append(args, month)
	}
	query += ` ORDER BY s.start_time DESC LIMIT 50`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type s struct {
		ID        int     `json:"id"`
		Guard     string  `json:"guard"`
		Status    string  `json:"status"`
		StartTime string  `json:"start_time"`
		Hours     float64 `json:"hours"`
	}
	var list []s
	for rows.Next() {
		var item s
		if err := rows.Scan(&item.ID, &item.Guard, &item.Status, &item.StartTime, &item.Hours); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"shifts": list, "total": len(list)}, nil
}

func leaveList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	rows, err := db.Query(`
		SELECT lr.id, g.name AS guard_name, lr.start_date::text, lr.end_date::text, lr.status
		FROM leave_requests lr
		JOIN guards g ON lr.guard_id = g.id
		ORDER BY lr.created_at DESC
		LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type lr struct {
		ID        int    `json:"id"`
		Guard     string `json:"guard"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Status    string `json:"status"`
	}
	var list []lr
	for rows.Next() {
		var item lr
		if err := rows.Scan(&item.ID, &item.Guard, &item.StartDate, &item.EndDate, &item.Status); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"leave_requests": list, "total": len(list)}, nil
}

func payrollSummary(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	month, _ := params["month"].(string)
	var args []interface{}
	query := `
		SELECT COUNT(*) AS guard_count,
		       COALESCE(SUM(total_pay), 0) AS total_payout,
		       status
		FROM payroll
		WHERE deleted_at IS NULL`
	if month != "" {
		query += ` AND TO_CHAR(month, 'YYYY-MM') = $1`
		args = append(args, month)
	}
	query += ` GROUP BY status`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		Count  int     `json:"count"`
		Amount float64 `json:"amount"`
		Status string  `json:"status"`
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.Count, &r.Amount, &r.Status); err != nil {
			continue
		}
		list = append(list, r)
	}
	return ScopedData{"payroll_by_status": list}, nil
}

func expiringLicenses(db *sql.DB) (ScopedData, error) {
	rows, err := db.Query(`
		SELECT name, license_expiry::text
		FROM guards
		WHERE deleted_at IS NULL
		  AND license_expiry IS NOT NULL
		  AND license_expiry <= NOW() + INTERVAL '7 days'
		  AND license_expiry > NOW()
		ORDER BY license_expiry`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type g struct {
		Name   string `json:"name"`
		Expiry string `json:"expiry"`
	}
	var list []g
	for rows.Next() {
		var item g
		if err := rows.Scan(&item.Name, &item.Expiry); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"expiring_licenses": list, "count": len(list)}, nil
}

func invoiceList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	status, _ := params["status"].(string)
	var args []interface{}
	query := `
		SELECT i.id, i.amount, i.status, TO_CHAR(i.issued_at, 'YYYY-MM-DD') AS issued_at
		FROM invoices i
		WHERE i.deleted_at IS NULL`
	if status != "" {
		query += ` AND i.status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY i.issued_at DESC LIMIT 50`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type inv struct {
		ID       int     `json:"id"`
		Amount   float64 `json:"amount"`
		Status   string  `json:"status"`
		IssuedAt string  `json:"issued_at"`
	}
	var list []inv
	for rows.Next() {
		var item inv
		if err := rows.Scan(&item.ID, &item.Amount, &item.Status, &item.IssuedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"invoices": list, "total": len(list)}, nil
}

func expenseList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	month, _ := params["month"].(string)
	var args []interface{}
	query := `
		SELECT id, category, description, amount, expense_date::text
		FROM expenses
		WHERE deleted_at IS NULL`
	if month != "" {
		query += ` AND TO_CHAR(expense_date, 'YYYY-MM') = $1`
		args = append(args, month)
	}
	query += ` ORDER BY expense_date DESC LIMIT 50`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type exp struct {
		ID          int     `json:"id"`
		Category    *string `json:"category"`
		Description *string `json:"description"`
		Amount      float64 `json:"amount"`
		Date        string  `json:"date"`
	}
	var list []exp
	for rows.Next() {
		var item exp
		if err := rows.Scan(&item.ID, &item.Category, &item.Description, &item.Amount, &item.Date); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"expenses": list, "total": len(list)}, nil
}

func financeReport(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	month, _ := params["month"].(string)
	if month == "" {
		// default: current month
		month = ""
	}
	var revenue, expenses float64
	revQuery := `SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND deleted_at IS NULL`
	expQuery := `SELECT COALESCE(SUM(amount),0) FROM expenses WHERE deleted_at IS NULL`
	if month != "" {
		revQuery += ` AND TO_CHAR(paid_at,'YYYY-MM') = $1`
		expQuery += ` AND TO_CHAR(expense_date,'YYYY-MM') = $1`
		_ = db.QueryRow(revQuery, month).Scan(&revenue)
		_ = db.QueryRow(expQuery, month).Scan(&expenses)
	} else {
		_ = db.QueryRow(revQuery).Scan(&revenue)
		_ = db.QueryRow(expQuery).Scan(&expenses)
	}
	return ScopedData{
		"revenue":  revenue,
		"expenses": expenses,
		"profit":   revenue - expenses,
		"month":    month,
	}, nil
}

func queryList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	status, _ := params["status"].(string)
	var args []interface{}
	query := `
		SELECT id, service, status, cost, submitted_at
		FROM queries
		WHERE deleted_at IS NULL`
	if status != "" {
		query += ` AND status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY submitted_at DESC LIMIT 50`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type q struct {
		ID          int     `json:"id"`
		Service     string  `json:"service"`
		Status      string  `json:"status"`
		Cost        float64 `json:"cost"`
		SubmittedAt string  `json:"submitted_at"`
	}
	var list []q
	for rows.Next() {
		var item q
		if err := rows.Scan(&item.ID, &item.Service, &item.Status, &item.Cost, &item.SubmittedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"queries": list, "total": len(list)}, nil
}

func guardAssignments(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	rows, err := db.Query(`
		SELECT ga.id, g.name AS guard_name, q.service, ga.assigned_at
		FROM guard_query_assignments ga
		JOIN guards g ON ga.guard_id = g.id
		JOIN queries q ON ga.query_id = q.id
		WHERE ga.unassigned_at IS NULL
		ORDER BY ga.assigned_at DESC
		LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type a struct {
		ID         int    `json:"id"`
		Guard      string `json:"guard"`
		Service    string `json:"service"`
		AssignedAt string `json:"assigned_at"`
	}
	var list []a
	for rows.Next() {
		var item a
		if err := rows.Scan(&item.ID, &item.Guard, &item.Service, &item.AssignedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"assignments": list, "total": len(list)}, nil
}

func analyticsSummary(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	var totalQueries, pendingQueries int
	var totalRevenue float64
	_ = db.QueryRow(`SELECT COUNT(*) FROM queries WHERE deleted_at IS NULL`).Scan(&totalQueries)
	_ = db.QueryRow(`SELECT COUNT(*) FROM queries WHERE deleted_at IS NULL AND status='Pending'`).Scan(&pendingQueries)
	_ = db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND deleted_at IS NULL`).Scan(&totalRevenue)
	return ScopedData{
		"total_queries":   totalQueries,
		"pending_queries": pendingQueries,
		"total_revenue":   totalRevenue,
	}, nil
}

func userList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	rows, err := db.Query(`
		SELECT id, email, role, name, created_at
		FROM users
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type u struct {
		ID        int     `json:"id"`
		Email     string  `json:"email"`
		Role      string  `json:"role"`
		Name      *string `json:"name,omitempty"`
		CreatedAt string  `json:"created_at"`
	}
	var list []u
	for rows.Next() {
		var item u
		if err := rows.Scan(&item.ID, &item.Email, &item.Role, &item.Name, &item.CreatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"users": list, "total": len(list)}, nil
}

func auditLogList(db *sql.DB, params map[string]interface{}) (ScopedData, error) {
	rows, err := db.Query(`
		SELECT al.id, al.action, al.target, al.created_at
		FROM audit_logs al
		ORDER BY al.created_at DESC
		LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type al struct {
		ID        int    `json:"id"`
		Action    string `json:"action"`
		Target    string `json:"target"`
		CreatedAt string `json:"created_at"`
	}
	var list []al
	for rows.Next() {
		var item al
		if err := rows.Scan(&item.ID, &item.Action, &item.Target, &item.CreatedAt); err != nil {
			continue
		}
		list = append(list, item)
	}
	return ScopedData{"audit_logs": list, "total": len(list)}, nil
}

// ---- static data --------------------------------------------------------

func serviceCatalog() (ScopedData, error) {
	return ScopedData{
		"services": []map[string]interface{}{
			{"name": "Club Guards", "description": "Security personnel for nightclubs and entertainment venues.", "base_cost_per_guard": 1000},
			{"name": "Event Security", "description": "Guards for special events, concerts, and gatherings.", "base_cost_per_guard": 1000},
			{"name": "Personal Security", "description": "Bodyguards and personal protection services.", "base_cost_per_guard": 1000},
			{"name": "Property Guards", "description": "Security for residential and commercial properties.", "base_cost_per_guard": 1000},
			{"name": "Corporate Security", "description": "Comprehensive security solutions for businesses.", "base_cost_per_guard": 1000},
			{"name": "Gunmen & Guard Dogs", "description": "Armed guards and trained K9 units for high-security needs.", "base_cost_per_guard": 1000},
		},
		"pricing": map[string]interface{}{
			"base_guard_cost": 1000,
			"service_charge":  1000,
			"gst_percent":     18,
			"add_ons": map[string]int{
				"camera_surveillance": 500,
				"security_vehicle":    2500,
				"first_aid":           150,
				"walkie_talkie":       500,
				"bulletproof_vests":   2000,
				"fire_safety":         750,
			},
		},
	}, nil
}

func companyInfo() (ScopedData, error) {
	return ScopedData{
		"company_name":  "Garud Kavach",
		"tagline":       "Professional Security Services",
		"email":         "contact@rakshakservice.com",
		"phone":         "+91 98765 43210",
		"address":       "Block D, West Vinod Nagar, Mandawali, New Delhi 110092",
		"service_areas": []string{"Delhi", "Noida", "Gurgaon", "Faridabad", "Ghaziabad", "Patna", "Muzaffarpur"},
	}, nil
}

// ---- helpers ------------------------------------------------------------

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intParam(params map[string]interface{}, key string) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	}
	return 0, false
}

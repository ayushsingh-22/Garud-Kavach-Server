package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"server/db"
	"server/helpers"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type Invoice struct {
	ID         int        `json:"id"`
	QueryID    *int       `json:"query_id"`
	ClientName *string    `json:"client_name"`
	Service    *string    `json:"service"`
	Amount     float64    `json:"amount"`
	Status     string     `json:"status"`
	IssuedAt   time.Time  `json:"issued_at"`
	PaidAt     *time.Time `json:"paid_at"`
	PaymentRef *string    `json:"payment_ref"`
}

type Expense struct {
	ID          int       `json:"id"`
	Category    *string   `json:"category"`
	Description *string   `json:"description"`
	Amount      float64   `json:"amount"`
	ExpenseDate string    `json:"expense_date"`
	AddedBy     *int      `json:"added_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func GetInvoices(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	query := `
		SELECT i.id, i.query_id, q.name AS client_name, q.service, i.amount, i.status, i.issued_at, i.paid_at, i.payment_ref
		FROM invoices i
		LEFT JOIN queries q ON i.query_id = q.id
		WHERE i.deleted_at IS NULL
	`
	var args []interface{}
	argIdx := 1

	if statusFilter != "" {
		query += ` AND i.status = $` + strconv.Itoa(argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	query += ` ORDER BY i.issued_at DESC`

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve invoices"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var invoices []Invoice
	for rows.Next() {
		var i Invoice
		if err := rows.Scan(&i.ID, &i.QueryID, &i.ClientName, &i.Service, &i.Amount, &i.Status, &i.IssuedAt, &i.PaidAt, &i.PaymentRef); err != nil {
			http.Error(w, `{"error":"Failed to scan invoice data"}`, http.StatusInternalServerError)
			return
		}
		invoices = append(invoices, i)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invoices)
}

func GetFinanceReports(w http.ResponseWriter, r *http.Request) {
	monthFilter := r.URL.Query().Get("month") // Expected format YYYY-MM
	if monthFilter == "" {
		http.Error(w, `{"error":"month parameter is required"}`, http.StatusBadRequest)
		return
	}

	// Calculate revenue
	var revenue float64
	err := db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM invoices
		WHERE status = 'paid' 
		  AND TO_CHAR(paid_at, 'YYYY-MM') = $1
		  AND deleted_at IS NULL
	`, monthFilter).Scan(&revenue)
	if err != nil {
		http.Error(w, `{"error":"Failed to calculate revenue"}`, http.StatusInternalServerError)
		return
	}

	// Calculate expenses
	var expenses float64
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM expenses
		WHERE TO_CHAR(expense_date, 'YYYY-MM') = $1
		  AND deleted_at IS NULL
	`, monthFilter).Scan(&expenses)
	if err != nil {
		http.Error(w, `{"error":"Failed to calculate expenses"}`, http.StatusInternalServerError)
		return
	}

	profit := revenue - expenses

	report := map[string]interface{}{
		"month":    monthFilter,
		"revenue":  revenue,
		"expenses": expenses,
		"profit":   profit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func CreateExpense(w http.ResponseWriter, r *http.Request) {
	var req Expense
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Amount <= 0 {
		http.Error(w, `{"error":"Amount must be greater than 0"}`, http.StatusBadRequest)
		return
	}

	if req.Category != nil {
		trimmed := strings.TrimSpace(*req.Category)
		if len(trimmed) > 200 {
			http.Error(w, `{"error":"Category must not exceed 200 characters"}`, http.StatusBadRequest)
			return
		}
		if trimmed == "" {
			req.Category = nil
		} else {
			req.Category = &trimmed
		}
	}

	if req.Description != nil {
		trimmed := strings.TrimSpace(*req.Description)
		if len(trimmed) > 1000 {
			http.Error(w, `{"error":"Description must not exceed 1000 characters"}`, http.StatusBadRequest)
			return
		}
		if trimmed == "" {
			req.Description = nil
		} else {
			req.Description = &trimmed
		}
	}

	expDate, err := time.Parse("2006-01-02", req.ExpenseDate)
	if err != nil {
		http.Error(w, `{"error":"Invalid expense date"}`, http.StatusBadRequest)
		return
	}

	if expDate.After(time.Now()) {
		http.Error(w, `{"error":"Expense date cannot be in the future"}`, http.StatusBadRequest)
		return
	}

	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	addedBy := int(currentUserID)

	err = db.DB.QueryRow(`
		INSERT INTO expenses (category, description, amount, expense_date, added_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, req.Category, req.Description, req.Amount, req.ExpenseDate, addedBy).Scan(&req.ID, &req.CreatedAt)

	if err != nil {
		http.Error(w, `{"error":"Failed to create expense"}`, http.StatusInternalServerError)
		return
	}

	if err := helpers.WriteAuditLog(db.DB, addedBy, "create_expense", "expense:"+strconv.Itoa(req.ID), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_expense: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(req)
}

func UpdateInvoiceStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid invoice ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if !helpers.ValidateStatus(req.Status, []string{"pending", "paid", "refunded"}) {
		http.Error(w, `{"error":"Invalid status"}`, http.StatusBadRequest)
		return
	}

	var query string
	var args []interface{}

	if req.Status == "paid" {
		query = "UPDATE invoices SET status = $1, paid_at = NOW() WHERE id = $2 AND deleted_at IS NULL"
		args = []interface{}{req.Status, id}
	} else {
		query = "UPDATE invoices SET status = $1 WHERE id = $2 AND deleted_at IS NULL"
		args = []interface{}{req.Status, id}
	}

	result, err := db.DB.Exec(query, args...)
	if err != nil {
		http.Error(w, `{"error":"Failed to update invoice"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Invoice not found"}`, http.StatusNotFound)
		return
	}

	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "update_invoice_status", "invoice:"+strconv.Itoa(id), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for update_invoice_status: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Invoice updated successfully"})
}

// GetFinanceOverview returns a comprehensive finance summary for the superadmin dashboard.
// All independent DB queries run concurrently via goroutines.
// Date range conditions are used instead of TO_CHAR() so indexes are utilised.
func GetFinanceOverview(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	// ── Read filter query params ────────────────────────────────────────
	qMonth := r.URL.Query().Get("month") // YYYY-MM, defaults to current
	if qMonth == "" {
		qMonth = now.Format("2006-01")
	}
	refDate, parseErr := time.Parse("2006-01", qMonth)
	if parseErr != nil {
		qMonth = now.Format("2006-01")
		refDate = now
	}

	trendMonths := 6
	if tm := r.URL.Query().Get("trend_months"); tm != "" {
		if v, e := strconv.Atoi(tm); e == nil && (v == 3 || v == 6 || v == 12) {
			trendMonths = v
		}
	}

	txnType := strings.ToLower(r.URL.Query().Get("txn_type"))
	txnStatus := strings.ToLower(r.URL.Query().Get("txn_status"))
	if txnType == "" {
		txnType = "all"
	}
	if txnStatus == "" {
		txnStatus = "all"
	}

	catScope := strings.ToLower(r.URL.Query().Get("cat_scope"))
	if catScope == "" {
		catScope = "all"
	}

	// ── Pre-compute date boundaries (avoids repeated string parsing) ────
	monthStart := time.Date(refDate.Year(), refDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)
	trendStart := monthStart.AddDate(0, -(trendMonths - 1), 0)

	// ── Result holders ──────────────────────────────────────────────────
	type MonthTrend struct {
		Month    string  `json:"month"`
		Revenue  float64 `json:"revenue"`
		Expenses float64 `json:"expenses"`
		Profit   float64 `json:"profit"`
	}
	type Transaction struct {
		ID          int     `json:"id"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
		Amount      float64 `json:"amount"`
		Status      string  `json:"status"`
		Date        string  `json:"date"`
		Category    string  `json:"category"`
	}
	type CategoryBreakdown struct {
		Category string  `json:"category"`
		Amount   float64 `json:"amount"`
	}

	var (
		revenue        float64
		expenseTotal   float64
		outstandingAmt float64
		outstandingCnt int
		totalCollected float64
		trends         []MonthTrend
		transactions   []Transaction
		categories     []CategoryBreakdown
	)

	var wg sync.WaitGroup
	var mu sync.Mutex

	// ── 1. Current-month revenue (index-friendly range) ─────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var v float64
		db.DB.QueryRow(
			`SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND paid_at >= $1 AND paid_at < $2 AND deleted_at IS NULL`,
			monthStart, monthEnd,
		).Scan(&v)
		mu.Lock()
		revenue = v
		mu.Unlock()
	}()

	// ── 2. Current-month expenses ────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var v float64
		db.DB.QueryRow(
			`SELECT COALESCE(SUM(amount),0) FROM expenses WHERE expense_date >= $1 AND expense_date < $2 AND deleted_at IS NULL`,
			monthStart, monthEnd,
		).Scan(&v)
		mu.Lock()
		expenseTotal = v
		mu.Unlock()
	}()

	// ── 3. Trend data — 2 aggregation queries instead of 2×N serial ─────
	wg.Add(1)
	go func() {
		defer wg.Done()

		revMap := make(map[string]float64, trendMonths)
		expMap := make(map[string]float64, trendMonths)

		// Revenue aggregation
		rRows, err := db.DB.Query(
			`SELECT TO_CHAR(date_trunc('month', paid_at), 'YYYY-MM'), COALESCE(SUM(amount),0)
			 FROM invoices
			 WHERE status='paid' AND deleted_at IS NULL AND paid_at >= $1 AND paid_at < $2
			 GROUP BY 1`,
			trendStart, monthEnd,
		)
		if err == nil {
			defer rRows.Close()
			for rRows.Next() {
				var m string
				var v float64
				rRows.Scan(&m, &v)
				revMap[m] = v
			}
		}

		// Expense aggregation
		eRows, err := db.DB.Query(
			`SELECT TO_CHAR(date_trunc('month', expense_date), 'YYYY-MM'), COALESCE(SUM(amount),0)
			 FROM expenses
			 WHERE deleted_at IS NULL AND expense_date >= $1 AND expense_date < $2
			 GROUP BY 1`,
			trendStart, monthEnd,
		)
		if err == nil {
			defer eRows.Close()
			for eRows.Next() {
				var m string
				var v float64
				eRows.Scan(&m, &v)
				expMap[m] = v
			}
		}

		result := make([]MonthTrend, 0, trendMonths)
		for i := 0; i < trendMonths; i++ {
			d := trendStart.AddDate(0, i, 0)
			key := d.Format("2006-01")
			rev := revMap[key]
			exp := expMap[key]
			result = append(result, MonthTrend{
				Month:    d.Format("Jan 06"),
				Revenue:  rev,
				Expenses: exp,
				Profit:   rev - exp,
			})
		}
		mu.Lock()
		trends = result
		mu.Unlock()
	}()

	// ── 4. Invoice transactions ──────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		if txnType == "expense" {
			return
		}
		q := `SELECT i.id, COALESCE(q.name,'N/A'), i.amount, i.status,
		             TO_CHAR(i.issued_at,'YYYY-MM-DD'), COALESCE(q.service,'')
		      FROM invoices i LEFT JOIN queries q ON i.query_id=q.id
		      WHERE i.deleted_at IS NULL`
		var args []interface{}
		argIdx := 1
		if txnStatus == "paid" || txnStatus == "pending" {
			q += ` AND i.status=$` + strconv.Itoa(argIdx)
			args = append(args, txnStatus)
			argIdx++
		}
		_ = argIdx
		q += ` ORDER BY i.issued_at DESC LIMIT 15`
		rows, err := db.DB.Query(q, args...)
		if err != nil {
			return
		}
		defer rows.Close()
		var local []Transaction
		for rows.Next() {
			var t Transaction
			rows.Scan(&t.ID, &t.Description, &t.Amount, &t.Status, &t.Date, &t.Category)
			t.Type = "invoice"
			local = append(local, t)
		}
		mu.Lock()
		transactions = append(transactions, local...)
		mu.Unlock()
	}()

	// ── 5. Expense transactions ──────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		if txnType == "invoice" || txnStatus == "pending" {
			return
		}
		rows, err := db.DB.Query(
			`SELECT id, COALESCE(description,''), amount, TO_CHAR(expense_date,'YYYY-MM-DD'), COALESCE(category,'Uncategorized')
			 FROM expenses WHERE deleted_at IS NULL ORDER BY expense_date DESC LIMIT 15`,
		)
		if err != nil {
			return
		}
		defer rows.Close()
		var local []Transaction
		for rows.Next() {
			var t Transaction
			rows.Scan(&t.ID, &t.Description, &t.Amount, &t.Date, &t.Category)
			t.Type = "expense"
			t.Status = "paid"
			local = append(local, t)
		}
		mu.Lock()
		transactions = append(transactions, local...)
		mu.Unlock()
	}()

	// ── 6. Expense category breakdown ────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		q := `SELECT COALESCE(category,'Uncategorized'), SUM(amount) FROM expenses WHERE deleted_at IS NULL`
		var args []interface{}
		if catScope == "month" {
			q += ` AND expense_date >= $1 AND expense_date < $2`
			args = append(args, monthStart, monthEnd)
		}
		q += ` GROUP BY COALESCE(category,'Uncategorized') ORDER BY SUM(amount) DESC`
		rows, err := db.DB.Query(q, args...)
		if err != nil {
			return
		}
		defer rows.Close()
		var result []CategoryBreakdown
		for rows.Next() {
			var cb CategoryBreakdown
			rows.Scan(&cb.Category, &cb.Amount)
			result = append(result, cb)
		}
		mu.Lock()
		categories = result
		mu.Unlock()
	}()

	// ── 7. Outstanding invoices ──────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var amt float64
		var cnt int
		db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0), COUNT(*) FROM invoices WHERE status='pending' AND deleted_at IS NULL`).Scan(&amt, &cnt)
		mu.Lock()
		outstandingAmt = amt
		outstandingCnt = cnt
		mu.Unlock()
	}()

	// ── 8. Total collected (all time) ────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		var v float64
		db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND deleted_at IS NULL`).Scan(&v)
		mu.Lock()
		totalCollected = v
		mu.Unlock()
	}()

	// Wait for all concurrent queries to finish
	wg.Wait()

	// Sort merged transactions by date desc, cap at 15
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Date > transactions[j].Date
	})
	if len(transactions) > 15 {
		transactions = transactions[:15]
	}
	if categories == nil {
		categories = []CategoryBreakdown{}
	}
	if transactions == nil {
		transactions = []Transaction{}
	}

	result := map[string]interface{}{
		"current_month":       qMonth,
		"revenue":             revenue,
		"expenses":            expenseTotal,
		"profit":              revenue - expenseTotal,
		"trends":              trends,
		"trend_months":        trendMonths,
		"recent_transactions": transactions,
		"category_breakdown":  categories,
		"cat_scope":           catScope,
		"outstanding_amount":  outstandingAmt,
		"outstanding_count":   outstandingCnt,
		"total_collected":     totalCollected,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetExpenses gets all non-deleted expenses
func GetExpenses(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`
		SELECT id, category, description, amount, TO_CHAR(expense_date, 'YYYY-MM-DD'), added_by, created_at
		FROM expenses
		WHERE deleted_at IS NULL
		ORDER BY expense_date DESC
	`)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve expenses"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var expenses []Expense
	for rows.Next() {
		var e Expense
		if err := rows.Scan(&e.ID, &e.Category, &e.Description, &e.Amount, &e.ExpenseDate, &e.AddedBy, &e.CreatedAt); err != nil {
			http.Error(w, `{"error":"Failed to scan expense data"}`, http.StatusInternalServerError)
			return
		}
		expenses = append(expenses, e)
	}

	if expenses == nil {
		expenses = []Expense{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(expenses)
}

// GetPayrollForFinance fetches finalized payroll records so Finance can review and disburse salaries.
// Route: GET /api/finance/payroll?month=YYYY-MM
func GetPayrollForFinance(w http.ResponseWriter, r *http.Request) {
	monthFilter := r.URL.Query().Get("month") // YYYY-MM, optional

	baseQuery := `
		SELECT p.id, p.guard_id, g.name AS guard_name, TO_CHAR(p.month, 'YYYY-MM') AS month,
		       p.total_hours, COALESCE(p.overtime_hours,0), COALESCE(p.paid_hours,0),
		       p.rate_per_hour, p.total_pay, p.status
		FROM payroll p
		LEFT JOIN guards g ON p.guard_id = g.id
		WHERE p.deleted_at IS NULL`

	var queryArgs []interface{}
	if monthFilter != "" {
		baseQuery += ` AND TO_CHAR(p.month, 'YYYY-MM') = $1`
		queryArgs = append(queryArgs, monthFilter)
	}
	baseQuery += ` ORDER BY p.month DESC, g.name ASC`

	rows, err := db.DB.Query(baseQuery, queryArgs...)
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

	if payroll == nil {
		payroll = []PayrollRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payroll)
}

// UpdatePayrollStatus allows Finance to mark a payroll record as paid (or revert to pending).
// Route: PUT /api/finance/payroll/{id}
func UpdatePayrollStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid payroll ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if !helpers.ValidateStatus(req.Status, []string{"pending", "paid"}) {
		http.Error(w, `{"error":"Invalid status"}`, http.StatusBadRequest)
		return
	}

	result, err := db.DB.Exec(
		"UPDATE payroll SET status = $1 WHERE id = $2 AND deleted_at IS NULL",
		req.Status, id,
	)
	if err != nil {
		http.Error(w, `{"error":"Failed to update payroll status"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Payroll record not found"}`, http.StatusNotFound)
		return
	}

	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "update_payroll_status", "payroll:"+strconv.Itoa(id), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for update_payroll_status: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Payroll status updated"})
}

// ─────────────────────────────────────────────────────────────────────────────
// DETAIL ENDPOINTS
// ─────────────────────────────────────────────────────────────────────────────

// GetInvoiceDetail returns the full detail of a single invoice including
// all query/client information (email, phone, service options, etc.).
// Route: GET /api/finance/invoices/{id}
func GetInvoiceDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid invoice ID"}`, http.StatusBadRequest)
		return
	}

	type InvoiceDetail struct {
		ID              int        `json:"id"`
		QueryID         *int       `json:"query_id"`
		ClientName      *string    `json:"client_name"`
		ClientEmail     *string    `json:"client_email"`
		ClientPhone     *string    `json:"client_phone"`
		Service         *string    `json:"service"`
		Amount          float64    `json:"amount"`
		Status          string     `json:"status"`
		IssuedAt        time.Time  `json:"issued_at"`
		PaidAt          *time.Time `json:"paid_at"`
		PaymentRef      *string    `json:"payment_ref"`
		NumGuards       *int       `json:"num_guards"`
		DurationType    *string    `json:"duration_type"`
		DurationValue   *float64   `json:"duration_value"`
		CameraRequired  *bool      `json:"camera_required"`
		VehicleRequired *bool      `json:"vehicle_required"`
		FirstAid        *bool      `json:"first_aid"`
		WalkieTalkie    *bool      `json:"walkie_talkie"`
		BulletProof     *bool      `json:"bullet_proof"`
		FireSafety      *bool      `json:"fire_safety"`
		Cost            *float64   `json:"cost"`
		Message         *string    `json:"message"`
	}

	var d InvoiceDetail
	err = db.DB.QueryRow(`
		SELECT i.id, i.query_id, q.name, q.email, q.phone, q.service,
		       i.amount, i.status, i.issued_at, i.paid_at, i.payment_ref,
		       q.num_guards, q.duration_type, q.duration_value,
		       q.camera_required, q.vehicle_required, q.first_aid,
		       q.walkie_talkie, q.bullet_proof, q.fire_safety, q.cost, q.message
		FROM invoices i
		LEFT JOIN queries q ON i.query_id = q.id
		WHERE i.id = $1 AND i.deleted_at IS NULL
	`, id).Scan(
		&d.ID, &d.QueryID, &d.ClientName, &d.ClientEmail, &d.ClientPhone, &d.Service,
		&d.Amount, &d.Status, &d.IssuedAt, &d.PaidAt, &d.PaymentRef,
		&d.NumGuards, &d.DurationType, &d.DurationValue,
		&d.CameraRequired, &d.VehicleRequired, &d.FirstAid,
		&d.WalkieTalkie, &d.BulletProof, &d.FireSafety, &d.Cost, &d.Message,
	)
	if err != nil {
		http.Error(w, `{"error":"Invoice not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// GetExpenseDetail returns the full detail of a single expense including
// the name and email of the user who recorded it.
// Route: GET /api/finance/expenses/{id}
func GetExpenseDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid expense ID"}`, http.StatusBadRequest)
		return
	}

	type ExpenseDetail struct {
		ID           int       `json:"id"`
		Category     *string   `json:"category"`
		Description  *string   `json:"description"`
		Amount       float64   `json:"amount"`
		ExpenseDate  string    `json:"expense_date"`
		CreatedAt    time.Time `json:"created_at"`
		AddedByID    *int      `json:"added_by_id"`
		AddedByName  *string   `json:"added_by_name"`
		AddedByEmail *string   `json:"added_by_email"`
	}

	var d ExpenseDetail
	err = db.DB.QueryRow(`
		SELECT e.id, e.category, e.description, e.amount,
		       TO_CHAR(e.expense_date, 'YYYY-MM-DD'), e.created_at,
		       e.added_by, u.name, u.email
		FROM expenses e
		LEFT JOIN users u ON e.added_by = u.id
		WHERE e.id = $1 AND e.deleted_at IS NULL
	`, id).Scan(
		&d.ID, &d.Category, &d.Description, &d.Amount,
		&d.ExpenseDate, &d.CreatedAt, &d.AddedByID, &d.AddedByName, &d.AddedByEmail,
	)
	if err != nil {
		http.Error(w, `{"error":"Expense not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

// GetPayrollDetail returns a payroll record together with the guard's individual
// shift schedule for that month (dates, times, hours, client, status).
// Route: GET /api/finance/payroll/{id}
func GetPayrollDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid payroll ID"}`, http.StatusBadRequest)
		return
	}

	type ShiftSummary struct {
		ID            int        `json:"id"`
		Date          string     `json:"date"`
		StartTime     *time.Time `json:"start_time"`
		EndTime       *time.Time `json:"end_time"`
		ActualHours   float64    `json:"actual_hours"`
		OvertimeHours float64    `json:"overtime_hours"`
		PaidHours     float64    `json:"paid_hours"`
		Status        string     `json:"status"`
		ClientName    *string    `json:"client_name"`
	}
	type PayrollDetail struct {
		ID            int            `json:"id"`
		GuardID       int            `json:"guard_id"`
		GuardName     *string        `json:"guard_name"`
		GuardPhone    *string        `json:"guard_phone"`
		GuardEmail    *string        `json:"guard_email"`
		Month         string         `json:"month"`
		TotalHours    float64        `json:"total_hours"`
		OvertimeHours float64        `json:"overtime_hours"`
		PaidHours     float64        `json:"paid_hours"`
		RatePerHour   float64        `json:"rate_per_hour"`
		TotalPay      float64        `json:"total_pay"`
		Status        string         `json:"status"`
		Shifts        []ShiftSummary `json:"shifts"`
	}

	var d PayrollDetail
	err = db.DB.QueryRow(`
		SELECT p.id, p.guard_id, g.name, g.phone, g.email,
		       TO_CHAR(p.month, 'YYYY-MM'), p.total_hours,
		       COALESCE(p.overtime_hours,0), COALESCE(p.paid_hours,0),
		       p.rate_per_hour, p.total_pay, p.status
		FROM payroll p
		LEFT JOIN guards g ON p.guard_id = g.id
		WHERE p.id = $1 AND p.deleted_at IS NULL
	`, id).Scan(
		&d.ID, &d.GuardID, &d.GuardName, &d.GuardPhone, &d.GuardEmail,
		&d.Month, &d.TotalHours, &d.OvertimeHours, &d.PaidHours, &d.RatePerHour, &d.TotalPay, &d.Status,
	)
	if err != nil {
		http.Error(w, `{"error":"Payroll record not found"}`, http.StatusNotFound)
		return
	}

	// Fetch all shifts for this guard in the payroll month
	monthStart, _ := time.Parse("2006-01", d.Month)
	monthEnd := monthStart.AddDate(0, 1, 0)

	shiftRows, err := db.DB.Query(`
		SELECT s.id, TO_CHAR(s.start_time AT TIME ZONE 'UTC', 'YYYY-MM-DD'),
		       s.start_time, s.end_time,
		       COALESCE(s.actual_hours, 0),
		       COALESCE(s.overtime_hours, 0),
		       COALESCE(s.paid_hours, COALESCE(s.actual_hours, 0)),
		       s.status, q.name
		FROM shifts s
		LEFT JOIN queries q ON s.query_id = q.id
		WHERE s.guard_id = $1
		  AND s.start_time >= $2 AND s.start_time < $3
		  AND s.deleted_at IS NULL
		ORDER BY s.start_time ASC
	`, d.GuardID, monthStart, monthEnd)
	if err == nil {
		defer shiftRows.Close()
		for shiftRows.Next() {
			var sh ShiftSummary
			shiftRows.Scan(&sh.ID, &sh.Date, &sh.StartTime, &sh.EndTime,
				&sh.ActualHours, &sh.OvertimeHours, &sh.PaidHours, &sh.Status, &sh.ClientName)
			d.Shifts = append(d.Shifts, sh)
		}
	}
	if d.Shifts == nil {
		d.Shifts = []ShiftSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

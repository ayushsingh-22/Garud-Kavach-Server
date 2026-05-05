package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"server/db"
	"server/helpers"
	"strconv"
	"strings"
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

// GetFinanceOverview returns a comprehensive finance summary for the superadmin dashboard:
// - current month P&L
// - last 6 months revenue/expense trend
// - 15 most recent transactions (invoices + expenses combined)
// - expense breakdown by category
// - outstanding (unpaid) invoice total
func GetFinanceOverview(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	// ── Read filter query params ────────────────────────────────────────
	qMonth := r.URL.Query().Get("month") // YYYY-MM, defaults to current
	if qMonth == "" {
		qMonth = now.Format("2006-01")
	}
	// Validate month format
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

	txnType := strings.ToLower(r.URL.Query().Get("txn_type"))     // all | invoice | expense
	txnStatus := strings.ToLower(r.URL.Query().Get("txn_status")) // all | paid | pending
	if txnType == "" {
		txnType = "all"
	}
	if txnStatus == "" {
		txnStatus = "all"
	}

	catScope := strings.ToLower(r.URL.Query().Get("cat_scope")) // all | month
	if catScope == "" {
		catScope = "all"
	}

	// ── Current month P&L ───────────────────────────────────────────────
	var revenue, expenseTotal float64
	_ = db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND TO_CHAR(paid_at,'YYYY-MM')=$1 AND deleted_at IS NULL`, qMonth).Scan(&revenue)
	_ = db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM expenses WHERE TO_CHAR(expense_date,'YYYY-MM')=$1 AND deleted_at IS NULL`, qMonth).Scan(&expenseTotal)

	// ── Trend (variable months) ─────────────────────────────────────────
	type MonthTrend struct {
		Month    string  `json:"month"`
		Revenue  float64 `json:"revenue"`
		Expenses float64 `json:"expenses"`
		Profit   float64 `json:"profit"`
	}
	trends := make([]MonthTrend, trendMonths)
	for i := trendMonths - 1; i >= 0; i-- {
		d := refDate.AddDate(0, -i, 0)
		m := d.Format("2006-01")
		label := d.Format("Jan 06")
		var rev, exp float64
		_ = db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND TO_CHAR(paid_at,'YYYY-MM')=$1 AND deleted_at IS NULL`, m).Scan(&rev)
		_ = db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM expenses WHERE TO_CHAR(expense_date,'YYYY-MM')=$1 AND deleted_at IS NULL`, m).Scan(&exp)
		trends[trendMonths-1-i] = MonthTrend{Month: label, Revenue: rev, Expenses: exp, Profit: rev - exp}
	}

	// ── Recent transactions (last 15) with type & status filters ────────
	type Transaction struct {
		ID          int     `json:"id"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
		Amount      float64 `json:"amount"`
		Status      string  `json:"status"`
		Date        string  `json:"date"`
		Category    string  `json:"category"`
	}
	var transactions []Transaction

	if txnType == "all" || txnType == "invoice" {
		invQuery := `
			SELECT i.id, COALESCE(q.name,'N/A'), i.amount, i.status,
			       TO_CHAR(i.issued_at,'YYYY-MM-DD'), COALESCE(q.service,'')
			FROM invoices i LEFT JOIN queries q ON i.query_id=q.id
			WHERE i.deleted_at IS NULL`
		var invArgs []interface{}
		argIdx := 1
		if txnStatus == "paid" || txnStatus == "pending" {
			invQuery += ` AND i.status=$` + strconv.Itoa(argIdx)
			invArgs = append(invArgs, txnStatus)
			argIdx++
		}
		_ = argIdx
		invQuery += ` ORDER BY i.issued_at DESC LIMIT 15`
		invRows, err := db.DB.Query(invQuery, invArgs...)
		if err == nil {
			defer invRows.Close()
			for invRows.Next() {
				var t Transaction
				var desc, cat string
				invRows.Scan(&t.ID, &desc, &t.Amount, &t.Status, &t.Date, &cat)
				t.Type = "invoice"
				t.Description = desc
				t.Category = cat
				transactions = append(transactions, t)
			}
		}
	}

	if txnType == "all" || txnType == "expense" {
		// expenses don't have a real status but if status filter is "pending", skip expenses
		if txnStatus != "pending" {
			expRows, err := db.DB.Query(`
				SELECT id, COALESCE(description,''), amount, 'expense',
				       TO_CHAR(expense_date,'YYYY-MM-DD'), COALESCE(category,'Uncategorized')
				FROM expenses WHERE deleted_at IS NULL ORDER BY expense_date DESC LIMIT 15`)
			if err == nil {
				defer expRows.Close()
				for expRows.Next() {
					var t Transaction
					expRows.Scan(&t.ID, &t.Description, &t.Amount, &t.Status, &t.Date, &t.Category)
					t.Type = "expense"
					t.Status = "paid"
					transactions = append(transactions, t)
				}
			}
		}
	}

	// Sort combined by date desc and take top 15
	for i := 0; i < len(transactions); i++ {
		for j := i + 1; j < len(transactions); j++ {
			if transactions[j].Date > transactions[i].Date {
				transactions[i], transactions[j] = transactions[j], transactions[i]
			}
		}
	}
	if len(transactions) > 15 {
		transactions = transactions[:15]
	}

	// ── Expense category breakdown (scoped) ─────────────────────────────
	type CategoryBreakdown struct {
		Category string  `json:"category"`
		Amount   float64 `json:"amount"`
	}
	var categories []CategoryBreakdown

	catQuery := `SELECT COALESCE(category,'Uncategorized'), SUM(amount) FROM expenses WHERE deleted_at IS NULL`
	var catArgs []interface{}
	if catScope == "month" {
		catQuery += ` AND TO_CHAR(expense_date,'YYYY-MM')=$1`
		catArgs = append(catArgs, qMonth)
	}
	catQuery += ` GROUP BY COALESCE(category,'Uncategorized') ORDER BY SUM(amount) DESC`
	catRows, err := db.DB.Query(catQuery, catArgs...)
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var cb CategoryBreakdown
			catRows.Scan(&cb.Category, &cb.Amount)
			categories = append(categories, cb)
		}
	}
	if categories == nil {
		categories = []CategoryBreakdown{}
	}

	// ── Outstanding invoices ────────────────────────────────────────────
	var outstandingAmount float64
	var outstandingCount int
	_ = db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0), COUNT(*) FROM invoices WHERE status='pending' AND deleted_at IS NULL`).Scan(&outstandingAmount, &outstandingCount)

	// ── Total collected (all time) ──────────────────────────────────────
	var totalCollected float64
	_ = db.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM invoices WHERE status='paid' AND deleted_at IS NULL`).Scan(&totalCollected)

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
		"outstanding_amount":  outstandingAmount,
		"outstanding_count":   outstandingCount,
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

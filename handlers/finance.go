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

	if req.Status != "pending" && req.Status != "paid" && req.Status != "refunded" {
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

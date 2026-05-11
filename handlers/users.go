package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"server/db"
	"server/helpers"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

// SignUpRequest represents the request body for user sign-up.
type SignUpRequest struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	AccountType  string `json:"accountType"`
	SecurityCode string `json:"securityCode"`
	AdminRole    string `json:"adminRole"`
}

// SignUpHandler handles user sign-up.
// If accountType is "customer" or missing, creates a customer account (identical to RegisterCustomerHandler).
// If accountType is "admin", validates ADMIN_SECURITY_CODE and creates a staff account.
func SignUpHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	accountType := strings.TrimSpace(strings.ToLower(req.AccountType))

	// Validate accountType if provided
	if accountType != "" && accountType != "customer" && accountType != "admin" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid accountType. Must be 'customer' or 'admin'."})
		return
	}

	if accountType == "admin" {
		signupAdmin(w, r, req)
		return
	}

	// Default: customer path (accountType is "customer" or missing)
	signupCustomer(w, req)
}

// signupCustomer creates a customer account. Behaviour is identical to RegisterCustomerHandler.
func signupCustomer(w http.ResponseWriter, req SignUpRequest) {
	name, err := helpers.ValidateTrimLength(req.Name, 200)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Name is required and must not exceed 200 characters"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	password := req.Password

	if !helpers.ValidateEmail(email) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email format"})
		return
	}

	if len(password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password must be at least 8 characters"})
		return
	}

	var exists bool
	err = db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)", email).Scan(&exists)
	if err != nil {
		log.Printf("ERROR: Database error checking email existence: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}
	if exists {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "An account with this email already exists"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Printf("ERROR: Failed to hash password: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		log.Printf("ERROR: Failed to begin transaction: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var newUserID int
	err = tx.QueryRow(
		"INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, 'customer') RETURNING id",
		name, email, string(hashedPassword),
	).Scan(&newUserID)
	if err != nil {
		log.Printf("ERROR: Failed to create user: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create account"})
		return
	}

	_, err = tx.Exec(
		"INSERT INTO customers (user_id) VALUES ($1)",
		newUserID,
	)
	if err != nil {
		log.Printf("ERROR: Failed to create customer record: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create account"})
		return
	}

	if err = tx.Commit(); err != nil {
		log.Printf("ERROR: Failed to commit transaction: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create account"})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Account created. Please log in."})
}

// signupAdmin creates a staff account after validating the security code.
func signupAdmin(w http.ResponseWriter, r *http.Request, req SignUpRequest) {
	expectedCode := os.Getenv("ADMIN_SECURITY_CODE")
	if expectedCode == "" || subtle.ConstantTimeCompare([]byte(req.SecurityCode), []byte(expectedCode)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized access"})
		return
	}

	adminRole := strings.TrimSpace(strings.ToLower(req.AdminRole))
	if !helpers.ValidateStatus(adminRole, []string{"manager", "finance", "hr"}) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid adminRole. Must be 'manager', 'finance', or 'hr'."})
		return
	}

	name, nameErr := helpers.ValidateTrimLength(req.Name, 200)
	if nameErr != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Name is required and must not exceed 200 characters"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))

	if !helpers.ValidateEmail(email) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email format"})
		return
	}

	if len(req.Password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password must be at least 8 characters"})
		return
	}

	var exists bool
	err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 AND deleted_at IS NULL)", email).Scan(&exists)
	if err != nil {
		log.Printf("ERROR: Database error checking email existence: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}
	if exists {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "An account with this email already exists"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		log.Printf("ERROR: Failed to hash password: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}

	var newUserID int
	err = db.DB.QueryRow(
		"INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, $4) RETURNING id",
		name, email, string(hashedPassword), adminRole,
	).Scan(&newUserID)
	if err != nil {
		log.Printf("ERROR: Failed to create admin user: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to create account"})
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(userID), "create_user", "user:"+strconv.Itoa(newUserID), map[string]string{
		"role": adminRole,
	}); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_user: %v", err)
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Account created. Please log in."})
}

// GetAdminUsers retrieves all non-customer administrative users.
func GetAdminUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query("SELECT id, name, email, role, created_at FROM users WHERE role != 'customer' AND deleted_at IS NULL ORDER BY name")
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve admin users"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AdminUser struct {
		ID        int       `json:"id"`
		Name      *string   `json:"name"`
		Email     string    `json:"email"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"created_at"`
	}

	users := []AdminUser{}
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			http.Error(w, `{"error":"Failed to scan admin user data"}`, http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}
	json.NewEncoder(w).Encode(users)
}

// CreateAdminUser adds a new administrative user to the database.
func CreateAdminUser(w http.ResponseWriter, r *http.Request) {
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validation
	if !helpers.ValidateStatus(req.Role, []string{"manager", "finance", "hr"}) {
		http.Error(w, `{"error":"Invalid role specified. Must be manager, finance, or hr."}`, http.StatusBadRequest)
		return
	}

	name, nameErr := helpers.ValidateTrimLength(req.Name, 200)
	if nameErr != nil {
		http.Error(w, `{"error":"Name is required and must not exceed 200 characters"}`, http.StatusBadRequest)
		return
	}
	req.Name = name

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !helpers.ValidateEmail(email) {
		http.Error(w, `{"error":"Invalid email format"}`, http.StatusBadRequest)
		return
	}
	req.Email = email

	if len(req.Password) < 8 {
		http.Error(w, `{"error":"Password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		http.Error(w, `{"error":"Failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	var newUserID int
	err = db.DB.QueryRow(
		"INSERT INTO users (name, email, password, role) VALUES ($1, $2, $3, $4) RETURNING id",
		req.Name, req.Email, string(hashedPassword), req.Role,
	).Scan(&newUserID)

	if err != nil {
		http.Error(w, `{"error":"Failed to create admin user"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(userID), "create_admin_user", "user:"+strconv.Itoa(newUserID), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_admin_user: %v", err)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"id": newUserID})
}

// UpdateAdminUser updates an existing admin user
func UpdateAdminUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid user ID"}`, http.StatusBadRequest)
		return
	}

	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validation
	if !helpers.ValidateStatus(req.Role, []string{"manager", "finance", "hr", "superadmin"}) {
		http.Error(w, `{"error":"Invalid role specified."}`, http.StatusBadRequest)
		return
	}

	name, nameErr := helpers.ValidateTrimLength(req.Name, 200)
	if nameErr != nil {
		http.Error(w, `{"error":"Name is required and must not exceed 200 characters"}`, http.StatusBadRequest)
		return
	}
	req.Name = name

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !helpers.ValidateEmail(email) {
		http.Error(w, `{"error":"Invalid email format"}`, http.StatusBadRequest)
		return
	}
	req.Email = email

	if req.Password != "" && len(req.Password) < 8 {
		http.Error(w, `{"error":"Password must be at least 8 characters"}`, http.StatusBadRequest)
		return
	}

	var result int64
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			http.Error(w, `{"error":"Failed to hash password"}`, http.StatusInternalServerError)
			return
		}

		res, err := db.DB.Exec(
			"UPDATE users SET name = $1, email = $2, role = $3, password = $4 WHERE id = $5 AND deleted_at IS NULL",
			req.Name, req.Email, req.Role, string(hashedPassword), id,
		)
		if err != nil {
			http.Error(w, `{"error":"Failed to update user"}`, http.StatusInternalServerError)
			return
		}
		result, _ = res.RowsAffected()
	} else {
		res, err := db.DB.Exec(
			"UPDATE users SET name = $1, email = $2, role = $3 WHERE id = $4 AND deleted_at IS NULL",
			req.Name, req.Email, req.Role, id,
		)
		if err != nil {
			http.Error(w, `{"error":"Failed to update user"}`, http.StatusInternalServerError)
			return
		}
		result, _ = res.RowsAffected()
	}

	if result == 0 {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// Audit Log
	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "update_user", "user:"+strconv.Itoa(id), map[string]interface{}{
		"updated_name": req.Name,
		"updated_role": req.Role,
	}); err != nil {
		log.Printf("ERROR: Failed to write audit log for update_user: %v", err)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
}

// SoftDeleteUser marks a user as deleted.
func SoftDeleteUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid user ID"}`, http.StatusBadRequest)
		return
	}

	// Prevent self-deletion
	currentUserID, _ := r.Context().Value(userIDKey).(float64)
	if int(currentUserID) == id {
		http.Error(w, `{"error":"You cannot delete your own account"}`, http.StatusBadRequest)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, `{"error":"Failed to delete user"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Nullify FK references with NO ACTION constraints
	tx.Exec("UPDATE expenses SET added_by = NULL WHERE added_by = $1", id)
	tx.Exec("UPDATE leave_requests SET reviewed_by = NULL WHERE reviewed_by = $1", id)
	tx.Exec("DELETE FROM customers WHERE user_id = $1", id)

	result, err := tx.Exec("DELETE FROM users WHERE id = $1", id)
	if err != nil {
		http.Error(w, `{"error":"Failed to delete user"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"Failed to delete user"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "delete_user", "user:"+strconv.Itoa(id), nil); err != nil {
		log.Printf("ERROR: Failed to write audit log for delete_user: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

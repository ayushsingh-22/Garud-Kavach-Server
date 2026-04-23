package handlers

import (
	"encoding/json"
	"net/http"
	"server/db"
	"server/helpers"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

// SignUpRequest represents the request body for user sign-up.
type SignUpRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// SignUpHandler handles user sign-up.
func SignUpHandler(w http.ResponseWriter, r *http.Request) {
	var req SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validation
	if req.Role != "manager" && req.Role != "finance" && req.Role != "hr" {
		http.Error(w, `{"error":"Invalid role specified. Must be manager, finance, or hr."}`, http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
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
		http.Error(w, `{"error":"Failed to create user"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	helpers.WriteAuditLog(db.DB, int(userID), "create_user", "user:"+strconv.Itoa(newUserID), req)

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "User created successfully"})
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
	if req.Role != "manager" && req.Role != "finance" && req.Role != "hr" {
		http.Error(w, `{"error":"Invalid role specified. Must be manager, finance, or hr."}`, http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
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
	helpers.WriteAuditLog(db.DB, int(userID), "create_admin_user", "user:"+strconv.Itoa(newUserID), req)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"id": newUserID})
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

	result, err := db.DB.Exec("UPDATE users SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL", id)
	if err != nil {
		http.Error(w, `{"error":"Failed to delete user"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
		return
	}

	// Audit Log
	helpers.WriteAuditLog(db.DB, int(currentUserID), "delete_user", "user:"+strconv.Itoa(id), nil)

	w.WriteHeader(http.StatusNoContent)
}

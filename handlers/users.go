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
		http.Error(w, `{"error":"Failed to create user"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(userID), "create_user", "user:"+strconv.Itoa(newUserID), req); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_user: %v", err)
	}

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
	if req.Role != "manager" && req.Role != "finance" && req.Role != "hr" && req.Role != "superadmin" {
		http.Error(w, `{"error":"Invalid role specified."}`, http.StatusBadRequest)
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
	if err := helpers.WriteAuditLog(db.DB, int(currentUserID), "delete_user", "user:"+strconv.Itoa(id), nil); err != nil {
		log.Printf("ERROR: Failed to write audit log for delete_user: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"server/db"
	"server/models"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type CustomerRegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func RegisterCustomerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	var req CustomerRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Validation
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(strings.ToLower(req.Email))
	password := req.Password

	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Name is required"})
		return
	}

	if !emailRegex.MatchString(email) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid email format"})
		return
	}

	if len(password) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Password must be at least 8 characters"})
		return
	}

	// Check if email already exists
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

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		log.Printf("ERROR: Failed to hash password: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}

	// Insert user and customer in a transaction
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

// GetCustomerProfile returns the joined user + customer record for the logged-in customer.
func GetCustomerProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID, ok := r.Context().Value(userIDKey).(float64)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	type ProfileResponse struct {
		ID      int     `json:"id"`
		Name    *string `json:"name"`
		Email   string  `json:"email"`
		Phone   *string `json:"phone"`
		Company *string `json:"company"`
		Address *string `json:"address"`
	}

	var profile ProfileResponse
	err := db.DB.QueryRow(`
		SELECT u.id, u.name, u.email, c.phone, c.company, c.address
		FROM users u
		LEFT JOIN customers c ON c.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`, int(userID)).Scan(&profile.ID, &profile.Name, &profile.Email, &profile.Phone, &profile.Company, &profile.Address)

	if err != nil {
		log.Printf("ERROR: Failed to get customer profile for user %d: %v", int(userID), err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load profile"})
		return
	}

	_ = json.NewEncoder(w).Encode(profile)
}

// UpdateCustomerProfile updates name, phone, company, address for the logged-in customer.
func UpdateCustomerProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID, ok := r.Context().Value(userIDKey).(float64)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name    string `json:"name"`
		Phone   string `json:"phone"`
		Company string `json:"company"`
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	uid := int(userID)

	// Update user name
	name := strings.TrimSpace(req.Name)
	if name != "" {
		if _, err := db.DB.Exec("UPDATE users SET name = $1 WHERE id = $2", name, uid); err != nil {
			log.Printf("ERROR: Failed to update user name: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update profile"})
			return
		}
	}

	// Upsert customer record
	_, err := db.DB.Exec(`
		INSERT INTO customers (user_id, phone, company, address)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE SET
			phone = EXCLUDED.phone,
			company = EXCLUDED.company,
			address = EXCLUDED.address
	`, uid, strings.TrimSpace(req.Phone), strings.TrimSpace(req.Company), strings.TrimSpace(req.Address))

	if err != nil {
		log.Printf("ERROR: Failed to update customer profile: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update profile"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Profile updated successfully"})
}

// GetCustomerQueries returns queries belonging to the logged-in customer.
func GetCustomerQueries(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID, ok := r.Context().Value(userIDKey).(float64)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, name, email, phone, service, message, num_guards, duration_type, duration_value,
			camera_required, vehicle_required, first_aid, walkie_talkie, bullet_proof, fire_safety,
			status, cost, submitted_at
		FROM queries
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY submitted_at DESC
	`, int(userID))
	if err != nil {
		log.Printf("ERROR: Failed to get customer queries: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load bookings"})
		return
	}
	defer rows.Close()

	var queries []models.Query
	for rows.Next() {
		var q models.Query
		if err := rows.Scan(
			&q.ID, &q.Name, &q.Email, &q.Phone, &q.Service, &q.Message,
			&q.NumGuards, &q.DurationType, &q.DurationValue,
			&q.CameraRequired, &q.VehicleRequired, &q.FirstAid,
			&q.WalkieTalkie, &q.BulletProof, &q.FireSafety,
			&q.Status, &q.Cost, &q.SubmittedAt,
		); err != nil {
			log.Printf("ERROR: Failed to scan query row: %v", err)
			continue
		}
		queries = append(queries, q)
	}

	if queries == nil {
		queries = []models.Query{}
	}

	_ = json.NewEncoder(w).Encode(queries)
}

// UpdateCustomerPassword updates the password for the logged-in customer.
func UpdateCustomerPassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	userID, ok := r.Context().Value(userIDKey).(float64)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if len(req.NewPassword) < 8 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "New password must be at least 8 characters"})
		return
	}

	uid := int(userID)
	var currentHash string
	err := db.DB.QueryRow("SELECT password FROM users WHERE id = $1 AND deleted_at IS NULL", uid).Scan(&currentHash)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to verify current password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Incorrect current password"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to process new password"})
		return
	}

	if _, err := db.DB.Exec("UPDATE users SET password = $1 WHERE id = $2", string(newHash), uid); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update password"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
}

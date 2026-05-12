package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"server/db"
	"server/helpers"
	"server/services"
	"strconv"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/gorilla/mux"
)

const maxGuardPhotoSize = 5 * 1024 * 1024

type Guard struct {
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	Phone         *string    `json:"phone"`
	Email         *string    `json:"email"`
	Address       *string    `json:"address"`
	LicenseNo     *string    `json:"license_no"`
	LicenseExpiry *time.Time `json:"license_expiry"`
	Status        string     `json:"status"`
	HourlyRate    float64    `json:"hourly_rate"`
	PhotoURL      *string    `json:"photo_url"`
	AssignedQuery *int       `json:"assigned_query_id,omitempty"`
	AssignedAt    *time.Time `json:"assigned_at,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func parseOptionalGuardDate(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return &parsed, nil
	}

	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func validateGuard(g *Guard) error {
	name, err := helpers.ValidateTrimLength(g.Name, 200)
	if err != nil {
		return errBadRequest("Guard name is required and must not exceed 200 characters")
	}
	g.Name = name

	if g.Status == "" {
		g.Status = "active"
	}
	if !helpers.ValidateStatus(g.Status, []string{"active", "inactive", "on_leave"}) {
		return errBadRequest("Invalid guard status")
	}

	if g.Email != nil {
		email := strings.TrimSpace(*g.Email)
		if email == "" {
			g.Email = nil
		} else if !helpers.ValidateEmail(email) {
			return errBadRequest("Invalid guard email")
		} else {
			g.Email = &email
		}
	}

	if g.Phone != nil {
		cleaned := helpers.ValidatePhone(*g.Phone)
		if cleaned == "" {
			g.Phone = nil
		} else {
			g.Phone = &cleaned
		}
	}

	if g.LicenseExpiry != nil && !g.LicenseExpiry.After(time.Now()) {
		return errBadRequest("License expiry date must be in the future")
	}
	if g.HourlyRate < 0 {
		return errBadRequest("Hourly rate cannot be negative")
	}

	return nil
}

type badRequestError string

func (e badRequestError) Error() string {
	return string(e)
}

func errBadRequest(message string) error {
	return badRequestError(message)
}

func writeValidationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(badRequestError); ok {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return true
	}
	http.Error(w, `{"error":"Invalid guard data"}`, http.StatusBadRequest)
	return true
}

func uploadGuardPhoto(file multipart.File, header *multipart.FileHeader) (*string, error) {
	mimeType := header.Header.Get("Content-Type")

	// Normalize non-standard type sent by some clients
	if mimeType == "image/jpg" {
		mimeType = "image/jpeg"
	}

	// If Content-Type is missing or generic (e.g. application/octet-stream), sniff
	// from the first 512 bytes — this handles iOS HEIC→JPEG output from
	// browser-image-compression which can strip the MIME type.
	if mimeType == "" || mimeType == "application/octet-stream" {
		buf := make([]byte, 512)
		n, _ := file.Read(buf)
		mimeType = http.DetectContentType(buf[:n])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("photo: failed to seek after MIME sniff: %w", err)
		}
		log.Printf("uploadGuardPhoto: sniffed MIME type = %q (header was empty/octet-stream)", mimeType)
	}

	if !strings.HasPrefix(mimeType, "image/") {
		return nil, errBadRequest(fmt.Sprintf("invalid image format (detected: %s)", mimeType))
	}
	if header.Size > maxGuardPhotoSize {
		return nil, errBadRequest("file size exceeds 5 MB")
	}

	cloudinaryURL := strings.TrimSpace(os.Getenv("CLOUDINARY_URL"))
	if cloudinaryURL == "" {
		return nil, fmt.Errorf("photo upload not configured: CLOUDINARY_URL env var is unset")
	}

	cld, err := cloudinary.NewFromURL(cloudinaryURL)
	if err != nil {
		return nil, fmt.Errorf("cloudinary URL parse error: %w", err)
	}
	resp, err := cld.Upload.Upload(context.Background(), file, uploader.UploadParams{Folder: "guards"})
	if err != nil {
		return nil, fmt.Errorf("cloudinary upload failed: %w", err)
	}

	// The Cloudinary SDK does NOT surface API-level errors as Go errors.
	// Check the embedded Error field explicitly.
	if resp.Error.Message != "" {
		return nil, fmt.Errorf("cloudinary API error: %s", resp.Error.Message)
	}

	// Prefer HTTPS URL; fall back to HTTP if SecureURL is somehow empty.
	url := resp.SecureURL
	if url == "" {
		url = resp.URL
	}
	if url == "" {
		log.Printf("uploadGuardPhoto: Cloudinary returned empty URL — PublicID=%q Format=%q Bytes=%d",
			resp.PublicID, resp.Format, resp.Bytes)
		return nil, fmt.Errorf("cloudinary returned an empty URL (PublicID=%q)", resp.PublicID)
	}

	return &url, nil
}

// GetGuards retrieves all non-deleted guards.
func GetGuards(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`
		SELECT
			g.id,
			g.name,
			g.phone,
			g.email,
			g.address,
			g.license_no,
			g.license_expiry,
			g.status,
			g.hourly_rate,
			g.photo_url,
			a.query_id,
			a.assigned_at,
			g.created_at
		FROM guards g
		LEFT JOIN LATERAL (
			SELECT query_id, assigned_at
			FROM guard_query_assignments
			WHERE guard_id = g.id AND unassigned_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		) a ON true
		WHERE g.deleted_at IS NULL
		ORDER BY g.name`)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve guards"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	guards := []Guard{}
	for rows.Next() {
		var g Guard
		if err := rows.Scan(&g.ID, &g.Name, &g.Phone, &g.Email, &g.Address, &g.LicenseNo, &g.LicenseExpiry, &g.Status, &g.HourlyRate, &g.PhotoURL, &g.AssignedQuery, &g.AssignedAt, &g.CreatedAt); err != nil {
			http.Error(w, `{"error":"Failed to scan guard data"}`, http.StatusInternalServerError)
			return
		}
		guards = append(guards, g)
	}
	json.NewEncoder(w).Encode(guards)
}

// GetGuardByID retrieves a single non-deleted guard with active assignment details.
func GetGuardByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid guard ID"}`, http.StatusBadRequest)
		return
	}

	var g Guard
	err = db.DB.QueryRow(`
		SELECT
			g.id,
			g.name,
			g.phone,
			g.email,
			g.address,
			g.license_no,
			g.license_expiry,
			g.status,
			g.hourly_rate,
			g.photo_url,
			a.query_id,
			a.assigned_at,
			g.created_at
		FROM guards g
		LEFT JOIN LATERAL (
			SELECT query_id, assigned_at
			FROM guard_query_assignments
			WHERE guard_id = g.id AND unassigned_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		) a ON true
		WHERE g.id = $1 AND g.deleted_at IS NULL`, id).Scan(
		&g.ID,
		&g.Name,
		&g.Phone,
		&g.Email,
		&g.Address,
		&g.LicenseNo,
		&g.LicenseExpiry,
		&g.Status,
		&g.HourlyRate,
		&g.PhotoURL,
		&g.AssignedQuery,
		&g.AssignedAt,
		&g.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Guard not found"})
			return
		}
		http.Error(w, `{"error":"Failed to retrieve guard"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(g)
}

// CreateGuard adds a new guard to the database.
func CreateGuard(w http.ResponseWriter, r *http.Request) {
	var g Guard

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle multipart
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, `{"error":"Failed to parse multipart form"}`, http.StatusBadRequest)
			return
		}

		g.Name = strings.TrimSpace(r.FormValue("name"))
		g.Phone = optionalString(r.FormValue("phone"))
		g.Email = optionalString(r.FormValue("email"))
		g.Address = optionalString(r.FormValue("address"))
		g.LicenseNo = optionalString(r.FormValue("license_no"))
		if exp := r.FormValue("license_expiry"); exp != "" {
			licenseExpiry, err := parseOptionalGuardDate(exp)
			if err != nil {
				http.Error(w, `{"error":"Invalid license expiry date"}`, http.StatusBadRequest)
				return
			}
			g.LicenseExpiry = licenseExpiry
		}
		g.Status = strings.TrimSpace(r.FormValue("status"))
		if g.Status == "" {
			g.Status = "active"
		}
		if hr := r.FormValue("hourly_rate"); hr != "" {
			rate, err := strconv.ParseFloat(hr, 64)
			if err != nil {
				http.Error(w, `{"error":"Invalid hourly rate"}`, http.StatusBadRequest)
				return
			}
			g.HourlyRate = rate
		}

		file, header, err := r.FormFile("photo")
		if err == nil {
			defer file.Close()
			photoURL, err := uploadGuardPhoto(file, header)
			if _, ok := err.(badRequestError); ok {
				writeValidationError(w, err)
				return
			}
			if err != nil {
				log.Printf("ERROR: Failed to upload guard photo: %v", err)
				http.Error(w, `{"error":"Failed to upload guard photo"}`, http.StatusInternalServerError)
				return
			}
			g.PhotoURL = photoURL
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
			return
		}
	}

	if err := validateGuard(&g); err != nil {
		writeValidationError(w, err)
		return
	}

	err := db.DB.QueryRow(
		"INSERT INTO guards (name, phone, email, address, license_no, license_expiry, status, hourly_rate, photo_url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id",
		g.Name, g.Phone, g.Email, g.Address, g.LicenseNo, g.LicenseExpiry, g.Status, g.HourlyRate, g.PhotoURL,
	).Scan(&g.ID)

	if err != nil {
		http.Error(w, `{"error":"Failed to create guard"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(userID), "create_guard", "guard:"+strconv.Itoa(g.ID), g); err != nil {
		log.Printf("ERROR: Failed to write audit log for create_guard: %v", err)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(g)
}

// UpdateGuard updates a guard's information
func UpdateGuard(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid guard ID"}`, http.StatusBadRequest)
		return
	}

	var g Guard

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle multipart
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, `{"error":"Failed to parse multipart form"}`, http.StatusBadRequest)
			return
		}

		g.Name = strings.TrimSpace(r.FormValue("name"))
		g.Phone = optionalString(r.FormValue("phone"))
		g.Email = optionalString(r.FormValue("email"))
		g.Address = optionalString(r.FormValue("address"))
		g.LicenseNo = optionalString(r.FormValue("license_no"))
		if exp := r.FormValue("license_expiry"); exp != "" {
			licenseExpiry, err := parseOptionalGuardDate(exp)
			if err != nil {
				http.Error(w, `{"error":"Invalid license expiry date"}`, http.StatusBadRequest)
				return
			}
			g.LicenseExpiry = licenseExpiry
		}
		g.Status = strings.TrimSpace(r.FormValue("status"))
		if hr := r.FormValue("hourly_rate"); hr != "" {
			rate, err := strconv.ParseFloat(hr, 64)
			if err != nil {
				http.Error(w, `{"error":"Invalid hourly rate"}`, http.StatusBadRequest)
				return
			}
			g.HourlyRate = rate
		}

		file, header, err := r.FormFile("photo")
		if err == nil {
			defer file.Close()
			photoURL, err := uploadGuardPhoto(file, header)
			if _, ok := err.(badRequestError); ok {
				writeValidationError(w, err)
				return
			}
			if err != nil {
				log.Printf("ERROR: Failed to upload guard photo: %v", err)
				http.Error(w, `{"error":"Failed to upload guard photo"}`, http.StatusInternalServerError)
				return
			}
			g.PhotoURL = photoURL
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
			return
		}
	}

	if err := validateGuard(&g); err != nil {
		writeValidationError(w, err)
		return
	}

	query := "UPDATE guards SET name=$1, phone=$2, email=$3, address=$4, license_no=$5, license_expiry=$6, status=$7, hourly_rate=$8"
	args := []interface{}{g.Name, g.Phone, g.Email, g.Address, g.LicenseNo, g.LicenseExpiry, g.Status, g.HourlyRate}
	argIdx := 9

	if g.PhotoURL != nil {
		query += ", photo_url=$" + strconv.Itoa(argIdx)
		args = append(args, g.PhotoURL)
		argIdx++
	}

	query += " WHERE id=$" + strconv.Itoa(argIdx) + " AND deleted_at IS NULL"
	args = append(args, id)

	result, err := db.DB.Exec(query, args...)
	if err != nil {
		http.Error(w, `{"error":"Failed to update guard"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Guard not found"}`, http.StatusNotFound)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	g.ID = id
	if err := helpers.WriteAuditLog(db.DB, int(userID), "update_guard", "guard:"+strconv.Itoa(id), g); err != nil {
		log.Printf("ERROR: Failed to write audit log for update_guard: %v", err)
	}

	json.NewEncoder(w).Encode(g)
}

// SoftDeleteGuard permanently deletes a guard and all related records.
func SoftDeleteGuard(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid guard ID"}`, http.StatusBadRequest)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, `{"error":"Failed to delete guard"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Remove child records with NO ACTION FK constraints
	tx.Exec("DELETE FROM guard_query_assignments WHERE guard_id = $1", id)
	tx.Exec("DELETE FROM shifts WHERE guard_id = $1", id)
	tx.Exec("DELETE FROM payroll WHERE guard_id = $1", id)
	tx.Exec("DELETE FROM leave_requests WHERE guard_id = $1", id)

	result, err := tx.Exec("DELETE FROM guards WHERE id = $1", id)
	if err != nil {
		http.Error(w, `{"error":"Failed to delete guard"}`, http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, `{"error":"Guard not found"}`, http.StatusNotFound)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"Failed to delete guard"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(db.DB, int(userID), "delete_guard", "guard:"+strconv.Itoa(id), nil); err != nil {
		log.Printf("ERROR: Failed to write audit log for delete_guard: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// AssignGuard assigns a guard to a query
func AssignGuard(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	guardID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"Invalid guard ID"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		QueryID int `json:"query_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.QueryID <= 0 {
		http.Error(w, `{"error":"Query ID is required"}`, http.StatusBadRequest)
		return
	}

	var guardStatus string
	var licenseExpiry sql.NullTime
	err = db.DB.QueryRow("SELECT status, license_expiry FROM guards WHERE id = $1 AND deleted_at IS NULL", guardID).Scan(&guardStatus, &licenseExpiry)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "guard not found"})
			return
		}
		http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
		return
	}
	if guardStatus != "active" {
		http.Error(w, `{"error":"Only active guards can be assigned"}`, http.StatusConflict)
		return
	}
	if licenseExpiry.Valid && !licenseExpiry.Time.After(time.Now()) {
		http.Error(w, `{"error":"Cannot assign a guard with an expired license"}`, http.StatusConflict)
		return
	}

	var requiredGuards int
	err = db.DB.QueryRow("SELECT num_guards FROM queries WHERE id = $1 AND deleted_at IS NULL", req.QueryID).Scan(&requiredGuards)
	if err != nil {
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "query not found"})
			return
		}
		http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
		return
	}
	if requiredGuards <= 0 {
		requiredGuards = 1
	}

	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var activeGuardAssignmentCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM guard_query_assignments WHERE guard_id = $1 AND unassigned_at IS NULL", guardID).Scan(&activeGuardAssignmentCount); err != nil {
		http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
		return
	}
	if activeGuardAssignmentCount > 0 {
		http.Error(w, `{"error":"Guard is already assigned to an active query"}`, http.StatusConflict)
		return
	}

	var assignedToQueryCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM guard_query_assignments WHERE query_id = $1 AND unassigned_at IS NULL", req.QueryID).Scan(&assignedToQueryCount); err != nil {
		http.Error(w, `{"error":"Database error"}`, http.StatusInternalServerError)
		return
	}
	if assignedToQueryCount >= requiredGuards {
		http.Error(w, `{"error":"Query already has the required number of guards assigned"}`, http.StatusConflict)
		return
	}

	if _, err := tx.Exec("INSERT INTO guard_query_assignments (guard_id, query_id) VALUES ($1, $2)", guardID, req.QueryID); err != nil {
		http.Error(w, `{"error":"Failed to assign guard"}`, http.StatusInternalServerError)
		return
	}

	// Audit Log
	userID, _ := r.Context().Value(userIDKey).(float64)
	if err := helpers.WriteAuditLog(tx, int(userID), "assign_guard", "guard:"+strconv.Itoa(guardID), map[string]int{"query_id": req.QueryID}); err != nil {
		log.Printf("ERROR: Failed to write audit log for assign_guard: %v", err)
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, `{"error":"Failed to assign guard"}`, http.StatusInternalServerError)
		return
	}

	// Phase 6: Notify the query owner about guard assignment
	go func() {
		var queryUserID *int
		var queryEmail, queryName string
		err := db.DB.QueryRow(
			"SELECT user_id, email, name FROM queries WHERE id = $1 AND deleted_at IS NULL",
			req.QueryID,
		).Scan(&queryUserID, &queryEmail, &queryName)
		if err != nil {
			log.Printf("WARNING: Could not fetch query owner for guard assignment notification: %v", err)
			return
		}

		msg := "A guard has been assigned to your request"
		if queryUserID != nil {
			_ = helpers.CreateNotification(db.DB, *queryUserID, msg, "success")
		}
		if strings.TrimSpace(queryEmail) != "" {
			greeting := fmt.Sprintf("Hello %s,", queryName)
			emailBody := fmt.Sprintf(
				`<p>Great news! A security guard has been assigned to your service request <strong>#%d</strong>.</p>
				<p>Our team is now preparing to serve you. You will receive further details about the deployment schedule shortly.</p>`,
				req.QueryID,
			)
			footer := "If you have questions, reply to this email or contact our support team."
			services.EnqueueEmail(queryEmail, queryName,
				fmt.Sprintf("Guard Assigned — Reference #%d", req.QueryID),
				services.EmailTemplate(greeting, emailBody, footer),
			)
		}
	}()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"message": "Guard assigned successfully"})
}

// GetExpiringGuards returns guards with license expiring in < 30 days
func GetExpiringGuards(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(`
		SELECT
			g.id,
			g.name,
			g.phone,
			g.email,
			g.address,
			g.license_no,
			g.license_expiry,
			g.status,
			g.hourly_rate,
			g.photo_url,
			a.query_id,
			a.assigned_at,
			g.created_at
		FROM guards g
		LEFT JOIN LATERAL (
			SELECT query_id, assigned_at
			FROM guard_query_assignments
			WHERE guard_id = g.id AND unassigned_at IS NULL
			ORDER BY assigned_at DESC
			LIMIT 1
		) a ON true
		WHERE g.deleted_at IS NULL
		  AND g.license_expiry BETWEEN NOW() AND NOW() + INTERVAL '30 days'
		ORDER BY g.license_expiry ASC`)
	if err != nil {
		http.Error(w, `{"error":"Failed to retrieve guards"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	guards := []Guard{}
	for rows.Next() {
		var g Guard
		if err := rows.Scan(&g.ID, &g.Name, &g.Phone, &g.Email, &g.Address, &g.LicenseNo, &g.LicenseExpiry, &g.Status, &g.HourlyRate, &g.PhotoURL, &g.AssignedQuery, &g.AssignedAt, &g.CreatedAt); err != nil {
			http.Error(w, `{"error":"Failed to scan guard data"}`, http.StatusInternalServerError)
			return
		}
		guards = append(guards, g)
	}
	json.NewEncoder(w).Encode(guards)
}

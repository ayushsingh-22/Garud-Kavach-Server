package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"server/db"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func ensureDB(t *testing.T) {
	t.Helper()
	if db.DB != nil {
		return
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	var err error
	db.DB, err = sql.Open("postgres", databaseURL)
	if err != nil {
		t.Skipf("Could not connect to database: %v", err)
	}
	if err := db.DB.Ping(); err != nil {
		t.Skipf("Could not ping database: %v", err)
	}
}

func cleanupTestUser(email string) {
	if db.DB == nil {
		return
	}
	var userID int
	err := db.DB.QueryRow("SELECT id FROM users WHERE email = $1", email).Scan(&userID)
	if err == nil {
		db.DB.Exec("DELETE FROM customers WHERE user_id = $1", userID)
		db.DB.Exec("DELETE FROM audit_logs WHERE target = $1", fmt.Sprintf("user:%d", userID))
		db.DB.Exec("DELETE FROM users WHERE id = $1", userID)
	}
}

func TestSignUpHandler(t *testing.T) {
	os.Setenv("ADMIN_SECURITY_CODE", "TEST_SECRET_CODE")
	defer os.Unsetenv("ADMIN_SECURITY_CODE")

	t.Run("Invalid accountType returns 400", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Test", "email": "test@test.com", "password": "12345678",
			"accountType": "invalid",
		})
		req, _ := http.NewRequest("POST", "/api/signup", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		SignUpHandler(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("Admin signup with wrong security code returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Test", "email": "test@test.com", "password": "12345678",
			"accountType": "admin", "securityCode": "WRONG_CODE", "adminRole": "manager",
		})
		req, _ := http.NewRequest("POST", "/api/signup", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		SignUpHandler(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}

		var resp map[string]string
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["message"] != "Unauthorized access" {
			t.Errorf("expected 'Unauthorized access', got '%s'", resp["message"])
		}
	})

	t.Run("Admin signup with missing security code returns 401", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Test", "email": "test@test.com", "password": "12345678",
			"accountType": "admin", "adminRole": "manager",
		})
		req, _ := http.NewRequest("POST", "/api/signup", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		SignUpHandler(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Admin signup with invalid adminRole returns 400", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"name": "Test", "email": "test@test.com", "password": "12345678",
			"accountType": "admin", "securityCode": "TEST_SECRET_CODE", "adminRole": "superadmin",
		})
		req, _ := http.NewRequest("POST", "/api/signup", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		SignUpHandler(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rr.Code)
		}
	})

	// --- Integration tests (require database) ---

	t.Run("Customer signup with missing accountType creates customer", func(t *testing.T) {
		ensureDB(t)
		email := fmt.Sprintf("test-cust-%d@test.local", time.Now().UnixNano())
		defer cleanupTestUser(email)

		body, _ := json.Marshal(map[string]string{
			"name": "Test Customer", "email": email, "password": "12345678",
		})
		req, _ := http.NewRequest("POST", "/api/signup", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		SignUpHandler(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]string
		json.Unmarshal(rr.Body.Bytes(), &resp)
		if resp["message"] != "Account created. Please log in." {
			t.Errorf("unexpected message: %s", resp["message"])
		}

		var role string
		err := db.DB.QueryRow("SELECT role FROM users WHERE email = $1 AND deleted_at IS NULL", email).Scan(&role)
		if err != nil {
			t.Fatalf("user not found in DB: %v", err)
		}
		if role != "customer" {
			t.Errorf("expected role 'customer', got '%s'", role)
		}
	})

	for _, adminRole := range []string{"manager", "hr", "finance"} {
		adminRole := adminRole
		t.Run(fmt.Sprintf("Admin signup with correct code creates %s", adminRole), func(t *testing.T) {
			ensureDB(t)
			email := fmt.Sprintf("test-admin-%s-%d@test.local", adminRole, time.Now().UnixNano())
			defer cleanupTestUser(email)

			body, _ := json.Marshal(map[string]string{
				"name": "Test Admin", "email": email, "password": "12345678",
				"accountType": "admin", "securityCode": "TEST_SECRET_CODE", "adminRole": adminRole,
			})
			req, _ := http.NewRequest("POST", "/api/signup", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			SignUpHandler(rr, req)

			if rr.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
			}

			var role string
			err := db.DB.QueryRow("SELECT role FROM users WHERE email = $1 AND deleted_at IS NULL", email).Scan(&role)
			if err != nil {
				t.Fatalf("user not found in DB: %v", err)
			}
			if role != adminRole {
				t.Errorf("expected role '%s', got '%s'", adminRole, role)
			}
		})
	}
}

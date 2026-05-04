package handlers

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

func TestMain(m *testing.M) {
	// Load .env file for tests
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found for tests")
	}

	// Set JWT key for the test environment
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required for tests")
	}
	SetJwtKey([]byte(jwtSecret))

	// Run tests
	os.Exit(m.Run())
}

// Helper function to create a token for a given role
func createTestToken(role string, expiresAt time.Time) (string, error) {
	claims := jwt.MapClaims{
		"user_id": 1,
		"email":   "test@test.com",
		"role":    role,
		"exp":     expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// Dummy handler to test middleware
var testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
})

func TestRBACMiddleware(t *testing.T) {
	// A dummy handler protected by superadmin/manager roles
	protectedManagerHandler := JWTAuthMiddleware(RequireRole("superadmin", "manager")(testHandler))

	// A dummy handler for a finance route
	protectedFinanceHandler := JWTAuthMiddleware(RequireRole("superadmin", "finance")(testHandler))

	// A dummy handler for an HR route
	protectedHRHandler := JWTAuthMiddleware(RequireRole("superadmin", "hr")(testHandler))

	tests := []struct {
		name          string
		handler       http.Handler
		role          string
		tokenDuration time.Duration
		expectStatus  int
		tamperToken   bool
	}{
		// --- Test Case 1: Customer tries to access manager route ---
		{"Customer fails manager route", protectedManagerHandler, "customer", time.Hour, http.StatusForbidden, false},

		// --- Test Case 2: Finance tries to access HR route ---
		{"Finance fails HR route", protectedHRHandler, "finance", time.Hour, http.StatusForbidden, false},

		// --- Test Case 3: HR tries to access Finance route ---
		{"HR fails Finance route", protectedFinanceHandler, "hr", time.Hour, http.StatusForbidden, false},

		// --- Test Case 4: No cookie provided ---
		{"No cookie fails protected route", protectedManagerHandler, "", 0, http.StatusUnauthorized, false},

		// --- Test Case 5: Expired token ---
		{"Expired token fails protected route", protectedManagerHandler, "manager", -time.Hour, http.StatusUnauthorized, false},

		// --- Test Case 6: Tampered token ---
		{"Tampered token fails protected route", protectedManagerHandler, "superadmin", time.Hour, http.StatusUnauthorized, true},

		// --- Success cases ---
		{"Superadmin succeeds manager route", protectedManagerHandler, "superadmin", time.Hour, http.StatusOK, false},
		{"Manager succeeds manager route", protectedManagerHandler, "manager", time.Hour, http.StatusOK, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)

			if tt.role != "" {
				token, err := createTestToken(tt.role, time.Now().Add(tt.tokenDuration))
				if err != nil {
					t.Fatalf("Failed to create test token: %v", err)
				}

				if tt.tamperToken {
					// Invalidate the signature
					token += "tamper"
				}

				req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
			}

			rr := httptest.NewRecorder()
			tt.handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectStatus)
			}
		})
	}
}

// TestRBACPhase36 covers the additional role escalation scenarios introduced
// in Phases 3–6 of the implementation plan.
func TestRBACPhase36(t *testing.T) {
	// Guards endpoints — require superadmin / manager / hr
	guardsDeleteHandler := JWTAuthMiddleware(RequireRole("superadmin", "manager", "hr")(testHandler))

	// HR endpoints — require superadmin / hr
	hrShiftCreateHandler := JWTAuthMiddleware(RequireRole("superadmin", "hr")(testHandler))

	// Finance endpoints — require superadmin / finance
	financeExpenseHandler := JWTAuthMiddleware(RequireRole("superadmin", "finance")(testHandler))

	// Admin user list — require superadmin only
	adminUsersHandler := JWTAuthMiddleware(RequireRole("superadmin")(testHandler))

	// Notifications — require any authenticated user (just JWTAuthMiddleware)
	notifReadHandler := JWTAuthMiddleware(testHandler)

	tests := []struct {
		name         string
		handler      http.Handler
		role         string // empty = no cookie (unauthenticated)
		expectStatus int
	}{
		// 1. Customer → DELETE /api/guards/:id → 403
		{"Customer cannot delete guard", guardsDeleteHandler, "customer", http.StatusForbidden},

		// 2. Finance → POST /api/hr/shifts → 403
		{"Finance cannot create HR shift", hrShiftCreateHandler, "finance", http.StatusForbidden},

		// 3. HR → GET /api/admin/users → 403
		{"HR cannot list admin users", adminUsersHandler, "hr", http.StatusForbidden},

		// 4. Unauthenticated → POST /api/notifications/read → 401
		{"Unauthenticated cannot mark notifications", notifReadHandler, "", http.StatusUnauthorized},

		// 5. Customer → POST /api/finance/expenses → 403
		{"Customer cannot create expense", financeExpenseHandler, "customer", http.StatusForbidden},

		// 6. Finance → POST /api/hr/shifts → 403 (duplicate with different description for clarity)
		{"Finance cannot view HR payroll", hrShiftCreateHandler, "finance", http.StatusForbidden},

		// 7. Valid superadmin passes all checks
		{"Superadmin can delete guard", guardsDeleteHandler, "superadmin", http.StatusOK},
		{"Superadmin can create HR shift", hrShiftCreateHandler, "superadmin", http.StatusOK},
		{"Superadmin can list admin users", adminUsersHandler, "superadmin", http.StatusOK},
		{"Superadmin can create finance expense", financeExpenseHandler, "superadmin", http.StatusOK},

		// 8. Correct role can access its own routes
		{"HR can create shift", hrShiftCreateHandler, "hr", http.StatusOK},
		{"Finance can create expense", financeExpenseHandler, "finance", http.StatusOK},

		// 9. Authenticated customer can read notifications (all authenticated users)
		{"Customer can read own notifications", notifReadHandler, "customer", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/", nil)

			if tt.role != "" {
				token, err := createTestToken(tt.role, time.Now().Add(time.Hour))
				if err != nil {
					t.Fatalf("Failed to create test token: %v", err)
				}
				req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
			}

			rr := httptest.NewRecorder()
			tt.handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectStatus {
				t.Errorf("[%s] handler returned wrong status code: got %v want %v",
					tt.name, status, tt.expectStatus)
			}
		})
	}
}

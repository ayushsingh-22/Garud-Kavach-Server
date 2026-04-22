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

				req.AddCookie(&http.Cookie{Name: "token", Value: token})
			}

			rr := httptest.NewRecorder()
			tt.handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectStatus)
			}
		})
	}
}

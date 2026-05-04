package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"server/db"
	"server/models"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// isProduction returns true when APP_ENV=production is set.
// In dev (HTTP), Secure cookies won't be sent by the browser, causing 401s.
func isProduction() bool {
	return os.Getenv("APP_ENV") == "production"
}

// cookieSameSite returns Strict in production, Lax in development.
// Lax allows cookies to be sent through the Vite proxy on HTTP localhost.
func cookieSameSite() http.SameSite {
	if isProduction() {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}

var jwtKey []byte

// SetJwtKey allows the main application to set the key after loading the environment
func SetJwtKey(key []byte) {
	jwtKey = key
}

func unauthorizedJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
}

func parseTokenClaims(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}

// parseTokenClaimsFromRequest reads the access_token cookie and validates it.
// Falls back to the Authorization: Bearer header, then the legacy "token" cookie.
func parseTokenClaimsFromRequest(r *http.Request) (jwt.MapClaims, error) {
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		return parseTokenClaims(c.Value)
	}
	// Authorization: Bearer <token> header (useful when cookies are blocked in dev)
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		return parseTokenClaims(strings.TrimPrefix(authHeader, "Bearer "))
	}
	// Legacy fallback — remove after all clients have cycled through login once.
	if c, err := r.Cookie("token"); err == nil && c.Value != "" {
		return parseTokenClaims(c.Value)
	}
	return nil, errors.New("missing token cookie")
}

// generateRefreshToken returns a cryptographically random URL-safe token string.
func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// issueAccessToken builds and signs a short-lived (15 min) JWT.
func issueAccessToken(userID int, email, role string) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"role":    role,
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
	})
	return tok.SignedString(jwtKey)
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	var login models.Admin
	if err := json.NewDecoder(r.Body).Decode(&login); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request payload"})
		return
	}

	email := strings.TrimSpace(strings.ToLower(login.Email))
	if email == "" || strings.TrimSpace(login.Password) == "" {
		unauthorizedJSON(w)
		return
	}

	var userID int
	var passwordHash string
	var role string
	err := db.DB.QueryRow(
		"SELECT id, password, role FROM users WHERE email = $1 AND deleted_at IS NULL",
		email,
	).Scan(&userID, &passwordHash, &role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			unauthorizedJSON(w)
			return
		}
		log.Printf("ERROR: Database error during login for email %s: %v", email, err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to verify credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(login.Password)); err != nil {
		unauthorizedJSON(w)
		return
	}

	// ── Access token (15 min) ────────────────────────────────────────────────
	accessTokenString, err := issueAccessToken(userID, email, role)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Could not generate token"})
		return
	}

	// ── Refresh token (7 days, stored as bcrypt hash) ────────────────────────
	rawRefresh, err := generateRefreshToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Could not generate session"})
		return
	}
	refreshHash, err := bcrypt.GenerateFromPassword([]byte(rawRefresh), 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Could not secure session"})
		return
	}
	refreshExpiry := time.Now().Add(7 * 24 * time.Hour)
	_, err = db.DB.Exec(
		"INSERT INTO user_sessions (user_id, refresh_token_hash, expires_at) VALUES ($1, $2, $3)",
		userID, string(refreshHash), refreshExpiry,
	)
	if err != nil {
		log.Printf("ERROR: Failed to store refresh session for user %d: %v", userID, err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Could not create session"})
		return
	}

	// ── Set cookies ──────────────────────────────────────────────────────────
	secure := isProduction()
	sameSite := cookieSameSite()
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessTokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  time.Now().Add(15 * time.Minute),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    rawRefresh,
		Path:     "/api/auth/refresh",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  refreshExpiry,
	})
	// Expire any legacy token cookie so old clients don't carry stale sessions.
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Login successful"})
}

// RefreshTokenHandler issues a new access_token when presented with a valid,
// non-revoked refresh_token cookie. The refresh token itself is NOT rotated
// on every call to keep the implementation simple; it remains valid until it
// expires or is explicitly revoked on logout.
func RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing refresh token"})
		return
	}

	// Scan all non-revoked, non-expired sessions and find the matching hash.
	// We purposely avoid storing the raw token in the DB, so we must check
	// each candidate row individually (bcrypt is non-reversible).
	rows, err := db.DB.Query(
		`SELECT id, user_id, refresh_token_hash FROM user_sessions
		 WHERE revoked_at IS NULL AND expires_at > NOW()`,
	)
	if err != nil {
		log.Printf("ERROR: Failed to query sessions for refresh: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Internal server error"})
		return
	}
	defer rows.Close()

	var sessionID int
	var sessionUserID int
	found := false
	for rows.Next() {
		var sid, uid int
		var hash string
		if err := rows.Scan(&sid, &uid, &hash); err != nil {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(cookie.Value)) == nil {
			sessionID = sid
			sessionUserID = uid
			found = true
			break
		}
	}
	if !found {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or expired refresh token"})
		return
	}

	// Look up the user to get fresh role / email in case they changed.
	var email, role string
	err = db.DB.QueryRow(
		"SELECT email, role FROM users WHERE id = $1 AND deleted_at IS NULL",
		sessionUserID,
	).Scan(&email, &role)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "User not found"})
		return
	}

	accessTokenString, err := issueAccessToken(sessionUserID, email, role)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Could not generate token"})
		return
	}

	// Touch the session last-used timestamp (optional future use) and clear out
	// any other data. For now we just use the session as-is.
	_ = sessionID // suppress unused warning until rotation is added

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    accessTokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   isProduction(),
		SameSite: cookieSameSite(),
		Expires:  time.Now().Add(15 * time.Minute),
	})

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Token refreshed"})
}

func CheckLoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	claims, err := parseTokenClaimsFromRequest(r)
	if err != nil {
		unauthorizedJSON(w)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    claims["user_id"],
		"email": claims["email"],
		"role":  claims["role"],
	})
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	// If there is a refresh_token cookie, revoke the session in the DB so that
	// even an intercepted token cannot be used to obtain new access tokens.
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		rows, qErr := db.DB.Query(
			`SELECT id, refresh_token_hash FROM user_sessions
			 WHERE revoked_at IS NULL AND expires_at > NOW()`,
		)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var sid int
				var hash string
				if scanErr := rows.Scan(&sid, &hash); scanErr != nil {
					continue
				}
				if bcrypt.CompareHashAndPassword([]byte(hash), []byte(cookie.Value)) == nil {
					_, _ = db.DB.Exec(
						"UPDATE user_sessions SET revoked_at = NOW() WHERE id = $1",
						sid,
					)
					break
				}
			}
		}
	}

	expireOpts := []*http.Cookie{
		{Name: "access_token", Path: "/"},
		{Name: "refresh_token", Path: "/api/auth/refresh"},
		{Name: "token", Path: "/"}, // clear legacy cookie
	}
	for _, c := range expireOpts {
		http.SetCookie(w, &http.Cookie{
			Name:     c.Name,
			Value:    "",
			Path:     c.Path,
			HttpOnly: true,
			Secure:   isProduction(),
			SameSite: cookieSameSite(),
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
		})
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
}

// Middleware to protect routes
func JWTAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := parseTokenClaimsFromRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Invalid or expired token"})
			return
		}

		// Add user info to context for downstream handlers
		ctx := r.Context()
		if role, ok := claims["role"].(string); ok {
			ctx = context.WithValue(ctx, userRoleKey, role)
		}
		if userID, ok := claims["user_id"]; ok {
			ctx = context.WithValue(ctx, userIDKey, userID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

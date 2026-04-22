package handlers

import (
	"context"
	"net/http"
)

type contextKey string

const userRoleKey contextKey = "user_role"
const userIDKey contextKey = "user_id"

// RequireRole is a middleware that checks for a specific set of roles in the JWT claims.
func RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := parseTokenClaimsFromRequest(r)
			if err != nil {
				http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			role, ok := claims["role"].(string)
			if !ok {
				http.Error(w, `{"error": "Forbidden: role claim missing"}`, http.StatusForbidden)
				return
			}

			isAllowed := false
			for _, allowedRole := range allowedRoles {
				if role == allowedRole {
					isAllowed = true
					break
				}
			}

			if !isAllowed {
				http.Error(w, `{"error": "Forbidden: insufficient role"}`, http.StatusForbidden)
				return
			}

			// Add user info to context for downstream handlers
			ctx := context.WithValue(r.Context(), userRoleKey, role)
			if userID, ok := claims["user_id"]; ok {
				ctx = context.WithValue(ctx, userIDKey, userID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

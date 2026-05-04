package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// generateNonce creates a cryptographically random base64-encoded nonce string.
func generateNonce() (string, error) {
	b := make([]byte, 18) // 18 bytes → 24-char base64, no padding issues
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// SecurityHeaders adds hardened HTTP security headers to every response and
// exposes the per-request CSP nonce via the X-CSP-Nonce response header so
// the frontend can use it for any server-rendered inline scripts.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := generateNonce()
		if err != nil {
			// Fallback: refuse to serve without a nonce rather than silently
			// weakening CSP by falling back to 'unsafe-inline'.
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Expose the nonce so clients/tests can read it.
		w.Header().Set("X-CSP-Nonce", nonce)

		// Prevent MIME-type sniffing.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Deny framing (clickjacking protection).
		w.Header().Set("X-Frame-Options", "DENY")

		// Force HTTPS for one year (preload-compatible).
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Minimise referrer information sent to third parties.
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Nonce-based CSP — no 'unsafe-inline'.
		// script-src: only same-origin scripts and scripts with the matching nonce.
		// style-src:  same-origin + inline styles (required by many CSS-in-JS libs).
		// object-src: block Flash and other legacy plugins entirely.
		// base-uri:   prevent <base> tag hijacking.
		// form-action: only allow form submissions to the same origin.
		csp := "default-src 'self'; " +
			"script-src 'self' 'nonce-" + nonce + "'; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"img-src 'self' data: https://res.cloudinary.com; " +
			"connect-src 'self'; " +
			"object-src 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r)
	})
}

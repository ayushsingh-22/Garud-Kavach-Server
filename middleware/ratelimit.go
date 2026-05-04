package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"server/services"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type LoginLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	window   time.Duration
}

func NewLoginLimiter() *LoginLimiter {
	rl := &LoginLimiter{
		visitors: make(map[string]*visitor),
		window:   15 * time.Minute,
	}

	go rl.evictStaleVisitors()
	return rl
}

func (rl *LoginLimiter) evictStaleVisitors() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-rl.window)

		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if v.lastSeen.Before(cutoff) {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}

	parts := strings.Split(remoteAddr, ":")
	if len(parts) > 0 {
		return parts[0]
	}

	return remoteAddr
}

func (rl *LoginLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if v, found := rl.visitors[ip]; found {
		v.lastSeen = now
		return v.limiter
	}

	limiter := rate.NewLimiter(rate.Every(3*time.Minute), 5)
	rl.visitors[ip] = &visitor{limiter: limiter, lastSeen: now}
	return limiter
}

func (rl *LoginLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		ip := clientIP(r.RemoteAddr)

		// ── Redis-backed rate limiting (when available) ──────────────────────
		if services.IsRedisAvailable() {
			const maxAttempts = 5
			const window = 3 * time.Minute
			key := fmt.Sprintf("rl:login:%s", ip)
			ctx := context.Background()

			count, err := services.RedisIncrWithExpire(ctx, key, window)
			if err == nil && count > maxAttempts {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "180")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "Too many login attempts. Please try again later."})
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// ── In-memory fallback ───────────────────────────────────────────────
		limiter := rl.getLimiter(ip)

		if !limiter.Allow() {
			reserve := limiter.Reserve()
			retryAfter := int(reserve.Delay().Seconds())
			reserve.Cancel()
			if retryAfter < 1 {
				retryAfter = 1
			}

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Too many login attempts. Please try again later."})
			return
		}

		next.ServeHTTP(w, r)
	})
}

var loginLimiter = NewLoginLimiter()

func LoginRateLimit(next http.Handler) http.Handler {
	return loginLimiter.Middleware(next)
}

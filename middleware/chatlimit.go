package middleware

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// chatVisitor tracks per-IP rate limiting state for the chat endpoint.
type chatVisitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// ChatLimiter provides per-IP rate limiting for the chat endpoint.
// Allows 20 requests per minute with a burst of 5 to handle quick follow-ups.
type ChatLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*chatVisitor
	window   time.Duration
}

// NewChatLimiter creates a new ChatLimiter and starts a background eviction goroutine.
func NewChatLimiter() *ChatLimiter {
	cl := &ChatLimiter{
		visitors: make(map[string]*chatVisitor),
		window:   5 * time.Minute,
	}
	go cl.evictStaleVisitors()
	return cl
}

func (cl *ChatLimiter) evictStaleVisitors() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-cl.window)
		cl.mu.Lock()
		for ip, v := range cl.visitors {
			if v.lastSeen.Before(cutoff) {
				delete(cl.visitors, ip)
			}
		}
		cl.mu.Unlock()
	}
}

func (cl *ChatLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now()
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if v, found := cl.visitors[ip]; found {
		v.lastSeen = now
		return v.limiter
	}
	// 20 requests per minute, burst of 5
	limiter := rate.NewLimiter(rate.Every(3*time.Second), 5)
	cl.visitors[ip] = &chatVisitor{limiter: limiter, lastSeen: now}
	return limiter
}

// Middleware returns an http.Handler middleware that rate-limits chat requests.
func (cl *ChatLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		ip := clientIP(r.RemoteAddr)
		limiter := cl.getLimiter(ip)

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
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Too many chat requests. Please wait a moment before trying again.",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

var chatLimiter = NewChatLimiter()

// ChatRateLimit wraps a handler with the per-IP chat rate limiter.
func ChatRateLimit(next http.Handler) http.Handler {
	return chatLimiter.Middleware(next)
}

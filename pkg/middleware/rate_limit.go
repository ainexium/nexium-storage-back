package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"nexium.ai/api/pkg/apierr"
	"nexium.ai/api/pkg/response"
)

// RateLimitPerProject limits external API calls by project ID (read from context after AuthenticateAPIKey).
// Default: 600 req/min per project (~10 req/s).
func RateLimitPerProject(n int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	windows := map[string]*ipWindow{}

	go func() {
		for range time.Tick(5 * time.Minute) {
			mu.Lock()
			now := time.Now()
			for k, w := range windows {
				if now.After(w.resetAt) {
					delete(windows, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID, ok := GetProjectID(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			key := projectID.String()
			now := time.Now()

			mu.Lock()
			win, ok := windows[key]
			if !ok || now.After(win.resetAt) {
				windows[key] = &ipWindow{count: 1, resetAt: now.Add(window)}
				mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}
			win.count++
			over := win.count > n
			mu.Unlock()

			if over {
				w.Header().Set("Retry-After", win.resetAt.Format(http.TimeFormat))
				response.Error(w, apierr.ErrTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type ipWindow struct {
	count    int
	resetAt  time.Time
}

// RateLimitPerIP allows at most n requests per window duration per client IP.
func RateLimitPerIP(n int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	windows := map[string]*ipWindow{}

	// Periodic cleanup to avoid unbounded map growth
	go func() {
		for range time.Tick(5 * time.Minute) {
			mu.Lock()
			now := time.Now()
			for ip, w := range windows {
				if now.After(w.resetAt) {
					delete(windows, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			now := time.Now()

			mu.Lock()
			win, ok := windows[ip]
			if !ok || now.After(win.resetAt) {
				windows[ip] = &ipWindow{count: 1, resetAt: now.Add(window)}
				mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}
			win.count++
			over := win.count > n
			mu.Unlock()

			if over {
				w.Header().Set("Retry-After", win.resetAt.Format(http.TimeFormat))
				response.Error(w, apierr.ErrTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	// CF-Connecting-IP is set by Cloudflare and cannot be spoofed by the client.
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		return strings.TrimSpace(cfIP)
	}
	// Fallback for local dev (no Cloudflare proxy).
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

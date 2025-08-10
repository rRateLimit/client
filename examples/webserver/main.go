package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rRateLimit/client/ratelimit"
)

// Response represents API response
type Response struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
}

// StatsResponse represents rate limiter statistics
type StatsResponse struct {
	Limiters map[string]int `json:"limiters"`
	Message  string         `json:"message"`
}

func main() {
	fmt.Println("=== Rate Limiting Web Server Example ===")
	fmt.Println("Server starting on :8080")
	fmt.Println()
	fmt.Println("Endpoints:")
	fmt.Println("  GET  /api/public    - Public endpoint (100 req/min per IP)")
	fmt.Println("  GET  /api/user      - User endpoint (50 req/min per user)")
	fmt.Println("  GET  /api/admin     - Admin endpoint (10 req/min, strict)")
	fmt.Println("  GET  /health        - Health check (no rate limit)")
	fmt.Println("  GET  /stats         - Rate limiter statistics")
	fmt.Println()

	// Create different rate limiters for different endpoints
	
	// Public API: IP-based rate limiting
	publicMiddleware := ratelimit.NewMiddleware(&ratelimit.MiddlewareConfig{
		LimiterFactory: func() ratelimit.Limiter {
			return ratelimit.NewTokenBucket(
				ratelimit.WithRate(100),
				ratelimit.WithPeriod(time.Minute),
				ratelimit.WithBurst(10),
			)
		},
		KeyFunc: ratelimit.IPKeyFunc,
		OnRateLimited: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "Rate limit exceeded",
				"message": "Too many requests from this IP. Please try again later.",
				"retry_after": 60,
			})
		},
		CleanupInterval: 5 * time.Minute,
		MaxIdleTime:     10 * time.Minute,
	})

	// User API: User-based rate limiting
	userMiddleware := ratelimit.NewMiddleware(&ratelimit.MiddlewareConfig{
		LimiterFactory: func() ratelimit.Limiter {
			return ratelimit.NewFixedWindow(
				ratelimit.WithRate(50),
				ratelimit.WithPeriod(time.Minute),
			)
		},
		KeyFunc: ratelimit.UserKeyFunc,
		OnRateLimited: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "User rate limit exceeded",
				"message": "You have exceeded your rate limit. Please wait before making more requests.",
				"retry_after": 60,
			})
		},
		CleanupInterval: 5 * time.Minute,
		MaxIdleTime:     10 * time.Minute,
	})

	// Admin API: Strict sliding window rate limiting
	adminMiddleware := ratelimit.NewMiddleware(&ratelimit.MiddlewareConfig{
		LimiterFactory: func() ratelimit.Limiter {
			return ratelimit.NewSlidingWindow(
				ratelimit.WithRate(10),
				ratelimit.WithPeriod(time.Minute),
			)
		},
		KeyFunc: func(r *http.Request) string {
			// Combine user and path for admin endpoints
			user := r.Header.Get("X-Admin-ID")
			if user == "" {
				user = "anonymous"
			}
			return fmt.Sprintf("admin:%s:%s", user, r.URL.Path)
		},
		OnRateLimited: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "Admin rate limit exceeded",
				"message": "Administrative endpoint rate limit reached. This is strictly enforced.",
				"retry_after": 60,
			})
		},
		CleanupInterval: 5 * time.Minute,
		MaxIdleTime:     10 * time.Minute,
	})

	// Setup routes
	mux := http.NewServeMux()

	// Public endpoint
	mux.Handle("/api/public", publicMiddleware.Handler(
		http.HandlerFunc(publicHandler),
	))

	// User endpoint
	mux.Handle("/api/user", userMiddleware.Handler(
		http.HandlerFunc(userHandler),
	))

	// Admin endpoint with wait functionality
	mux.Handle("/api/admin", adminMiddleware.WaitHandler(
		http.HandlerFunc(adminHandler),
		5*time.Second, // Wait up to 5 seconds for rate limit
	))

	// Health check (no rate limiting)
	mux.HandleFunc("/health", healthHandler)

	// Stats endpoint
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		statsHandler(w, r, map[string]*ratelimit.Middleware{
			"public": publicMiddleware,
			"user":   userMiddleware,
			"admin":  adminMiddleware,
		})
	})

	// Add logging middleware
	handler := loggingMiddleware(mux)

	// Start server
	server := &http.Server{
		Addr:         ":8080",
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Fatal(server.ListenAndServe())
}

// Handler functions

func publicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Message:   "Public API response",
		Timestamp: time.Now(),
		Path:      r.URL.Path,
		Method:    r.Method,
	})
}

func userHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "User API response",
		"user_id":   userID,
		"timestamp": time.Now(),
		"path":      r.URL.Path,
		"method":    r.Method,
	})
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	adminID := r.Header.Get("X-Admin-ID")
	if adminID == "" {
		adminID = "anonymous"
	}

	// Simulate some processing time
	time.Sleep(100 * time.Millisecond)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Admin API response",
		"admin_id":  adminID,
		"timestamp": time.Now(),
		"path":      r.URL.Path,
		"method":    r.Method,
		"sensitive": "This is a protected admin endpoint",
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"uptime":    time.Since(startTime).String(),
	})
}

func statsHandler(w http.ResponseWriter, r *http.Request, middlewares map[string]*ratelimit.Middleware) {
	stats := make(map[string]interface{})
	
	for name, mw := range middlewares {
		stats[name] = mw.Stats()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":   "Rate limiter statistics",
		"timestamp": time.Now(),
		"limiters":  stats,
	})
}

// Middleware

var startTime = time.Now()

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		
		next.ServeHTTP(wrapped, r)
		
		duration := time.Since(start)
		log.Printf("[%s] %s %s - Status: %d - Duration: %v - IP: %s",
			time.Now().Format("15:04:05"),
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration,
			r.RemoteAddr,
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
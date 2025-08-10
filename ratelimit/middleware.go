package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// KeyFunc is a function that extracts a key from an HTTP request.
// This key is used to identify and group requests for rate limiting.
type KeyFunc func(r *http.Request) string

// IPKeyFunc returns the client's IP address as the key.
func IPKeyFunc(r *http.Request) string {
	// Try to get real IP from X-Forwarded-For or X-Real-IP headers
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// UserKeyFunc returns a user identifier from the request.
// This example uses a header, but you can customize it.
func UserKeyFunc(r *http.Request) string {
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}
	// Fall back to IP-based limiting
	return IPKeyFunc(r)
}

// PathKeyFunc returns the request path as the key.
func PathKeyFunc(r *http.Request) string {
	return r.URL.Path
}

// MiddlewareConfig configures the rate limiting middleware.
type MiddlewareConfig struct {
	// Limiter is a function that creates a new rate limiter for each key.
	LimiterFactory func() Limiter
	
	// KeyFunc extracts the key from the request.
	KeyFunc KeyFunc
	
	// OnRateLimited is called when a request is rate limited.
	OnRateLimited func(w http.ResponseWriter, r *http.Request)
	
	// CleanupInterval is how often to clean up unused limiters.
	CleanupInterval time.Duration
	
	// MaxIdleTime is how long a limiter can be idle before cleanup.
	MaxIdleTime time.Duration
}

// DefaultMiddlewareConfig returns a default middleware configuration.
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		LimiterFactory: func() Limiter {
			return NewTokenBucket(
				WithRate(100),
				WithPeriod(time.Minute),
				WithBurst(10),
			)
		},
		KeyFunc: IPKeyFunc,
		OnRateLimited: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		},
		CleanupInterval: 5 * time.Minute,
		MaxIdleTime:     10 * time.Minute,
	}
}

// limiterEntry holds a rate limiter and its last access time.
type limiterEntry struct {
	limiter    Limiter
	lastAccess time.Time
}

// Middleware creates an HTTP middleware for rate limiting.
type Middleware struct {
	config   *MiddlewareConfig
	limiters map[string]*limiterEntry
	mu       sync.RWMutex
	done     chan struct{}
}

// NewMiddleware creates a new rate limiting middleware.
func NewMiddleware(config *MiddlewareConfig) *Middleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}
	
	m := &Middleware{
		config:   config,
		limiters: make(map[string]*limiterEntry),
		done:     make(chan struct{}),
	}
	
	// Start cleanup goroutine
	go m.cleanup()
	
	return m
}

// Handler returns an HTTP handler that applies rate limiting.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.config.KeyFunc(r)
		limiter := m.getLimiter(key)
		
		if !limiter.Allow() {
			m.config.OnRateLimited(w, r)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// HandlerFunc returns an HTTP handler function that applies rate limiting.
func (m *Middleware) HandlerFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.Handler(http.HandlerFunc(next)).ServeHTTP
}

// WaitHandler returns an HTTP handler that waits for rate limit availability.
func (m *Middleware) WaitHandler(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.config.KeyFunc(r)
		limiter := m.getLimiter(key)
		
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		
		if err := limiter.Wait(ctx); err != nil {
			if err == context.DeadlineExceeded {
				http.Error(w, "Request timeout while waiting for rate limit", http.StatusRequestTimeout)
			} else {
				http.Error(w, fmt.Sprintf("Rate limit error: %v", err), http.StatusTooManyRequests)
			}
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// getLimiter returns the rate limiter for the given key.
func (m *Middleware) getLimiter(key string) Limiter {
	m.mu.RLock()
	entry, exists := m.limiters[key]
	m.mu.RUnlock()
	
	if exists {
		// Update last access time
		m.mu.Lock()
		entry.lastAccess = time.Now()
		m.mu.Unlock()
		return entry.limiter
	}
	
	// Create new limiter
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Double-check after acquiring write lock
	if entry, exists := m.limiters[key]; exists {
		entry.lastAccess = time.Now()
		return entry.limiter
	}
	
	limiter := m.config.LimiterFactory()
	m.limiters[key] = &limiterEntry{
		limiter:    limiter,
		lastAccess: time.Now(),
	}
	
	return limiter
}

// cleanup periodically removes idle limiters.
func (m *Middleware) cleanup() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			m.cleanupIdle()
		case <-m.done:
			return
		}
	}
}

// cleanupIdle removes limiters that haven't been accessed recently.
func (m *Middleware) cleanupIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	for key, entry := range m.limiters {
		if now.Sub(entry.lastAccess) > m.config.MaxIdleTime {
			delete(m.limiters, key)
		}
	}
}

// Close stops the cleanup goroutine and releases resources.
func (m *Middleware) Close() {
	close(m.done)
}

// Stats returns statistics about the current limiters.
func (m *Middleware) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stats := make(map[string]int)
	for key, entry := range m.limiters {
		stats[key] = entry.limiter.Available()
	}
	
	return stats
}
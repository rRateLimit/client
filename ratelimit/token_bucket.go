package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TokenBucket implements the token bucket rate limiting algorithm.
// It allows bursts of traffic while maintaining an average rate.
type TokenBucket struct {
	config       *Config
	tokens       float64
	lastRefill   time.Time
	mu           sync.Mutex
	refillAmount float64
	refillPeriod time.Duration
}

// NewTokenBucket creates a new TokenBucket rate limiter.
func NewTokenBucket(opts ...Option) *TokenBucket {
	cfg := NewConfig(opts...)
	
	if cfg.Burst == 0 {
		cfg.Burst = cfg.Rate
	}
	
	refillPeriod := cfg.Period / time.Duration(cfg.Rate)
	
	return &TokenBucket{
		config:       cfg,
		tokens:       float64(cfg.Burst),
		lastRefill:   cfg.Clock.Now(),
		refillAmount: 1.0,
		refillPeriod: refillPeriod,
	}
}

// Allow checks if a single request can proceed.
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN checks if n requests can proceed.
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	tb.refill()
	
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	
	return false
}

// Wait blocks until a request can proceed or context is cancelled.
func (tb *TokenBucket) Wait(ctx context.Context) error {
	return tb.WaitN(ctx, 1)
}

// WaitN blocks until n requests can proceed or context is cancelled.
func (tb *TokenBucket) WaitN(ctx context.Context, n int) error {
	if n > tb.config.Burst {
		return fmt.Errorf("requested tokens %d exceeds burst size %d", n, tb.config.Burst)
	}
	
	for {
		tb.mu.Lock()
		tb.refill()
		
		if tb.tokens >= float64(n) {
			tb.tokens -= float64(n)
			tb.mu.Unlock()
			return nil
		}
		
		// Calculate wait time for required tokens
		tokensNeeded := float64(n) - tb.tokens
		waitDuration := time.Duration(tokensNeeded * float64(tb.refillPeriod))
		tb.mu.Unlock()
		
		// Wait with context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tb.config.Clock.After(waitDuration):
			// Continue to next iteration
		}
	}
}

// Reset resets the rate limiter to its initial state.
func (tb *TokenBucket) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	tb.tokens = float64(tb.config.Burst)
	tb.lastRefill = tb.config.Clock.Now()
}

// Available returns the number of available tokens.
func (tb *TokenBucket) Available() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	tb.refill()
	return int(tb.tokens)
}

// refill adds tokens based on elapsed time since last refill.
func (tb *TokenBucket) refill() {
	now := tb.config.Clock.Now()
	elapsed := now.Sub(tb.lastRefill)
	
	// Calculate tokens to add based on elapsed time
	tokensToAdd := elapsed.Seconds() / tb.refillPeriod.Seconds() * tb.refillAmount
	
	if tokensToAdd > 0 {
		tb.tokens = min(tb.tokens+tokensToAdd, float64(tb.config.Burst))
		tb.lastRefill = now
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
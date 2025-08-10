package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FixedWindow implements the fixed window rate limiting algorithm.
// It tracks requests within fixed time windows.
type FixedWindow struct {
	config      *Config
	count       int
	windowStart time.Time
	mu          sync.Mutex
}

// NewFixedWindow creates a new FixedWindow rate limiter.
func NewFixedWindow(opts ...Option) *FixedWindow {
	cfg := NewConfig(opts...)
	
	return &FixedWindow{
		config:      cfg,
		count:       0,
		windowStart: cfg.Clock.Now(),
	}
}

// Allow checks if a single request can proceed.
func (fw *FixedWindow) Allow() bool {
	return fw.AllowN(1)
}

// AllowN checks if n requests can proceed.
func (fw *FixedWindow) AllowN(n int) bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	fw.resetIfNewWindow()
	
	if fw.count+n <= fw.config.Rate {
		fw.count += n
		return true
	}
	
	return false
}

// Wait blocks until a request can proceed or context is cancelled.
func (fw *FixedWindow) Wait(ctx context.Context) error {
	return fw.WaitN(ctx, 1)
}

// WaitN blocks until n requests can proceed or context is cancelled.
func (fw *FixedWindow) WaitN(ctx context.Context, n int) error {
	if n > fw.config.Rate {
		return fmt.Errorf("requested %d exceeds rate limit %d", n, fw.config.Rate)
	}
	
	for {
		fw.mu.Lock()
		fw.resetIfNewWindow()
		
		if fw.count+n <= fw.config.Rate {
			fw.count += n
			fw.mu.Unlock()
			return nil
		}
		
		// Calculate wait time until next window
		nextWindow := fw.windowStart.Add(fw.config.Period)
		waitDuration := nextWindow.Sub(fw.config.Clock.Now())
		fw.mu.Unlock()
		
		// Wait with context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-fw.config.Clock.After(waitDuration):
			// Continue to next iteration
		}
	}
}

// Reset resets the rate limiter to its initial state.
func (fw *FixedWindow) Reset() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	fw.count = 0
	fw.windowStart = fw.config.Clock.Now()
}

// Available returns the number of available requests in the current window.
func (fw *FixedWindow) Available() int {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	fw.resetIfNewWindow()
	available := fw.config.Rate - fw.count
	if available < 0 {
		return 0
	}
	return available
}

// resetIfNewWindow checks if we've moved to a new window and resets if needed.
func (fw *FixedWindow) resetIfNewWindow() {
	now := fw.config.Clock.Now()
	windowEnd := fw.windowStart.Add(fw.config.Period)
	
	if now.After(windowEnd) || now.Equal(windowEnd) {
		// Calculate how many windows have passed
		windowsPassed := int(now.Sub(fw.windowStart) / fw.config.Period)
		fw.windowStart = fw.windowStart.Add(time.Duration(windowsPassed) * fw.config.Period)
		fw.count = 0
	}
}
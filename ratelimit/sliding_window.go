package ratelimit

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

// SlidingWindow implements the sliding window rate limiting algorithm.
// It provides more accurate rate limiting than fixed window by tracking
// individual request timestamps.
type SlidingWindow struct {
	config    *Config
	requests  *list.List
	mu        sync.Mutex
}

// requestTime represents a request with its timestamp and count.
type requestTime struct {
	time  time.Time
	count int
}

// NewSlidingWindow creates a new SlidingWindow rate limiter.
func NewSlidingWindow(opts ...Option) *SlidingWindow {
	cfg := NewConfig(opts...)
	
	return &SlidingWindow{
		config:   cfg,
		requests: list.New(),
	}
}

// Allow checks if a single request can proceed.
func (sw *SlidingWindow) Allow() bool {
	return sw.AllowN(1)
}

// AllowN checks if n requests can proceed.
func (sw *SlidingWindow) AllowN(n int) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	now := sw.config.Clock.Now()
	sw.removeOldRequests(now)
	
	currentCount := sw.countRequests()
	if currentCount+n <= sw.config.Rate {
		sw.requests.PushBack(&requestTime{
			time:  now,
			count: n,
		})
		return true
	}
	
	return false
}

// Wait blocks until a request can proceed or context is cancelled.
func (sw *SlidingWindow) Wait(ctx context.Context) error {
	return sw.WaitN(ctx, 1)
}

// WaitN blocks until n requests can proceed or context is cancelled.
func (sw *SlidingWindow) WaitN(ctx context.Context, n int) error {
	if n > sw.config.Rate {
		return fmt.Errorf("requested %d exceeds rate limit %d", n, sw.config.Rate)
	}
	
	for {
		sw.mu.Lock()
		now := sw.config.Clock.Now()
		sw.removeOldRequests(now)
		
		currentCount := sw.countRequests()
		if currentCount+n <= sw.config.Rate {
			sw.requests.PushBack(&requestTime{
				time:  now,
				count: n,
			})
			sw.mu.Unlock()
			return nil
		}
		
		// Calculate wait time based on oldest request
		var waitDuration time.Duration
		if sw.requests.Len() > 0 {
			oldest := sw.requests.Front().Value.(*requestTime)
			waitDuration = sw.config.Period - now.Sub(oldest.time)
		} else {
			waitDuration = time.Millisecond * 10 // Small wait if no requests
		}
		sw.mu.Unlock()
		
		// Wait with context
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sw.config.Clock.After(waitDuration):
			// Continue to next iteration
		}
	}
}

// Reset resets the rate limiter to its initial state.
func (sw *SlidingWindow) Reset() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	sw.requests.Init()
}

// Available returns the number of available requests in the current window.
func (sw *SlidingWindow) Available() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	
	now := sw.config.Clock.Now()
	sw.removeOldRequests(now)
	
	available := sw.config.Rate - sw.countRequests()
	if available < 0 {
		return 0
	}
	return available
}

// removeOldRequests removes requests outside the current window.
func (sw *SlidingWindow) removeOldRequests(now time.Time) {
	windowStart := now.Add(-sw.config.Period)
	
	// Remove all requests older than the window
	for sw.requests.Len() > 0 {
		front := sw.requests.Front()
		req := front.Value.(*requestTime)
		
		if req.time.Before(windowStart) {
			sw.requests.Remove(front)
		} else {
			break
		}
	}
}

// countRequests counts the total number of requests in the list.
func (sw *SlidingWindow) countRequests() int {
	count := 0
	for e := sw.requests.Front(); e != nil; e = e.Next() {
		req := e.Value.(*requestTime)
		count += req.count
	}
	return count
}
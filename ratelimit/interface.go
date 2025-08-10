// Package ratelimit provides rate limiting functionality for Go applications.
// It includes multiple algorithms such as Token Bucket, Fixed Window, and Sliding Window.
package ratelimit

import (
	"context"
	"time"
)

// Limiter is the core interface for all rate limiting implementations.
type Limiter interface {
	// Allow checks if a single request can proceed.
	Allow() bool

	// AllowN checks if n requests can proceed.
	AllowN(n int) bool

	// Wait blocks until a request can proceed or context is cancelled.
	Wait(ctx context.Context) error

	// WaitN blocks until n requests can proceed or context is cancelled.
	WaitN(ctx context.Context, n int) error

	// Reset resets the rate limiter to its initial state.
	Reset()

	// Available returns the number of available tokens/requests.
	Available() int
}

// Config represents the common configuration for rate limiters.
type Config struct {
	// Rate is the number of requests allowed per period.
	Rate int

	// Period is the time period for the rate limit.
	Period time.Duration

	// Burst is the maximum number of requests that can be made at once.
	// This is mainly used by token bucket algorithm.
	Burst int

	// Clock allows for custom time source (useful for testing).
	Clock Clock
}

// Clock is an interface for time operations, allowing for testing.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
	After(d time.Duration) <-chan time.Time
}

// SystemClock implements Clock using the system time.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time {
	return time.Now()
}

// Sleep pauses the current goroutine for at least the duration d.
func (SystemClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// After waits for the duration to elapse and then sends the current time.
func (SystemClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// DefaultConfig returns a default configuration with reasonable values.
func DefaultConfig() *Config {
	return &Config{
		Rate:   100,
		Period: time.Second,
		Burst:  10,
		Clock:  SystemClock{},
	}
}

// Option is a function that modifies a Config.
type Option func(*Config)

// WithRate sets the rate limit.
func WithRate(rate int) Option {
	return func(c *Config) {
		c.Rate = rate
	}
}

// WithPeriod sets the time period.
func WithPeriod(period time.Duration) Option {
	return func(c *Config) {
		c.Period = period
	}
}

// WithBurst sets the burst size.
func WithBurst(burst int) Option {
	return func(c *Config) {
		c.Burst = burst
	}
}

// WithClock sets a custom clock implementation.
func WithClock(clock Clock) Option {
	return func(c *Config) {
		c.Clock = clock
	}
}

// NewConfig creates a new configuration with the given options.
func NewConfig(opts ...Option) *Config {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
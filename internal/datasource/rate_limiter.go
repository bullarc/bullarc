package datasource

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
)

// RateLimiterConfig holds configuration for a RateLimiter.
type RateLimiterConfig struct {
	// Rate is the number of tokens replenished per second.
	Rate float64
	// Burst is the maximum number of tokens the bucket can hold.
	Burst int
}

// RateLimiter implements a token bucket rate limiter. Tokens are replenished
// at a steady rate up to a burst capacity. Callers can either block until a
// token is available (Wait) or check availability without blocking (Allow).
type RateLimiter struct {
	mu     sync.Mutex
	config RateLimiterConfig

	tokens   float64
	lastTime time.Time

	// now is a clock function for testing. Defaults to time.Now.
	now func() time.Time
}

// NewRateLimiter creates a RateLimiter with the given configuration.
// Rate must be > 0 and Burst must be >= 1.
func NewRateLimiter(cfg RateLimiterConfig) (*RateLimiter, error) {
	if cfg.Rate <= 0 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("rate must be > 0, got %f", cfg.Rate))
	}
	if cfg.Burst < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("burst must be >= 1, got %d", cfg.Burst))
	}
	now := time.Now()
	return &RateLimiter{
		config:   cfg,
		tokens:   float64(cfg.Burst),
		lastTime: now,
		now:      time.Now,
	}, nil
}

// Allow reports whether a single token is available and consumes it if so.
// This method never blocks. Returns false when no tokens are available.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refillLocked()
	if rl.tokens >= 1.0 {
		rl.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available or ctx is cancelled. Returns
// bullarc.ErrRateLimitExceeded wrapping the context error when the context
// is done before a token becomes available.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		rl.mu.Lock()
		rl.refillLocked()
		if rl.tokens >= 1.0 {
			rl.tokens--
			rl.mu.Unlock()
			return nil
		}
		// Calculate how long until the next token arrives.
		deficit := 1.0 - rl.tokens
		waitDur := time.Duration(deficit / rl.config.Rate * float64(time.Second))
		if waitDur < time.Millisecond {
			waitDur = time.Millisecond
		}
		rl.mu.Unlock()

		select {
		case <-ctx.Done():
			return bullarc.ErrRateLimitExceeded.Wrap(
				fmt.Errorf("context cancelled while waiting for rate limiter: %w", ctx.Err()))
		case <-time.After(waitDur):
			// Loop back to try acquiring a token.
		}
	}
}

// refillLocked adds tokens based on elapsed time since the last refill.
// Must be called with rl.mu held.
func (rl *RateLimiter) refillLocked() {
	now := rl.now()
	elapsed := now.Sub(rl.lastTime)
	if elapsed <= 0 {
		return
	}
	rl.lastTime = now
	rl.tokens += elapsed.Seconds() * rl.config.Rate
	max := float64(rl.config.Burst)
	if rl.tokens > max {
		rl.tokens = max
	}
}

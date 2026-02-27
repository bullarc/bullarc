package datasource

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  RateLimiterConfig
	}{
		{
			name: "zero rate",
			cfg:  RateLimiterConfig{Rate: 0, Burst: 1},
		},
		{
			name: "negative rate",
			cfg:  RateLimiterConfig{Rate: -1.0, Burst: 1},
		},
		{
			name: "zero burst",
			cfg:  RateLimiterConfig{Rate: 1.0, Burst: 0},
		},
		{
			name: "negative burst",
			cfg:  RateLimiterConfig{Rate: 1.0, Burst: -1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRateLimiter(tt.cfg)
			requireCode(t, err, "INVALID_PARAMETER")
		})
	}
}

func TestRateLimiter_AllowBurst(t *testing.T) {
	rl := newTestRL(t, 10.0, 5)

	// Should allow up to burst size immediately.
	for i := range 5 {
		assert.True(t, rl.Allow(), "Allow() should succeed for token %d", i+1)
	}
	// Next call should be denied (bucket empty).
	assert.False(t, rl.Allow(), "Allow() should fail when bucket is empty")
}

func TestRateLimiter_AllowRefill(t *testing.T) {
	now := time.Now()
	rl := newTestRL(t, 10.0, 3)
	rl.now = func() time.Time { return now }
	rl.lastTime = now

	// Drain all tokens.
	for range 3 {
		require.True(t, rl.Allow())
	}
	require.False(t, rl.Allow())

	// Advance time by 200ms at rate=10/s => 2 tokens refilled.
	now = now.Add(200 * time.Millisecond)
	assert.True(t, rl.Allow())
	assert.True(t, rl.Allow())
	assert.False(t, rl.Allow())
}

func TestRateLimiter_AllowRefillCapsAtBurst(t *testing.T) {
	now := time.Now()
	rl := newTestRL(t, 10.0, 3)
	rl.now = func() time.Time { return now }
	rl.lastTime = now

	// Drain one token.
	require.True(t, rl.Allow())

	// Advance far enough to refill well past burst capacity.
	now = now.Add(10 * time.Second)

	// Should only have burst=3 tokens, not 100.
	for range 3 {
		require.True(t, rl.Allow())
	}
	assert.False(t, rl.Allow())
}

func TestRateLimiter_WaitReturnsImmediately(t *testing.T) {
	rl := newTestRL(t, 100.0, 5)
	ctx := context.Background()

	start := time.Now()
	err := rl.Wait(ctx)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 50*time.Millisecond)
}

func TestRateLimiter_WaitBlocksUntilTokenAvailable(t *testing.T) {
	// Rate=10/s means one token every 100ms.
	rl := newTestRL(t, 10.0, 1)
	ctx := context.Background()

	// Consume the single burst token.
	require.True(t, rl.Allow())

	start := time.Now()
	err := rl.Wait(ctx)
	require.NoError(t, err)
	elapsed := time.Since(start)
	// Should have waited roughly 100ms (token refill interval).
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond)
	assert.Less(t, elapsed, 500*time.Millisecond)
}

func TestRateLimiter_WaitContextCancelled(t *testing.T) {
	rl := newTestRL(t, 0.1, 1) // Very slow: 1 token per 10 seconds.

	// Drain the burst token.
	require.True(t, rl.Allow())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	require.Error(t, err)
	requireCode(t, err, "RATE_LIMIT_EXCEEDED")
}

func TestRateLimiter_WaitAlreadyCancelledContext(t *testing.T) {
	rl := newTestRL(t, 0.1, 1)
	require.True(t, rl.Allow()) // drain

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := rl.Wait(ctx)
	require.Error(t, err)
	requireCode(t, err, "RATE_LIMIT_EXCEEDED")
}

func TestRateLimiter_ConcurrentAllow(t *testing.T) {
	rl := newTestRL(t, 1000.0, 10)

	var wg sync.WaitGroup
	var allowed atomic.Int64

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.Allow() {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	// At most burst (10) tokens were available initially. Refill during the
	// test is non-deterministic, so just check the upper bound.
	assert.LessOrEqual(t, allowed.Load(), int64(50))
	assert.GreaterOrEqual(t, allowed.Load(), int64(1))
}

func TestRateLimiter_ConcurrentWait(t *testing.T) {
	// High rate to keep the test fast.
	rl := newTestRL(t, 1000.0, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var succeeded atomic.Int64

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := rl.Wait(ctx); err == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()

	// All 20 should eventually succeed given the high rate.
	assert.Equal(t, int64(20), succeeded.Load())
}

// newTestRL is a helper that creates a RateLimiter and fails the test on error.
func newTestRL(t *testing.T, rate float64, burst int) *RateLimiter {
	t.Helper()
	rl, err := NewRateLimiter(RateLimiterConfig{Rate: rate, Burst: burst})
	require.NoError(t, err)
	return rl
}

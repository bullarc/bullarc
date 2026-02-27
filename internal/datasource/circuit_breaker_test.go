package datasource

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  CircuitBreakerConfig
	}{
		{
			name: "zero failure threshold",
			cfg:  CircuitBreakerConfig{FailureThreshold: 0, OpenTimeout: time.Second, HalfOpenSuccesses: 1},
		},
		{
			name: "negative failure threshold",
			cfg:  CircuitBreakerConfig{FailureThreshold: -1, OpenTimeout: time.Second, HalfOpenSuccesses: 1},
		},
		{
			name: "zero open timeout",
			cfg:  CircuitBreakerConfig{FailureThreshold: 3, OpenTimeout: 0, HalfOpenSuccesses: 1},
		},
		{
			name: "negative open timeout",
			cfg:  CircuitBreakerConfig{FailureThreshold: 3, OpenTimeout: -time.Second, HalfOpenSuccesses: 1},
		},
		{
			name: "zero half-open successes",
			cfg:  CircuitBreakerConfig{FailureThreshold: 3, OpenTimeout: time.Second, HalfOpenSuccesses: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCircuitBreaker(tt.cfg)
			requireCode(t, err, "INVALID_PARAMETER")
		})
	}
}

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := newTestCB(t, 3, time.Minute, 1)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_SuccessKeepsClosed(t *testing.T) {
	cb := newTestCB(t, 3, time.Minute, 1)

	for range 5 {
		err := cb.Execute(func() error { return nil })
		require.NoError(t, err)
	}
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_FailuresBelowThresholdStayClosed(t *testing.T) {
	cb := newTestCB(t, 3, time.Minute, 1)
	sentinel := errors.New("transient")

	for range 2 {
		_ = cb.Execute(func() error { return sentinel })
	}
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := newTestCB(t, 3, time.Minute, 1)
	sentinel := errors.New("transient")

	// Two failures, then a success resets the counter.
	_ = cb.Execute(func() error { return sentinel })
	_ = cb.Execute(func() error { return sentinel })
	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)

	// Two more failures should not trip (total < threshold after reset).
	_ = cb.Execute(func() error { return sentinel })
	_ = cb.Execute(func() error { return sentinel })
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_TripsOpenAfterThreshold(t *testing.T) {
	cb := newTestCB(t, 3, time.Minute, 1)
	sentinel := errors.New("transient")

	for range 3 {
		_ = cb.Execute(func() error { return sentinel })
	}
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_OpenRejectsWithoutCallingFn(t *testing.T) {
	cb := newTestCB(t, 1, time.Minute, 1)
	_ = cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, CircuitOpen, cb.State())

	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.False(t, called, "fn should not be called when circuit is open")
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestCircuitBreaker_OpenToHalfOpenAfterTimeout(t *testing.T) {
	now := time.Now()
	cb := newTestCB(t, 1, 5*time.Second, 1)
	cb.now = func() time.Time { return now }

	_ = cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, CircuitOpen, cb.State())

	// Advance time past the open timeout.
	now = now.Add(6 * time.Second)
	assert.Equal(t, CircuitHalfOpen, cb.State())
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	now := time.Now()
	cb := newTestCB(t, 1, 5*time.Second, 2)
	cb.now = func() time.Time { return now }

	_ = cb.Execute(func() error { return errors.New("fail") })
	require.Equal(t, CircuitOpen, cb.State())

	// Advance to half-open.
	now = now.Add(6 * time.Second)
	require.Equal(t, CircuitHalfOpen, cb.State())

	// First success in half-open stays half-open (need 2).
	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CircuitHalfOpen, cb.State())

	// Second success closes the circuit.
	err = cb.Execute(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	now := time.Now()
	cb := newTestCB(t, 1, 5*time.Second, 2)
	cb.now = func() time.Time { return now }

	_ = cb.Execute(func() error { return errors.New("fail") })
	now = now.Add(6 * time.Second)
	require.Equal(t, CircuitHalfOpen, cb.State())

	// Failure in half-open re-opens immediately.
	err := cb.Execute(func() error { return errors.New("still broken") })
	require.Error(t, err)
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_FullLifecycle(t *testing.T) {
	now := time.Now()
	cb := newTestCB(t, 2, 10*time.Second, 1)
	cb.now = func() time.Time { return now }

	// Closed -> Open (2 failures).
	_ = cb.Execute(func() error { return errors.New("1") })
	_ = cb.Execute(func() error { return errors.New("2") })
	require.Equal(t, CircuitOpen, cb.State())

	// Open -> Half-Open (timeout elapses).
	now = now.Add(11 * time.Second)
	require.Equal(t, CircuitHalfOpen, cb.State())

	// Half-Open -> Closed (1 success needed).
	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State())

	// Closed again -> confirm normal operation.
	err = cb.Execute(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, CircuitClosed, cb.State())
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := newTestCB(t, 100, time.Minute, 1)

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(func() error {
				return errors.New("fail")
			})
			if err != nil {
				errorCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	// All 200 calls should have returned errors (all fail).
	assert.Equal(t, int64(200), errorCount.Load())
	assert.Equal(t, int64(0), successCount.Load())
	// After 200 failures with threshold=100, breaker should be open.
	assert.Equal(t, CircuitOpen, cb.State())
}

func TestCircuitBreaker_ConcurrentMixedCalls(t *testing.T) {
	cb := newTestCB(t, 50, time.Minute, 1)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				_ = cb.Execute(func() error { return nil })
			} else {
				_ = cb.Execute(func() error { return errors.New("fail") })
			}
		}(i)
	}
	wg.Wait()
	// Just verify no panics or races occurred.
	_ = cb.State()
}

func TestCircuitState_String(t *testing.T) {
	assert.Equal(t, "closed", CircuitClosed.String())
	assert.Equal(t, "open", CircuitOpen.String())
	assert.Equal(t, "half-open", CircuitHalfOpen.String())
	assert.Equal(t, "unknown", CircuitState(99).String())
}

// newTestCB is a helper that creates a CircuitBreaker and fails the test on error.
func newTestCB(t *testing.T, threshold int, timeout time.Duration, halfOpenSuccesses int) *CircuitBreaker {
	t.Helper()
	cb, err := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold:  threshold,
		OpenTimeout:       timeout,
		HalfOpenSuccesses: halfOpenSuccesses,
	})
	require.NoError(t, err)
	return cb
}

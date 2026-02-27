package datasource

import (
	"fmt"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal operating state where requests pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen rejects all requests immediately without executing them.
	CircuitOpen
	// CircuitHalfOpen allows a limited number of probe requests to test recovery.
	CircuitHalfOpen
)

// String returns a human-readable representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for a CircuitBreaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures that trips the
	// breaker from Closed to Open.
	FailureThreshold int
	// OpenTimeout is how long the breaker stays Open before transitioning to
	// Half-Open to probe for recovery.
	OpenTimeout time.Duration
	// HalfOpenSuccesses is the number of consecutive successes in Half-Open
	// state required to transition back to Closed.
	HalfOpenSuccesses int
}

// CircuitBreaker implements the circuit breaker resilience pattern. It wraps
// function calls and short-circuits when the downstream dependency is failing,
// preventing cascading failures and giving the dependency time to recover.
type CircuitBreaker struct {
	mu     sync.Mutex
	config CircuitBreakerConfig

	state             CircuitState
	consecutiveFails  int
	halfOpenSuccesses int
	openedAt          time.Time

	// now is a clock function for testing. Defaults to time.Now.
	now func() time.Time
}

// NewCircuitBreaker creates a CircuitBreaker with the given configuration.
// FailureThreshold must be >= 1, OpenTimeout must be > 0, and
// HalfOpenSuccesses must be >= 1.
func NewCircuitBreaker(cfg CircuitBreakerConfig) (*CircuitBreaker, error) {
	if cfg.FailureThreshold < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("failure threshold must be >= 1, got %d", cfg.FailureThreshold))
	}
	if cfg.OpenTimeout <= 0 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("open timeout must be > 0, got %s", cfg.OpenTimeout))
	}
	if cfg.HalfOpenSuccesses < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("half-open successes must be >= 1, got %d", cfg.HalfOpenSuccesses))
	}
	return &CircuitBreaker{
		config: cfg,
		state:  CircuitClosed,
		now:    time.Now,
	}, nil
}

// State returns the current circuit breaker state. The state may transition
// from Open to Half-Open if the open timeout has elapsed.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkOpenTimeoutLocked()
	return cb.state
}

// Execute runs fn through the circuit breaker. When the breaker is Open,
// fn is not called and an error is returned immediately. In the Half-Open
// state, fn is executed as a probe; successes move toward Closed while a
// failure re-opens the breaker.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	cb.checkOpenTimeoutLocked()

	switch cb.state {
	case CircuitOpen:
		cb.mu.Unlock()
		return bullarc.ErrDataSourceUnavailable.Wrap(
			fmt.Errorf("circuit breaker is open"))

	case CircuitHalfOpen:
		cb.mu.Unlock()
		err := fn()
		cb.mu.Lock()
		defer cb.mu.Unlock()
		if err != nil {
			cb.tripLocked()
			return err
		}
		cb.halfOpenSuccesses++
		if cb.halfOpenSuccesses >= cb.config.HalfOpenSuccesses {
			cb.resetLocked()
		}
		return nil

	default: // CircuitClosed
		cb.mu.Unlock()
		err := fn()
		cb.mu.Lock()
		defer cb.mu.Unlock()
		if err != nil {
			cb.consecutiveFails++
			if cb.consecutiveFails >= cb.config.FailureThreshold {
				cb.tripLocked()
			}
			return err
		}
		cb.consecutiveFails = 0
		return nil
	}
}

// checkOpenTimeoutLocked transitions from Open to Half-Open when the timeout
// has elapsed. Must be called with cb.mu held.
func (cb *CircuitBreaker) checkOpenTimeoutLocked() {
	if cb.state == CircuitOpen && cb.now().Sub(cb.openedAt) >= cb.config.OpenTimeout {
		cb.state = CircuitHalfOpen
		cb.halfOpenSuccesses = 0
	}
}

// tripLocked transitions to Open state. Must be called with cb.mu held.
func (cb *CircuitBreaker) tripLocked() {
	cb.state = CircuitOpen
	cb.openedAt = cb.now()
	cb.consecutiveFails = 0
	cb.halfOpenSuccesses = 0
}

// resetLocked transitions back to Closed state. Must be called with cb.mu held.
func (cb *CircuitBreaker) resetLocked() {
	cb.state = CircuitClosed
	cb.consecutiveFails = 0
	cb.halfOpenSuccesses = 0
}

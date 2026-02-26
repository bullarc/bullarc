package datasource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"
)

// retryConfig controls retry behaviour for transient errors.
type retryConfig struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// defaultRetryConfig is the retry configuration used by HTTP data sources.
var defaultRetryConfig = retryConfig{
	maxAttempts: 3,
	baseDelay:   time.Second,
	maxDelay:    30 * time.Second,
}

// httpStatusError represents a non-2xx HTTP response.
type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// isRetryable reports whether err should trigger a retry attempt.
// Only transient server-side and rate-limit errors are retried.
func isRetryable(err error) bool {
	var he *httpStatusError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 429, 500, 502, 503, 504:
			return true
		}
	}
	return false
}

// withRetry calls fn up to cfg.maxAttempts times, backing off exponentially
// between attempts on retryable errors. Context cancellation stops retries immediately.
func withRetry(ctx context.Context, cfg retryConfig, fn func() error) error {
	var last error
	for attempt := range cfg.maxAttempts {
		if err := ctx.Err(); err != nil {
			return err
		}

		last = fn()
		if last == nil {
			return nil
		}

		if !isRetryable(last) {
			return last
		}

		if attempt == cfg.maxAttempts-1 {
			break
		}

		delay := jitteredDelay(cfg.baseDelay, cfg.maxDelay, attempt)
		slog.Debug("retrying after transient error",
			"attempt", attempt+1,
			"max", cfg.maxAttempts,
			"delay", delay,
			"err", last)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return last
}

// jitteredDelay returns an exponential backoff duration with ±25% random jitter.
func jitteredDelay(base, max time.Duration, attempt int) time.Duration {
	shift := attempt
	if shift > 10 {
		shift = 10 // cap exponent to prevent overflow
	}
	d := base * (1 << shift)
	if d > max {
		d = max
	}
	// apply ±25% jitter
	quarter := int64(d) / 4
	if quarter > 0 {
		d += time.Duration(rand.Int63n(quarter*2) - quarter)
	}
	if d < base/2 {
		d = base / 2
	}
	return d
}

package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/bullarc/bullarc"
)

// Watch polls for new market data at pollInterval and calls onResult whenever
// a new bar is detected. The initial analysis runs synchronously before the
// first poll tick. Watch returns when ctx is cancelled (ctx.Err()) or, in the
// unlikely case of a tick-channel race, nil.
//
// onResult is called from the same goroutine as Watch; the caller must not
// block inside onResult for longer than pollInterval to avoid missing ticks.
func (e *Engine) Watch(
	ctx context.Context,
	req bullarc.AnalysisRequest,
	pollInterval time.Duration,
	onResult func(bullarc.AnalysisResult),
) error {
	var (
		lastBarTime time.Time
		initialized bool
	)

	run := func() {
		result, err := e.Analyze(ctx, req)
		if err != nil {
			slog.Warn("watch: analysis failed", "symbol", req.Symbol, "err", err)
			return
		}
		t := latestBarTime(result)
		if !initialized {
			// Always emit the first result regardless of whether bars are present.
			initialized = true
			lastBarTime = t
			onResult(result)
			return
		}
		// Emit only when new bars have arrived.
		if !t.IsZero() && t.After(lastBarTime) {
			lastBarTime = t
			onResult(result)
		}
	}

	// Perform the initial analysis before the first tick.
	run()

	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			run()
		}
	}
}

// latestBarTime returns the timestamp of the most recent bar across all
// indicator value series in the result. Returns the zero time if no values
// are present.
func latestBarTime(result bullarc.AnalysisResult) time.Time {
	var latest time.Time
	for _, values := range result.IndicatorValues {
		if len(values) > 0 {
			t := values[len(values)-1].Time
			if t.After(latest) {
				latest = t
			}
		}
	}
	return latest
}

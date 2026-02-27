package bullarc

import "time"

// BarSignal pairs a bar with the composite signal computed at that point in time.
type BarSignal struct {
	Bar    OHLCV  `json:"bar"`
	Signal Signal `json:"signal"`
}

// BacktestRequest specifies parameters for a backtest run.
type BacktestRequest struct {
	// Symbol labels the backtest result; it is not used for data fetching.
	Symbol string `json:"symbol"`
	// Bars is the historical data to replay, sorted chronologically (oldest first).
	Bars []OHLCV `json:"bars"`
	// Indicators optionally limits which registered indicators are used.
	// Empty means all registered indicators.
	Indicators []string `json:"indicators,omitempty"`
}

// BacktestSummary contains aggregated statistics from a completed backtest run.
type BacktestSummary struct {
	TotalSignals int `json:"total_signals"`
	BuyCount     int `json:"buy_count"`
	SellCount    int `json:"sell_count"`
	HoldCount    int `json:"hold_count"`
	// SimReturn is the percentage return of a simple long-only fixed-size strategy.
	SimReturn float64 `json:"sim_return"`
	// MaxDrawdown is the maximum peak-to-trough equity drawdown percentage.
	MaxDrawdown float64 `json:"max_drawdown"`
	// WinRate is the percentage of completed trades that were profitable.
	WinRate float64 `json:"win_rate"`
}

// BacktestResult is the complete result of a backtest run.
type BacktestResult struct {
	Symbol      string          `json:"symbol"`
	BarSignals  []BarSignal     `json:"bar_signals"`
	Summary     BacktestSummary `json:"summary"`
	LLMAnalysis string          `json:"llm_analysis,omitempty"`
	Timestamp   time.Time       `json:"timestamp"`
}

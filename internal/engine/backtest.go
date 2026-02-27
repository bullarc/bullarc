package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/datasource"
	"github.com/bullarc/bullarc/internal/llm"
	"github.com/bullarc/bullarc/internal/signal"
)

// Backtest replays bars chronologically, computing indicators and signals at each
// time step with no look-ahead bias. Each signal is generated using only the data
// available up to and including that bar.
func (e *Engine) Backtest(ctx context.Context, req bullarc.BacktestRequest) (bullarc.BacktestResult, error) {
	result := bullarc.BacktestResult{
		Symbol:    req.Symbol,
		Timestamp: time.Now(),
	}
	if len(req.Bars) == 0 {
		slog.Info("backtest: no bars provided", "symbol", req.Symbol)
		return result, nil
	}

	e.mu.RLock()
	indicators := e.selectIndicatorsLocked(req.Indicators)
	e.mu.RUnlock()
	slog.Info("backtest started",
		"symbol", req.Symbol,
		"bars", len(req.Bars),
		"indicators", len(indicators))

	for i := range req.Bars {
		if ctx.Err() != nil {
			return result, bullarc.ErrTimeout.Wrap(ctx.Err())
		}

		window := req.Bars[:i+1]
		currentBar := req.Bars[i]
		var indSignals []bullarc.Signal

		for _, ind := range indicators {
			values, err := ind.Compute(window)
			if err != nil {
				continue // insufficient data for this indicator at this bar
			}
			if len(values) == 0 {
				continue
			}
			gen := signal.ForIndicator(ind.Meta().Name)
			if gen == nil {
				continue
			}
			sig, ok := gen(ind.Meta().Name, req.Symbol, currentBar, values)
			if ok {
				indSignals = append(indSignals, sig)
			}
		}

		if len(indSignals) == 0 {
			continue
		}

		composite := signal.Aggregate(req.Symbol, indSignals)
		result.BarSignals = append(result.BarSignals, bullarc.BarSignal{
			Bar:    currentBar,
			Signal: composite,
		})
	}

	result.Summary = computeBacktestSummary(result.BarSignals)
	slog.Info("backtest complete",
		"symbol", req.Symbol,
		"bar_signals", len(result.BarSignals),
		"buy", result.Summary.BuyCount,
		"sell", result.Summary.SellCount,
		"hold", result.Summary.HoldCount,
		"sim_return_pct", fmt.Sprintf("%.2f", result.Summary.SimReturn),
	)
	return result, nil
}

// BacktestCSV loads all bars from a CSV file and runs Backtest.
// symbol labels the result (not used for fetching). indicators optionally limits
// which registered indicators participate; empty means all defaults.
func (e *Engine) BacktestCSV(ctx context.Context, csvPath, symbol string, indicators []string) (bullarc.BacktestResult, error) {
	if csvPath == "" {
		return bullarc.BacktestResult{}, fmt.Errorf("csv_path is required")
	}
	src := datasource.NewCSVSource(csvPath)
	bars, err := src.Fetch(ctx, bullarc.DataQuery{Symbol: symbol})
	if err != nil {
		return bullarc.BacktestResult{}, fmt.Errorf("load csv %q: %w", csvPath, err)
	}
	if symbol == "" {
		symbol = "UNKNOWN"
	}
	return e.Backtest(ctx, bullarc.BacktestRequest{
		Symbol:     symbol,
		Bars:       bars,
		Indicators: indicators,
	})
}

// ExplainBacktestCSV loads all bars from a CSV file, runs Backtest, and optionally
// generates an AI-powered explanation of the results using the configured LLM provider.
// If no LLM provider is registered, the raw backtest result is returned with LLMAnalysis empty.
func (e *Engine) ExplainBacktestCSV(ctx context.Context, csvPath, symbol string, indicators []string) (bullarc.BacktestResult, error) {
	result, err := e.BacktestCSV(ctx, csvPath, symbol, indicators)
	if err != nil {
		return result, err
	}

	e.mu.RLock()
	provider := e.llmProvider
	e.mu.RUnlock()

	if provider == nil {
		return result, nil
	}

	prompt := llm.BacktestPrompt(result)
	resp, llmErr := provider.Complete(ctx, bullarc.LLMRequest{Prompt: prompt, MaxTokens: 768})
	if llmErr != nil {
		slog.Warn("llm backtest explanation failed", "symbol", result.Symbol, "err", llmErr)
		return result, nil
	}
	slog.Info("llm backtest explanation generated",
		"symbol", result.Symbol, "tokens", resp.TokensUsed, "model", resp.Model)
	result.LLMAnalysis = resp.Text
	return result, nil
}

// ListIndicators returns metadata for all indicators currently registered with the engine.
func (e *Engine) ListIndicators() []bullarc.IndicatorMeta {
	e.mu.RLock()
	defer e.mu.RUnlock()
	metas := make([]bullarc.IndicatorMeta, 0, len(e.indicators))
	for _, ind := range e.indicators {
		metas = append(metas, ind.Meta())
	}
	return metas
}

// computeBacktestSummary aggregates counts and simulation statistics from recorded signals.
func computeBacktestSummary(barSignals []bullarc.BarSignal) bullarc.BacktestSummary {
	s := bullarc.BacktestSummary{TotalSignals: len(barSignals)}
	for _, bs := range barSignals {
		switch bs.Signal.Type {
		case bullarc.SignalBuy:
			s.BuyCount++
		case bullarc.SignalSell:
			s.SellCount++
		default:
			s.HoldCount++
		}
	}
	s.SimReturn, s.MaxDrawdown, s.WinRate = simulateTrades(barSignals)
	return s
}

// simulateTrades runs a simple long-only fixed-size simulation.
// It enters on the first BUY signal and exits on the first subsequent SELL signal.
// Returns simulated return %, maximum drawdown %, and win rate %.
func simulateTrades(barSignals []bullarc.BarSignal) (simReturn, maxDrawdown, winRate float64) {
	const initialEquity = 10_000.0
	equity := initialEquity
	peak := initialEquity
	var entryPrice float64
	inPosition := false
	var wins, trades int

	for _, bs := range barSignals {
		price := bs.Bar.Close
		switch bs.Signal.Type {
		case bullarc.SignalBuy:
			if !inPosition {
				entryPrice = price
				inPosition = true
			}
		case bullarc.SignalSell:
			if inPosition {
				pnl := (price - entryPrice) / entryPrice
				equity *= (1 + pnl)
				trades++
				if pnl > 0 {
					wins++
				}
				inPosition = false
			}
		}
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			if dd := (peak - equity) / peak * 100; dd > maxDrawdown {
				maxDrawdown = dd
			}
		}
	}

	// Close any open position at the last recorded price.
	if inPosition && len(barSignals) > 0 {
		lastPrice := barSignals[len(barSignals)-1].Bar.Close
		pnl := (lastPrice - entryPrice) / entryPrice
		equity *= (1 + pnl)
		trades++
		if pnl > 0 {
			wins++
		}
	}

	simReturn = (equity - initialEquity) / initialEquity * 100
	if trades > 0 {
		winRate = float64(wins) / float64(trades) * 100
	}
	return
}

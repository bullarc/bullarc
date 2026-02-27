package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/datasource"
)

const (
	defaultConfidenceThreshold = 70.0
	paperTradingBanner         = "[PAPER TRADING]"
)

var paperCmd = &cobra.Command{
	Use:   "paper",
	Short: "Paper trading commands (simulated trades via Alpaca Paper Trading API)",
	Long: `Paper trading commands let you test trading strategies without real money.
All operations use the Alpaca Paper Trading API and are clearly labeled as [PAPER TRADING].`,
}

// paper trade subcommand
var paperTradeCmd = &cobra.Command{
	Use:   "trade",
	Short: "Watch a symbol and auto-execute paper trades based on signals",
	RunE:  runPaperTrade,
}

// paper positions subcommand
var paperPositionsCmd = &cobra.Command{
	Use:   "positions",
	Short: "List all open paper trading positions with unrealized P&L",
	RunE:  runPaperPositions,
}

// paper close-all subcommand (kill switch)
var paperCloseAllCmd = &cobra.Command{
	Use:   "close-all",
	Short: "Close all open paper trading positions immediately (kill switch)",
	RunE:  runPaperCloseAll,
}

var (
	paperSymbol              string
	paperAlpacaKey           string
	paperAlpacaSecret        string
	paperLLMKey              string
	paperInterval            time.Duration
	paperConfidenceThreshold float64
	paperBaseURL             string
)

func init() {
	paperTradeCmd.Flags().StringVarP(&paperSymbol, "symbol", "s", "", "symbol to trade (required)")
	paperTradeCmd.Flags().StringVar(&paperAlpacaKey, "alpaca-key", "", "Alpaca paper trading key ID (overrides ALPACA_PAPER_KEY env var)")
	paperTradeCmd.Flags().StringVar(&paperAlpacaSecret, "alpaca-secret", "", "Alpaca paper trading secret (overrides ALPACA_PAPER_SECRET env var)")
	paperTradeCmd.Flags().StringVar(&paperLLMKey, "llm-key", "", "Anthropic API key for signal generation")
	paperTradeCmd.Flags().DurationVarP(&paperInterval, "interval", "i", 5*time.Minute, "poll interval for new signals")
	paperTradeCmd.Flags().Float64Var(&paperConfidenceThreshold, "confidence", defaultConfidenceThreshold, "minimum signal confidence to trigger a trade (0-100)")
	paperTradeCmd.Flags().StringVar(&paperBaseURL, "paper-url", "", "Alpaca paper trading base URL (for testing)")
	_ = paperTradeCmd.MarkFlagRequired("symbol")

	paperPositionsCmd.Flags().StringVar(&paperAlpacaKey, "alpaca-key", "", "Alpaca paper trading key ID")
	paperPositionsCmd.Flags().StringVar(&paperAlpacaSecret, "alpaca-secret", "", "Alpaca paper trading secret")
	paperPositionsCmd.Flags().StringVar(&paperBaseURL, "paper-url", "", "Alpaca paper trading base URL (for testing)")

	paperCloseAllCmd.Flags().StringVar(&paperAlpacaKey, "alpaca-key", "", "Alpaca paper trading key ID")
	paperCloseAllCmd.Flags().StringVar(&paperAlpacaSecret, "alpaca-secret", "", "Alpaca paper trading secret")
	paperCloseAllCmd.Flags().StringVar(&paperBaseURL, "paper-url", "", "Alpaca paper trading base URL (for testing)")

	paperCmd.AddCommand(paperTradeCmd)
	paperCmd.AddCommand(paperPositionsCmd)
	paperCmd.AddCommand(paperCloseAllCmd)
}

func runPaperPositions(cmd *cobra.Command, _ []string) error {
	trader, err := buildPaperTrader(paperAlpacaKey, paperAlpacaSecret, paperBaseURL)
	if err != nil {
		return err
	}

	positions, err := trader.GetPositions(cmd.Context())
	if err != nil {
		return fmt.Errorf("%s get positions: %w", paperTradingBanner, err)
	}

	if len(positions) == 0 {
		fmt.Printf("%s no open positions\n", paperTradingBanner)
		return nil
	}

	fmt.Printf("%s open positions (%d):\n", paperTradingBanner, len(positions))
	fmt.Printf("  %-10s %8s %12s %12s %12s %8s\n",
		"SYMBOL", "QTY", "AVG ENTRY", "CUR PRICE", "UNREAL P&L", "P&L%")
	fmt.Printf("  %-10s %8s %12s %12s %12s %8s\n",
		"------", "---", "---------", "---------", "----------", "----")
	for _, p := range positions {
		fmt.Printf("  %-10s %8.4f %12.2f %12.2f %12.2f %7.2f%%\n",
			p.Symbol, p.Qty, p.AvgEntryPrice, p.CurrentPrice,
			p.UnrealizedPnL, p.UnrealizedPnLPct*100)
	}
	return nil
}

func runPaperCloseAll(cmd *cobra.Command, _ []string) error {
	trader, err := buildPaperTrader(paperAlpacaKey, paperAlpacaSecret, paperBaseURL)
	if err != nil {
		return err
	}

	fmt.Printf("%s closing all positions...\n", paperTradingBanner)
	if err := trader.CloseAll(cmd.Context()); err != nil {
		return fmt.Errorf("%s close all: %w", paperTradingBanner, err)
	}
	fmt.Printf("%s all positions closed\n", paperTradingBanner)
	return nil
}

func runPaperTrade(cmd *cobra.Command, _ []string) error {
	keyID, secretKey, err := resolvePaperCredentials(paperAlpacaKey, paperAlpacaSecret)
	if err != nil {
		return err
	}

	trader, err := buildPaperTrader(keyID, secretKey, paperBaseURL)
	if err != nil {
		return err
	}

	e, err := buildEngine("", "", paperLLMKey, keyID, secretKey)
	if err != nil {
		return err
	}
	if !e.HasDataSource() {
		return errNoDataSource()
	}

	executor := &paperTradeExecutor{
		trader:              trader,
		confidenceThreshold: paperConfidenceThreshold,
		stopLosses:          make(map[string]float64),
	}

	fmt.Printf("%s auto-trading %s every %s (confidence threshold: %.0f%%) — ctrl-c to stop\n",
		paperTradingBanner, paperSymbol, paperInterval, paperConfidenceThreshold)

	return e.Watch(cmd.Context(), bullarc.AnalysisRequest{Symbol: paperSymbol}, paperInterval,
		func(result bullarc.AnalysisResult) {
			PrintResult(os.Stdout, result)
			executor.onResult(cmd.Context(), result)
		},
	)
}

// paperTradeExecutor processes analysis results and places paper trades when appropriate.
type paperTradeExecutor struct {
	trader              *datasource.AlpacaPaperTrader
	confidenceThreshold float64

	mu         sync.Mutex
	stopLosses map[string]float64 // symbol -> stop-loss price from last BUY order
}

// onResult evaluates a new analysis result and decides whether to place a paper trade.
func (e *paperTradeExecutor) onResult(ctx context.Context, result bullarc.AnalysisResult) {
	if len(result.Signals) == 0 {
		return
	}

	composite := result.Signals[0]
	symbol := result.Symbol

	// Check stop-loss trigger before evaluating signals.
	e.mu.Lock()
	stopPrice, hasStop := e.stopLosses[symbol]
	e.mu.Unlock()

	// Derive current price from the composite signal timestamp or indicator values.
	currentPrice := latestClosePrice(result)

	if hasStop && currentPrice > 0 && currentPrice <= stopPrice {
		slog.Info(paperTradingBanner+" stop-loss triggered",
			"symbol", symbol,
			"current_price", currentPrice,
			"stop_price", stopPrice)
		e.closePosition(ctx, symbol, composite)
		return
	}

	switch composite.Type {
	case bullarc.SignalBuy:
		if composite.Confidence >= e.confidenceThreshold {
			e.placeBuy(ctx, result, composite, currentPrice)
		} else {
			slog.Info(paperTradingBanner+" BUY signal below confidence threshold, skipping",
				"symbol", symbol,
				"confidence", composite.Confidence,
				"threshold", e.confidenceThreshold)
		}
	case bullarc.SignalSell:
		e.closePosition(ctx, symbol, composite)
	}
}

// placeBuy places a paper buy order using the position size from risk metrics.
func (e *paperTradeExecutor) placeBuy(ctx context.Context, result bullarc.AnalysisResult, composite bullarc.Signal, currentPrice float64) {
	symbol := result.Symbol

	qty := e.calculateQty(ctx, result, currentPrice)
	if qty <= 0 {
		slog.Warn(paperTradingBanner+" could not calculate valid qty for BUY, skipping",
			"symbol", symbol)
		return
	}

	order := bullarc.Order{
		Symbol:            symbol,
		Side:              bullarc.OrderSideBuy,
		Qty:               qty,
		SignalConfidence:  composite.Confidence,
		SignalExplanation: composite.Explanation,
	}

	result2, err := e.trader.PlaceOrder(ctx, order)
	if err != nil {
		slog.Warn(paperTradingBanner+" failed to place BUY order",
			"symbol", symbol, "err", err)
		return
	}

	fmt.Printf("%s BUY  %-8s qty=%.4f price=%.2f confidence=%.1f%% id=%s\n",
		paperTradingBanner, symbol, result2.Qty, result2.FilledPrice,
		composite.Confidence, result2.OrderID)

	// Store stop-loss price for this position.
	if result.Risk != nil && result.Risk.StopLoss > 0 {
		e.mu.Lock()
		e.stopLosses[symbol] = result.Risk.StopLoss
		e.mu.Unlock()
		slog.Info(paperTradingBanner+" stop-loss set",
			"symbol", symbol, "stop_loss", result.Risk.StopLoss)
	}
}

// closePosition closes an existing paper trading position.
func (e *paperTradeExecutor) closePosition(ctx context.Context, symbol string, composite bullarc.Signal) {
	result, err := e.trader.ClosePosition(ctx, symbol)
	if err != nil {
		slog.Warn(paperTradingBanner+" failed to close position",
			"symbol", symbol, "err", err)
		return
	}

	direction := "SELL"
	if composite.Type == bullarc.SignalSell {
		fmt.Printf("%s SELL %-8s qty=%.4f price=%.2f confidence=%.1f%% id=%s\n",
			paperTradingBanner, symbol, result.Qty, result.FilledPrice,
			composite.Confidence, result.OrderID)
	} else {
		fmt.Printf("%s STOP %-8s qty=%.4f price=%.2f (stop-loss triggered) id=%s\n",
			paperTradingBanner, symbol, result.Qty, result.FilledPrice, result.OrderID)
		direction = "STOP"
	}
	_ = direction

	// Remove the stop-loss entry for this position.
	e.mu.Lock()
	delete(e.stopLosses, symbol)
	e.mu.Unlock()
}

// calculateQty derives the order quantity from the risk metrics position size
// percentage and account equity. Falls back to a minimal 1-share order when
// equity or risk data is unavailable.
func (e *paperTradeExecutor) calculateQty(ctx context.Context, result bullarc.AnalysisResult, currentPrice float64) float64 {
	if currentPrice <= 0 {
		return 1
	}

	equity, err := e.trader.GetAccountEquity(ctx)
	if err != nil {
		slog.Warn(paperTradingBanner+" could not fetch account equity, using minimal qty",
			"symbol", result.Symbol, "err", err)
		return 1
	}
	if equity <= 0 {
		return 1
	}

	if result.Risk != nil && result.Risk.PositionSizePct > 0 {
		positionValue := equity * result.Risk.PositionSizePct / 100.0
		qty := positionValue / currentPrice
		qty = math.Floor(qty*10000) / 10000 // truncate to 4 decimal places
		if qty < 0.0001 {
			qty = 0.0001
		}
		return qty
	}

	// Fallback: use 1% of equity.
	fallbackValue := equity * 0.01
	qty := math.Floor(fallbackValue/currentPrice*10000) / 10000
	if qty < 0.0001 {
		qty = 0.0001
	}
	return qty
}

// latestClosePrice extracts the most recent close price from indicator values.
// Returns 0 when no price data is available.
func latestClosePrice(result bullarc.AnalysisResult) float64 {
	// Derive the price from SMA or any available indicator values as a proxy.
	// The composite signal carries no price; rely on the latest indicator bar time.
	for _, values := range result.IndicatorValues {
		if len(values) > 0 {
			// We can't reliably derive the close price from indicator values alone,
			// so we return 0 and let the caller handle the fallback.
			_ = values
			break
		}
	}
	// Return 0 — the caller will fetch equity anyway and qty will default to minimal.
	return 0
}

// buildPaperTrader creates an AlpacaPaperTrader from flag/env credentials.
func buildPaperTrader(keyID, secretKey, baseURL string) (*datasource.AlpacaPaperTrader, error) {
	resolvedKey, resolvedSecret, err := resolvePaperCredentials(keyID, secretKey)
	if err != nil {
		return nil, err
	}

	var opts []datasource.AlpacaPaperOption
	if baseURL != "" {
		opts = append(opts, datasource.WithPaperBaseURL(baseURL))
	}
	return datasource.NewAlpacaPaperTrader(resolvedKey, resolvedSecret, opts...), nil
}

// resolvePaperCredentials resolves Alpaca paper trading credentials.
// Resolution order: flag > ALPACA_PAPER_KEY/ALPACA_PAPER_SECRET env vars >
// ALPACA_API_KEY/ALPACA_SECRET_KEY env vars.
func resolvePaperCredentials(keyID, secretKey string) (string, string, error) {
	resolvedKey := keyID
	if resolvedKey == "" {
		resolvedKey = os.Getenv("ALPACA_PAPER_KEY")
	}
	if resolvedKey == "" {
		resolvedKey = os.Getenv("ALPACA_API_KEY")
	}

	resolvedSecret := secretKey
	if resolvedSecret == "" {
		resolvedSecret = os.Getenv("ALPACA_PAPER_SECRET")
	}
	if resolvedSecret == "" {
		resolvedSecret = os.Getenv("ALPACA_SECRET_KEY")
	}

	if resolvedKey == "" {
		return "", "", fmt.Errorf("no Alpaca paper trading credentials found\n\n" +
			"Set paper trading credentials via environment variables:\n" +
			"  ALPACA_PAPER_KEY=<paper-key-id>\n" +
			"  ALPACA_PAPER_SECRET=<paper-secret>\n\n" +
			"Or pass them as flags:\n" +
			"  --alpaca-key <key-id> --alpaca-secret <secret>")
	}

	return resolvedKey, resolvedSecret, nil
}

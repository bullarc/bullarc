// Package sdk provides a high-level Go client for the bullarc analysis engine.
//
// # Overview
//
// The SDK wraps the internal engine behind a stable, versioned surface that
// external Go programs can import without being affected by engine refactors.
// It exposes four delivery modes:
//
//   - [Client.Analyze]       – one-shot analysis for a single symbol
//   - [Client.Backtest]      – chronological backtest over historical bars
//   - [Client.Stream]        – poll-based signal delivery to a channel
//   - [Client.StreamPush]    – push-based delivery via the engine's signal bus
//   - [Client.Subscribe]     – subscribe to signals without starting a poll loop
//   - [Client.StreamSymbols] – multiplex streaming across a symbol list
//
// # Constructing a Client
//
// The simplest way to create a client is with a pre-built engine:
//
//	import (
//	    "github.com/bullarc/bullarc/internal/engine"
//	    "github.com/bullarc/bullarc/pkg/sdk"
//	)
//
//	e := engine.New()
//	for _, ind := range engine.DefaultIndicators() {
//	    e.RegisterIndicator(ind)
//	}
//	client := sdk.New(e)
//
// Use [NewWithOptions] to configure the client at construction time:
//
//	client, err := sdk.NewWithOptions(e,
//	    sdk.WithAlpacaDataSource(alpacaKeyID, alpacaSecret),
//	    sdk.WithAnthropicProvider(anthropicAPIKey, ""),
//	    sdk.WithSymbols("AAPL", "MSFT"),
//	    sdk.WithInterval("1Day"),
//	)
//
// # Configuration Options
//
// Options are plain functions of type [Option]. They validate their arguments
// and return an error for invalid values. All options may be applied at
// construction time via [NewWithOptions] or at runtime via [Client.Configure].
//
// Available options:
//
//   - [WithSymbols]           – default symbols used when none is given in a request
//   - [WithIndicators]        – restrict analysis to a named subset of indicators
//   - [WithInterval]          – bar interval for data fetching (e.g. "1Day", "1Hour")
//   - [WithDataSource]        – set an arbitrary [bullarc.DataSource] implementation
//   - [WithAlpacaDataSource]  – configure Alpaca Markets as the data source
//   - [WithLLMProvider]       – set an arbitrary [bullarc.LLMProvider] implementation
//   - [WithAnthropicProvider] – configure Anthropic Claude as the LLM provider
//
// # Supported Intervals
//
// The following interval strings are accepted by [WithInterval]:
// 1Min, 5Min, 15Min, 30Min, 1Hour, 2Hour, 4Hour, 1Day, 1Week, 1Month.
//
// # Indicator Names
//
// [WithIndicators] accepts indicator names that match the default set or follow
// a parameterised name pattern:
//
//   - SMA_<period>           e.g. "SMA_14", "SMA_50"
//   - EMA_<period>           e.g. "EMA_14"
//   - RSI_<period>           e.g. "RSI_14", "RSI_21"
//   - ATR_<period>           e.g. "ATR_14"
//   - MACD_<fast>_<slow>_<sig>  e.g. "MACD_12_26_9"
//   - BB_<period>_<multiplier>  e.g. "BB_20_2"
//   - SuperTrend_<period>_<multiplier>  e.g. "SuperTrend_7_3"
//   - Stoch_<period>_<smoothK>_<smoothD>  e.g. "Stoch_14_3_3"
//   - VWAP
//   - OBV
//
// # File-Based Configuration
//
// [FileConfig] is a JSON-serializable struct for persisting SDK configuration.
// Use [SaveFileConfig] and [LoadFileConfig] to read/write it, and
// [FromFileConfig] to convert it into a slice of options:
//
//	fc := sdk.FileConfig{
//	    Symbols:  []string{"AAPL", "TSLA"},
//	    Interval: "1Day",
//	    DataSource: sdk.FileDataSource{
//	        Type:   "alpaca",
//	        APIKey: os.Getenv("ALPACA_API_KEY"),
//	        Secret: os.Getenv("ALPACA_SECRET_KEY"),
//	    },
//	    LLM: sdk.FileLLM{
//	        Type:   "anthropic",
//	        APIKey: os.Getenv("ANTHROPIC_API_KEY"),
//	    },
//	}
//
//	opts, err := sdk.FromFileConfig(fc)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	client, err := sdk.NewWithOptions(e, opts...)
//
// # Error Handling
//
// All errors returned by the SDK and the underlying engine are typed
// [*bullarc.Error] values with machine-readable codes:
//
//   - INSUFFICIENT_DATA   – not enough bars for the indicator's warmup period
//   - INVALID_PARAMETER   – an invalid indicator name, interval, or option value
//   - DATA_SOURCE_UNAVAILABLE – data source returned no bars or failed
//   - LLM_UNAVAILABLE     – LLM provider request failed
//
// Errors can be inspected with [errors.Is] or by type assertion:
//
//	result, err := client.Analyze(ctx, req)
//	if errors.Is(err, bullarc.ErrInsufficientData) {
//	    // need more historical bars
//	}
package sdk

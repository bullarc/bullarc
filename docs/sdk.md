# SDK API Reference

The `pkg/sdk` package provides a high-level Go client for embedding bullarc
analysis in your own programs.

Import path: `github.com/bullarc/bullarc/pkg/sdk`

For a full architecture overview see [architecture.md](architecture.md).

---

## Quick start

```go
import (
    "context"
    "fmt"
    "log"

    "github.com/bullarc/bullarc"
    "github.com/bullarc/bullarc/internal/engine"
    "github.com/bullarc/bullarc/pkg/sdk"
)

func main() {
    // 1. Build the engine with default indicators.
    e := engine.New()
    for _, ind := range engine.DefaultIndicators() {
        e.RegisterIndicator(ind)
    }

    // 2. Create the client with Alpaca data and Anthropic LLM.
    client, err := sdk.NewWithOptions(e,
        sdk.WithAlpacaDataSource(os.Getenv("ALPACA_API_KEY"), os.Getenv("ALPACA_SECRET_KEY")),
        sdk.WithAnthropicProvider(os.Getenv("ANTHROPIC_API_KEY"), ""),
        sdk.WithSymbols("AAPL", "MSFT"),
        sdk.WithInterval("1Day"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 3. Run a one-shot analysis.
    result, err := client.Analyze(context.Background(), bullarc.AnalysisRequest{
        Symbol: "AAPL",
        UseLLM: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, sig := range result.Signals {
        fmt.Printf("%s  %s  %.2f\n", sig.Indicator, sig.Type, sig.Confidence)
    }
    fmt.Println(result.LLMAnalysis)
}
```

---

## Constructors

### `sdk.New(engine bullarc.Engine) *Client`

Creates a client with no configuration. The engine is used as-is; any data
sources and indicators registered on it before calling `New` are active.

```go
e := engine.New()
for _, ind := range engine.DefaultIndicators() {
    e.RegisterIndicator(ind)
}
client := sdk.New(e)
```

### `sdk.NewWithOptions(eng bullarc.Engine, opts ...Option) (*Client, error)`

Creates a client and applies functional options at construction time. Returns
an error if any option is invalid (e.g. an empty API key or unknown indicator
name).

```go
client, err := sdk.NewWithOptions(e,
    sdk.WithAlpacaDataSource(keyID, secret),
    sdk.WithSymbols("AAPL", "TSLA"),
    sdk.WithInterval("1Hour"),
)
```

---

## Options

Options are values of type `Option` (a function `func(*ClientConfig) error`).
All options can be applied at construction time or at runtime via
`Client.Configure`.

### `WithSymbols(symbols ...string) Option`

Sets the default symbols used by analysis methods when no symbol is given in
the request. Each symbol must be a non-empty string.

```go
sdk.WithSymbols("AAPL", "MSFT", "GOOGL")
```

### `WithIndicators(indicators ...string) Option`

Restricts analysis to the named subset of indicators. Names must match the
default set or a supported parameterised pattern. See
[configuration.md](configuration.md#indicator-names) for the full list.

```go
sdk.WithIndicators("SMA_14", "RSI_14", "MACD_12_26_9")
```

### `WithInterval(interval string) Option`

Sets the data bar interval. Must be one of:
`1Min`, `5Min`, `15Min`, `30Min`, `1Hour`, `2Hour`, `4Hour`, `1Day`, `1Week`, `1Month`.

```go
sdk.WithInterval("1Day")
```

### `WithDataSource(ds bullarc.DataSource) Option`

Sets an arbitrary `DataSource` implementation. Cannot be combined with
`WithAlpacaDataSource` (returns an error).

```go
sdk.WithDataSource(myCustomSource)
```

### `WithAlpacaDataSource(keyID, secretKey string) Option`

Configures the Alpaca Markets data source. `keyID` must be non-empty.
Cannot be combined with `WithDataSource`.

```go
sdk.WithAlpacaDataSource(
    os.Getenv("ALPACA_API_KEY"),
    os.Getenv("ALPACA_SECRET_KEY"),
)
```

### `WithLLMProvider(p bullarc.LLMProvider) Option`

Sets an arbitrary `LLMProvider` implementation. Cannot be combined with
`WithAnthropicProvider`.

```go
sdk.WithLLMProvider(myCustomLLM)
```

### `WithAnthropicProvider(apiKey, model string) Option`

Configures the Anthropic Claude LLM provider. `apiKey` must be non-empty. If
`model` is empty the provider uses the default model. Cannot be combined with
`WithLLMProvider`.

```go
sdk.WithAnthropicProvider(os.Getenv("ANTHROPIC_API_KEY"), "")
// or with an explicit model:
sdk.WithAnthropicProvider(os.Getenv("ANTHROPIC_API_KEY"), "claude-opus-4-6")
```

---

## Client methods

### `Analyze(ctx, AnalysisRequest) (AnalysisResult, error)`

Runs a one-shot analysis for a single symbol. Applies configured default symbol
and indicators if the request fields are empty.

```go
result, err := client.Analyze(ctx, bullarc.AnalysisRequest{
    Symbol: "AAPL",
    UseLLM: true,   // requires an LLM provider
})
```

`AnalysisResult` fields:

| Field | Type | Description |
|-------|------|-------------|
| `Symbol` | `string` | The analysed symbol |
| `Signals` | `[]Signal` | One signal per indicator |
| `IndicatorValues` | `map[string][]IndicatorValue` | Raw computed values |
| `LLMAnalysis` | `string` | Plain-English explanation (empty if UseLLM=false) |
| `Timestamp` | `time.Time` | When the analysis was run |

### `Backtest(ctx, BacktestRequest) (BacktestResult, error)`

Runs a chronological backtest over the provided bars. The engine must support
backtesting (the default `*engine.Engine` does); otherwise an error is returned.

```go
bars, _ := testutil.LoadBarsFromCSV(t, "data.csv")
result, err := client.Backtest(ctx, bullarc.BacktestRequest{
    Symbol: "AAPL",
    Bars:   bars,
})
```

`BacktestResult.Summary` fields:

| Field | Type | Description |
|-------|------|-------------|
| `TotalSignals` | `int` | Total signals generated |
| `BuyCount` | `int` | BUY signal count |
| `SellCount` | `int` | SELL signal count |
| `HoldCount` | `int` | HOLD signal count |
| `SimReturn` | `float64` | Simulated return (%) |
| `MaxDrawdown` | `float64` | Maximum drawdown (%) |
| `WinRate` | `float64` | Win rate (%) |

### `Stream(ctx, AnalysisRequest, pollInterval) <-chan Signal`

Polls the engine at `pollInterval` and delivers each `Signal` from every result
to the returned channel. The channel is closed when `ctx` is cancelled.

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, time.Minute)
for sig := range ch {
    fmt.Printf("%s  %s  %.2f\n", sig.Indicator, sig.Type, sig.Confidence)
    if someCondition(sig) {
        cancel() // stop streaming
    }
}
```

### `StreamSymbols(ctx, symbols []string, pollInterval) <-chan Signal`

Like `Stream` but fans out over multiple symbols, merging all signals into a
single channel. If `symbols` is nil or empty and the client has configured
symbols, those are used.

```go
ch := client.StreamSymbols(ctx, []string{"AAPL", "TSLA", "MSFT"}, time.Minute)
for sig := range ch {
    fmt.Printf("[%s] %s  %s  %.2f\n", sig.Symbol, sig.Indicator, sig.Type, sig.Confidence)
}
```

### `Subscribe(ctx, AnalysisRequest) <-chan Signal`

Returns a channel that receives signals whenever the engine's internal signal
bus publishes them — without the consumer needing to poll. Useful when another
goroutine is already calling `Analyze` or running a `Watch` loop.

```go
ch := client.Subscribe(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})

// Trigger analysis separately (e.g. in response to a webhook or timer).
go func() {
    for {
        client.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
        time.Sleep(time.Minute)
    }
}()

for sig := range ch {
    fmt.Println(sig)
}
```

### `StreamPush(ctx, AnalysisRequest, pollInterval) <-chan Signal`

Combines `Subscribe` and an internal `Watch` loop: starts polling at
`pollInterval` and delivers signals via the engine's signal bus. Falls back to
`Stream` if the engine does not support push delivery.

```go
// One call sets up both the polling loop and the signal subscription.
ch := client.StreamPush(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, time.Minute)
for sig := range ch {
    fmt.Println(sig)
}
```

### `Configure(opts ...Option) error`

Applies options at runtime. On error the configuration is left unchanged.

```go
if err := client.Configure(sdk.WithInterval("1Hour")); err != nil {
    log.Fatal(err)
}
```

### `Config() ClientConfig`

Returns a snapshot of the current client configuration.

```go
cfg := client.Config()
fmt.Println("symbols:", cfg.Symbols)
fmt.Println("interval:", cfg.Interval)
```

---

## File-based configuration

`FileConfig` is a JSON-serializable struct for persisting SDK configuration.

### Save to disk

```go
fc := sdk.FileConfig{
    Symbols:  []string{"AAPL", "MSFT"},
    Interval: "1Day",
    DataSource: sdk.FileDataSource{
        Type:   "alpaca",
        APIKey: os.Getenv("ALPACA_API_KEY"),
        Secret: os.Getenv("ALPACA_SECRET_KEY"),
    },
    LLM: sdk.FileLLM{
        Type:   "anthropic",
        APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    },
}

if err := sdk.SaveFileConfig("/path/to/sdk-config.json", fc); err != nil {
    log.Fatal(err)
}
```

### Load from disk

```go
fc, err := sdk.LoadFileConfig("/path/to/sdk-config.json")
if err != nil {
    log.Fatal(err)
}

opts, err := sdk.FromFileConfig(fc)
if err != nil {
    log.Fatal(err)
}

client, err := sdk.NewWithOptions(e, opts...)
```

---

## Error handling

All errors are typed `*bullarc.Error` values:

```go
result, err := client.Analyze(ctx, req)
if err != nil {
    var berr *bullarc.Error
    if errors.As(err, &berr) {
        fmt.Println("code:", berr.Code)     // e.g. "INSUFFICIENT_DATA"
        fmt.Println("message:", berr.Message)
    }
    // Or use sentinel comparison:
    if errors.Is(err, bullarc.ErrInsufficientData) {
        // handle insufficient data
    }
}
```

Sentinel errors:

| Sentinel | Code | When |
|----------|------|------|
| `ErrInsufficientData` | `INSUFFICIENT_DATA` | Not enough bars for an indicator |
| `ErrInvalidParameter` | `INVALID_PARAMETER` | Bad option value (e.g. empty symbol) |
| `ErrDataSourceUnavailable` | `DATA_SOURCE_UNAVAILABLE` | Data source failed or returned no bars |
| `ErrLLMUnavailable` | `LLM_UNAVAILABLE` | LLM provider request failed |
| `ErrSymbolNotFound` | `SYMBOL_NOT_FOUND` | Symbol not recognised by the data source |
| `ErrTimeout` | `TIMEOUT` | Operation exceeded the context deadline |

---

## Common patterns

### Analyse a watchlist and print a table

```go
symbols := []string{"AAPL", "MSFT", "GOOGL", "TSLA"}
for _, sym := range symbols {
    result, err := client.Analyze(ctx, bullarc.AnalysisRequest{Symbol: sym})
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %s: %v\n", sym, err)
        continue
    }
    composite := result.Signals[0]
    fmt.Printf("%-6s  %-4s  %.2f\n", sym, composite.Type, composite.Confidence)
}
```

### React to signals above a confidence threshold

```go
ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, time.Minute)
for sig := range ch {
    if sig.Type == bullarc.SignalBuy && sig.Confidence >= 0.7 {
        fmt.Printf("High-confidence BUY for %s from %s (%.2f)\n",
            sig.Symbol, sig.Indicator, sig.Confidence)
    }
}
```

### Backtest with a custom indicator set

```go
bars, err := loadBarsFromFile("aapl_2023.csv")
if err != nil {
    log.Fatal(err)
}

client.Configure(sdk.WithIndicators("RSI_14", "MACD_12_26_9"))

result, err := client.Backtest(ctx, bullarc.BacktestRequest{
    Symbol: "AAPL",
    Bars:   bars,
})
fmt.Printf("sim return: %.2f%%  max drawdown: %.2f%%  win rate: %.2f%%\n",
    result.Summary.SimReturn,
    result.Summary.MaxDrawdown,
    result.Summary.WinRate,
)
```

### Use a local CSV as the data source

```go
import "github.com/bullarc/bullarc/internal/datasource"

e := engine.New()
for _, ind := range engine.DefaultIndicators() {
    e.RegisterIndicator(ind)
}
e.RegisterDataSource(datasource.NewCSVSource("/path/to/data.csv"))

client := sdk.New(e)
result, err := client.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "MYSTOCK"})
```

Note: `datasource` is an internal package; import it only in programs that own
the engine construction. If you require a fully encapsulated option, set the
data source via `WithDataSource`.

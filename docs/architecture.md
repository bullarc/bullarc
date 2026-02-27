# Architecture Overview

bullarc is a modular, layered financial analysis engine. Each layer has a single
responsibility and communicates only through the interfaces defined in the root
package (`github.com/bullarc/bullarc`).

---

## Component Map

```
┌─────────────────────────────────────────────────────────────────┐
│                         Delivery Layer                          │
│                                                                 │
│   cmd/bullarc/   (CLI)          pkg/sdk/   (Go SDK)            │
│   internal/mcp/  (MCP server)                                   │
└────────────────────────┬────────────────────────────────────────┘
                         │ calls
┌────────────────────────▼────────────────────────────────────────┐
│                      internal/engine/                           │
│                                                                 │
│  Orchestrates: data fetching → indicator computation →         │
│  signal generation → LLM explanation → result delivery         │
└───────────┬─────────────────┬──────────────────────────────────┘
            │                 │
   ┌────────▼──────┐  ┌───────▼────────┐
   │  Data Sources │  │   Indicators   │
   │               │  │                │
   │ internal/     │  │ internal/      │
   │ datasource/   │  │ indicator/     │
   └───────────────┘  └────────────────┘
            │                 │
   ┌────────▼──────┐  ┌───────▼────────┐
   │  LLM Provider │  │ Signal Engine  │
   │               │  │                │
   │ internal/llm/ │  │ internal/      │
   └───────────────┘  │ signal/        │
                      └────────────────┘

All interfaces and shared types: bullarc.go (root package)
```

---

## Root Package (`bullarc.go`)

Every interface and shared type lives in a single file at the module root. This
is the API contract — downstream code imports only this package for types.

Key types:

| Type | Description |
|------|-------------|
| `OHLCV` / `Bar` | Single candlestick (Open, High, Low, Close, Volume + time) |
| `Indicator` | Computes indicator values from a slice of bars |
| `DataSource` | Fetches OHLCV bars from an external market data provider |
| `LLMProvider` | Sends a prompt to an LLM and returns a text response |
| `Engine` | Orchestrates all components to produce an `AnalysisResult` |
| `Signal` | A typed trading signal (BUY / SELL / HOLD) with confidence |
| `AnalysisResult` | Full output of one analysis run |
| `BacktestResult` | Output of a chronological backtest |
| `Error` | Typed error with a machine-readable code |

---

## Data Sources (`internal/datasource/`)

Data sources implement `bullarc.DataSource`:

```go
type DataSource interface {
    Meta() DataSourceMeta
    Fetch(ctx context.Context, query DataQuery) ([]OHLCV, error)
}
```

`DataQuery` specifies symbol, start/end time, and interval. The engine calls
`Fetch` when it needs historical bars for a symbol.

Built-in sources:

| Source | Description |
|--------|-------------|
| `AlpacaSource` | Alpaca Markets REST API (requires API key + secret) |
| `MassiveSource` | Massive Finance data API (requires API key) |
| `CSVSource` | Local CSV file — no API credentials needed |

The CSV format expected by `CSVSource` and the `backtest` command:

```
date,open,high,low,close,volume
2024-01-02,185.20,188.44,183.13,185.92,74423850
2024-01-03,184.22,185.88,183.43,184.25,58892529
...
```

Dates must be in `2006-01-02` format.

---

## Indicator Engine (`internal/indicator/`)

Indicators implement `bullarc.Indicator`:

```go
type Indicator interface {
    Meta() IndicatorMeta
    Compute(bars []OHLCV) ([]IndicatorValue, error)
}
```

`Compute` is a **pure function** — no side effects, no I/O, no logging. It
takes a slice of bars and returns a slice of `IndicatorValue`. The output
length is `len(bars) - warmupPeriod + 1`.

Built-in indicators:

| Name | Category | Parameters |
|------|----------|------------|
| `SMA_14`, `SMA_50` | Trend | period |
| `EMA_14` | Trend | period |
| `RSI_14` | Momentum | period |
| `MACD_12_26_9` | Momentum | fast, slow, signal |
| `BB_20_2` | Volatility | period, stddev multiplier |
| `ATR_14` | Volatility | period |
| `SuperTrend_7_3` | Trend | period, multiplier |
| `Stoch_14_3_3` | Momentum | period, smoothK, smoothD |
| `VWAP` | Volume | — |
| `OBV` | Volume | — |

Custom indicator periods are constructed from parameterised names (see
[configuration.md](configuration.md#indicator-names)).

---

## Signal Engine (`internal/signal/`)

The signal engine sits between indicators and the engine. It aggregates
`IndicatorValue` outputs from all registered indicators for a symbol and
produces a typed `Signal` (BUY / SELL / HOLD) with a confidence score
between 0.0 and 1.0.

Signal aggregation applies per-indicator rules to classify each output as
bullish, bearish, or neutral, then computes a weighted confidence score across
all indicators. The composite signal type is determined by the direction that
receives the highest aggregate confidence.

---

## Engine (`internal/engine/`)

The engine is the only package permitted to import multiple internal packages.
It wires together data sources, indicators, the signal engine, and LLM
providers into a single `Analyze` call.

```
Analyze(ctx, AnalysisRequest{Symbol: "AAPL", UseLLM: true})
  1. Fetch bars from the registered DataSource
  2. For each registered Indicator, call Compute(bars)
  3. Pass IndicatorValues to the Signal engine → []Signal
  4. If UseLLM: call LLMProvider.Complete with a structured prompt
  5. Return AnalysisResult{Symbol, Signals, IndicatorValues, LLMAnalysis}
```

Additional engine capabilities:

- **Watch**: polls `Analyze` at a configurable interval, calling a callback on
  each result. Used by `bullarc watch` and `sdk.Client.Stream`.
- **BacktestCSV**: runs a chronological simulation over a CSV file, computing
  indicators and signals for every bar and accumulating summary statistics.
- **Signal bus**: an internal pub/sub bus that pushes each `Signal` to
  subscribers without requiring them to poll. Used by `sdk.Client.Subscribe`
  and `sdk.Client.StreamPush`.

---

## LLM Provider (`internal/llm/`)

LLM providers implement `bullarc.LLMProvider`:

```go
type LLMProvider interface {
    Name() string
    Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
}
```

The built-in provider is `AnthropicProvider`, which calls the Anthropic
Messages API. The engine passes a structured prompt containing the indicator
values and signals; the provider returns a plain-English explanation.

---

## Delivery Layer

### CLI (`cmd/bullarc/`)

The CLI is a thin wrapper over the engine. Commands parse flags and environment
variables, resolve credentials using the priority order documented in
[configuration.md](configuration.md), build an engine, and delegate to it.

Commands: `analyze`, `watch`, `backtest`, `mcp`, `configure`, `version`.

### Go SDK (`pkg/sdk/`)

The SDK wraps the internal engine behind a stable versioned surface. External
Go programs import `github.com/bullarc/bullarc/pkg/sdk` and never import
`internal/` packages directly.

See [sdk.md](sdk.md) for the full API reference.

### MCP Server (`internal/mcp/`)

The MCP (Model Context Protocol) server exposes bullarc tools over stdio to
any MCP-compatible client (Claude Desktop, Cursor, etc.).

Available MCP tools:

| Tool | Description |
|------|-------------|
| `get_signals` | Analyse one or more symbols and return composite signals |
| `explain_signal` | Generate a plain-English explanation using LLM |
| `stream_signals` | Push-based signal delivery within a timeout window |
| `backtest_strategy` | Run a backtest from a CSV file and return statistics |
| `explain_backtest` | Run a backtest and return an AI-generated explanation |
| `list_indicators` | List all registered indicators with metadata |

Start the MCP server:

```bash
bullarc mcp
# or with a config file:
bullarc mcp -c config.yaml
```

---

## Dependency Rules

```
bullarc.go (root)          ← imported by all packages
internal/indicator/        ← imports root only
internal/datasource/       ← imports root only
internal/llm/              ← imports root only
internal/signal/           ← imports root only
internal/config/           ← imports stdlib only
internal/engine/           ← imports root + all internal/* packages
internal/mcp/              ← imports root + internal/engine
pkg/sdk/                   ← imports root + internal/engine
cmd/bullarc/               ← imports root + pkg/sdk + internal/*
```

No cross-internal imports are permitted. `internal/indicator` must not import
`internal/datasource`, etc. This enforces a clean separation between pure
computation and I/O, making each layer independently testable.

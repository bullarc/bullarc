<p align="center">
  <img src="logo.png" alt="bullarc" width="200"/>
</p>

<h1 align="center">bullarc</h1>

<p align="center">Your terminal is now a trading desk.</p>

bullarc fuses price technicals, news sentiment, social velocity, on-chain flows,
whale alerts, and options activity into a single BUY/SELL/HOLD signal — powered
by 11 indicators and AI-driven confidence scoring.

![bullarc demo](demo.gif)

## Quick Start

```bash
# Install
go install github.com/bullarc/bullarc/cmd/bullarc@latest

# Run the built-in demo (zero config, embedded data)
bullarc demo
```

No API keys, no config files, no data downloads. Results in under a second.

## What Can It Do?

**Analyze** — 6 data layers, 11 indicators, 1 composite signal

```bash
bullarc analyze --symbol AAPL --llm
```

**Backtest** — replay strategies on historical data

```bash
bullarc backtest --csv data.csv --indicators RSI_14,MACD_12_26_9,SuperTrend_7_3.0
```

**Paper trade** — auto-execute with real market data, simulated money

```bash
bullarc paper trade --symbol AAPL --confidence 70
```

**AI review** — Claude analyzes your trading patterns after 20+ trades

```bash
bullarc journal review
```

**MCP server** — let Claude be your analyst in Claude Desktop or Cursor

```bash
MASSIVE_API_KEY=xxx bullarc mcp install   # one-command Claude Code setup
```

## Features

- **11 technical indicators**: SMA, EMA, RSI, MACD, Bollinger Bands, SuperTrend, Stochastic, ATR, VWAP, OBV
- **Multi-source signal fusion**: price + news sentiment + Reddit social velocity + on-chain crypto flows + whale alerts + options activity
- **AI-powered analysis**: LLM-driven signal explanation, market regime detection, anomaly detection, portfolio correlation
- **Backtesting**: historical replay with simulated returns, max drawdown, win rate
- **Paper trading**: auto-execute via Alpaca with configurable confidence threshold
- **Trade journal**: log every trade, query by filters, AI-powered pattern review
- **MCP server**: 10 tools for Claude Desktop / Cursor integration
- **Go SDK**: embed analysis in your own applications
- **Zero-config demo**: embedded sample data, works out of the box

## Data Sources

| Source | Type | Auth |
|--------|------|------|
| **Massive** (formerly Polygon.io) | Equities, crypto, options | API key |
| **Alpaca** | Equities, crypto, paper trading | API key + secret |
| **CSV / JSON** | Local files | None |
| **Reddit (Tradestie)** | Social sentiment | None |
| **Glassnode** | On-chain crypto metrics | API key |
| **WhaleAlert** | Large crypto transfers | API key |
| **Polygon Options** | Unusual options activity | API key |
| **Alpaca News** | News articles | API key |

## Setup

```bash
# Store credentials once (optional — works without any keys via CSV mode)
bullarc configure --massive-key $MASSIVE_API_KEY
bullarc configure --alpaca-key $ALPACA_API_KEY --alpaca-secret $ALPACA_SECRET_KEY
bullarc configure --llm-key $ANTHROPIC_API_KEY

# Or just set env vars — bullarc auto-detects them
export MASSIVE_API_KEY=xxx        # Massive market data
export ALPACA_API_KEY=xxx         # Alpaca market data (alternative)
export ALPACA_SECRET_KEY=xxx
export ANTHROPIC_API_KEY=xxx      # LLM explanations (optional)
```

Or use a config file:

```yaml
data_sources:
  default: massive
  massive:
    enabled: true
    api_key: "${MASSIVE_API_KEY}"

llm:
  provider: anthropic
  api_key: "${ANTHROPIC_API_KEY}"
  model: claude-opus-4-6
```

Credential resolution order: flags > env vars > keystore (`~/.config/bullarc/credentials`) > config file.

## CLI Commands

| Command | Description |
|---------|-------------|
| `bullarc demo` | Zero-config demo with embedded data |
| `bullarc analyze` | One-shot analysis (single or multi-symbol) |
| `bullarc backtest` | Backtest strategy on historical CSV |
| `bullarc watch` | Continuous polling with live signals |
| `bullarc paper trade` | Auto paper trading via Alpaca |
| `bullarc paper positions` | List open paper positions |
| `bullarc paper close-all` | Close all paper positions |
| `bullarc journal list` | List closed trades |
| `bullarc journal query` | Filter trades by symbol, date, winners/losers |
| `bullarc journal review` | AI-powered trade pattern review |
| `bullarc mcp` | Start MCP server for Claude Desktop / Cursor |
| `bullarc mcp install` | One-command MCP setup for Claude Code |
| `bullarc mcp uninstall` | Remove MCP server from Claude Code |
| `bullarc configure` | Store credentials and watchlist |
| `bullarc version` | Print version |

## MCP Server

```bash
# One-command setup for Claude Code (auto-detects env vars)
MASSIVE_API_KEY=xxx bullarc mcp install

# Or start manually
bullarc mcp
```

The install command registers bullarc globally in Claude Code — no config files
needed. It detects `MASSIVE_API_KEY`, `ALPACA_API_KEY`, `ALPACA_SECRET_KEY`, and
`ANTHROPIC_API_KEY` from your environment and forwards them to the MCP server.

Exposes 10 tools over JSON-RPC 2.0 stdio:

| Tool | Description |
|------|-------------|
| `get_signals` | Analyze symbols, return composite signal + confidence |
| `backtest_strategy` | Run backtest on CSV, return summary stats |
| `list_indicators` | List all registered indicator metadata |
| `explain_signal` | LLM explanation of trading signal |
| `stream_signals` | Push-based signal delivery |
| `explain_backtest` | Backtest + AI explanation of performance |
| `get_news_sentiment` | Fetch and score news sentiment |
| `get_risk_metrics` | ATR-based position sizing and stop-loss |
| `analyze_with_ai` | Multi-step LLM reasoning |
| `compare_symbols` | Compare signals across multiple symbols |

## Architecture

```
bullarc.go               # All interfaces and types. The API contract.
cmd/bullarc/             # CLI (cobra). Thin wrapper over the SDK.
internal/engine/         # Wires indicators, data sources, LLMs together.
internal/indicator/      # Pure computation. No I/O, no imports from other internals.
internal/datasource/     # Market data fetching. Implements DataSource interface.
internal/llm/            # LLM adapters. Implements LLMProvider interface.
internal/signal/         # Aggregates indicator outputs into Signal values.
internal/config/         # Config loading and credential storage.
internal/mcp/            # MCP server: JSON-RPC 2.0 over stdio.
internal/journal/        # Trade journal logging and querying.
pkg/sdk/                 # Public Go SDK. Wraps the engine behind a stable surface.
testutil/                # Shared test helpers.
testdata/                # CSV fixtures and reference values.
```

All shared types and interfaces live in the root package. Internal packages
implement those interfaces but never import each other. Only `internal/engine`
wires them together.

## Go SDK

```go
client := sdk.New(engine)

// One-shot analysis
result, err := client.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL", UseLLM: true})

// Backtest
result, err := client.Backtest(ctx, bullarc.BacktestRequest{Symbol: "AAPL", Bars: bars})

// Stream signals (polling)
ch := client.StreamSymbols(ctx, []string{"AAPL", "MSFT"}, 5*time.Minute)
for sig := range ch {
    // sig.Type, sig.Confidence, sig.Symbol
}

// Push-based subscription
ch := client.Subscribe(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
```

## Build

```bash
make build       # Produces bin/bullarc
make test        # All tests with race detector
make check       # fmt + vet + test
make verify      # Build all + smoke tests
make lint        # golangci-lint
make demo        # Run the built-in demo
make demo-gif    # Generate demo.gif (requires VHS)
```

## Docker

```bash
docker run --rm \
  -e MASSIVE_API_KEY=<key> \
  -e ANTHROPIC_API_KEY=<key> \
  ghcr.io/bullarc/bullarc:latest analyze --symbol AAPL --llm
```

## Requirements

- Go 1.22+
- API keys for data sources and LLM providers (optional — CSV mode works without any)

## License

MIT — see [LICENSE](LICENSE) for details.

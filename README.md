# bullarc

A modular technical analysis engine in Go. Computes indicators from OHLCV data,
generates buy/sell/hold signals with confidence scores, and optionally routes
results through an LLM for plain-English explanation. Exposes capabilities as a
CLI, a Go SDK, and an MCP server.

**Status:** Core architecture and interfaces are complete. Indicator implementations
(SMA, EMA, RSI, MACD, Bollinger Bands) and data source adapters (Polygon, Alpaca)
are in progress.

## Architecture

The design follows a strict dependency rule: all shared types and interfaces live
in the root package (`github.com/bullarc/bullarc`). Internal packages implement
those interfaces but never import each other. Only `internal/engine` is permitted
to import multiple internal packages, which is where they get wired together.

```
bullarc.go               # All interfaces and types. The API contract.
cmd/bullarc/             # CLI (cobra). Thin wrapper over the SDK.
internal/engine/         # Wires indicators, data sources, LLMs together.
internal/indicator/      # Pure computation. No I/O, no imports from other internals.
internal/datasource/     # Market data fetching. Implements DataSource interface.
internal/llm/            # LLM adapters. Implements LLMProvider interface.
internal/signal/         # Aggregates indicator outputs into Signal values.
internal/config/         # Config struct with YAML/JSON tags. Loader not yet wired.
internal/mcp/            # MCP server: exposes get_signals, explain_signal, etc.
pkg/sdk/                 # Public Go SDK. Wraps the engine behind a stable surface.
testutil/                # Shared test helpers: MakeBars, LoadBarsFromCSV, float assertions.
testdata/                # CSV fixtures and reference_values.json for indicator correctness.
```

The separation between `internal/indicator` and `internal/signal` is intentional.
Indicators are pure mathematical functions over bars — they have no opinion on what
the signal should be. Signal generation is a separate concern that aggregates
multiple indicator outputs, applies weighting, and produces a typed `Signal` with
a confidence score. This makes it straightforward to test each layer independently.

The `pkg/sdk` package exists because `internal/engine` is not a stable public API —
it can change as the wiring evolves. The SDK is the surface external consumers
should depend on.

## Error handling

Errors are typed sentinel values with machine-readable codes:

```go
var ErrInsufficientData = &Error{Code: "INSUFFICIENT_DATA", ...}
```

Callers can wrap them with context while preserving the type:

```go
return bullarc.ErrInsufficientData.Wrap(fmt.Errorf("need %d bars, got %d", needed, got))
```

And check by unwrapping:

```go
if errors.Is(err, bullarc.ErrInsufficientData) { ... }
```

Raw `errors.New` from public APIs is not permitted. This makes error handling at
the engine level uniform regardless of which indicator or data source originated
the failure.

## Data model

The core type is `OHLCV` (Open, High, Low, Close, Volume), aliased as `Bar`.
`IndicatorValue` carries a timestamp, a primary float value, and an `Extra` map
for indicators that produce multiple outputs (e.g., MACD produces `macd`,
`signal`, and `histogram`).

`Signal` carries a `Confidence` float (0.0–1.0), the indicator that produced it,
and an `Explanation` string populated when LLM analysis is enabled.

## Requirements

- Go 1.22+
- `golangci-lint` for linting (`brew install golangci-lint`)
- API keys for data sources and LLM providers (via environment variables or config file)

## Build

```bash
make build       # Produces bin/bullarc
make check       # fmt + vet + test
make verify      # Build all packages + run smoke tests
make lint        # golangci-lint
```

**Note on CGO:** Go 1.22 on macOS 15+ has a dyld incompatibility with the race
detector. Run tests with `CGO_ENABLED=0`:

```bash
CGO_ENABLED=0 go test -count=1 ./...
```

The `make test` target handles this automatically.

## Testing

Tests use `testify` for assertions and the `testutil` package for shared helpers.

```bash
make test        # All tests
make test-v      # Verbose
```

Reference values for indicator correctness are in `testdata/reference_values.json`.
These were computed independently and serve as the ground truth for regression
testing — if an indicator implementation drifts, the tests catch it by index.
Test fixtures are real OHLCV data in `testdata/ohlcv_100.csv`.

Indicator tests follow the pattern:

```go
func TestSMA_InsufficientData(t *testing.T) {
    bars := testutil.MakeBars(1.0, 2.0) // fewer than period
    _, err := sma.Compute(bars)
    require.ErrorIs(t, err, bullarc.ErrInsufficientData)
}

func TestSMA_KnownValues(t *testing.T) {
    bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
    vals, err := sma.Compute(bars)
    require.NoError(t, err)
    testutil.AssertFloatEqual(t, 156.017857, vals[0].Value, 1e-4)
}
```

Smoke tests (prefixed `TestSmoke_`) are the subset run by `make verify` — fast
checks that interfaces, types, and error contracts are intact before any
integration work.

## Adding an indicator

1. Create `internal/indicator/name.go` implementing `bullarc.Indicator`.
2. Create `internal/indicator/name_test.go`. Test insufficient data, known values
   from `reference_values.json`, and edge cases.
3. Register in `internal/engine/defaults.go`.
4. `make check`.

The `Compute` method must be pure — no side effects, no I/O, no logging. It takes
a slice of bars and returns a slice of `IndicatorValue`. The output length is
`len(bars) - warmupPeriod + 1`; callers should not have to guess.

## Configuration

Config is YAML or JSON, matching the `Config` struct in `internal/config/config.go`.
API keys can also be provided via environment variables (preferred for secrets).

```yaml
engine:
  default_symbol: AAPL
  default_interval: 1d
  timeout: 30s
  max_bars: 500

data_sources:
  default: polygon
  polygon:
    enabled: true
    api_key: ${POLYGON_API_KEY}

llm:
  provider: anthropic
  model: claude-opus-4-6
  api_key: ${ANTHROPIC_API_KEY}
  max_tokens: 1024
  temperature: 0.2

mcp:
  enabled: false
  address: :8080
```

## MCP server

bullarc exposes an MCP server for integration with LLM tool ecosystems (Claude
Desktop, Cursor, etc.). Tools: `get_signals`, `explain_signal`, `list_indicators`,
`backtest_strategy`.

```bash
bullarc mcp --address :8080
```

## License

MIT

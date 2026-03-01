# Configuration Reference

bullarc reads configuration from three sources, applied in priority order from
highest to lowest:

```
1. CLI flags        (--flag value)
2. Environment variables
3. Keystore         (~/.config/bullarc/credentials, set via `bullarc configure`)
4. Config file      (-c path/to/config.yaml)
```

A value at a higher priority always wins. If a value is absent at every level the
feature it controls is simply disabled (e.g. no Alpaca credentials → no live data).

---

## CLI flags

Every command accepts a `-c / --config` flag to load a YAML config file.
Additional per-command flags override individual config-file values.

### `bullarc analyze`

| Flag | Description |
|------|-------------|
| `-s, --symbol` | Single symbol to analyze (e.g. `AAPL`) |
| `--symbols` | Comma-separated symbols for table output (e.g. `AAPL,MSFT,TSLA`) |
| `-c, --config` | Path to a YAML config file |
| `--csv` | Path to a local OHLCV CSV file (bypasses live data sources) |
| `--llm` | Enable plain-English explanation via LLM |
| `--llm-key` | Anthropic API key — overrides `ANTHROPIC_API_KEY` and keystore |
| `--alpaca-key` | Alpaca API key ID — overrides `ALPACA_API_KEY` and keystore |
| `--alpaca-secret` | Alpaca secret key — overrides `ALPACA_SECRET_KEY` and keystore |

### `bullarc watch`

| Flag | Description |
|------|-------------|
| `-s, --symbol` | Symbol to watch (defaults to first symbol in saved watchlist) |
| `-c, --config` | Path to a YAML config file |
| `--csv` | Path to a local OHLCV CSV file |
| `-i, --interval` | Poll interval as a Go duration (default `1m`, e.g. `30s`, `5m`) |
| `--llm-key` | Anthropic API key |
| `--alpaca-key` | Alpaca API key ID |
| `--alpaca-secret` | Alpaca secret key |

### `bullarc backtest`

| Flag | Description |
|------|-------------|
| `--csv` | Path to a CSV file with historical OHLCV data (**required**) |
| `-s, --symbol` | Ticker label for the backtest output (default `UNKNOWN`) |
| `--indicators` | Comma-separated indicator names to use (default: all registered) |
| `-c, --config` | Path to a YAML config file |

### `bullarc mcp`

| Flag | Description |
|------|-------------|
| `-c, --config` | Path to a YAML config file |

#### `bullarc mcp install`

Registers bullarc as a global MCP server in Claude Code (`~/.claude.json`).
Detects API keys from the current environment and forwards them.

```bash
MASSIVE_API_KEY=xxx bullarc mcp install
```

#### `bullarc mcp uninstall`

Removes the bullarc MCP server entry from Claude Code.

### `bullarc configure`

Persists credentials to `~/.config/bullarc/credentials` (mode `0600`).

| Flag | Description |
|------|-------------|
| `--llm-key` | Anthropic API key to store persistently |
| `--alpaca-key` | Alpaca API key ID to store persistently |
| `--alpaca-secret` | Alpaca secret key to store persistently |
| `--massive-key` | Massive API key to store persistently |
| `--watchlist` | Default symbol list (e.g. `AAPL,MSFT,BTC/USD`) |

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `MASSIVE_API_KEY` | Massive Finance API key for live market data |
| `ALPACA_API_KEY` | Alpaca Markets API key ID for live data |
| `ALPACA_SECRET_KEY` | Alpaca Markets secret key for live data |
| `ANTHROPIC_API_KEY` | Anthropic API key for LLM explanations |

Environment variables override the keystore and config file for the same
credential. Setting them is the recommended approach in CI/CD and containerised
environments.

---

## Keystore (`bullarc configure`)

The keystore stores credentials at `~/.config/bullarc/credentials` with mode
`0600` (readable only by the current user). Use `bullarc configure` to set values:

```bash
# Store Massive API key
bullarc configure --massive-key <your-massive-key>

# Store Alpaca credentials
bullarc configure --alpaca-key PKXXXXXXXXXXXXXXXX --alpaca-secret <secret>

# Store Anthropic API key
bullarc configure --llm-key sk-ant-...

# Set a default watchlist
bullarc configure --watchlist AAPL,MSFT,GOOGL

# All at once
bullarc configure \
  --massive-key <your-massive-key> \
  --alpaca-key PKXXXXXXXXXXXXXXXX \
  --alpaca-secret <secret> \
  --llm-key sk-ant-... \
  --watchlist AAPL,MSFT
```

Keystore values are lower priority than environment variables. Exporting
`ALPACA_API_KEY` in your shell will always take precedence over the stored value.

---

## Config File (YAML)

Pass a config file with `-c config.yaml`. The file is YAML (or JSON) and maps
directly to the `Config` struct in `internal/config/config.go`.

### Full example

```yaml
engine:
  default_symbol: AAPL        # used when no --symbol flag is given
  default_interval: 1Day      # data bar interval
  timeout: 30s                # per-request timeout
  max_bars: 500               # maximum bars fetched per query

data_sources:
  default: alpaca             # which source to use when multiple are enabled
  alpaca:
    enabled: true
    key_id: PKXXXXXXXXXXXXXXXX       # can also use env var ALPACA_API_KEY
    secret_key: <alpaca-secret>      # can also use env var ALPACA_SECRET_KEY
    base_url: ""                     # optional, override for paper trading

indicators:
  enabled:                    # restrict to this subset; omit to use all defaults
    - SMA_14
    - SMA_50
    - RSI_14
    - MACD_12_26_9
  parameters: {}              # reserved for future per-indicator config

llm:
  provider: anthropic
  model: claude-opus-4-6      # defaults to latest model when omitted
  api_key: ""                 # prefer ANTHROPIC_API_KEY env var
  max_tokens: 1024
  temperature: 0.2

mcp:
  enabled: false
  address: :8080              # unused — MCP server runs on stdio

webhooks:
  enabled: false
  urls:
    - https://hooks.example.com/bullarc
  timeout: 5s
```

### Indicator names

The `indicators.enabled` list accepts the default indicator names and any
parameterised name pattern:

| Pattern | Examples |
|---------|----------|
| `SMA_<period>` | `SMA_14`, `SMA_50`, `SMA_200` |
| `EMA_<period>` | `EMA_14`, `EMA_20` |
| `RSI_<period>` | `RSI_14`, `RSI_21` |
| `ATR_<period>` | `ATR_14` |
| `MACD_<fast>_<slow>_<sig>` | `MACD_12_26_9`, `MACD_10_22_9` |
| `BB_<period>_<mult>` | `BB_20_2`, `BB_20_2.5` |
| `SuperTrend_<period>_<mult>` | `SuperTrend_7_3`, `SuperTrend_10_2` |
| `Stoch_<period>_<smoothK>_<smoothD>` | `Stoch_14_3_3` |
| `VWAP` | `VWAP` |
| `OBV` | `OBV` |

Default indicators (used when `indicators.enabled` is empty or absent):
`SMA_14`, `SMA_50`, `EMA_14`, `RSI_14`, `MACD_12_26_9`, `BB_20_2`, `ATR_14`,
`SuperTrend_7_3`, `Stoch_14_3_3`, `VWAP`, `OBV`.

---

## Precedence Summary

```
Credential: Alpaca API key
  1. --alpaca-key flag
  2. ALPACA_API_KEY environment variable
  3. bullarc configure --alpaca-key (keystore)
  4. data_sources.alpaca.key_id in config file (only when enabled: true)

Credential: Alpaca secret key
  1. --alpaca-secret flag
  2. ALPACA_SECRET_KEY environment variable
  3. bullarc configure --alpaca-secret (keystore)
  4. data_sources.alpaca.secret_key in config file (only when enabled: true)

Credential: Massive API key
  1. MASSIVE_API_KEY environment variable
  2. bullarc configure --massive-key (keystore)
  3. data_sources.massive.api_key in config file (only when enabled: true)

Credential: Anthropic API key
  1. --llm-key flag
  2. ANTHROPIC_API_KEY environment variable
  3. bullarc configure --llm-key (keystore)
  4. llm.api_key in config file

Default watchlist
  1. --symbol / --symbols flags
  2. bullarc configure --watchlist (keystore)
  (no config file equivalent)
```

---

## Minimal working setup (environment variables only)

```bash
# Option A: Massive
export MASSIVE_API_KEY=<your-massive-key>

# Option B: Alpaca
export ALPACA_API_KEY=PKXXXXXXXXXXXXXXXX
export ALPACA_SECRET_KEY=<secret>

bullarc analyze --symbol AAPL
```

## Minimal working setup (keystore)

```bash
bullarc configure \
  --alpaca-key PKXXXXXXXXXXXXXXXX \
  --alpaca-secret <secret> \
  --watchlist AAPL,MSFT,GOOGL

bullarc analyze          # uses saved watchlist
bullarc watch            # watches first symbol (AAPL)
```

## Minimal working setup (config file)

```yaml
# config.yaml
data_sources:
  alpaca:
    enabled: true
    key_id: PKXXXXXXXXXXXXXXXX
    secret_key: <secret>
```

```bash
bullarc analyze -c config.yaml --symbol AAPL
```

# Getting Started

This guide takes you from zero to a working watchlist in under five minutes.

---

## Prerequisites

- Go 1.22 or later (`go version` to check)
- An [Alpaca Markets](https://app.alpaca.markets) account with API credentials
  (free paper-trading account is sufficient)
- Optional: An [Anthropic](https://console.anthropic.com) API key for
  plain-English signal explanations

---

## Step 1 — Install

```bash
go install github.com/bullarc/bullarc/cmd/bullarc@latest
```

Or build from source:

```bash
git clone https://github.com/bullarc/bullarc.git
cd bullarc
make build          # produces bin/bullarc
export PATH="$PWD/bin:$PATH"
```

Verify the install:

```bash
bullarc version
# bullarc dev  (or a release tag if installed via go install)
```

---

## Step 2 — Set your API credentials

The quickest way to make credentials available to every future command is to
save them to the keystore:

```bash
bullarc configure \
  --alpaca-key   PKXXXXXXXXXXXXXXXX \
  --alpaca-secret <your-alpaca-secret>
```

This writes `~/.config/bullarc/credentials` (mode `0600`). You only need to do
this once. To verify the file was written:

```bash
ls -l ~/.config/bullarc/credentials
# -rw------- 1 you staff 128 ...
```

Alternatively, export environment variables in your shell profile:

```bash
export ALPACA_API_KEY=PKXXXXXXXXXXXXXXXX
export ALPACA_SECRET_KEY=<your-alpaca-secret>
```

Environment variables take priority over the keystore. See
[configuration.md](configuration.md) for the full precedence order.

---

## Step 3 — Analyse your first symbol

```bash
bullarc analyze --symbol AAPL
```

You should see output similar to:

```
Symbol: AAPL
Timestamp: 2026-02-27T14:30:00Z

Signals:
  SMA_14       HOLD   0.50
  RSI_14       BUY    0.72
  MACD_12_26_9 BUY    0.65
  ...
```

Each row is one indicator's signal (BUY / SELL / HOLD) and its confidence
score (0.0–1.0).

---

## Step 4 — Set up a watchlist

Save a default watchlist so you don't have to type symbols every time:

```bash
bullarc configure --watchlist AAPL,MSFT,GOOGL
```

Now `analyze` uses the watchlist automatically and prints a summary table:

```bash
bullarc analyze
```

```
SYMBOL  SIGNAL  CONFIDENCE  TIMESTAMP
AAPL    BUY     0.68        2026-02-27T14:30:00Z
MSFT    HOLD    0.51        2026-02-27T14:30:00Z
GOOGL   SELL    0.44        2026-02-27T14:30:00Z
```

---

## Step 5 — Enable LLM explanations (optional)

If you have an Anthropic API key, store it and re-run with `--llm`:

```bash
bullarc configure --llm-key sk-ant-...
bullarc analyze --symbol AAPL --llm
```

The output gains an `Explanation` field with a plain-English summary of what
the indicators mean.

---

## Step 6 — Watch a symbol continuously

`watch` polls at a configurable interval and prints results whenever new bars
arrive:

```bash
bullarc watch --symbol AAPL --interval 1m
# watching AAPL every 1m0s (ctrl-c to stop)
```

Without `--symbol`, the first symbol from your saved watchlist is used:

```bash
bullarc watch           # watches AAPL (first in the list)
```

---

## Step 7 — Run a backtest (optional)

If you have a CSV file with historical OHLCV data, you can backtest your
indicator set against it:

```bash
bullarc backtest --csv testdata/ohlcv_100.csv --symbol AAPL
```

The CSV must have a header row followed by columns in this order:
`date,open,high,low,close,volume` with dates in `2006-01-02` format.

---

## Next steps

- Read [configuration.md](configuration.md) to understand every option and the
  full precedence order.
- Read [architecture.md](architecture.md) to understand how the engine works.
- Read [sdk.md](sdk.md) to embed bullarc in your own Go program.
- Run `bullarc --help` or `bullarc <command> --help` for inline CLI reference.

---

## Troubleshooting

### "no data source configured"

You have not provided Alpaca credentials. Either:

```bash
# Option A: environment variables
export ALPACA_API_KEY=...
export ALPACA_SECRET_KEY=...

# Option B: keystore (persists across sessions)
bullarc configure --alpaca-key ... --alpaca-secret ...

# Option C: local CSV (no live data needed)
bullarc analyze --symbol MYSTOCK --csv /path/to/data.csv
```

### "not enough data bars for computation"

Some indicators need a minimum number of bars (e.g. SMA_50 needs ≥ 50 bars).
Increase the `engine.max_bars` setting in your config file, or restrict the
indicator set to shorter-period ones:

```bash
bullarc analyze --symbol AAPL     # uses all default indicators
```

If you are using a CSV file, make sure it has enough rows.

### Credentials not being picked up

Check the priority order. A keystore value is ignored when the corresponding
environment variable is set. Run `env | grep ALPACA` or `env | grep ANTHROPIC`
to see what is exported in your current shell.

### "provide --symbol or --symbols" when using analyze

You have not set a default watchlist. Either pass `--symbol AAPL` explicitly
or save a watchlist:

```bash
bullarc configure --watchlist AAPL,MSFT
```

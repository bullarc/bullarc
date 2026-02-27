package signal_test

import (
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeChainMetrics builds a ChainMetrics value for testing.
func makeChainMetrics(symbol string, netFlow float64, dir bullarc.FlowDirection) *bullarc.ChainMetrics {
	return &bullarc.ChainMetrics{
		Symbol:        symbol,
		NetFlow:       netFlow,
		FlowDirection: dir,
		Timestamp:     time.Now().UTC(),
	}
}

// makeWhaleTx builds a WhaleTransaction value for testing.
func makeWhaleTx(symbol string, amountUSD float64, toType string) bullarc.WhaleTransaction {
	return bullarc.WhaleTransaction{
		Amount:    10.0,
		AmountUSD: amountUSD,
		Symbol:    symbol,
		FromType:  "unknown",
		ToType:    toType,
		TxHash:    "txhash",
		Timestamp: time.Now().UTC(),
	}
}

// TestOnChainSignal_EquitySymbol_NilResult verifies that equity symbols (no "/")
// produce no on-chain signal.
func TestOnChainSignal_EquitySymbol_NilResult(t *testing.T) {
	metrics := makeChainMetrics("AAPL", -500, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{makeWhaleTx("AAPL", 1_000_000, "wallet")}

	sig := signal.OnChainSignal("AAPL", metrics, txns)
	assert.Nil(t, sig, "equity symbols must produce no on-chain signal")

	sig = signal.OnChainSignal("MSFT", metrics, txns)
	assert.Nil(t, sig)

	sig = signal.OnChainSignal("TSLA", nil, nil)
	assert.Nil(t, sig)
}

// TestOnChainSignal_NilMetrics_NilResult verifies that nil chain metrics yields
// no signal even for a crypto symbol.
func TestOnChainSignal_NilMetrics_NilResult(t *testing.T) {
	txns := []bullarc.WhaleTransaction{makeWhaleTx("BTC/USD", 1_000_000, "wallet")}
	sig := signal.OnChainSignal("BTC/USD", nil, txns)
	assert.Nil(t, sig)
}

// TestOnChainSignal_EmptyTxns_NilResult verifies that an empty whale transaction
// slice yields no signal.
func TestOnChainSignal_EmptyTxns_NilResult(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -500, bullarc.FlowDirectionOutflow)
	sig := signal.OnChainSignal("BTC/USD", metrics, nil)
	assert.Nil(t, sig)

	sig = signal.OnChainSignal("BTC/USD", metrics, []bullarc.WhaleTransaction{})
	assert.Nil(t, sig)
}

// TestOnChainSignal_UnknownTxnsOnly_NilResult verifies that whale transactions
// with unknown destination types (neither "exchange" nor "wallet") yield no signal.
func TestOnChainSignal_UnknownTxnsOnly_NilResult(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -500, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 2_000_000, "unknown"),
	}
	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	assert.Nil(t, sig)
}

// TestOnChainSignal_Buy_NetOutflow_WhaleAccumulation verifies that net outflow
// combined with whale accumulation (moving to cold wallets) generates a BUY signal.
func TestOnChainSignal_Buy_NetOutflow_WhaleAccumulation(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -800, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 7_000_000, "wallet"),
		makeWhaleTx("BTC/USD", 3_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)

	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, "on_chain", sig.Indicator)
	assert.Equal(t, "BTC/USD", sig.Symbol)
	assert.Greater(t, sig.Confidence, 50.0)
}

// TestOnChainSignal_Sell_NetInflow_WhaleDistribution verifies that net inflow
// combined with whale distribution (moving to exchanges) generates a SELL signal.
func TestOnChainSignal_Sell_NetInflow_WhaleDistribution(t *testing.T) {
	metrics := makeChainMetrics("ETH/USD", 1200, bullarc.FlowDirectionInflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("ETH/USD", 6_000_000, "exchange"),
		makeWhaleTx("ETH/USD", 4_000_000, "exchange"),
	}

	sig := signal.OnChainSignal("ETH/USD", metrics, txns)
	require.NotNil(t, sig)

	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.Equal(t, "on_chain", sig.Indicator)
	assert.Equal(t, "ETH/USD", sig.Symbol)
	assert.Greater(t, sig.Confidence, 50.0)
}

// TestOnChainSignal_Hold_NetOutflow_WhaleDistribution verifies that conflicting
// signals (outflow but whales moving to exchanges) result in a HOLD.
func TestOnChainSignal_Hold_NetOutflow_WhaleDistribution(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -400, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 8_000_000, "exchange"),
		makeWhaleTx("BTC/USD", 2_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
	assert.InDelta(t, 50.0, sig.Confidence, 0.001)
}

// TestOnChainSignal_Hold_NetInflow_WhaleAccumulation verifies that conflicting
// signals (inflow but whales accumulating to wallets) result in a HOLD.
func TestOnChainSignal_Hold_NetInflow_WhaleAccumulation(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", 300, bullarc.FlowDirectionInflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 3_000_000, "exchange"),
		makeWhaleTx("BTC/USD", 7_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
}

// TestOnChainSignal_Confidence_100PercentWhaleDirection verifies full confidence
// when all whale USD flows in one direction and chain metrics confirms.
func TestOnChainSignal_Confidence_100PercentWhaleDirection(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -1000, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 5_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	// 100% wallet + flow boost (capped at 100)
	assert.LessOrEqual(t, sig.Confidence, 100.0)
	assert.GreaterOrEqual(t, sig.Confidence, 50.0)
}

// TestOnChainSignal_Confidence_SellProportional verifies confidence is proportional
// to the whale directional dominance for a SELL signal.
func TestOnChainSignal_Confidence_SellProportional(t *testing.T) {
	metrics := makeChainMetrics("ETH/USD", 500, bullarc.FlowDirectionInflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("ETH/USD", 6_000_000, "exchange"),
		makeWhaleTx("ETH/USD", 4_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("ETH/USD", metrics, txns)
	require.NotNil(t, sig)
	// walletUSD (4M) > exchangeUSD (6M)? No: exchange > wallet, so it should be HOLD
	// because inflow requires exchangeUSD > walletUSD *strictly*
	// Here 6M exchange > 4M wallet → SELL should be produced
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	// Base confidence = 6M/10M * 100 = 60; plus flow boost for netFlow=500 → +2.5
	assert.InDelta(t, 62.5, sig.Confidence, 0.001)
}

// TestOnChainSignal_MetadataPopulated verifies that signal metadata fields are
// correctly populated.
func TestOnChainSignal_MetadataPopulated(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -600, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 4_000_000, "wallet"),
		makeWhaleTx("BTC/USD", 1_000_000, "exchange"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)
	require.NotNil(t, sig.Metadata)

	assert.Equal(t, 2, sig.Metadata["transaction_count"])
	assert.InDelta(t, 5_000_000.0, sig.Metadata["total_usd"], 0.001)
	assert.InDelta(t, 1_000_000.0, sig.Metadata["exchange_usd"], 0.001)
	assert.InDelta(t, 4_000_000.0, sig.Metadata["wallet_usd"], 0.001)
	assert.InDelta(t, -600.0, sig.Metadata["net_flow"], 0.001)
	assert.Equal(t, string(bullarc.FlowDirectionOutflow), sig.Metadata["flow_direction"])
}

// TestOnChainSignal_ExplanationContent verifies that the signal explanation
// mentions key quantities.
func TestOnChainSignal_ExplanationContent(t *testing.T) {
	metrics := makeChainMetrics("BTC/USD", -500, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 3_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)
	assert.Contains(t, sig.Explanation, "1 whale transaction(s)")
	assert.Contains(t, sig.Explanation, "net outflow + whale accumulation")
}

// TestOnChainSignal_ParticipatesInAggregate verifies that on-chain signals can
// participate in signal.Aggregate() like any other signal.
func TestOnChainSignal_ParticipatesInAggregate(t *testing.T) {
	metricsOutflow := makeChainMetrics("BTC/USD", -1000, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 5_000_000, "wallet"),
	}
	onChain := signal.OnChainSignal("BTC/USD", metricsOutflow, txns)
	require.NotNil(t, onChain)

	// Add another signal and aggregate.
	other := bullarc.Signal{
		Type:       bullarc.SignalBuy,
		Confidence: 70,
		Indicator:  "rsi",
		Symbol:     "BTC/USD",
		Timestamp:  time.Now().UTC(),
	}

	composite := signal.Aggregate("BTC/USD", []bullarc.Signal{*onChain, other})
	assert.Equal(t, bullarc.SignalBuy, composite.Type)
	assert.Equal(t, "composite", composite.Indicator)
}

// TestOnChainSignal_EqualWhaleFlow_NeitherDirectionDominates verifies that
// when exchange and wallet USD are equal, HOLD is produced regardless of chain flow.
func TestOnChainSignal_EqualWhaleFlow_NeitherDirectionDominates(t *testing.T) {
	// Equal USD: walletUSD == exchangeUSD, strict > check fails for both.
	metrics := makeChainMetrics("BTC/USD", -1000, bullarc.FlowDirectionOutflow)
	txns := []bullarc.WhaleTransaction{
		makeWhaleTx("BTC/USD", 5_000_000, "exchange"),
		makeWhaleTx("BTC/USD", 5_000_000, "wallet"),
	}

	sig := signal.OnChainSignal("BTC/USD", metrics, txns)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
}

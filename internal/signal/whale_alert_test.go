package signal_test

import (
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeWhale(symbol string, amountUSD float64, toType string) bullarc.WhaleTransaction {
	return bullarc.WhaleTransaction{
		Amount:    10.0,
		AmountUSD: amountUSD,
		Symbol:    symbol,
		FromType:  "unknown",
		ToType:    toType,
		TxHash:    "hash1",
		Timestamp: time.Now().UTC(),
	}
}

// TestWhaleAlertSignal_Empty verifies that no signal is returned when the
// transaction slice is empty.
func TestWhaleAlertSignal_Empty(t *testing.T) {
	sig := signal.WhaleAlertSignal("BTC", nil)
	assert.Nil(t, sig)

	sig = signal.WhaleAlertSignal("BTC", []bullarc.WhaleTransaction{})
	assert.Nil(t, sig)
}

// TestWhaleAlertSignal_ExchangeBound_Bearish verifies that transfers to
// exchanges generate a SELL signal with the whale_exchange_inflow risk flag.
func TestWhaleAlertSignal_ExchangeBound_Bearish(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 3_000_000, "exchange"),
		makeWhale("BTC", 2_000_000, "exchange"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	require.NotNil(t, sig)

	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.Contains(t, sig.RiskFlags, signal.RiskFlagWhaleExchangeInflow)
	assert.Equal(t, "whale_alert", sig.Indicator)
	assert.Equal(t, "BTC", sig.Symbol)
	assert.InDelta(t, 100.0, sig.Confidence, 0.001, "100% exchange-bound = full confidence")
}

// TestWhaleAlertSignal_ColdStorageBound_Bullish verifies that transfers to
// cold wallets generate a BUY signal with the whale_cold_storage_outflow flag.
func TestWhaleAlertSignal_ColdStorageBound_Bullish(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("ETH", 5_000_000, "wallet"),
	}
	sig := signal.WhaleAlertSignal("ETH", txns)
	require.NotNil(t, sig)

	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Contains(t, sig.RiskFlags, signal.RiskFlagWhaleColdStorageOutflow)
	assert.InDelta(t, 100.0, sig.Confidence, 0.001)
}

// TestWhaleAlertSignal_MixedFlow_ExchangeDominant verifies that when exchange
// USD exceeds wallet USD the signal is SELL.
func TestWhaleAlertSignal_MixedFlow_ExchangeDominant(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 6_000_000, "exchange"),
		makeWhale("BTC", 4_000_000, "wallet"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	require.NotNil(t, sig)

	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.InDelta(t, 60.0, sig.Confidence, 0.001, "6M / 10M = 60%")
}

// TestWhaleAlertSignal_MixedFlow_WalletDominant verifies that when wallet USD
// exceeds exchange USD the signal is BUY.
func TestWhaleAlertSignal_MixedFlow_WalletDominant(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 3_000_000, "exchange"),
		makeWhale("BTC", 7_000_000, "wallet"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	require.NotNil(t, sig)

	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.InDelta(t, 70.0, sig.Confidence, 0.001, "7M / 10M = 70%")
}

// TestWhaleAlertSignal_UnknownOnly verifies that transactions with unknown
// destination type do not produce a directional signal.
func TestWhaleAlertSignal_UnknownOnly(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 2_000_000, "unknown"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	assert.Nil(t, sig, "unknown-only destinations should yield no signal")
}

// TestWhaleAlertSignal_MetadataPopulated verifies that the metadata fields are
// present and accurate.
func TestWhaleAlertSignal_MetadataPopulated(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 4_000_000, "exchange"),
		makeWhale("BTC", 1_000_000, "wallet"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	require.NotNil(t, sig)

	require.NotNil(t, sig.Metadata)
	assert.Equal(t, 2, sig.Metadata["transaction_count"])
	assert.InDelta(t, 5_000_000.0, sig.Metadata["total_usd"], 0.001)
	assert.InDelta(t, 4_000_000.0, sig.Metadata["exchange_usd"], 0.001)
	assert.InDelta(t, 1_000_000.0, sig.Metadata["wallet_usd"], 0.001)
}

// TestWhaleAlertSignal_ExplanationContainsTxnCount verifies explanation string.
func TestWhaleAlertSignal_ExplanationContainsTxnCount(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 2_000_000, "exchange"),
		makeWhale("BTC", 3_000_000, "exchange"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	require.NotNil(t, sig)
	assert.Contains(t, sig.Explanation, "2 whale transaction(s)")
	assert.Contains(t, sig.Explanation, "exchange inflow")
}

// TestWhaleAlertSignal_EqualExchangeAndWallet_FallsToSell verifies that equal
// USD flow (exchange == wallet) results in a SELL signal (exchange takes precedence
// when values are equal).
func TestWhaleAlertSignal_EqualExchangeAndWallet_FallsToSell(t *testing.T) {
	txns := []bullarc.WhaleTransaction{
		makeWhale("BTC", 5_000_000, "exchange"),
		makeWhale("BTC", 5_000_000, "wallet"),
	}
	sig := signal.WhaleAlertSignal("BTC", txns)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.InDelta(t, 50.0, sig.Confidence, 0.001)
}

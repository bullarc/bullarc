package signal

import (
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testExpiry = time.Date(2024, 6, 21, 0, 0, 0, 0, time.UTC)

func makeOptionsActivity(symbol, direction string, premium float64, actType bullarc.OptionsActivityType) bullarc.OptionsActivity {
	return bullarc.OptionsActivity{
		Symbol:       symbol,
		Strike:       150.0,
		Expiration:   testExpiry,
		Direction:    direction,
		Volume:       500,
		OpenInterest: 100,
		Premium:      premium,
		ActivityType: actType,
	}
}

func TestOptionsActivitySignal_EmptyEvents_ReturnsNil(t *testing.T) {
	sig := OptionsActivitySignal(nil)
	assert.Nil(t, sig)

	sig2 := OptionsActivitySignal([]bullarc.OptionsActivity{})
	assert.Nil(t, sig2)
}

func TestOptionsActivitySignal_CallDominant_ReturnsBuy(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("AAPL", "call", 300_000, bullarc.OptionsActivityBlock),
		makeOptionsActivity("AAPL", "put", 100_000, bullarc.OptionsActivityUnusualVolume),
	}

	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, "AAPL", sig.Symbol)
	assert.Equal(t, "options_activity", sig.Indicator)
	assert.InDelta(t, 75.0, sig.Confidence, 0.01) // 300k / 400k
	assert.Contains(t, sig.RiskFlags, RiskFlagUnusualCallActivity)
	assert.NotEmpty(t, sig.Explanation)
}

func TestOptionsActivitySignal_PutDominant_ReturnsSell(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("TSLA", "put", 500_000, bullarc.OptionsActivitySweep),
		makeOptionsActivity("TSLA", "call", 100_000, bullarc.OptionsActivityBlock),
	}

	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.Equal(t, "TSLA", sig.Symbol)
	assert.InDelta(t, 83.333, sig.Confidence, 0.01) // 500k / 600k
	assert.Contains(t, sig.RiskFlags, RiskFlagUnusualPutActivity)
}

func TestOptionsActivitySignal_EqualPremium_ReturnsHold(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("SPY", "call", 200_000, bullarc.OptionsActivityBlock),
		makeOptionsActivity("SPY", "put", 200_000, bullarc.OptionsActivityBlock),
	}

	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
	assert.InDelta(t, 50.0, sig.Confidence, 0.01)
}

func TestOptionsActivitySignal_OnlyCallsNoSweep_Buy100Confidence(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("NVDA", "call", 150_000, bullarc.OptionsActivityBlock),
	}

	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01)
}

func TestOptionsActivitySignal_OnlyPuts_Sell100Confidence(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("AMD", "put", 200_000, bullarc.OptionsActivitySweep),
	}

	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01)
}

func TestOptionsActivitySignal_ZeroPremium_ReturnsNil(t *testing.T) {
	events := []bullarc.OptionsActivity{
		{Symbol: "AAPL", Direction: "call", Premium: 0, ActivityType: bullarc.OptionsActivityUnusualVolume},
	}

	sig := OptionsActivitySignal(events)
	assert.Nil(t, sig)
}

func TestOptionsActivitySignal_Metadata_Populated(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("AAPL", "call", 200_000, bullarc.OptionsActivitySweep),
		makeOptionsActivity("AAPL", "call", 100_000, bullarc.OptionsActivityBlock),
		makeOptionsActivity("AAPL", "put", 50_000, bullarc.OptionsActivityUnusualVolume),
	}

	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)

	assert.Equal(t, 3, sig.Metadata["event_count"])
	assert.InDelta(t, 300_000.0, sig.Metadata["call_premium"], 1)
	assert.InDelta(t, 50_000.0, sig.Metadata["put_premium"], 1)
	assert.InDelta(t, 350_000.0, sig.Metadata["total_premium"], 1)
	assert.Equal(t, 1, sig.Metadata["sweep_count"])
	assert.Equal(t, 1, sig.Metadata["block_count"])
	assert.Equal(t, 1, sig.Metadata["unusual_vol_count"])
}

func TestOptionsActivitySignal_Timestamp_IsRecent(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("AAPL", "call", 150_000, bullarc.OptionsActivityBlock),
	}
	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.True(t, sig.Timestamp.After(before))
}

func TestOptionsActivitySignal_ExplanationContainsKeyInfo(t *testing.T) {
	events := []bullarc.OptionsActivity{
		makeOptionsActivity("AAPL", "call", 300_000, bullarc.OptionsActivitySweep),
	}
	sig := OptionsActivitySignal(events)
	require.NotNil(t, sig)
	assert.Contains(t, sig.Explanation, "sweep")
	assert.Contains(t, sig.Explanation, "call buying")
}

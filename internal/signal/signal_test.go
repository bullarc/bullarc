package signal_test

import (
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testSymbol = "AAPL"
	testBar    = bullarc.OHLCV{
		Time:   time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC),
		Open:   150,
		High:   155,
		Low:    148,
		Close:  152,
		Volume: 1_000_000,
	}
)

func iv(value float64, extra map[string]float64) bullarc.IndicatorValue {
	return bullarc.IndicatorValue{
		Time:  testBar.Time,
		Value: value,
		Extra: extra,
	}
}

// TestForIndicator verifies that indicator names route to the correct generator.
func TestForIndicator(t *testing.T) {
	cases := []struct {
		name    string
		wantNil bool
	}{
		{"RSI_14", false},
		{"MACD_12_26_9", false},
		{"BB_20_2.0", false},
		{"SMA_14", false},
		{"SMA_50", false},
		{"EMA_14", false},
		{"SuperTrend_7_3.0", false},
		{"Stoch_14_3_3", false},
		{"VWAP", false},
		{"OBV", false},
		{"ATR_14", true},
		{"Unknown", true},
		{"", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gen := signal.ForIndicator(tc.name)
			if tc.wantNil {
				assert.Nil(t, gen)
			} else {
				assert.NotNil(t, gen)
			}
		})
	}
}

func TestRSIGenerator(t *testing.T) {
	gen := signal.ForIndicator("RSI_14")
	require.NotNil(t, gen)

	cases := []struct {
		name        string
		rsi         float64
		wantType    bullarc.SignalType
		wantMinConf float64
	}{
		{"deeply oversold (<20)", 15, bullarc.SignalBuy, 80},
		{"oversold (<30)", 25, bullarc.SignalBuy, 60},
		{"neutral low", 40, bullarc.SignalHold, 45},
		{"neutral high", 60, bullarc.SignalHold, 45},
		{"overbought (>70)", 75, bullarc.SignalSell, 60},
		{"deeply overbought (>80)", 85, bullarc.SignalSell, 80},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("RSI_14", testSymbol, testBar, []bullarc.IndicatorValue{iv(tc.rsi, nil)})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
			assert.GreaterOrEqual(t, sig.Confidence, tc.wantMinConf)
			assert.Equal(t, "RSI_14", sig.Indicator)
			assert.Equal(t, testSymbol, sig.Symbol)
		})
	}

	t.Run("empty values returns no signal", func(t *testing.T) {
		_, ok := gen("RSI_14", testSymbol, testBar, nil)
		assert.False(t, ok)
	})
}

func TestMACDGenerator(t *testing.T) {
	gen := signal.ForIndicator("MACD_12_26_9")
	require.NotNil(t, gen)

	extra := func(hist float64) map[string]float64 {
		return map[string]float64{"histogram": hist, "signal": 1.0}
	}

	cases := []struct {
		name     string
		hist     float64
		wantType bullarc.SignalType
	}{
		{"positive histogram", 0.5, bullarc.SignalBuy},
		{"negative histogram", -0.3, bullarc.SignalSell},
		{"zero histogram", 0, bullarc.SignalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("MACD_12_26_9", testSymbol, testBar,
				[]bullarc.IndicatorValue{iv(1.0, extra(tc.hist))})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
		})
	}

	t.Run("missing histogram returns no signal", func(t *testing.T) {
		_, ok := gen("MACD_12_26_9", testSymbol, testBar,
			[]bullarc.IndicatorValue{iv(1.0, map[string]float64{"signal": 1.0})})
		assert.False(t, ok)
	})

	t.Run("empty values returns no signal", func(t *testing.T) {
		_, ok := gen("MACD_12_26_9", testSymbol, testBar, nil)
		assert.False(t, ok)
	})
}

func TestBBGenerator(t *testing.T) {
	gen := signal.ForIndicator("BB_20_2.0")
	require.NotNil(t, gen)

	bands := func(upper, lower float64) map[string]float64 {
		return map[string]float64{"upper": upper, "lower": lower}
	}

	// testBar.Close = 152
	cases := []struct {
		name     string
		upper    float64
		lower    float64
		wantType bullarc.SignalType
	}{
		{"price below lower band", 160, 155, bullarc.SignalBuy},
		{"price above upper band", 150, 140, bullarc.SignalSell},
		{"price within bands", 160, 145, bullarc.SignalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("BB_20_2.0", testSymbol, testBar,
				[]bullarc.IndicatorValue{iv(152, bands(tc.upper, tc.lower))})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
		})
	}

	t.Run("missing bands returns no signal", func(t *testing.T) {
		_, ok := gen("BB_20_2.0", testSymbol, testBar,
			[]bullarc.IndicatorValue{iv(152, nil)})
		assert.False(t, ok)
	})
}

func TestSMACrossGenerator(t *testing.T) {
	gen := signal.ForIndicator("SMA_14")
	require.NotNil(t, gen)

	// testBar.Close = 152
	cases := []struct {
		name     string
		sma      float64
		wantType bullarc.SignalType
	}{
		{"price >2% above SMA", 148, bullarc.SignalBuy},
		{"price >2% below SMA", 156, bullarc.SignalSell},
		{"price within 2% of SMA", 151.5, bullarc.SignalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("SMA_14", testSymbol, testBar,
				[]bullarc.IndicatorValue{iv(tc.sma, nil)})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
		})
	}
}

func TestEMACrossGenerator(t *testing.T) {
	gen := signal.ForIndicator("EMA_14")
	require.NotNil(t, gen)

	sig, ok := gen("EMA_14", testSymbol, testBar,
		[]bullarc.IndicatorValue{iv(140, nil)})
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
}

func TestSuperTrendGenerator(t *testing.T) {
	gen := signal.ForIndicator("SuperTrend_7_3.0")
	require.NotNil(t, gen)

	cases := []struct {
		name     string
		dir      float64
		wantType bullarc.SignalType
	}{
		{"bullish (direction=1)", 1, bullarc.SignalBuy},
		{"bearish (direction=-1)", -1, bullarc.SignalSell},
		{"no trend (direction=0)", 0, bullarc.SignalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("SuperTrend_7_3.0", testSymbol, testBar,
				[]bullarc.IndicatorValue{iv(150, map[string]float64{"direction": tc.dir})})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
		})
	}

	t.Run("missing direction returns no signal", func(t *testing.T) {
		_, ok := gen("SuperTrend_7_3.0", testSymbol, testBar,
			[]bullarc.IndicatorValue{iv(150, nil)})
		assert.False(t, ok)
	})
}

func TestStochasticGenerator(t *testing.T) {
	gen := signal.ForIndicator("Stoch_14_3_3")
	require.NotNil(t, gen)

	cases := []struct {
		name     string
		k        float64
		wantType bullarc.SignalType
	}{
		{"K < 20 oversold", 15, bullarc.SignalBuy},
		{"K > 80 overbought", 85, bullarc.SignalSell},
		{"K neutral", 50, bullarc.SignalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("Stoch_14_3_3", testSymbol, testBar,
				[]bullarc.IndicatorValue{iv(tc.k, nil)})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
		})
	}
}

func TestVWAPGenerator(t *testing.T) {
	gen := signal.ForIndicator("VWAP")
	require.NotNil(t, gen)

	// testBar.Close = 152
	cases := []struct {
		name     string
		vwap     float64
		wantType bullarc.SignalType
	}{
		{"price above VWAP", 148, bullarc.SignalBuy},
		{"price below VWAP", 156, bullarc.SignalSell},
		{"price equals VWAP", 152, bullarc.SignalHold},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig, ok := gen("VWAP", testSymbol, testBar,
				[]bullarc.IndicatorValue{iv(tc.vwap, nil)})
			require.True(t, ok)
			assert.Equal(t, tc.wantType, sig.Type)
		})
	}
}

func TestOBVGenerator(t *testing.T) {
	gen := signal.ForIndicator("OBV")
	require.NotNil(t, gen)

	makeVals := func(vals ...float64) []bullarc.IndicatorValue {
		out := make([]bullarc.IndicatorValue, len(vals))
		for i, v := range vals {
			out[i] = iv(v, nil)
		}
		return out
	}

	t.Run("rising OBV", func(t *testing.T) {
		sig, ok := gen("OBV", testSymbol, testBar,
			makeVals(1000, 1100, 1200, 1300, 1400))
		require.True(t, ok)
		assert.Equal(t, bullarc.SignalBuy, sig.Type)
	})

	t.Run("falling OBV", func(t *testing.T) {
		sig, ok := gen("OBV", testSymbol, testBar,
			makeVals(1400, 1300, 1200, 1100, 1000))
		require.True(t, ok)
		assert.Equal(t, bullarc.SignalSell, sig.Type)
	})

	t.Run("flat OBV", func(t *testing.T) {
		sig, ok := gen("OBV", testSymbol, testBar,
			makeVals(1000, 1000, 1000, 1000, 1000))
		require.True(t, ok)
		assert.Equal(t, bullarc.SignalHold, sig.Type)
	})

	t.Run("fewer than 5 values returns no signal", func(t *testing.T) {
		_, ok := gen("OBV", testSymbol, testBar, makeVals(1000, 1100, 1200))
		assert.False(t, ok)
	})
}

// TestAggregate_EmptySignals verifies that empty input yields HOLD.
func TestAggregate_EmptySignals(t *testing.T) {
	sig := signal.Aggregate(testSymbol, nil)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
	assert.Equal(t, "composite", sig.Indicator)
	assert.Equal(t, testSymbol, sig.Symbol)
}

// TestAggregate_AllBuy verifies that unanimous BUY signals produce a BUY composite.
func TestAggregate_AllBuy(t *testing.T) {
	signals := []bullarc.Signal{
		{Type: bullarc.SignalBuy, Confidence: 80, Indicator: "RSI_14", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalBuy, Confidence: 70, Indicator: "MACD", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalBuy, Confidence: 75, Indicator: "BB", Symbol: testSymbol, Timestamp: testBar.Time},
	}
	sig := signal.Aggregate(testSymbol, signals)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, "composite", sig.Indicator)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01)
}

// TestAggregate_AllSell verifies that unanimous SELL signals produce a SELL composite.
func TestAggregate_AllSell(t *testing.T) {
	signals := []bullarc.Signal{
		{Type: bullarc.SignalSell, Confidence: 80, Indicator: "RSI_14", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalSell, Confidence: 65, Indicator: "MACD", Symbol: testSymbol, Timestamp: testBar.Time},
	}
	sig := signal.Aggregate(testSymbol, signals)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01)
}

// TestAggregate_MajorityWins verifies that the type with higher total confidence wins.
func TestAggregate_MajorityWins(t *testing.T) {
	signals := []bullarc.Signal{
		{Type: bullarc.SignalBuy, Confidence: 80, Indicator: "A", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalBuy, Confidence: 70, Indicator: "B", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalSell, Confidence: 60, Indicator: "C", Symbol: testSymbol, Timestamp: testBar.Time},
	}
	sig := signal.Aggregate(testSymbol, signals)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	// BUY score = 150, SELL score = 60, total = 210 → confidence ≈ 71.4
	assert.InDelta(t, 150.0/210.0*100, sig.Confidence, 0.1)
}

// TestAggregate_ExplanationFormat verifies the explanation contains vote counts.
func TestAggregate_ExplanationFormat(t *testing.T) {
	signals := []bullarc.Signal{
		{Type: bullarc.SignalBuy, Confidence: 80, Indicator: "A", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalSell, Confidence: 60, Indicator: "B", Symbol: testSymbol, Timestamp: testBar.Time},
		{Type: bullarc.SignalHold, Confidence: 50, Indicator: "C", Symbol: testSymbol, Timestamp: testBar.Time},
	}
	sig := signal.Aggregate(testSymbol, signals)
	assert.Contains(t, sig.Explanation, "1 buy")
	assert.Contains(t, sig.Explanation, "1 sell")
	assert.Contains(t, sig.Explanation, "1 hold")
}

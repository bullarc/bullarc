package indicator

import (
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertUpdateMatchesCompute feeds bars one-by-one via Update on the updater,
// collects non-nil results, and asserts they match batch Compute output exactly.
func assertUpdateMatchesCompute(
	t *testing.T,
	bars []bullarc.OHLCV,
	compute func() ([]bullarc.IndicatorValue, error),
	update func(bar bullarc.OHLCV) *bullarc.IndicatorValue,
	extraKeys []string,
) {
	t.Helper()

	batchVals, err := compute()
	require.NoError(t, err)
	require.NotEmpty(t, batchVals)

	var incremental []bullarc.IndicatorValue
	for _, bar := range bars {
		v := update(bar)
		if v != nil {
			incremental = append(incremental, *v)
		}
	}

	require.Len(t, incremental, len(batchVals),
		"Update produced %d values, Compute produced %d", len(incremental), len(batchVals))

	for i := range batchVals {
		testutil.AssertFloatEqual(t, batchVals[i].Value, incremental[i].Value, 1e-9)
		assert.Equal(t, batchVals[i].Time, incremental[i].Time, "time mismatch at index %d", i)
		for _, key := range extraKeys {
			testutil.AssertFloatEqual(t, batchVals[i].Extra[key], incremental[i].Extra[key], 1e-9)
		}
	}
}

func TestSMA_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("period_14", func(t *testing.T) {
		sma, err := NewSMA(14)
		require.NoError(t, err)
		smaBatch, err := NewSMA(14)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return smaBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return sma.Update(bar) },
			nil,
		)
	})

	t.Run("period_50", func(t *testing.T) {
		sma, err := NewSMA(50)
		require.NoError(t, err)
		smaBatch, err := NewSMA(50)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return smaBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return sma.Update(bar) },
			nil,
		)
	})

	t.Run("period_1", func(t *testing.T) {
		sma, err := NewSMA(1)
		require.NoError(t, err)
		smaBatch, err := NewSMA(1)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return smaBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return sma.Update(bar) },
			nil,
		)
	})
}

func TestSMA_Update_WarmupReturnsNil(t *testing.T) {
	sma, err := NewSMA(5)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4)
	for _, bar := range bars {
		v := sma.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestEMA_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("period_14", func(t *testing.T) {
		ema, err := NewEMA(14)
		require.NoError(t, err)
		emaBatch, err := NewEMA(14)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return emaBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return ema.Update(bar) },
			nil,
		)
	})

	t.Run("period_1", func(t *testing.T) {
		ema, err := NewEMA(1)
		require.NoError(t, err)
		emaBatch, err := NewEMA(1)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return emaBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return ema.Update(bar) },
			nil,
		)
	})
}

func TestEMA_Update_WarmupReturnsNil(t *testing.T) {
	ema, err := NewEMA(5)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4)
	for _, bar := range bars {
		v := ema.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestATR_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("period_14", func(t *testing.T) {
		atr, err := NewATR(14)
		require.NoError(t, err)
		atrBatch, err := NewATR(14)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return atrBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return atr.Update(bar) },
			nil,
		)
	})

	t.Run("period_3", func(t *testing.T) {
		atr, err := NewATR(3)
		require.NoError(t, err)
		atrBatch, err := NewATR(3)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return atrBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return atr.Update(bar) },
			nil,
		)
	})
}

func TestATR_Update_WarmupReturnsNil(t *testing.T) {
	atr, err := NewATR(5)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4, 5)
	for _, bar := range bars {
		v := atr.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestRSI_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("period_14", func(t *testing.T) {
		rsi, err := NewRSI(14)
		require.NoError(t, err)
		rsiBatch, err := NewRSI(14)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return rsiBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return rsi.Update(bar) },
			nil,
		)
	})

	t.Run("period_3", func(t *testing.T) {
		rsi, err := NewRSI(3)
		require.NoError(t, err)
		rsiBatch, err := NewRSI(3)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return rsiBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return rsi.Update(bar) },
			nil,
		)
	})
}

func TestRSI_Update_WarmupReturnsNil(t *testing.T) {
	rsi, err := NewRSI(5)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4, 5)
	for _, bar := range bars {
		v := rsi.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestMACD_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("12_26_9", func(t *testing.T) {
		macd, err := NewMACD(12, 26, 9)
		require.NoError(t, err)
		macdBatch, err := NewMACD(12, 26, 9)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return macdBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return macd.Update(bar) },
			[]string{"signal", "histogram"},
		)
	})

	t.Run("3_5_2", func(t *testing.T) {
		macd, err := NewMACD(3, 5, 2)
		require.NoError(t, err)
		macdBatch, err := NewMACD(3, 5, 2)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return macdBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return macd.Update(bar) },
			[]string{"signal", "histogram"},
		)
	})
}

func TestMACD_Update_WarmupReturnsNil(t *testing.T) {
	macd, err := NewMACD(12, 26, 9)
	require.NoError(t, err)

	// Warmup = slow + signal - 1 = 26 + 9 - 1 = 34 bars needed, so 33 should be nil.
	bars := testutil.MakeBars(make([]float64, 33)...)
	for _, bar := range bars {
		v := macd.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestBollingerBands_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("20_2.0", func(t *testing.T) {
		bb, err := NewBollingerBands(20, 2.0)
		require.NoError(t, err)
		bbBatch, err := NewBollingerBands(20, 2.0)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return bbBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return bb.Update(bar) },
			[]string{"upper", "lower", "bandwidth", "percent_b"},
		)
	})

	t.Run("5_1.5", func(t *testing.T) {
		bb, err := NewBollingerBands(5, 1.5)
		require.NoError(t, err)
		bbBatch, err := NewBollingerBands(5, 1.5)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return bbBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return bb.Update(bar) },
			[]string{"upper", "lower", "bandwidth", "percent_b"},
		)
	})
}

func TestBollingerBands_Update_WarmupReturnsNil(t *testing.T) {
	bb, err := NewBollingerBands(5, 2.0)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4)
	for _, bar := range bars {
		v := bb.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestSuperTrend_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("7_3.0", func(t *testing.T) {
		st, err := NewSuperTrend(7, 3.0)
		require.NoError(t, err)
		stBatch, err := NewSuperTrend(7, 3.0)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return stBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return st.Update(bar) },
			[]string{"direction"},
		)
	})

	t.Run("3_2.0", func(t *testing.T) {
		st, err := NewSuperTrend(3, 2.0)
		require.NoError(t, err)
		stBatch, err := NewSuperTrend(3, 2.0)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return stBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return st.Update(bar) },
			[]string{"direction"},
		)
	})
}

func TestSuperTrend_Update_WarmupReturnsNil(t *testing.T) {
	st, err := NewSuperTrend(7, 3.0)
	require.NoError(t, err)

	// ATR warmup = period+1 = 8 bars, so 7 bars should be nil.
	bars := testutil.MakeBars(1, 2, 3, 4, 5, 6, 7)
	for _, bar := range bars {
		v := st.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestStochastic_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("14_3_3", func(t *testing.T) {
		stoch, err := NewStochastic(14, 3, 3)
		require.NoError(t, err)
		stochBatch, err := NewStochastic(14, 3, 3)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return stochBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return stoch.Update(bar) },
			[]string{"d"},
		)
	})

	t.Run("5_1_1", func(t *testing.T) {
		stoch, err := NewStochastic(5, 1, 1)
		require.NoError(t, err)
		stochBatch, err := NewStochastic(5, 1, 1)
		require.NoError(t, err)

		assertUpdateMatchesCompute(t, bars,
			func() ([]bullarc.IndicatorValue, error) { return stochBatch.Compute(bars) },
			func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return stoch.Update(bar) },
			[]string{"d"},
		)
	})
}

func TestStochastic_Update_WarmupReturnsNil(t *testing.T) {
	stoch, err := NewStochastic(14, 3, 3)
	require.NoError(t, err)

	// Warmup = period + smoothK + smoothD - 2 = 14 + 3 + 3 - 2 = 18, so 17 bars produce nil.
	bars := testutil.MakeBars(make([]float64, 17)...)
	for _, bar := range bars {
		v := stoch.Update(bar)
		assert.Nil(t, v, "should return nil during warmup")
	}
}

func TestVWAP_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	vwap := NewVWAP()
	vwapBatch := NewVWAP()

	assertUpdateMatchesCompute(t, bars,
		func() ([]bullarc.IndicatorValue, error) { return vwapBatch.Compute(bars) },
		func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return vwap.Update(bar) },
		nil,
	)
}

func TestVWAP_Update_AlwaysReturnsValue(t *testing.T) {
	vwap := NewVWAP()

	bars := testutil.MakeBars(10, 20, 30)
	for _, bar := range bars {
		v := vwap.Update(bar)
		require.NotNil(t, v, "VWAP Update should always return a value")
	}
}

func TestOBV_Update_MatchesCompute(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	obv := NewOBV()
	obvBatch := NewOBV()

	assertUpdateMatchesCompute(t, bars,
		func() ([]bullarc.IndicatorValue, error) { return obvBatch.Compute(bars) },
		func(bar bullarc.OHLCV) *bullarc.IndicatorValue { return obv.Update(bar) },
		nil,
	)
}

func TestOBV_Update_AlwaysReturnsValue(t *testing.T) {
	obv := NewOBV()

	bars := testutil.MakeBars(10, 11, 9)
	for _, bar := range bars {
		v := obv.Update(bar)
		require.NotNil(t, v, "OBV Update should always return a value")
	}
}

func TestOBV_Update_MatchesDirectComputation(t *testing.T) {
	obv := NewOBV()

	bars := testutil.MakeBars(10, 11, 12, 11, 10)
	var vals []*bullarc.IndicatorValue
	for _, bar := range bars {
		v := obv.Update(bar)
		vals = append(vals, v)
	}

	require.Len(t, vals, 5)
	testutil.AssertFloatEqual(t, 0.0, vals[0].Value, 1e-9)
	testutil.AssertFloatEqual(t, 1000.0, vals[1].Value, 1e-9)
	testutil.AssertFloatEqual(t, 2000.0, vals[2].Value, 1e-9)
	testutil.AssertFloatEqual(t, 1000.0, vals[3].Value, 1e-9)
	testutil.AssertFloatEqual(t, 0.0, vals[4].Value, 1e-9)
}

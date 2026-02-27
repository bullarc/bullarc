package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// MACD computes the Moving Average Convergence Divergence indicator.
// Value holds the MACD line. Extra contains "signal" and "histogram".
type MACD struct {
	fastPeriod   int
	slowPeriod   int
	signalPeriod int

	// Incremental state for Update.
	fastSeedSum   float64   // running sum for fast EMA seed
	slowSeedSum   float64   // running sum for slow EMA seed
	signalSeedSum float64   // running sum for signal EMA seed
	fastEMA       float64   // current fast EMA value
	slowEMA       float64   // current slow EMA value
	signalEMA     float64   // current signal EMA value
	fastSeeded    bool      // true once fast EMA is seeded
	slowSeeded    bool      // true once slow EMA is seeded
	signalSeeded  bool      // true once signal EMA is seeded
	macdLines     []float64 // accumulated MACD line values for signal seed
	count         int       // total bars received
}

// NewMACD creates a new MACD indicator with the given periods.
func NewMACD(fast, slow, signal int) (*MACD, error) {
	if fast < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("MACD fast period must be >= 1, got %d", fast),
		)
	}
	if slow <= fast {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("MACD slow period must be > fast period (%d), got %d", fast, slow),
		)
	}
	if signal < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("MACD signal period must be >= 1, got %d", signal),
		)
	}
	return &MACD{fastPeriod: fast, slowPeriod: slow, signalPeriod: signal}, nil
}

// Meta returns metadata for the MACD indicator.
func (m *MACD) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:        fmt.Sprintf("MACD_%d_%d_%d", m.fastPeriod, m.slowPeriod, m.signalPeriod),
		Description: "Moving Average Convergence Divergence",
		Category:    "momentum",
		Parameters: map[string]any{
			"fast":   m.fastPeriod,
			"slow":   m.slowPeriod,
			"signal": m.signalPeriod,
		},
		WarmupPeriod: m.slowPeriod + m.signalPeriod - 2,
	}
}

// Compute calculates MACD values for the given bars.
func (m *MACD) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	needed := m.slowPeriod + m.signalPeriod - 1
	if len(bars) < needed {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("MACD(%d,%d,%d) needs %d bars, got %d",
				m.fastPeriod, m.slowPeriod, m.signalPeriod, needed, len(bars)),
		)
	}

	closes := barCloses(bars)

	// Compute fast and slow EMAs (each starting from bar 0).
	emaFast := computeEMAOverFloats(closes, m.fastPeriod)
	emaSlow := computeEMAOverFloats(closes, m.slowPeriod)

	// Align: emaFast[fastOffset+k] and emaSlow[k] both correspond to bar slowPeriod-1+k.
	fastOffset := m.slowPeriod - m.fastPeriod

	// Build MACD line (same length as emaSlow).
	macdLine := make([]float64, len(emaSlow))
	for i := range macdLine {
		macdLine[i] = emaFast[fastOffset+i] - emaSlow[i]
	}

	// Signal = EMA(signalPeriod) of MACD line.
	signalLine := computeEMAOverFloats(macdLine, m.signalPeriod)

	// Output aligns with signalLine: output[j] is at bar slowPeriod-1 + signalPeriod-1 + j.
	startBar := m.slowPeriod + m.signalPeriod - 2
	macdOffset := m.signalPeriod - 1

	n := len(signalLine)
	values := make([]bullarc.IndicatorValue, n)
	for j := range n {
		macdVal := macdLine[macdOffset+j]
		sigVal := signalLine[j]
		values[j] = bullarc.IndicatorValue{
			Time:  bars[startBar+j].Time,
			Value: macdVal,
			Extra: map[string]float64{
				"signal":    sigVal,
				"histogram": macdVal - sigVal,
			},
		}
	}

	return values, nil
}

// Update processes a single new bar incrementally and returns the new MACD value.
// Returns nil during the warmup period.
func (m *MACD) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	m.count++
	c := bar.Close

	// Phase 1: accumulate closes until both fast and slow EMAs are seeded.
	m.fastSeedSum += c
	m.slowSeedSum += c

	fastK := 2.0 / float64(m.fastPeriod+1)
	slowK := 2.0 / float64(m.slowPeriod+1)
	signalK := 2.0 / float64(m.signalPeriod+1)

	// Seed or update fast EMA.
	if !m.fastSeeded {
		if m.count < m.fastPeriod {
			// Still accumulating for fast seed.
		} else if m.count == m.fastPeriod {
			m.fastEMA = m.fastSeedSum / float64(m.fastPeriod)
			m.fastSeeded = true
		}
	} else {
		m.fastEMA = c*fastK + m.fastEMA*(1-fastK)
	}

	// Seed or update slow EMA.
	if !m.slowSeeded {
		if m.count < m.slowPeriod {
			return nil
		}
		// count == slowPeriod: seed slow EMA.
		m.slowEMA = m.slowSeedSum / float64(m.slowPeriod)
		m.slowSeeded = true
		// Fast EMA was seeded earlier; now catch it up to the current bar.
		// The fast EMA was seeded at bar fastPeriod. For bars fastPeriod+1..slowPeriod,
		// we need to have already applied incremental updates. But we did not store
		// intermediate closes. Instead, recompute: seed the fast EMA from its seed sum,
		// but we already updated it above via the fastSeeded branch. So fastEMA is correct.
	} else {
		m.slowEMA = c*slowK + m.slowEMA*(1-slowK)
	}

	macdVal := m.fastEMA - m.slowEMA

	// Phase 2: accumulate MACD line values until signal EMA is seeded.
	if !m.signalSeeded {
		m.macdLines = append(m.macdLines, macdVal)
		m.signalSeedSum += macdVal
		if len(m.macdLines) < m.signalPeriod {
			return nil
		}
		// Seed signal EMA from SMA of the first signalPeriod MACD values.
		m.signalEMA = m.signalSeedSum / float64(m.signalPeriod)
		m.signalSeeded = true
		m.macdLines = nil // free memory
		return &bullarc.IndicatorValue{
			Time:  bar.Time,
			Value: macdVal,
			Extra: map[string]float64{
				"signal":    m.signalEMA,
				"histogram": macdVal - m.signalEMA,
			},
		}
	}

	m.signalEMA = macdVal*signalK + m.signalEMA*(1-signalK)
	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: macdVal,
		Extra: map[string]float64{
			"signal":    m.signalEMA,
			"histogram": macdVal - m.signalEMA,
		},
	}
}

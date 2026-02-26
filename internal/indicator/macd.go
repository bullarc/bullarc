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

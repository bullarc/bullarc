package indicator

// computeEMAOverFloats computes EMA values over a slice of float64 values.
// Returns a slice of length len(values) - period + 1, seeded from the mean of
// the first period values. Returns nil if len(values) < period.
func computeEMAOverFloats(values []float64, period int) []float64 {
	if len(values) < period {
		return nil
	}
	k := 2.0 / float64(period+1)

	var sum float64
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	ema := sum / float64(period)

	n := len(values) - period + 1
	result := make([]float64, n)
	result[0] = ema

	for i := 1; i < n; i++ {
		ema = values[period+i-1]*k + ema*(1-k)
		result[i] = ema
	}
	return result
}

// smaSlice computes the simple moving average of a float64 slice.
// Returns a slice of length len(values) - period + 1.
// Returns nil if len(values) < period.
func smaSlice(values []float64, period int) []float64 {
	if len(values) < period {
		return nil
	}

	var sum float64
	for i := 0; i < period; i++ {
		sum += values[i]
	}

	n := len(values) - period + 1
	result := make([]float64, n)
	result[0] = sum / float64(period)

	for i := 1; i < n; i++ {
		sum += values[period+i-1] - values[i-1]
		result[i] = sum / float64(period)
	}
	return result
}

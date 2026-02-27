package sdk_test

import (
	"context"
	"fmt"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/pkg/sdk"
)

// buildExampleEngine returns an engine pre-loaded with all default indicators
// and the given bars registered as an in-memory data source.
func buildExampleEngine(bars []bullarc.OHLCV) *engine.Engine {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&exampleDataSource{bars: bars})
	return e
}

// exampleDataSource is a minimal DataSource backed by a static slice of bars.
type exampleDataSource struct {
	bars []bullarc.OHLCV
}

func (s *exampleDataSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{Name: "example", Description: "static bars for documentation examples"}
}

func (s *exampleDataSource) Fetch(_ context.Context, _ bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	return s.bars, nil
}

// makeSteadyBars returns n bars with price incrementing from start by step.
func makeSteadyBars(n int, start, step float64) []bullarc.OHLCV {
	bars := make([]bullarc.OHLCV, n)
	for i := range bars {
		p := start + float64(i)*step
		bars[i] = bullarc.OHLCV{
			Time:   time.Now().Add(-time.Duration(n-i) * 24 * time.Hour),
			Open:   p,
			High:   p + 1,
			Low:    p - 1,
			Close:  p,
			Volume: 1_000_000,
		}
	}
	return bars
}

// ExampleNew demonstrates how to create an SDK client from a pre-built engine.
func ExampleNew() {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	client := sdk.New(e)

	_ = client // ready to use
	fmt.Println("client created")
	// Output: client created
}

// ExampleNewWithOptions demonstrates construction with functional options,
// including symbol defaults, interval, and a custom data source.
func ExampleNewWithOptions() {
	bars := makeSteadyBars(100, 150.0, 0.5)
	e := buildExampleEngine(bars)

	client, err := sdk.NewWithOptions(e,
		sdk.WithSymbols("AAPL", "MSFT"),
		sdk.WithInterval("1Day"),
		sdk.WithIndicators("SMA_14", "RSI_14"),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	cfg := client.Config()
	fmt.Println("symbols:", cfg.Symbols)
	fmt.Println("interval:", cfg.Interval)
	// Output:
	// symbols: [AAPL MSFT]
	// interval: 1Day
}

// ExampleClient_Analyze demonstrates a one-shot analysis request.
func ExampleClient_Analyze() {
	bars := makeSteadyBars(100, 150.0, 0.5)
	e := buildExampleEngine(bars)
	client := sdk.New(e)

	ctx := context.Background()
	result, err := client.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("symbol:", result.Symbol)
	fmt.Println("has signals:", len(result.Signals) > 0)
	// Output:
	// symbol: AAPL
	// has signals: true
}

// ExampleClient_Configure demonstrates runtime reconfiguration via Configure.
func ExampleClient_Configure() {
	bars := makeSteadyBars(100, 150.0, 0.5)
	e := buildExampleEngine(bars)
	client := sdk.New(e)

	if err := client.Configure(
		sdk.WithSymbols("TSLA"),
		sdk.WithInterval("1Hour"),
	); err != nil {
		fmt.Println("error:", err)
		return
	}

	cfg := client.Config()
	fmt.Println("symbols:", cfg.Symbols)
	fmt.Println("interval:", cfg.Interval)
	// Output:
	// symbols: [TSLA]
	// interval: 1Hour
}

// ExampleClient_Stream demonstrates polling-based signal delivery via a channel.
func ExampleClient_Stream() {
	bars := makeSteadyBars(100, 150.0, 0.5)
	e := buildExampleEngine(bars)
	client := sdk.New(e)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 500*time.Millisecond)

	var count int
	for sig := range ch {
		count++
		_ = sig
		if count >= 1 {
			cancel() // stop after receiving the first signal
		}
	}

	fmt.Println("received signals:", count >= 1)
	// Output: received signals: true
}

// ExampleClient_StreamSymbols demonstrates streaming signals across multiple symbols.
func ExampleClient_StreamSymbols() {
	bars := makeSteadyBars(100, 150.0, 0.5)
	e := buildExampleEngine(bars)
	client := sdk.New(e)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	seen := make(map[string]bool)
	ch := client.StreamSymbols(ctx, []string{"AAPL", "TSLA"}, 500*time.Millisecond)
	for sig := range ch {
		seen[sig.Symbol] = true
		if len(seen) == 2 {
			cancel()
		}
	}

	fmt.Println("covered AAPL:", seen["AAPL"])
	fmt.Println("covered TSLA:", seen["TSLA"])
	// Output:
	// covered AAPL: true
	// covered TSLA: true
}

// ExampleWithDataSource demonstrates injecting a custom DataSource.
func ExampleWithDataSource() {
	bars := makeSteadyBars(100, 200.0, 1.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	client, err := sdk.NewWithOptions(e,
		sdk.WithDataSource(&exampleDataSource{bars: bars}),
		sdk.WithSymbols("AAPL"),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	ctx := context.Background()
	result, err := client.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("analyzed:", result.Symbol)
	// Output: analyzed: AAPL
}

// ExampleFromFileConfig demonstrates loading a FileConfig and converting it
// into SDK options that can be passed to [NewWithOptions].
func ExampleFromFileConfig() {
	fc := sdk.FileConfig{
		Symbols:  []string{"AAPL", "MSFT"},
		Interval: "1Day",
		// DataSource and LLM are intentionally left empty for this example.
	}

	opts, err := sdk.FromFileConfig(fc)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("got %d option(s)\n", len(opts))
	// Output: got 2 option(s)
}

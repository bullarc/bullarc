package signal_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeSignal(symbol string) bullarc.Signal {
	return bullarc.Signal{
		Type:      bullarc.SignalBuy,
		Symbol:    symbol,
		Indicator: "test",
		Timestamp: time.Now(),
	}
}

// TestBus_PublishDeliveredToSubscriber verifies that a single published signal
// reaches a subscriber without polling.
func TestBus_PublishDeliveredToSubscriber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := signal.NewBus()
	ch := b.Subscribe(ctx, nil)

	sig := makeSignal("AAPL")
	b.Publish([]bullarc.Signal{sig})

	select {
	case got := <-ch:
		assert.Equal(t, "AAPL", got.Symbol)
	case <-time.After(time.Second):
		t.Fatal("signal not received within 1s")
	}
}

// TestBus_MultipleSubscribersEachReceiveIndependently verifies that each
// subscriber gets its own copy of every published signal.
func TestBus_MultipleSubscribersEachReceiveIndependently(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := signal.NewBus()
	ch1 := b.Subscribe(ctx, nil)
	ch2 := b.Subscribe(ctx, nil)
	ch3 := b.Subscribe(ctx, nil)

	sigs := []bullarc.Signal{makeSignal("AAPL"), makeSignal("TSLA")}
	b.Publish(sigs)

	for i, ch := range []<-chan bullarc.Signal{ch1, ch2, ch3} {
		var got []bullarc.Signal
		for len(got) < 2 {
			select {
			case s := <-ch:
				got = append(got, s)
			case <-time.After(time.Second):
				t.Fatalf("subscriber %d: timed out after receiving %d signals", i+1, len(got))
			}
		}
		assert.Equal(t, "AAPL", got[0].Symbol, "subscriber %d: first signal symbol", i+1)
		assert.Equal(t, "TSLA", got[1].Symbol, "subscriber %d: second signal symbol", i+1)
	}
}

// TestBus_DisconnectReclainsResources verifies that cancelling a subscriber's
// context removes it from the bus and closes its channel.
func TestBus_DisconnectReclaimsResources(t *testing.T) {
	b := signal.NewBus()

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	ch1 := b.Subscribe(ctx1, nil)
	_ = b.Subscribe(ctx2, nil)

	assert.Equal(t, 2, b.Len(), "bus must have 2 subscribers before disconnect")

	cancel1()

	// Wait for cleanup goroutine to run.
	require.Eventually(t, func() bool {
		return b.Len() == 1
	}, time.Second, 5*time.Millisecond, "bus should have 1 subscriber after disconnect")

	// ch1 must be closed.
	select {
	case _, open := <-ch1:
		assert.False(t, open, "disconnected subscriber channel must be closed")
	case <-time.After(time.Second):
		t.Fatal("ch1 not closed after context cancellation")
	}
}

// TestBus_SignalsDeliveredInOrder verifies that a burst of signals is received
// in the same order it was published.
func TestBus_SignalsDeliveredInOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := signal.NewBus()
	ch := b.Subscribe(ctx, nil)

	const n = 20
	sigs := make([]bullarc.Signal, n)
	for i := range sigs {
		sigs[i] = bullarc.Signal{
			Type:       bullarc.SignalBuy,
			Symbol:     "AAPL",
			Indicator:  "test",
			Confidence: float64(i),
		}
	}
	b.Publish(sigs)

	for i := 0; i < n; i++ {
		select {
		case got := <-ch:
			assert.InDelta(t, float64(i), got.Confidence, 0.001,
				"signal %d: wrong confidence (order mismatch)", i)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for signal %d", i)
		}
	}
}

// TestBus_FilterBySymbol verifies that a subscriber with a symbol filter
// only receives signals matching that symbol.
func TestBus_FilterBySymbol(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := signal.NewBus()
	appleFilter := func(s bullarc.Signal) bool { return s.Symbol == "AAPL" }
	ch := b.Subscribe(ctx, appleFilter)

	b.Publish([]bullarc.Signal{
		makeSignal("TSLA"),
		makeSignal("AAPL"),
		makeSignal("MSFT"),
	})

	select {
	case got := <-ch:
		assert.Equal(t, "AAPL", got.Symbol, "filter must only deliver AAPL signals")
	case <-time.After(time.Second):
		t.Fatal("AAPL signal not received within 1s")
	}

	// No further signals should arrive (TSLA and MSFT filtered out).
	select {
	case extra := <-ch:
		t.Errorf("unexpected signal received: %+v", extra)
	case <-time.After(50 * time.Millisecond):
		// OK: channel is empty
	}
}

// TestBus_EmptyPublishIsNoop verifies that Publish with an empty slice does
// not crash or deliver anything.
func TestBus_EmptyPublishIsNoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := signal.NewBus()
	ch := b.Subscribe(ctx, nil)

	b.Publish(nil)
	b.Publish([]bullarc.Signal{})

	select {
	case s := <-ch:
		t.Errorf("unexpected signal: %+v", s)
	case <-time.After(50 * time.Millisecond):
		// OK
	}
}

// TestBus_NoSubscribers verifies that Publish does not panic when there are
// no active subscribers.
func TestBus_NoSubscribers(t *testing.T) {
	b := signal.NewBus()
	assert.NotPanics(t, func() {
		b.Publish([]bullarc.Signal{makeSignal("AAPL")})
	})
}

// TestBus_NoDuplicationAcrossBatches verifies that a subscriber receives each
// signal exactly once across multiple Publish calls.
func TestBus_NoDuplicationAcrossBatches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := signal.NewBus()
	ch := b.Subscribe(ctx, nil)

	sig := makeSignal("AAPL")
	b.Publish([]bullarc.Signal{sig})
	b.Publish([]bullarc.Signal{sig})

	got := make([]bullarc.Signal, 0, 3)
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case s := <-ch:
			got = append(got, s)
		case <-deadline:
			goto done
		}
	}
done:
	assert.Len(t, got, 2, "each Publish call must deliver exactly one copy")
}

package signal

import (
	"context"
	"sync"

	"github.com/bullarc/bullarc"
)

// Bus is a publish-subscribe hub for bullarc signals. Publishers call Publish
// to fan out signals to all active subscribers. Each subscriber receives its
// own independent copy of every published signal.
//
// Bus methods are safe for concurrent use.
type Bus struct {
	mu   sync.Mutex
	subs map[uint64]*busSub
	seq  uint64
}

// busSub is a single subscriber slot on the Bus.
type busSub struct {
	mu     sync.Mutex
	ch     chan bullarc.Signal
	closed bool
	filter func(bullarc.Signal) bool
}

// send delivers sig to the subscriber's channel. If the subscriber is already
// closed the call is a no-op. If the buffer is full, the signal is dropped
// rather than blocking the publisher.
func (s *busSub) send(sig bullarc.Signal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- sig:
	default:
		// drop for slow consumer
	}
}

// close marks the subscriber as done and closes the underlying channel so
// that range-based consumers detect disconnection.
func (s *busSub) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// NewBus creates a new, empty Bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[uint64]*busSub)}
}

// Subscribe returns a channel that receives signals matching filter. If filter
// is nil, all published signals are delivered. The channel is closed when ctx
// is cancelled, at which point the subscriber is removed and its resources are
// reclaimed. Slow consumers receive a dropped signal instead of blocking
// publishers.
func (b *Bus) Subscribe(ctx context.Context, filter func(bullarc.Signal) bool) <-chan bullarc.Signal {
	sub := &busSub{
		ch:     make(chan bullarc.Signal, 64),
		filter: filter,
	}
	b.mu.Lock()
	id := b.seq
	b.seq++
	b.subs[id] = sub
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs, id)
		b.mu.Unlock()
		sub.close()
	}()

	return sub.ch
}

// Publish fans out each signal in signals to all matching subscribers in order.
// Signals within a batch are always delivered in the order they appear in the
// slice, satisfying the ordering guarantee for a burst of rapid updates.
func (b *Bus) Publish(signals []bullarc.Signal) {
	if len(signals) == 0 {
		return
	}
	b.mu.Lock()
	subs := make([]*busSub, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()

	for _, sub := range subs {
		for _, sig := range signals {
			if sub.filter != nil && !sub.filter(sig) {
				continue
			}
			sub.send(sig)
		}
	}
}

// Len returns the number of active subscribers. It is primarily useful in tests.
func (b *Bus) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subs)
}

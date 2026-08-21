// Package nvsentineltest provides a fake nvsentinel.Source for component
// tests.
package nvsentineltest

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/nvsentinel"
)

var _ nvsentinel.Source = &FakeSource{}

// FakeSource is an in-memory nvsentinel.Source. Send delivers an event
// exactly like the real source does: the recent-event index is updated
// before subscribers see the event.
type FakeSource struct {
	ch chan nvsentinel.HealthEvent

	mu     sync.Mutex
	events []nvsentinel.HealthEvent
}

func NewFakeSource() *FakeSource {
	return &FakeSource{ch: make(chan nvsentinel.HealthEvent, 16)}
}

func (f *FakeSource) Subscribe() (<-chan nvsentinel.HealthEvent, func()) {
	return f.ch, func() {}
}

func (f *FakeSource) Covers(window time.Duration, match func(nvsentinel.HealthEvent) bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().Add(-window)
	for _, ev := range f.events {
		if ev.GeneratedTimestamp.After(cutoff) && match(ev) {
			return true
		}
	}
	return false
}

func (f *FakeSource) LastReceived() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) == 0 {
		return time.Time{}
	}
	return f.events[len(f.events)-1].GeneratedTimestamp
}

func (f *FakeSource) Close() error { return nil }

// Send records and delivers one event.
func (f *FakeSource) Send(ev nvsentinel.HealthEvent) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
	f.ch <- ev
}

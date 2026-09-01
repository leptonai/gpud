// Package nvsentineltest provides a fake nvsentinel.Source for component
// tests.
package nvsentineltest

import (
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/nvsentinel"
)

var _ nvsentinel.Source = &FakeSource{}

// FakeSource is an in-memory nvsentinel.Source. It mirrors the real
// source's semantics: Send only broadcasts the event to subscribers, and
// the event becomes visible to Covers only after RecordCoverage is called
// (which components do once they have persisted the data point).
type FakeSource struct {
	ch chan nvsentinel.HealthEvent

	mu           sync.Mutex
	events       []nvsentinel.HealthEvent
	lastReceived time.Time
}

func NewFakeSource() *FakeSource {
	return &FakeSource{ch: make(chan nvsentinel.HealthEvent, 16)}
}

func (f *FakeSource) Subscribe() (<-chan nvsentinel.HealthEvent, func()) {
	return f.ch, func() {}
}

// RecordCoverage makes the event visible to Covers, mirroring the real
// source: healthy events are ignored because they carry no new data point.
func (f *FakeSource) RecordCoverage(ev nvsentinel.HealthEvent) {
	if ev.IsHealthy {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
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
	return f.lastReceived
}

func (f *FakeSource) Close() error { return nil }

// Send broadcasts one event to subscribers, exactly like the real source
// does. It does NOT make the event visible to Covers — tests that need
// coverage either run a component that persists the event (the component
// then calls RecordCoverage) or call RecordCoverage directly.
func (f *FakeSource) Send(ev nvsentinel.HealthEvent) {
	f.mu.Lock()
	f.lastReceived = time.Now()
	f.mu.Unlock()
	f.ch <- ev
}

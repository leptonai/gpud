package nvsentineltest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvsentinel"
)

func TestFakeSourceSendAndSubscribe(t *testing.T) {
	src := NewFakeSource()
	require.NoError(t, src.Close()) // Close is a no-op for the fake source

	ch, unsub := src.Subscribe()
	t.Cleanup(unsub)

	ev := nvsentinel.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ComponentClass:     "GPU",
		ErrorCodes:         []string{"79"},
		GeneratedTimestamp: time.Now().UTC(),
	}
	src.Send(ev)

	select {
	case got := <-ch:
		assert.Equal(t, "SysLogsXIDError", got.CheckName)
		assert.Equal(t, "GPU", got.ComponentClass)
		assert.Equal(t, []string{"79"}, got.ErrorCodes)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event from FakeSource")
	}
}

func TestFakeSourceCovers(t *testing.T) {
	src := NewFakeSource()
	t.Cleanup(func() { _ = src.Close() })

	// No events yet: nothing covers.
	assert.False(t, src.Covers(time.Hour, func(nvsentinel.HealthEvent) bool { return true }))

	now := time.Now().UTC()

	// Send only broadcasts: like the real source, the event is not covered
	// until a persisting component reports coverage.
	src.Send(nvsentinel.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ErrorCodes:         []string{"79"},
		GeneratedTimestamp: now,
	})
	matchXid79 := func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError" && len(ev.ErrorCodes) > 0 && ev.ErrorCodes[0] == "79"
	}
	assert.False(t, src.Covers(time.Hour, matchXid79))

	// Once coverage is reported, a matching predicate within the window
	// covers.
	src.RecordCoverage(nvsentinel.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ErrorCodes:         []string{"79"},
		GeneratedTimestamp: now,
	})
	assert.True(t, src.Covers(time.Hour, matchXid79))

	// Non-matching predicate does not cover.
	assert.False(t, src.Covers(time.Hour, func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsSXIDError"
	}))

	// A healthy event is never covered.
	src.RecordCoverage(nvsentinel.HealthEvent{
		CheckName:          "SysLogsSXIDError",
		IsHealthy:          true,
		GeneratedTimestamp: now,
	})
	assert.False(t, src.Covers(time.Hour, func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsSXIDError"
	}))

	// An event older than the window does not cover.
	src2 := NewFakeSource()
	t.Cleanup(func() { _ = src2.Close() })
	src2.RecordCoverage(nvsentinel.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ErrorCodes:         []string{"79"},
		GeneratedTimestamp: time.Now().UTC().Add(-2 * time.Hour),
	})
	assert.False(t, src2.Covers(time.Minute, matchXid79))
}

func TestFakeSourceLastReceived(t *testing.T) {
	src := NewFakeSource()
	t.Cleanup(func() { _ = src.Close() })

	// No events: zero time.
	assert.True(t, src.LastReceived().IsZero())

	// LastReceived tracks the receive time (like the real source), not the
	// event's generated timestamp.
	src.Send(nvsentinel.HealthEvent{GeneratedTimestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	first := src.LastReceived()
	assert.False(t, first.IsZero())

	// A later send updates LastReceived.
	src.Send(nvsentinel.HealthEvent{GeneratedTimestamp: time.Now().UTC()})
	assert.True(t, !src.LastReceived().Before(first))
}

func TestFakeSourceCloseIsNoOp(t *testing.T) {
	src := NewFakeSource()
	// Close returns nil and can be called multiple times.
	require.NoError(t, src.Close())
	require.NoError(t, src.Close())
}

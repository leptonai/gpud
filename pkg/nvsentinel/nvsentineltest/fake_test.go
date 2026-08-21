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
	src.Send(nvsentinel.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ErrorCodes:         []string{"79"},
		GeneratedTimestamp: now,
	})

	// Matching predicate within a wide window covers.
	matchXid79 := func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError" && len(ev.ErrorCodes) > 0 && ev.ErrorCodes[0] == "79"
	}
	assert.True(t, src.Covers(time.Hour, matchXid79))

	// Non-matching predicate does not cover.
	assert.False(t, src.Covers(time.Hour, func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsSXIDError"
	}))

	// An event older than the window does not cover.
	src2 := NewFakeSource()
	t.Cleanup(func() { _ = src2.Close() })
	src2.Send(nvsentinel.HealthEvent{
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

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	src.Send(nvsentinel.HealthEvent{GeneratedTimestamp: ts})
	assert.Equal(t, ts, src.LastReceived())

	// A later event updates LastReceived.
	ts2 := ts.Add(time.Hour)
	src.Send(nvsentinel.HealthEvent{GeneratedTimestamp: ts2})
	assert.Equal(t, ts2, src.LastReceived())
}

func TestFakeSourceCloseIsNoOp(t *testing.T) {
	src := NewFakeSource()
	// Close returns nil and can be called multiple times.
	require.NoError(t, src.Close())
	require.NoError(t, src.Close())
}

package sxid

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvsentinel"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func newNVSentinelSXidEvent(sxid string) nvsentinel.HealthEvent {
	return nvsentinel.HealthEvent{
		Agent:          "syslog-health-monitor",
		ComponentClass: "GPU", // NVSentinel labels its SXid syslog check "GPU"
		CheckName:      "SysLogsSXIDError",
		IsFatal:        true,
		Message:        "SXid " + sxid + " detected",
		Action:         nvsentinel.RecommendedActionRestartBM,
		ErrorCodes:     []string{sxid},
		Entities: []nvsentinel.Entity{
			{Type: "NVSWITCH", Value: "3"},
			{Type: "PCI", Value: "0000:05:00.0"},
			{Type: "NVLINK", Value: "32"},
		},
		NodeName:           "node-1",
		ID:                 "nvs-event-1",
		GeneratedTimestamp: time.Now().UTC(),
	}
}

func newTestComponentWithNVSentinel(t *testing.T, src nvsentinel.Source) *component {
	t.Helper()

	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	t.Cleanup(cleanup)

	store, err := eventstore.New(dbRW, dbRO, GetLookbackPeriod())
	require.NoError(t, err)

	gpudInstance := &components.GPUdInstance{
		RootCtx:                    ctx,
		EventStore:                 store,
		RebootEventStore:           pkghost.NewRebootEventStore(store),
		NVSentinel:                 src,
		NVSentinelEventDedupWindow: 2 * time.Minute,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	t.Cleanup(func() { _ = comp.Close() })

	c, ok := comp.(*component)
	require.True(t, ok)
	return c
}

func TestMatchNVSentinelSXid(t *testing.T) {
	c := &component{}

	// The SXid syslog check matches by check name.
	num, _, ok := c.matchNVSentinelSXid(newNVSentinelSXidEvent("12028"))
	assert.True(t, ok)
	assert.Equal(t, 12028, num)

	// The NVSWITCH component class also identifies SXid data points.
	ev := newNVSentinelSXidEvent("12028")
	ev.CheckName = "CustomSwitchCheck"
	ev.ComponentClass = "NVSWITCH"
	_, _, ok = c.matchNVSentinelSXid(ev)
	assert.True(t, ok)

	// The plain Xid check belongs to the xid component.
	ev = newNVSentinelSXidEvent("79")
	ev.CheckName = "SysLogsXIDError"
	ev.ComponentClass = "GPU"
	_, _, ok = c.matchNVSentinelSXid(ev)
	assert.False(t, ok)

	// A non-numeric error code carries no SXid data point.
	ev = newNVSentinelSXidEvent("not-a-number")
	_, _, ok = c.matchNVSentinelSXid(ev)
	assert.False(t, ok)
}

func TestNVSentinelSXidEventInserted(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	src.Send(newNVSentinelSXidEvent("12028"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)

	stored := events[0]
	assert.Equal(t, EventNameErrorSXid, stored.Name)
	assert.Equal(t, string(apiv1.EventTypeFatal), stored.Type)
	assert.Equal(t, "nvs-event-1", stored.ExtraInfo[EventKeyNVSentinelEventID])

	var detail sxidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(stored.ExtraInfo[EventKeyErrorSXidData]), &detail))
	assert.Equal(t, dataSourceNVSentinel, detail.DataSource)
	assert.Equal(t, uint64(12028), detail.SXid)
	// The NVSentinel recommended action wins over the catalog action.
	require.NotNil(t, detail.SuggestedActionsByGPUd)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, detail.SuggestedActionsByGPUd.RepairActions)
}

func TestNVSentinelSXidKmsgSuppressedWhenCovered(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// NVSentinel reports the incident first; the kmsg twin is suppressed.
	src.Send(newNVSentinelSXidEvent("12028"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
	}

	// The kmsg copy must not appear; only the NVSentinel copy stays stored.
	time.Sleep(time.Second)
	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestNVSentinelSXidKmsgKeptWithoutNVSentinel(t *testing.T) {
	// No NVSentinel source: the component behaves exactly as before.
	c := newTestComponentWithNVSentinel(t, nil)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
	}

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)
}
func TestHasStoredSXidTwinJSONFormat(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	payload := sxidErrorEventDetail{
		Time:       metav1.NewTime(now),
		DataSource: "gpud",
		DeviceUUID: "GPU-abc",
		SXid:       12028,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: EventNameErrorSXid,
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData: string(raw),
			EventKeyDeviceUUID:    "GPU-abc",
		},
	}))

	// Same SXid and device: twin found.
	assert.True(t, c.hasStoredSXidTwin(12028, "GPU-abc", now))

	// Different SXid: no twin.
	assert.False(t, c.hasStoredSXidTwin(12029, "GPU-abc", now))

	// Different device UUID: no twin.
	assert.False(t, c.hasStoredSXidTwin(12028, "GPU-other", now))

	// Empty device UUID on the query: still a twin (UUID mismatch is skipped when either side is empty).
	assert.True(t, c.hasStoredSXidTwin(12028, "", now))
}

func TestHasStoredSXidTwinLegacyFormat(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	// Legacy format stores the SXid number as a plain string in EventKeyErrorSXidData.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: EventNameErrorSXid,
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData: "12028",
		},
	}))

	assert.True(t, c.hasStoredSXidTwin(12028, "", now))
	assert.False(t, c.hasStoredSXidTwin(12029, "", now))
}

func TestHasStoredSXidTwinDifferentEventName(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	// An event with a different name does not count as a twin.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: "some-other-event",
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData: "12028",
		},
	}))

	assert.False(t, c.hasStoredSXidTwin(12028, "", now))
}

func TestHasStoredSXidTwinOutsideWindow(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// An event far in the past is outside the dedup window.
	old := time.Now().UTC().Add(-3 * time.Hour)
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: old,
		Name: EventNameErrorSXid,
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData: "12028",
		},
	}))

	assert.False(t, c.hasStoredSXidTwin(12028, "", time.Now().UTC()))
}

func TestHasStoredSXidTwinCorruptPayload(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	// A corrupt JSON payload that is also not a plain number is skipped.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: EventNameErrorSXid,
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData: "not-json-not-number",
		},
	}))

	assert.False(t, c.hasStoredSXidTwin(12028, "", now))
}

func TestNVSentinelSXidNonFatalEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	ev := newNVSentinelSXidEvent("12028")
	ev.IsFatal = false
	src.Send(ev)

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)
	// Non-fatal events are stored as Critical, not Fatal.
	assert.Equal(t, string(apiv1.EventTypeCritical), events[0].Type)
}
func TestNVSentinelSXidNonMatchingEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// An Xid event (not SXid) is not matched by the sxid component.
	ev := newNVSentinelSXidEvent("79")
	ev.CheckName = "SysLogsXIDError"
	ev.ComponentClass = "GPU"
	src.Send(ev)

	time.Sleep(500 * time.Millisecond)
	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestNVSentinelSXidEventBucketNil(t *testing.T) {
	// When eventBucket is nil, onNVSentinelEvent returns early without panic.
	c := &component{ctx: context.Background()}
	c.onNVSentinelEvent(newNVSentinelSXidEvent("12028"))
	// No panic, no crash.
}

func TestNVSentinelSXidCoversNilSource(t *testing.T) {
	// nvsentinelCoversSXid returns false when source is nil.
	c := &component{}
	assert.False(t, c.nvsentinelCoversSXid(nil))
}

func TestNVSentinelSXidCoversNilError(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)
	// nvsentinelCoversSXid returns false when sxidErr is nil.
	assert.False(t, c.nvsentinelCoversSXid(nil))
}
func TestNVSentinelSXidCoversMatchingEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// Send an SXid event to the source.
	src.Send(newNVSentinelSXidEvent("12028"))

	// The Covers check should find the matching event.
	sxidErr := &Error{SXid: 12028}
	assert.True(t, c.nvsentinelCoversSXid(sxidErr))

	// A different SXid number does not cover.
	sxidErr2 := &Error{SXid: 12029}
	assert.False(t, c.nvsentinelCoversSXid(sxidErr2))
}

func TestNVSentinelSXidEventSkippedWhenGPUdTwinStored(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// GPUd's kmsg detection stores its copy first.
	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
	}

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	// The NVSentinel copy of the same incident is skipped.
	src.Send(newNVSentinelSXidEvent("12028"))
	time.Sleep(time.Second)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestNVSentinelSXidWatchNilSource(t *testing.T) {
	// watchNVSentinel with nil source returns immediately without panic.
	c := &component{ctx: context.Background()}
	c.watchNVSentinel()
}

func TestNVSentinelSXidDefaultDedupWindowFallback(t *testing.T) {
	// When NVSentinelEventDedupWindow is 0, the component falls back to
	// nvsentinel.DefaultEventDedupWindow.
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	t.Cleanup(cleanup)

	store, err := eventstore.New(dbRW, dbRO, GetLookbackPeriod())
	require.NoError(t, err)

	src := nvsentineltest.NewFakeSource()
	gpudInstance := &components.GPUdInstance{
		RootCtx:                    ctx,
		EventStore:                 store,
		RebootEventStore:           pkghost.NewRebootEventStore(store),
		NVSentinel:                 src,
		NVSentinelEventDedupWindow: 0, // zero triggers the default fallback
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	t.Cleanup(func() { _ = comp.Close() })

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.Equal(t, nvsentinel.DefaultEventDedupWindow, c.nvsDedupWindow)
}
func TestNVSentinelSXidHealthyEventSkipped(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	ev := newNVSentinelSXidEvent("12028")
	ev.IsFatal = false
	ev.IsHealthy = true
	src.Send(ev)

	// Healthy events are logged and skipped, never stored.
	time.Sleep(500 * time.Millisecond)
	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestNVSentinelSXidCoversWithNonMatchingEventInSource(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// An event that does not match the SXid predicate at all (wrong check name).
	ev := newNVSentinelSXidEvent("12028")
	ev.CheckName = "SysLogsXIDError"
	ev.ComponentClass = "GPU"
	src.Send(ev)

	assert.False(t, c.nvsentinelCoversSXid(&Error{SXid: 12028}))
}

func TestHasStoredSXidTwinSkipsFutureEvents(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	// An event timestamped in the future (beyond the dedup window) is not a twin.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now.Add(3 * time.Hour),
		Name: EventNameErrorSXid,
		ExtraInfo: map[string]string{
			EventKeyErrorSXidData: "12028",
		},
	}))

	assert.False(t, c.hasStoredSXidTwin(12028, "", now))
}

func TestNewNVSentinelWithoutEventStore(t *testing.T) {
	// NVSentinel set but EventStore nil: eventBucket is nil so the NVSentinel
	// wiring is skipped entirely.
	src := nvsentineltest.NewFakeSource()
	gpudInstance := &components.GPUdInstance{
		RootCtx:    context.Background(),
		NVSentinel: src,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	t.Cleanup(func() { _ = comp.Close() })

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.Nil(t, c.nvsSource)
}

// TestNVSentinelSXidEventsProcessedWithoutKmsgWatcher is a regression test for
// the Start gating: NVSentinel events must be stored even when the kmsg
// watcher is absent (non-root or /dev/kmsg unavailable). NVSentinel's syslog
// monitor may be the only kmsg reader on the node, so the event loop must run
// with just the NVSentinel source. Previously Start only ran the loop when
// kmsgWatcher was non-nil, silently dropping NVSentinel events into
// extraEventCh.
func TestNVSentinelSXidEventsProcessedWithoutKmsgWatcher(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// No kmsgWatcher (zero value): this is the non-root / no-kmsg case.
	require.Nil(t, c.kmsgWatcher)
	require.NoError(t, c.Start())

	src.Send(newNVSentinelSXidEvent("12028"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)
}

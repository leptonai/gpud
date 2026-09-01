package xid

import (
	"context"
	"encoding/json"
	"errors"
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

func newNVSentinelXidEvent(xid string, deviceUUID string) nvsentinel.HealthEvent {
	ev := nvsentinel.HealthEvent{
		Agent:              "syslog-health-monitor",
		ComponentClass:     "GPU",
		CheckName:          "SysLogsXIDError",
		IsFatal:            true,
		Message:            "Xid " + xid + " detected",
		Action:             nvsentinel.RecommendedActionRestartBM,
		ErrorCodes:         []string{xid},
		NodeName:           "node-1",
		ID:                 "nvs-event-1",
		GeneratedTimestamp: time.Now().UTC(),
	}
	if deviceUUID != "" {
		ev.Entities = []nvsentinel.Entity{{Type: "GPU", Value: deviceUUID}}
	}
	return ev
}

func newTestComponentWithNVSentinel(t *testing.T, src nvsentinel.Source) (*component, eventstore.Store) {
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

	return mustComponent(t, comp), store
}

func TestMatchNVSentinelXid(t *testing.T) {
	c := &component{}

	// GPU class with a catalog-known code matches.
	xidNum, uuid, ok := c.matchNVSentinelXid(newNVSentinelXidEvent("79", "GPU-abc"))
	assert.True(t, ok)
	assert.Equal(t, 79, xidNum)
	assert.Equal(t, "GPU-abc", uuid)

	// A code the GPUd catalog does not know still matches; the catalog only
	// enriches, and NVSentinel's detection must not be dropped.
	xidNum, _, ok = c.matchNVSentinelXid(newNVSentinelXidEvent("9999", "GPU-abc"))
	assert.True(t, ok)
	assert.Equal(t, 9999, xidNum)

	// A non-numeric error code carries no Xid data point.
	_, _, ok = c.matchNVSentinelXid(newNVSentinelXidEvent("DCGM_FR_UNKNOWN", "GPU-abc"))
	assert.False(t, ok)

	// The SXid check belongs to the sxid component even though NVSentinel
	// labels it with the GPU component class.
	ev2 := newNVSentinelXidEvent("12028", "")
	ev2.CheckName = "SysLogsSXIDError"
	_, _, ok = c.matchNVSentinelXid(ev2)
	assert.False(t, ok)

	// Non-GPU component classes belong to other components.
	ev := newNVSentinelXidEvent("79", "")
	ev.ComponentClass = "NIC"
	_, _, ok = c.matchNVSentinelXid(ev)
	assert.False(t, ok)
}

func TestNVSentinelEventInsertedWithNVSeverityAndAction(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	src.Send(newNVSentinelXidEvent("79", "GPU-abc"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		if err != nil || len(events) != 1 {
			return false
		}
		return true
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)

	stored := events[0]
	assert.Equal(t, EventNameErrorXid, stored.Name)
	// The NVSentinel fatal verdict maps to a fatal GPUd event.
	assert.Equal(t, string(apiv1.EventTypeFatal), stored.Type)
	assert.Equal(t, "nvs-event-1", stored.ExtraInfo[EventKeyNVSentinelEventID])
	assert.Equal(t, "SysLogsXIDError", stored.ExtraInfo[EventKeyNVSentinelCheckName])

	var detail xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(stored.ExtraInfo[EventKeyErrorXidData]), &detail))
	assert.Equal(t, dataSourceNVSentinel, detail.DataSource)
	assert.Equal(t, uint64(79), detail.Xid)
	assert.Equal(t, "GPU-abc", detail.DeviceUUID)
	// The NVSentinel recommended action wins over the catalog action.
	require.NotNil(t, detail.SuggestedActionsByGPUd)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, detail.SuggestedActionsByGPUd.RepairActions)

	// The health evolution treats the NVSentinel fatal event like a native one.
	require.Eventually(t, func() bool {
		states := c.LastHealthStates()
		return len(states) == 1 && states[0].Health == apiv1.HealthStateTypeUnhealthy
	}, 10*time.Second, 50*time.Millisecond)
}

func TestNVSentinelHealthyEventNotStored(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	ev := newNVSentinelXidEvent("79", "GPU-abc")
	ev.IsFatal = false
	ev.IsHealthy = true
	ev.Action = nvsentinel.RecommendedActionNone
	src.Send(ev)

	// Give the forwarder a chance to (not) store the event.
	time.Sleep(500 * time.Millisecond)

	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestKmsgInsertSuppressedWhenNVSentinelCovers(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// NVSentinel reports the incident first; the kmsg twin is suppressed.
	src.Send(newNVSentinelXidEvent("79", ""))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
	}

	// The kmsg copy must not appear; only the NVSentinel copy stays stored.
	time.Sleep(time.Second)
	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)

	var detail xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(events[0].ExtraInfo[EventKeyErrorXidData]), &detail))
	assert.Equal(t, dataSourceNVSentinel, detail.DataSource)
}

func TestKmsgInsertKeptWhenDevicesDiffer(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// NVSentinel reported Xid 79 on a different GPU: the kmsg event for
	// PCI:0000:05:00 is a separate incident and must be stored.
	src.Send(newNVSentinelXidEvent("79", "GPU-other"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
	}

	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 2
	}, 10*time.Second, 50*time.Millisecond)
}

func TestKmsgInsertKeptWithoutNVSentinel(t *testing.T) {
	// No NVSentinel source: the component behaves exactly as before.
	c, _ := newTestComponentWithNVSentinel(t, nil)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
	}

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)
}

func TestNVSentinelEventSkippedWhenGPUdTwinStored(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// GPUd's kmsg detection wins the race and stores its copy first.
	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
	}

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	// The NVSentinel copy of the same incident is skipped: one event total.
	// The NVSentinel event names no GPU entity, so the stored twin matches.
	src.Send(newNVSentinelXidEvent("79", ""))
	time.Sleep(time.Second)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
func TestHasStoredXidTwinJSONFormat(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	payload := xidErrorEventDetail{
		Time:       metav1.NewTime(now),
		DataSource: "gpud",
		DeviceUUID: "GPU-abc",
		Xid:        79,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: EventNameErrorXid,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: string(raw),
			EventKeyDeviceUUID:   "GPU-abc",
		},
	}))

	// Same Xid and device: twin found.
	assert.True(t, c.hasStoredXidTwin(79, "GPU-abc", now))

	// Different Xid: no twin.
	assert.False(t, c.hasStoredXidTwin(94, "GPU-abc", now))

	// Different device UUID: no twin.
	assert.False(t, c.hasStoredXidTwin(79, "GPU-other", now))

	// Empty device UUID on the query: still a twin.
	assert.True(t, c.hasStoredXidTwin(79, "", now))
}

func TestHasStoredXidTwinLegacyFormat(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	// Legacy format stores the Xid number as a plain string.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: EventNameErrorXid,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "79",
			EventKeyDeviceUUID:   "GPU-abc",
		},
	}))

	assert.True(t, c.hasStoredXidTwin(79, "GPU-abc", now))
	assert.False(t, c.hasStoredXidTwin(94, "GPU-abc", now))
	// Legacy event with empty stored UUID: twin when query UUID is also empty.
	assert.True(t, c.hasStoredXidTwin(79, "", now))
}

func TestHasStoredXidTwinDifferentEventName(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: "some-other-event",
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "79",
		},
	}))

	assert.False(t, c.hasStoredXidTwin(79, "", now))
}

func TestHasStoredXidTwinOutsideWindow(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	old := time.Now().UTC().Add(-3 * time.Hour)
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: old,
		Name: EventNameErrorXid,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "79",
		},
	}))

	assert.False(t, c.hasStoredXidTwin(79, "", time.Now().UTC()))
}

func TestHasStoredXidTwinCorruptPayload(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: EventNameErrorXid,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "not-json-not-number",
		},
	}))

	assert.False(t, c.hasStoredXidTwin(79, "", now))
}

func TestNVSentinelXidNonFatalEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	ev := newNVSentinelXidEvent("79", "GPU-abc")
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

func TestNVSentinelXidUnknownCodeEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// An Xid code not in the GPUd catalog is still stored.
	src.Send(newNVSentinelXidEvent("9999", "GPU-xyz"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)

	var detail xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(events[0].ExtraInfo[EventKeyErrorXidData]), &detail))
	assert.Equal(t, dataSourceNVSentinel, detail.DataSource)
	assert.Equal(t, uint64(9999), detail.Xid)
	assert.Equal(t, "GPU-xyz", detail.DeviceUUID)
}
func TestNVSentinelXidNonMatchingEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// An SXid event is not matched by the xid component.
	ev := newNVSentinelXidEvent("12028", "")
	ev.CheckName = "SysLogsSXIDError"
	src.Send(ev)

	time.Sleep(500 * time.Millisecond)
	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestNVSentinelXidEventBucketNil(t *testing.T) {
	// When eventBucket is nil, onNVSentinelEvent returns early without panic.
	c := &component{ctx: context.Background()}
	c.onNVSentinelEvent(newNVSentinelXidEvent("79", "GPU-abc"))
	// No panic, no crash.
}

func TestNVSentinelXidCoversNilSource(t *testing.T) {
	c := &component{}
	assert.False(t, c.nvsentinelCoversXid(nil))
}

func TestNVSentinelXidCoversNilError(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)
	assert.False(t, c.nvsentinelCoversXid(nil))
}
func TestNVSentinelXidCoversMatchingEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	// Report coverage for an Xid event the way a persisting component does.
	src.RecordCoverage(newNVSentinelXidEvent("79", "GPU-abc"))

	// The Covers check should find the matching event.
	xidErr := &Error{Xid: 79, DeviceUUID: "GPU-abc"}
	assert.True(t, c.nvsentinelCoversXid(xidErr))

	// A different Xid number does not cover.
	xidErr2 := &Error{Xid: 94, DeviceUUID: "GPU-abc"}
	assert.False(t, c.nvsentinelCoversXid(xidErr2))
}

func TestNVSentinelXidEventSkippedWhenGPUdTwinStoredWithUUID(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	// GPUd's kmsg detection stores its copy first with a specific device.
	kmsgCh <- kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "NVRM: Xid (PCI:0000:05:00): 79, pid='<unknown>', name=<unknown>, GPU has fallen off the bus.",
	}

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	// The NVSentinel copy with the same device is skipped.
	src.Send(newNVSentinelXidEvent("79", "PCI:0000:05:00"))
	time.Sleep(time.Second)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestNVSentinelXidWatchNilSource(t *testing.T) {
	// watchNVSentinel with nil source returns immediately without panic.
	c := &component{ctx: context.Background()}
	c.watchNVSentinel()
}

func TestNVSentinelXidDefaultDedupWindowFallback(t *testing.T) {
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
		NVSentinelEventDedupWindow: 0,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	t.Cleanup(func() { _ = comp.Close() })

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.Equal(t, nvsentinel.DefaultEventDedupWindow, c.nvsDedupWindow)
}
func TestMatchNVSentinelXidRowRemappingDiscard(t *testing.T) {
	// Xid 63/64 are discarded in favor of the remapped-rows component when
	// the platform supports row remapping.
	c := &component{nvmlInstance: createMockNVMLInstanceWithRowRemapping()}

	for _, xid := range []string{"63", "64"} {
		_, _, ok := c.matchNVSentinelXid(newNVSentinelXidEvent(xid, "GPU-abc"))
		assert.False(t, ok, "xid %s should be discarded with row remapping", xid)
	}

	// Other xids still match with row remapping supported.
	_, _, ok := c.matchNVSentinelXid(newNVSentinelXidEvent("79", "GPU-abc"))
	assert.True(t, ok)
}

func TestNVSentinelXidCoversDifferentDevice(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	// Source has persisted Xid 79 on GPU-abc.
	src.RecordCoverage(newNVSentinelXidEvent("79", "GPU-abc"))

	// Same xid, same device: covered.
	assert.True(t, c.nvsentinelCoversXid(&Error{Xid: 79, DeviceUUID: "GPU-abc"}))
	// Same xid, different device: NOT covered — distinct incidents on different GPUs.
	assert.False(t, c.nvsentinelCoversXid(&Error{Xid: 79, DeviceUUID: "GPU-other"}))
}

func TestHasStoredXidTwinSkipsFutureEvents(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now.Add(3 * time.Hour),
		Name: EventNameErrorXid,
		ExtraInfo: map[string]string{
			EventKeyErrorXidData: "79",
		},
	}))

	assert.False(t, c.hasStoredXidTwin(79, "", now))
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

func TestMatchNVSentinelXidWithGPUUUIDEntity(t *testing.T) {
	c := &component{}

	// NVSentinel reports the GPU UUID under the GPU_UUID entity type.
	ev := newNVSentinelXidEvent("79", "")
	ev.Entities = []nvsentinel.Entity{{Type: "GPU_UUID", Value: "GPU-uuid-from-nvsentinel"}}
	xidNum, uuid, ok := c.matchNVSentinelXid(ev)
	require.True(t, ok)
	assert.Equal(t, 79, xidNum)
	assert.Equal(t, "GPU-uuid-from-nvsentinel", uuid)
}

func TestMatchNVSentinelXidWithGPUEntityFallback(t *testing.T) {
	c := &component{}

	// When GPU_UUID entity is absent, fall back to GPU entity type.
	ev := newNVSentinelXidEvent("79", "")
	ev.Entities = []nvsentinel.Entity{{Type: "GPU", Value: "GPU-fallback-uuid"}}
	xidNum, uuid, ok := c.matchNVSentinelXid(ev)
	require.True(t, ok)
	assert.Equal(t, 79, xidNum)
	assert.Equal(t, "GPU-fallback-uuid", uuid)
}

// TestNVSentinelEventsProcessedWithoutKmsgWatcher is a regression test for the
// Start gating: NVSentinel events must be stored even when the kmsg watcher is
// absent (non-root or /dev/kmsg unavailable). NVSentinel's syslog monitor may
// be the only kmsg reader on the node, so the event loop must run with just
// the NVSentinel source. Previously Start only ran the loop when kmsgWatcher
// was non-nil, silently dropping NVSentinel events into extraEventCh.
func TestNVSentinelEventsProcessedWithoutKmsgWatcher(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	// No kmsgWatcher (zero value): this is the non-root / no-kmsg case.
	require.Nil(t, c.kmsgWatcher)
	require.NoError(t, c.Start())

	src.Send(newNVSentinelXidEvent("79", "GPU-abc"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)
}

func TestNVSentinelXidHealthyEventDoesNotCover(t *testing.T) {
	// Regression: a healthy event stores no data point, so it must never
	// enter the coverage index. Otherwise a healthy event carrying Xid N
	// could suppress a real native Xid N within the dedup window.
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	ev := newNVSentinelXidEvent("79", "GPU-abc")
	ev.IsFatal = false
	ev.IsHealthy = true
	ev.Action = nvsentinel.RecommendedActionNone
	src.Send(ev)

	// Give the forwarder a chance to process (and skip) the event.
	time.Sleep(500 * time.Millisecond)

	assert.False(t, c.nvsentinelCoversXid(&Error{Xid: 79, DeviceUUID: "GPU-abc"}))
	assert.False(t, src.Covers(time.Hour, func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError"
	}))
}

func TestNVSentinelXidInsertFailureDoesNotCover(t *testing.T) {
	// Regression: when the event-store insert fails, the data point is not
	// durable, so the event must not cover. Otherwise a later native Xid
	// would be suppressed and the incident permanently lost.
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	// Force every insert to fail: no data point becomes durable.
	c.eventBucket = &stubEventBucket{insertErr: errors.New("insert failed")}

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	src.Send(newNVSentinelXidEvent("79", "GPU-abc"))

	// Give the pipeline a chance to attempt (and fail) the insert.
	time.Sleep(500 * time.Millisecond)

	assert.False(t, c.nvsentinelCoversXid(&Error{Xid: 79, DeviceUUID: "GPU-abc"}))
	assert.False(t, src.Covers(time.Hour, func(ev nvsentinel.HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError"
	}))
}

func TestNVSentinelXidEventCoveredAfterPersist(t *testing.T) {
	// The happy path: once the NVSentinel copy is durably stored, the
	// native kmsg twin is covered and suppressed.
	src := nvsentineltest.NewFakeSource()
	c, _ := newTestComponentWithNVSentinel(t, src)

	kmsgCh := make(chan kmsg.Message, 1)
	go c.start(kmsgCh, time.Hour)

	src.Send(newNVSentinelXidEvent("79", "GPU-abc"))

	// Coverage lags persistence slightly: the start loop reports it after
	// the bucket insert succeeds.
	require.Eventually(t, func() bool {
		return c.nvsentinelCoversXid(&Error{Xid: 79, DeviceUUID: "GPU-abc"})
	}, 10*time.Second, 50*time.Millisecond)
}

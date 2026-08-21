package xid

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

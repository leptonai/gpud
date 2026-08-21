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

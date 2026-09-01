package infiniband

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvsentinel"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func newNVSentinelNICEvent(pattern string, device string) nvsentinel.HealthEvent {
	ev := nvsentinel.HealthEvent{
		Agent:          "syslog-health-monitor",
		ComponentClass: "NIC",
		CheckName:      "SysLogsNICDriverError",
		IsFatal:        true,
		Message:        "Detected insufficient power on the PCIe slot (27W)",
		Action:         nvsentinel.RecommendedActionContactSupport,
		ErrorCodes:     []string{pattern},
		NodeName:       "node-1",
		ID:             "nvs-event-1",

		GeneratedTimestamp: time.Now().UTC(),
	}
	if device != "" {
		ev.Entities = []nvsentinel.Entity{{Type: "NIC", Value: device}}
	}
	return ev
}

func newTestComponentWithNVSentinel(t *testing.T, src nvsentinel.Source) *component {
	t.Helper()

	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	t.Cleanup(cleanup)

	store, err := eventstore.New(dbRW, dbRO, time.Hour)
	require.NoError(t, err)

	gpudInstance := &components.GPUdInstance{
		RootCtx:                    ctx,
		EventStore:                 store,
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

func TestMatchNVSentinelNIC(t *testing.T) {
	name, ok := matchNVSentinelNIC(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))
	assert.True(t, ok)
	assert.Equal(t, eventPCIPowerInsufficient, name)

	name, ok = matchNVSentinelNIC(newNVSentinelNICEvent("port_module_high_temp", "mlx5_0"))
	assert.True(t, ok)
	assert.Equal(t, eventPortModuleHighTemperature, name)

	// NVSentinel patterns without a GPUd counterpart are skipped.
	_, ok = matchNVSentinelNIC(newNVSentinelNICEvent("cmd_exec_timeout", "mlx5_0"))
	assert.False(t, ok)

	// Other checks are not NIC driver data points.
	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
	ev.CheckName = "InfiniBandStateCheck"
	_, ok = matchNVSentinelNIC(ev)
	assert.False(t, ok)
}

func TestNVSentinelIBEventInserted(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	src.Send(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)

	stored := events[0]
	assert.Equal(t, eventPCIPowerInsufficient, stored.Name)
	assert.Equal(t, string(apiv1.EventTypeFatal), stored.Type)
	assert.Equal(t, "nvs-event-1", stored.ExtraInfo[EventKeyNVSentinelEventID])
	assert.Equal(t, "mlx5_0", stored.ExtraInfo[EventKeyNVSentinelNICDevice])

	// The events API surfaces the NVSentinel event under the component's own
	// event name.
	apiEvents, err := c.Events(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, apiEvents, 1)
	assert.Equal(t, eventPCIPowerInsufficient, apiEvents[0].Name)
}

func TestNVSentinelIBHealthyEventNotStored(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
	ev.IsFatal = false
	ev.IsHealthy = true
	src.Send(ev)

	time.Sleep(500 * time.Millisecond)

	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestNVSentinelIBEventSkippedWhenGPUdTwinStored(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// GPUd's kmsg detection stores its copy first.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: time.Now().UTC(),
		Name: eventPCIPowerInsufficient,
		Type: string(apiv1.EventTypeCritical),
	}))

	// The NVSentinel copy of the same incident is skipped.
	src.Send(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))
	time.Sleep(500 * time.Millisecond)

	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestMatchPreferNVSentinel(t *testing.T) {
	const line = "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W)."

	// Without coverage the kmsg matcher behaves as before.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	name, message := c.matchPreferNVSentinel(line)
	assert.Equal(t, eventPCIPowerInsufficient, name)
	assert.NotEmpty(t, message)

	// With a persisted NVSentinel event for the same pattern, the kmsg line
	// is suppressed. Coverage is reported by the persisting component; the
	// test reports it directly to stay deterministic.
	src.RecordCoverage(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))
	name, message = c.matchPreferNVSentinel(line)
	assert.Empty(t, name)
	assert.Empty(t, message)

	// A different pattern is not covered.
	name, _ = c.matchPreferNVSentinel("mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature")
	assert.Equal(t, eventPortModuleHighTemperature, name)
}

func TestMatchPreferNVSentinelWithoutSource(t *testing.T) {
	c := &component{}
	name, _ := c.matchPreferNVSentinel("mlx5_core 0000:5c:00.0: Detected insufficient power on the PCIe slot (27W).")
	assert.Equal(t, eventPCIPowerInsufficient, name)
}
func TestNVSentinelIBNonFatalEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
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

func TestNVSentinelIBEventNoEntity(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// An event without a NIC entity is still stored; the device is empty.
	src.Send(newNVSentinelNICEvent("pci_power_insufficient", ""))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "", events[0].ExtraInfo[EventKeyNVSentinelNICDevice])
}

func TestNVSentinelIBAccessRegFailedEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	src.Send(newNVSentinelNICEvent("access_reg_failed", "mlx5_1"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 10*time.Second, 50*time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, eventAccessRegFailed, events[0].Name)
	assert.Equal(t, "mlx5_1", events[0].ExtraInfo[EventKeyNVSentinelNICDevice])
}

func TestHasStoredEventTwinOutsideWindow(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// An event far in the past is outside the dedup window.
	old := time.Now().UTC().Add(-3 * time.Hour)
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: old,
		Name: eventPCIPowerInsufficient,
	}))

	assert.False(t, c.hasStoredEventTwin(eventPCIPowerInsufficient, "", time.Now().UTC()))
}

func TestHasStoredEventTwinDifferentName(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	now := time.Now().UTC()
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: now,
		Name: eventPortModuleHighTemperature,
	}))

	// A different event name is not a twin.
	assert.False(t, c.hasStoredEventTwin(eventPCIPowerInsufficient, "", now))
	// The same event name within the window is a twin.
	assert.True(t, c.hasStoredEventTwin(eventPortModuleHighTemperature, "", now))
}
func TestNVSentinelIBNonMatchingEvent(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// An Xid event is not matched by the infiniband component.
	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
	ev.CheckName = "SysLogsXIDError"
	ev.ComponentClass = "GPU"
	src.Send(ev)

	time.Sleep(500 * time.Millisecond)
	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestNVSentinelIBEventBucketNil(t *testing.T) {
	// When eventBucket is nil, onNVSentinelEvent returns early without panic.
	c := &component{ctx: context.Background()}
	c.onNVSentinelEvent(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))
	// No panic, no crash.
}

func TestMatchNVSentinelNICEmptyErrorCodes(t *testing.T) {
	ev := nvsentinel.HealthEvent{
		CheckName:      checkNameSyslogsNICDriverError,
		ComponentClass: "NIC",
	}
	_, ok := matchNVSentinelNIC(ev)
	assert.False(t, ok)
}

func TestMatchNVSentinelNICUnknownPattern(t *testing.T) {
	ev := nvsentinel.HealthEvent{
		CheckName:      checkNameSyslogsNICDriverError,
		ComponentClass: "NIC",
		ErrorCodes:     []string{"unknown_pattern"},
	}
	_, ok := matchNVSentinelNIC(ev)
	assert.False(t, ok)
}
func TestNVSentinelIBWatchNilSource(t *testing.T) {
	// watchNVSentinel with nil source returns immediately without panic.
	c := &component{ctx: context.Background()}
	c.watchNVSentinel()
}

func TestNVSentinelIBDefaultDedupWindowFallback(t *testing.T) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	t.Cleanup(cleanup)

	store, err := eventstore.New(dbRW, dbRO, time.Hour)
	require.NoError(t, err)

	src := nvsentineltest.NewFakeSource()
	gpudInstance := &components.GPUdInstance{
		RootCtx:                    ctx,
		EventStore:                 store,
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

func TestNVSentinelIBOnNVSentinelEventHealthySkipped(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
	ev.IsHealthy = true
	src.Send(ev)

	time.Sleep(500 * time.Millisecond)
	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestNVSentinelIBEventSkippedWhenGPUdTwinStoredWithEventName(t *testing.T) {
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// GPUd stores its copy first.
	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: time.Now().UTC(),
		Name: eventPortModuleHighTemperature,
	}))

	// The NVSentinel copy of the same incident is skipped.
	src.Send(newNVSentinelNICEvent("port_module_high_temp", "mlx5_0"))
	time.Sleep(500 * time.Millisecond)

	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
func TestMatchPreferNVSentinelWithUnrelatedEventInSource(t *testing.T) {
	// An unrelated persisted event (wrong check name) does not cover the
	// kmsg data point, so the matcher still returns the event.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
	ev.CheckName = "SomeOtherCheck"
	src.RecordCoverage(ev)

	const line = "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W)."
	name, message := c.matchPreferNVSentinel(line)
	assert.Equal(t, eventPCIPowerInsufficient, name)
	assert.NotEmpty(t, message)
}

func TestNVSentinelIBAccessRegFailedPerDevice(t *testing.T) {
	// Regression: ACCESS_REG dedup is per-device (see kmsgEventDedupWindow).
	// Two NVSentinel ACCESS_REG failures from different NICs within the
	// dedup window are distinct incidents and must both be stored.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	src.Send(newNVSentinelNICEvent("access_reg_failed", "mlx5_0"))
	src.Send(newNVSentinelNICEvent("access_reg_failed", "mlx5_1"))

	ctx := context.Background()
	require.Eventually(t, func() bool {
		events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
		return err == nil && len(events) == 2
	}, 10*time.Second, 50*time.Millisecond)

	// A repeated failure on the SAME device within the window is a twin
	// and stays suppressed.
	src.Send(newNVSentinelNICEvent("access_reg_failed", "mlx5_0"))
	time.Sleep(500 * time.Millisecond)

	events, err := c.eventBucket.Get(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestNVSentinelIBAccessRegFailedTwinFallbackWithoutDevice(t *testing.T) {
	// A native kmsg copy carries no NVSentinel NIC device key: the twin
	// check deliberately falls back to the name-only match.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	require.NoError(t, c.eventBucket.Insert(context.Background(), eventstore.Event{
		Time: time.Now().UTC(),
		Name: eventAccessRegFailed,
		Type: string(apiv1.EventTypeCritical),
	}))

	src.Send(newNVSentinelNICEvent("access_reg_failed", "mlx5_1"))
	time.Sleep(500 * time.Millisecond)

	events, err := c.eventBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestNVSentinelIBHealthyEventDoesNotCover(t *testing.T) {
	// Regression: a healthy event stores no data point, so it must never
	// enter the coverage index and suppress a later native incident.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	ev := newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0")
	ev.IsFatal = false
	ev.IsHealthy = true
	src.Send(ev)

	// Give the forwarder a chance to process (and skip) the event.
	time.Sleep(500 * time.Millisecond)

	const line = "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W)."
	name, message := c.matchPreferNVSentinel(line)
	assert.Equal(t, eventPCIPowerInsufficient, name)
	assert.NotEmpty(t, message)
}

func TestNVSentinelIBInsertFailureDoesNotCover(t *testing.T) {
	// Regression: when the event-store insert fails, the data point is not
	// durable, so the event must not cover and suppress a native incident.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	// Force every insert to fail: no data point becomes durable.
	c.eventBucket = &mockEventBucket{insertErr: errors.New("insert failed")}

	src.Send(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))

	// Give the pipeline a chance to attempt (and fail) the insert.
	time.Sleep(500 * time.Millisecond)

	const line = "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W)."
	name, message := c.matchPreferNVSentinel(line)
	assert.Equal(t, eventPCIPowerInsufficient, name)
	assert.NotEmpty(t, message)
}

func TestNVSentinelIBEventCoveredAfterPersist(t *testing.T) {
	// The happy path: once the NVSentinel copy is durably stored, the
	// native kmsg twin is covered and suppressed.
	src := nvsentineltest.NewFakeSource()
	c := newTestComponentWithNVSentinel(t, src)

	src.Send(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))

	// Coverage is reported after the bucket insert succeeds.
	require.Eventually(t, func() bool {
		return src.Covers(c.nvsDedupWindow, func(ev nvsentinel.HealthEvent) bool {
			return ev.CheckName == checkNameSyslogsNICDriverError &&
				len(ev.ErrorCodes) > 0 && ev.ErrorCodes[0] == "pci_power_insufficient"
		})
	}, 10*time.Second, 50*time.Millisecond)

	const line = "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W)."
	name, message := c.matchPreferNVSentinel(line)
	assert.Empty(t, name)
	assert.Empty(t, message)
}

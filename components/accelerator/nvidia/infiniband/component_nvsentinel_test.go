package infiniband

import (
	"context"
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

	// With a recent NVSentinel event for the same pattern, the kmsg line is
	// suppressed.
	src.Send(newNVSentinelNICEvent("pci_power_insufficient", "mlx5_0"))
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

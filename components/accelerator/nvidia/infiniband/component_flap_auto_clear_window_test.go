package infiniband

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	infinibandstore "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/store"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

// sevenActiveOneDown returns 7 ACTIVE ports (mlx5_0..mlx5_6) plus a DOWN mlx5_7,
// i.e. one short of an 8-port threshold.
func sevenActiveOneDown() infinibandclass.Devices {
	devices := createHealthyDevices(7, 400)
	return append(devices, infinibandclass.Device{
		Name: "mlx5_7",
		Ports: []infinibandclass.Port{
			{
				Port:      uint(1),
				State:     "DOWN",
				PhysState: "Polling",
				RateGBSec: 0,
				LinkLayer: "InfiniBand",
			},
		},
	})
}

func newFlapTestComponent(mockStore *mockIBPortsStoreForStickyDrop, flapAutoClearWindow time.Duration, now *time.Time) *component {
	return &component{
		ctx:                 context.Background(),
		flapAutoClearWindow: flapAutoClearWindow,
		ibPortsStore:        mockStore,
		nvmlInstance:        &mockNVMLInstance{exists: true, productName: "Test GPU"},
		getTimeNowFunc: func() time.Time {
			return *now
		},
		getThresholdsFunc: func() types.ExpectedPortStates {
			return types.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400}
		},
	}
}

func TestDefaultFlapAutoClearWindowAccessors(t *testing.T) {
	oldWindow := GetDefaultFlapAutoClearWindow()
	t.Cleanup(func() {
		SetDefaultFlapAutoClearWindow(oldWindow)
	})

	window := 7 * time.Minute
	SetDefaultFlapAutoClearWindow(window)
	assert.Equal(t, window, GetDefaultFlapAutoClearWindow())
}

// TestFlapAutoClearWindowDisabledIsAlwaysSticky verifies the default behavior
// (flapAutoClearWindow <= 0): a flap stays surfaced even after the port has been
// stably ACTIVE, until an operator runs set-healthy.
func TestFlapAutoClearWindowDisabledIsAlwaysSticky(t *testing.T) {
	now := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{
			{
				Time:        now.Add(-1 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_1", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_1 port 1 flapping",
			},
		},
	}
	c := newFlapTestComponent(mockStore, 0, &now)
	c.getClassDevicesFunc = func(_ map[string]struct{}) (infinibandclass.Devices, error) {
		return createHealthyDevices(8, 400), nil // all ports healthy now
	}

	cr := requireCheckResult(t, c.Check())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"with flap sticky window disabled, an old flap must stay sticky even when ports are healthy")
	assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_1")
}

// TestFlapAutoClearWindowOptInRecovers verifies that with the opt-in window set, a
// flapping port that becomes stably ACTIVE clears once the recovery window
// elapses — symmetric with the drop sticky window.
func TestFlapAutoClearWindowOptInRecovers(t *testing.T) {
	now := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{
			{
				// Old flap timestamp: must NOT be used as the recovery signal.
				Time:        now.Add(-1 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_7 port 1 flapping",
			},
		},
	}
	c := newFlapTestComponent(mockStore, 10*time.Minute, &now)

	// Step 1: port currently DOWN (thresholds failing) -> flap surfaced.
	c.getClassDevicesFunc = func(_ map[string]struct{}) (infinibandclass.Devices, error) {
		return sevenActiveOneDown(), nil
	}
	cr := requireCheckResult(t, c.Check())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, "down port must surface the flap")
	assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_7")

	// Step 2: port recovers (8/8). The failing->passing transition starts the
	// recovery timer; within the window the flap is still surfaced.
	c.getClassDevicesFunc = func(_ map[string]struct{}) (infinibandclass.Devices, error) {
		return createHealthyDevices(8, 400), nil
	}
	cr = requireCheckResult(t, c.Check())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"just-recovered port must remain sticky during the recovery window")
	assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_7")

	// Step 3: stay healthy past the window -> flap clears.
	now = now.Add(11 * time.Minute)
	cr = requireCheckResult(t, c.Check())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"after the recovery window elapses with stable ports, the flap must clear")
	assert.Contains(t, cr.reason, "no infiniband port issue")
}

// TestFlapAutoClearWindowOptInSurfacesRecentFlapAfterRestart verifies that a
// recent persisted flap is still surfaced after gpud restarts and loses its
// in-memory recovery timer.
func TestFlapAutoClearWindowOptInSurfacesRecentFlapAfterRestart(t *testing.T) {
	now := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{
			{
				Time:        now.Add(-1 * time.Minute),
				Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_7 port 1 flapping",
			},
		},
	}
	c := newFlapTestComponent(mockStore, 10*time.Minute, &now)
	c.getClassDevicesFunc = func(_ map[string]struct{}) (infinibandclass.Devices, error) {
		return createHealthyDevices(8, 400), nil
	}

	cr := requireCheckResult(t, c.Check())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
		"a recent persisted flap must remain visible after restart even before an in-memory recovery transition")
	assert.Contains(t, cr.reason, "device(s) flapping between ACTIVE<>DOWN: mlx5_7")
}

// TestFlapAutoClearWindowOptInClearsOldRecoveredFlapAfterRestart verifies that
// the restart bootstrap fallback does not make all recovered flaps sticky again.
func TestFlapAutoClearWindowOptInClearsOldRecoveredFlapAfterRestart(t *testing.T) {
	now := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{
			{
				Time:        now.Add(-1 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_7 port 1 flapping",
			},
		},
	}
	c := newFlapTestComponent(mockStore, 10*time.Minute, &now)
	c.getClassDevicesFunc = func(_ map[string]struct{}) (infinibandclass.Devices, error) {
		return createHealthyDevices(8, 400), nil
	}

	cr := requireCheckResult(t, c.Check())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health,
		"an old recovered flap should still auto-clear after restart")
	assert.Contains(t, cr.reason, "no infiniband port issue")
}

// TestFlapAutoClearWindowOptInStaysStickyWhileFlapping verifies a port that keeps
// dipping below threshold never clears: each dip resets the recovery timer.
func TestFlapAutoClearWindowOptInStaysStickyWhileFlapping(t *testing.T) {
	now := time.Now().UTC()
	mockStore := &mockIBPortsStoreForStickyDrop{
		events: []infinibandstore.Event{
			{
				Time:        now.Add(-2 * time.Hour),
				Port:        types.IBPort{Device: "mlx5_7", Port: uint(1)},
				EventType:   infinibandstore.EventTypeIbPortFlap,
				EventReason: "mlx5_7 port 1 flapping",
			},
		},
	}
	c := newFlapTestComponent(mockStore, 10*time.Minute, &now)

	down := func(_ map[string]struct{}) (infinibandclass.Devices, error) { return sevenActiveOneDown(), nil }
	up := func(_ map[string]struct{}) (infinibandclass.Devices, error) { return createHealthyDevices(8, 400), nil }

	// Flap a few cycles, each separated by less than the window, and confirm it
	// never reports Healthy: the recovery timer resets on every dip.
	for i := 0; i < 3; i++ {
		c.getClassDevicesFunc = down
		cr := requireCheckResult(t, c.Check())
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health, "down phase must be unhealthy")

		now = now.Add(2 * time.Minute)
		c.getClassDevicesFunc = up
		cr = requireCheckResult(t, c.Check())
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health,
			"recovered-but-within-window must remain unhealthy while still flapping")

		now = now.Add(2 * time.Minute)
	}
}

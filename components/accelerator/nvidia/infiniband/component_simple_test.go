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

// TestSimpleDropProcessing tests the most basic case of drop processing
func TestSimpleDropProcessing(t *testing.T) {
	now := time.Now().UTC()

	// Test 1: Recent drop with thresholds passing
	t.Run("recent_drop_thresholds_passing", func(t *testing.T) {
		mockStore := &mockIBPortsStoreSimple{
			events: []infinibandstore.Event{
				{
					Time:        now.Add(-5 * time.Minute), // Recent drop
					Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_0 port 1 down",
				},
			},
		}

		c := &component{
			ctx:              context.Background(),
			dropStickyWindow: 10 * time.Minute,
			ibPortsStore:     mockStore,
			nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
			getTimeNowFunc: func() time.Time {
				return now
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{
					AtLeastPorts: 8,
					AtLeastRate:  400,
				}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				// All thresholds met
				return createHealthyDevices(8, 400), nil
			},
		}

		cr := c.Check().(*checkResult)

		// Recent drop should be processed even with thresholds passing
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "mlx5_0")
	})

	// Test 2: Old drop with thresholds failing
	t.Run("old_drop_thresholds_failing", func(t *testing.T) {
		mockStore := &mockIBPortsStoreSimple{
			events: []infinibandstore.Event{
				{
					Time:        now.Add(-2 * time.Hour), // Old drop
					Port:        types.IBPort{Device: "mlx5_0", Port: uint(1)},
					EventType:   infinibandstore.EventTypeIbPortDrop,
					EventReason: "mlx5_0 port 1 down",
				},
			},
		}

		c := &component{
			ctx:              context.Background(),
			dropStickyWindow: 10 * time.Minute,
			ibPortsStore:     mockStore,
			nvmlInstance:     &mockNVMLInstance{exists: true, productName: "Test GPU"},
			getTimeNowFunc: func() time.Time {
				return now
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{
					AtLeastPorts: 8,
					AtLeastRate:  400,
				}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				// Only 7 ports - thresholds failing
				return createHealthyDevices(7, 400), nil
			},
		}

		cr := c.Check().(*checkResult)

		// Should be unhealthy due to thresholds
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		// Should mention threshold failure
		assert.Contains(t, cr.reason, "only 7 port(s)")
		// Old drop should ALSO be included when thresholds fail
		assert.Contains(t, cr.reason, "mlx5_0",
			"Old drop should be included when thresholds are failing")
	})
}

// Mock store for simple tests
type mockIBPortsStoreSimple struct {
	events []infinibandstore.Event
}

func (m *mockIBPortsStoreSimple) Insert(time.Time, []types.IBPort) error {
	return nil
}

func (m *mockIBPortsStoreSimple) Scan() error {
	return nil
}

func (m *mockIBPortsStoreSimple) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStoreSimple) SetEventType(string, uint, time.Time, string, string) error {
	return nil
}

func (m *mockIBPortsStoreSimple) SetHealthy() error {
	return nil
}

func (m *mockIBPortsStoreSimple) Tombstone(timestamp time.Time) error {
	return nil
}

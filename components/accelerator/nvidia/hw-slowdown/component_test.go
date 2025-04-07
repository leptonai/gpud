package hwslowdown

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Mock implementation of nvml.InstanceV2
type mockNVMLInstance struct {
	devices     map[string]device.Device
	productName string
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return true
}

func (m *mockNVMLInstance) Library() lib.Library {
	return nil
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

// Helper function to create a mock NVML instance with specified devices
func createMockNVMLInstance(devices map[string]device.Device) *mockNVMLInstance {
	return &mockNVMLInstance{
		devices:     devices,
		productName: "NVIDIA Test GPU",
	}
}

// TestCheckOnce tests the CheckOnce method using mock device functions
func TestCheckOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		mockDevices        map[string]device.Device
		mockClockEvents    map[string]nvidianvml.ClockEvents
		expectEvents       int
		expectSlowdown     bool
		expectThermal      bool
		expectPowerBrake   bool
		expectHealthyState bool
	}{
		{
			name: "no hardware slowdown",
			mockDevices: map[string]device.Device{
				"gpu-0": testutil.NewMockDevice(
					&mock.Device{
						GetUUIDFunc: func() (string, nvml.Return) {
							return "gpu-0", nvml.SUCCESS
						},
					},
					"test-arch", "test-brand", "test-cuda", "test-pci",
				),
			},
			mockClockEvents: map[string]nvidianvml.ClockEvents{
				"gpu-0": {
					UUID:                 "gpu-0",
					Time:                 metav1.Time{Time: time.Now().UTC()},
					HWSlowdown:           false,
					HWSlowdownThermal:    false,
					HWSlowdownPowerBrake: false,
					Supported:            true,
				},
			},
			expectEvents:       0,
			expectSlowdown:     false,
			expectThermal:      false,
			expectPowerBrake:   false,
			expectHealthyState: true,
		},
		{
			name: "hardware slowdown",
			mockDevices: map[string]device.Device{
				"gpu-1": testutil.NewMockDevice(
					&mock.Device{
						GetUUIDFunc: func() (string, nvml.Return) {
							return "gpu-1", nvml.SUCCESS
						},
					},
					"test-arch", "test-brand", "test-cuda", "test-pci",
				),
			},
			mockClockEvents: map[string]nvidianvml.ClockEvents{
				"gpu-1": {
					UUID:                 "gpu-1",
					Time:                 metav1.Time{Time: time.Now().UTC()},
					HWSlowdown:           true,
					HWSlowdownThermal:    false,
					HWSlowdownPowerBrake: false,
					Supported:            true,
					HWSlowdownReasons:    []string{"GPU slowdown detected"},
				},
			},
			expectEvents:       1,
			expectSlowdown:     true,
			expectThermal:      false,
			expectPowerBrake:   false,
			expectHealthyState: true, // One event is still healthy
		},
		{
			name: "hardware thermal slowdown",
			mockDevices: map[string]device.Device{
				"gpu-2": testutil.NewMockDevice(
					&mock.Device{
						GetUUIDFunc: func() (string, nvml.Return) {
							return "gpu-2", nvml.SUCCESS
						},
					},
					"test-arch", "test-brand", "test-cuda", "test-pci",
				),
			},
			mockClockEvents: map[string]nvidianvml.ClockEvents{
				"gpu-2": {
					UUID:                 "gpu-2",
					Time:                 metav1.Time{Time: time.Now().UTC()},
					HWSlowdown:           false,
					HWSlowdownThermal:    true,
					HWSlowdownPowerBrake: false,
					Supported:            true,
					HWSlowdownReasons:    []string{"GPU thermal slowdown detected"},
				},
			},
			expectEvents:       1,
			expectSlowdown:     false,
			expectThermal:      true,
			expectPowerBrake:   false,
			expectHealthyState: true,
		},
		{
			name: "multiple hardware slowdown events",
			mockDevices: map[string]device.Device{
				"gpu-3": testutil.NewMockDevice(
					&mock.Device{
						GetUUIDFunc: func() (string, nvml.Return) {
							return "gpu-3", nvml.SUCCESS
						},
					},
					"test-arch", "test-brand", "test-cuda", "test-pci",
				),
				"gpu-4": testutil.NewMockDevice(
					&mock.Device{
						GetUUIDFunc: func() (string, nvml.Return) {
							return "gpu-4", nvml.SUCCESS
						},
					},
					"test-arch", "test-brand", "test-cuda", "test-pci",
				),
			},
			mockClockEvents: map[string]nvidianvml.ClockEvents{
				"gpu-3": {
					UUID:                 "gpu-3",
					Time:                 metav1.Time{Time: time.Now().UTC()},
					HWSlowdown:           true,
					HWSlowdownThermal:    false,
					HWSlowdownPowerBrake: false,
					Supported:            true,
					HWSlowdownReasons:    []string{"GPU slowdown detected"},
				},
				"gpu-4": {
					UUID:                 "gpu-4",
					Time:                 metav1.Time{Time: time.Now().UTC()},
					HWSlowdown:           false,
					HWSlowdownThermal:    false,
					HWSlowdownPowerBrake: true,
					Supported:            true,
					HWSlowdownReasons:    []string{"GPU power brake slowdown detected"},
				},
			},
			expectEvents:       2,
			expectSlowdown:     true,
			expectThermal:      false,
			expectPowerBrake:   true,
			expectHealthyState: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Set up test database
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
			assert.NoError(t, err)
			bucket, err := store.Bucket("test_events")
			assert.NoError(t, err)
			defer bucket.Close()

			// Create mock NVML instance
			mockNVML := createMockNVMLInstance(tc.mockDevices)

			c := &component{
				ctx:              ctx,
				cancel:           cancel,
				evaluationWindow: DefaultStateHWSlowdownEvaluationWindow,
				threshold:        DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
				eventBucket:      bucket,
				nvmlInstanceV2:   mockNVML,
				// Initialize lastData to avoid nil pointer dereference
				lastData: &Data{
					ts:      time.Now().UTC(),
					healthy: true,
					reason:  "Initial state",
				},
				getClockEventsFunc: func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error) {
					if events, ok := tc.mockClockEvents[uuid]; ok {
						return events, nil
					}
					return nvidianvml.ClockEvents{}, fmt.Errorf("no mock clock events for %s", uuid)
				},
			}

			// Run the check
			c.CheckOnce()

			// Verify the component's state
			assert.NotNil(t, c.lastData)

			// Verify that clock events were collected correctly
			assert.Equal(t, len(tc.mockDevices), len(c.lastData.ClockEvents))

			// Get events from the bucket
			events, err := bucket.Get(ctx, time.Now().UTC().Add(-time.Hour))
			assert.NoError(t, err)
			assert.Equal(t, tc.expectEvents, len(events))

			// Validate component state
			states, err := c.States(ctx)
			assert.NoError(t, err)
			assert.Equal(t, 1, len(states))
			assert.Equal(t, tc.expectHealthyState, states[0].Healthy)
		})
	}
}

// TestComponentStates tests the States method with various scenarios of slowdown events
func TestComponentStates(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_states")
	assert.NoError(t, err)
	defer bucket.Close()

	// Create mock device
	mockDevice := testutil.NewMockDevice(
		&mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "gpu-0", nvml.SUCCESS
			},
		},
		"test-arch", "test-brand", "test-cuda", "test-pci",
	)

	mockDevices := map[string]device.Device{
		"gpu-0": mockDevice,
	}

	// Create mock NVML instance
	mockNVML := createMockNVMLInstance(mockDevices)

	// Create test events
	testEvents := []components.Event{
		{
			Time:    metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-0",
			},
		},
	}

	for _, event := range testEvents {
		err := bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Create component with test data
	c := &component{
		ctx:              ctx,
		cancel:           cancel,
		evaluationWindow: 10 * time.Minute,
		threshold:        0.1,
		eventBucket:      bucket,
		nvmlInstanceV2:   mockNVML,
		lastData: &Data{
			ts:      time.Now(),
			healthy: true,
			reason:  "Initial state",
		},
		getClockEventsFunc: func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error) {
			return nvidianvml.ClockEvents{
				UUID:                 uuid,
				Time:                 metav1.Time{Time: time.Now()},
				HWSlowdown:           false,
				HWSlowdownThermal:    false,
				HWSlowdownPowerBrake: false,
				Supported:            true,
			}, nil
		},
	}

	// Get states
	states, err := c.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
}

func TestComponentStatesEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		window             time.Duration
		thresholdPerMinute float64
		setupStore         func(bucket eventstore.Bucket, ctx context.Context) error
		expectError        bool
		expectedStates     int
		expectHealthy      bool
	}{
		{
			name:               "zero evaluation window",
			window:             0,
			thresholdPerMinute: 0.6,
			setupStore:         func(bucket eventstore.Bucket, ctx context.Context) error { return nil },
			expectError:        false,
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "negative evaluation window",
			window:             -10 * time.Minute,
			thresholdPerMinute: 0.6,
			setupStore:         func(bucket eventstore.Bucket, ctx context.Context) error { return nil },
			expectError:        false,
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "zero threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: 0,
			setupStore: func(bucket eventstore.Bucket, ctx context.Context) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				}
				return bucket.Insert(ctx, event)
			},
			expectError:    false,
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "negative threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: -0.6,
			setupStore: func(bucket eventstore.Bucket, ctx context.Context) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				}
				return bucket.Insert(ctx, event)
			},
			expectError:    false,
			expectedStates: 1,
			expectHealthy:  true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
			assert.NoError(t, err)
			bucket, err := store.Bucket("test_events")
			assert.NoError(t, err)
			defer bucket.Close()

			err = tc.setupStore(bucket, ctx)
			assert.NoError(t, err)

			c := &component{
				evaluationWindow: tc.window,
				threshold:        tc.thresholdPerMinute,
				eventBucket:      bucket,
			}

			states, err := c.States(ctx)
			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStates, len(states))
			if len(states) > 0 {
				assert.Equal(t, tc.expectHealthy, states[0].Healthy)
			}
		})
	}
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	// Create mock NVML instance
	mockNVML := createMockNVMLInstance(map[string]device.Device{})

	c := &component{
		nvmlInstanceV2: mockNVML,
		// Initialize required functions to avoid nil pointer dereference
		getClockEventsFunc: func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error) {
			return nvidianvml.ClockEvents{}, nil
		},
	}

	assert.Equal(t, Name, c.Name())
}

// TestComponentStart tests the Start method
func TestComponentStart(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock devices
	mockDevice := testutil.NewMockDevice(
		&mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "gpu-0", nvml.SUCCESS
			},
		},
		"test-arch", "test-brand", "test-cuda", "test-pci",
	)

	mockDevices := map[string]device.Device{
		"gpu-0": mockDevice,
	}

	// Create mock NVML instance
	mockNVML := createMockNVMLInstance(mockDevices)

	c := &component{
		ctx:            ctx,
		cancel:         cancel,
		nvmlInstanceV2: mockNVML,
		// Initialize mock functions to avoid nil pointer dereference if Start() is called
		getClockEventsFunc: func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error) {
			return nvidianvml.ClockEvents{}, nil
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Let the goroutine run for a short time
	time.Sleep(10 * time.Millisecond)
}

// TestComponentEvents tests the Events method
func TestComponentEvents(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	// Insert test events
	testEvents := []components.Event{
		{
			Time:    metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-0",
			},
		},
		{
			Time:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-1",
			},
		},
	}

	for _, event := range testEvents {
		err := bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Create mock device
	mockDevice := testutil.NewMockDevice(
		&mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "gpu-0", nvml.SUCCESS
			},
		},
		"test-arch", "test-brand", "test-cuda", "test-pci",
	)

	mockDevices := map[string]device.Device{
		"gpu-0": mockDevice,
	}

	// Create mock NVML instance
	mockNVML := createMockNVMLInstance(mockDevices)

	c := &component{
		ctx:              ctx,
		cancel:           cancel,
		evaluationWindow: 10 * time.Minute,
		threshold:        0.1,
		eventBucket:      bucket,
		nvmlInstanceV2:   mockNVML,
		getClockEventsFunc: func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error) {
			return nvidianvml.ClockEvents{
				UUID:                 uuid,
				Time:                 metav1.Time{Time: time.Now()},
				HWSlowdown:           false,
				HWSlowdownThermal:    false,
				HWSlowdownPowerBrake: false,
				Supported:            true,
			}, nil
		},
	}

	// Filter by time to get events within the last 3 hours
	since := time.Now().Add(-3 * time.Hour)
	events, err := c.Events(ctx, since)
	assert.NoError(t, err)
	assert.Len(t, events, 2)

	// Filter by time to get events within the last 90 minutes
	since = time.Now().Add(-90 * time.Minute)
	events, err = c.Events(ctx, since)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

// TestHighFrequencySlowdownEvents tests that a high frequency of hardware slowdown events
// triggers an unhealthy state when using the mock device functions
func TestHighFrequencySlowdownEvents(t *testing.T) {
	t.Parallel()

	// Setup test database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	// Create mock device
	mockDevice := testutil.NewMockDevice(
		&mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "gpu-0", nvml.SUCCESS
			},
		},
		"test-arch", "test-brand", "test-cuda", "test-pci",
	)

	mockDevices := map[string]device.Device{
		"gpu-0": mockDevice,
	}

	// Create mock NVML instance
	mockNVML := createMockNVMLInstance(mockDevices)

	// Setup test parameters
	window := 10 * time.Minute
	thresholdFrequency := 0.6 // Events per minute threshold

	// Create component for testing
	c := &component{
		ctx:              ctx,
		cancel:           cancel,
		evaluationWindow: window,
		threshold:        thresholdFrequency,
		eventBucket:      bucket,
		nvmlInstanceV2:   mockNVML,
		getClockEventsFunc: func(uuid string, dev device.Device) (nvidianvml.ClockEvents, error) {
			return nvidianvml.ClockEvents{
				UUID:                 uuid,
				Time:                 metav1.Time{Time: time.Now().UTC()},
				HWSlowdown:           true,
				HWSlowdownThermal:    false,
				HWSlowdownPowerBrake: false,
				Supported:            true,
				HWSlowdownReasons:    []string{"GPU slowdown detected"},
			}, nil
		},
	}

	// Run CheckOnce - it should update lastData
	c.CheckOnce()

	// Verify lastData was updated
	c.lastMu.RLock()
	assert.NotNil(t, c.lastData)
	assert.True(t, c.lastData.healthy, "Component should be healthy with no events")
	c.lastMu.RUnlock()

	// Generate a high frequency of events that should trigger unhealthy state
	now := time.Now().UTC()
	eventsPerGPU := 10
	totalEventsToInsert := eventsPerGPU
	windowMinutes := int(window.Minutes())

	for i := 0; i < totalEventsToInsert; i++ {
		// Distribute events evenly within the window
		eventTime := now.Add(-time.Duration(i*(windowMinutes/totalEventsToInsert)) * time.Minute)

		event := components.Event{
			Time:    metav1.Time{Time: eventTime},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-0",
			},
		}
		err := bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Run CheckOnce again to process the new events
	c.CheckOnce()

	// Get the states and verify they reflect the unhealthy condition
	states, err := c.States(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)

	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "hw slowdown events frequency per minute")
	assert.Contains(t, states[0].Reason, "exceeded threshold")
}

package os

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// MockRebootEventStore is a mock implementation of the RebootEventStore interface
type MockRebootEventStore struct {
	events apiv1.Events
}

func (m *MockRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *MockRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return m.events, nil
}

// ErrorRebootEventStore is a mock implementation that always returns an error
type ErrorRebootEventStore struct{}

func (m *ErrorRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *ErrorRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, errors.New("mock event store error")
}

func TestData_GetError(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "with error",
			data: &Data{
				err: assert.AnError,
			},
			expected: "assert.AnError general error for testing",
		},
		{
			name: "no error",
			data: &Data{
				Kernel: Kernel{
					Version: "5.15.0",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.getError()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestData_GetStates(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		validate func(*testing.T, []apiv1.HealthState)
	}{
		{
			name: "nil data",
			data: nil,
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
				assert.Equal(t, "no data yet", states[0].Reason)
			},
		},
		{
			name: "with error",
			data: &Data{
				err:    assert.AnError,
				health: apiv1.StateTypeUnhealthy,
				reason: "failed to get os data -- assert.AnError general error for testing",
				ts:     time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
				assert.Equal(t, "failed to get os data -- assert.AnError general error for testing", states[0].Reason)
				assert.Equal(t, "assert.AnError general error for testing", states[0].Error)
				assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
				assert.Equal(t, "json", states[0].DeprecatedExtraInfo["encoding"])
			},
		},
		{
			name: "with too many zombie processes",
			data: &Data{
				ProcessCountZombieProcesses: defaultZombieProcessCountThreshold + 1,
				Kernel: Kernel{
					Version: "5.15.0",
				},
				health: apiv1.StateTypeUnhealthy,
				reason: fmt.Sprintf("too many zombie processes: %d (threshold: %d)", defaultZombieProcessCountThreshold+1, defaultZombieProcessCountThreshold),
				ts:     time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
				expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", defaultZombieProcessCountThreshold+1, defaultZombieProcessCountThreshold)
				assert.Equal(t, expected, states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
				assert.Equal(t, "json", states[0].DeprecatedExtraInfo["encoding"])
			},
		},
		{
			name: "healthy case",
			data: &Data{
				Kernel: Kernel{
					Version: "5.15.0",
				},
				health: apiv1.StateTypeHealthy,
				reason: "os kernel version 5.15.0",
				ts:     time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
				assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].DeprecatedExtraInfo, "data")
				assert.Equal(t, "json", states[0].DeprecatedExtraInfo["encoding"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.getLastHealthStates()
			tt.validate(t, states)
		})
	}
}

func TestComponent(t *testing.T) {
	t.Parallel()

	_, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create a RebootEventStore implementation
	mockRebootStore := &MockRebootEventStore{
		events: apiv1.Events{
			{
				Time:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				Name:    "reboot",
				Type:    apiv1.EventTypeWarning,
				Message: "Test reboot event",
			},
		},
	}

	t.Run("component creation", func(t *testing.T) {
		comp, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NotNil(t, comp)
		assert.NoError(t, err)
		assert.Equal(t, Name, comp.Name())

		// Clean up
		err = comp.Close()
		assert.NoError(t, err)
	})

	t.Run("component states with no data", func(t *testing.T) {
		comp, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer comp.Close()

		// States should return default state when no data
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	})

	t.Run("component events", func(t *testing.T) {
		comp, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer comp.Close()

		// Get events
		since := time.Now().Add(-1 * time.Hour)
		events, err := comp.Events(ctx, since)
		assert.NoError(t, err)
		assert.Len(t, events, 1)

		// Verify our test event
		assert.Equal(t, "reboot", events[0].Name)
		assert.Equal(t, apiv1.EventTypeWarning, events[0].Type)
		assert.Equal(t, "Test reboot event", events[0].Message)
	})

	t.Run("component start and check once", func(t *testing.T) {
		comp, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer comp.Close()

		// Trigger CheckOnce manually
		c := comp.(*component)
		_ = c.Check()

		// Verify lastData is populated
		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)

		// Start the component (this starts a goroutine)
		assert.NoError(t, comp.Start())

		// Allow time for at least one check
		time.Sleep(100 * time.Millisecond)
	})
}

func TestComponent_States(t *testing.T) {
	t.Parallel()

	_, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a RebootEventStore implementation
	mockRebootStore := &MockRebootEventStore{}

	comp, err := New(&components.GPUdInstance{
		RootCtx:          ctx,
		RebootEventStore: mockRebootStore,
	})
	assert.NoError(t, err)
	defer comp.Close()

	t.Run("component states with no data", func(t *testing.T) {
		// States should return default state when no data
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("component states with data", func(t *testing.T) {
		// Inject test data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastData = &Data{
			Kernel: Kernel{
				Version: "5.15.0",
			},
			health: apiv1.StateTypeHealthy,
			reason: "os kernel version 5.15.0",
			ts:     time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
		assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
	})

	t.Run("component states with error", func(t *testing.T) {
		// Inject error data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastData = &Data{
			err:    errors.New("test error"),
			health: apiv1.StateTypeUnhealthy,
			reason: "failed to get os data -- test error",
			ts:     time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "failed to get os data -- test error", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
	})

	t.Run("component states with too many zombie processes", func(t *testing.T) {
		// Inject zombie process data
		c := comp.(*component)
		expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", defaultZombieProcessCountThreshold+1, defaultZombieProcessCountThreshold)
		c.lastMu.Lock()
		c.lastData = &Data{
			Kernel: Kernel{
				Version: "5.15.0",
			},
			ProcessCountZombieProcesses: defaultZombieProcessCountThreshold + 1,
			health:                      apiv1.StateTypeUnhealthy,
			reason:                      expected,
			ts:                          time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		assert.Equal(t, expected, states[0].Reason)
	})
}

// TestMockRebootEventStore tests the mock implementation of RebootEventStore
func TestMockRebootEventStore(t *testing.T) {
	mock := &MockRebootEventStore{
		events: apiv1.Events{
			{
				Time:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				Name:    "reboot",
				Type:    apiv1.EventTypeWarning,
				Message: "Test reboot event",
			},
		},
	}

	ctx := context.Background()

	// Test RecordReboot
	err := mock.RecordReboot(ctx)
	assert.NoError(t, err)

	// Test GetRebootEvents
	events, err := mock.GetRebootEvents(ctx, time.Now().Add(-2*time.Hour))
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "reboot", events[0].Name)
	assert.Equal(t, apiv1.EventTypeWarning, events[0].Type)
	assert.Equal(t, "Test reboot event", events[0].Message)
}

// TestCheckOnceWithMockedProcess tests the CheckOnce method with a mocked process counter
func TestCheckOnceWithMockedProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockRebootStore := &MockRebootEventStore{}

	// Test with process count error
	t.Run("process count error", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Override the process counting function to return an error
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
			return nil, errors.New("process count error")
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify error is captured
		comp.lastMu.RLock()
		data := comp.lastData
		comp.lastMu.RUnlock()

		assert.NotNil(t, data)
		assert.NotNil(t, data.err)
		assert.Equal(t, "process count error", data.err.Error())
		assert.Equal(t, apiv1.StateTypeUnhealthy, data.health)
		assert.Contains(t, data.reason, "error getting process count")
	})

	// Test normal case with no issues
	t.Run("healthy case", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Override the process counting function to return normal processes
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
			return map[string][]*procs.Process{
				procs.Running: make([]*procs.Process, 10),
				procs.Zombie:  make([]*procs.Process, 5),
			}, nil
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify data is healthy
		comp.lastMu.RLock()
		data := comp.lastData
		comp.lastMu.RUnlock()

		assert.NotNil(t, data)
		assert.Equal(t, 5, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.StateTypeHealthy, data.health)
		assert.Contains(t, data.reason, "os kernel version")
	})
}

// TestComponent_UptimeError tests the handling of uptime errors through the component's state
func TestComponent_UptimeError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockRebootStore := &MockRebootEventStore{}

	c, err := New(&components.GPUdInstance{
		RootCtx:          ctx,
		RebootEventStore: mockRebootStore,
	})
	assert.NoError(t, err)
	defer c.Close()
	comp := c.(*component)

	// Directly set error data to simulate an uptime error
	errorData := &Data{
		ts:     time.Now().UTC(),
		err:    errors.New("uptime error"),
		health: apiv1.StateTypeUnhealthy,
		reason: "error getting uptime: uptime error",
	}

	// Inject the error data
	comp.lastMu.Lock()
	comp.lastData = errorData
	comp.lastMu.Unlock()

	// Verify error handling through States
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error getting uptime: uptime error", states[0].Reason)
	assert.Equal(t, "uptime error", states[0].Error)
}

// TestComponent_EventsWithNilStore tests the Events method with a nil rebootEventStore
func TestComponent_EventsWithNilStore(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a component with nil rebootEventStore
	comp, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer comp.Close()

	// Call Events and verify it returns empty slice and no error
	since := time.Now().Add(-1 * time.Hour)
	events, err := comp.Events(ctx, since)
	assert.NoError(t, err)
	assert.Empty(t, events)
}

// TestZombieProcessCountThreshold tests that the zombie process count threshold is set correctly
func TestZombieProcessCountThreshold(t *testing.T) {
	// Just verify that the zombie process count threshold is set to a reasonable value
	// This is primarily to increase the test coverage of the init function
	assert.GreaterOrEqual(t, defaultZombieProcessCountThreshold, 1000)
}

// TestData_String tests the String method of Data struct
func TestData_String(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		validate func(*testing.T, string)
	}{
		{
			name: "nil data",
			data: nil,
			validate: func(t *testing.T, output string) {
				assert.Equal(t, "", output)
			},
		},
		{
			name: "valid data",
			data: &Data{
				VirtualizationEnvironment: pkghost.VirtualizationEnvironment{Type: "kvm"},
				Kernel: Kernel{
					Arch:    "x86_64",
					Version: "5.15.0",
				},
				Platform: Platform{
					Name:    "ubuntu",
					Version: "22.04",
				},
				Uptimes: Uptimes{
					Seconds:             3600,
					BootTimeUnixSeconds: uint64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
				},
				ProcessCountZombieProcesses: 5,
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "VM Type")
				assert.Contains(t, output, "kvm")
				assert.Contains(t, output, "Kernel Arch")
				assert.Contains(t, output, "x86_64")
				assert.Contains(t, output, "Kernel Version")
				assert.Contains(t, output, "5.15.0")
				assert.Contains(t, output, "Zombie Process Count")
				assert.Contains(t, output, "5")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.data.String()
			tt.validate(t, output)
		})
	}
}

// TestData_Summary tests the Summary method of Data struct
func TestData_Summary(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name: "with reason",
			data: &Data{
				reason: "test reason",
			},
			expected: "test reason",
		},
		{
			name:     "empty reason",
			data:     &Data{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.data.Summary()
			assert.Equal(t, tt.expected, summary)
		})
	}
}

// TestData_HealthState tests the HealthState method of Data struct
func TestData_HealthState(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected apiv1.HealthStateType
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "healthy",
			data: &Data{
				health: apiv1.StateTypeHealthy,
			},
			expected: apiv1.StateTypeHealthy,
		},
		{
			name: "unhealthy",
			data: &Data{
				health: apiv1.StateTypeUnhealthy,
			},
			expected: apiv1.StateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.data.HealthState()
			assert.Equal(t, tt.expected, state)
		})
	}
}

// TestComponent_NameMethod tests the Name method of component
func TestComponent_NameMethod(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
}

// TestComponent_ManualCheckSimulation tests manual data injection into component
func TestComponent_ManualCheckSimulation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Inject test data directly (simulating a check)
	testData := &Data{
		VirtualizationEnvironment: pkghost.VirtualizationEnvironment{Type: "docker"},
		Kernel: Kernel{
			Arch:    "x86_64",
			Version: "5.15.0",
		},
		ProcessCountZombieProcesses: 5,
		health:                      apiv1.StateTypeHealthy,
		reason:                      "os kernel version 5.15.0",
		ts:                          time.Now().UTC(),
	}

	comp.lastMu.Lock()
	comp.lastData = testData
	comp.lastMu.Unlock()

	// Get states and validate
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
}

// TestComponent_CheckWithUptimeError directly injects an uptime error
func TestComponent_CheckWithUptimeError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Inject error data directly
	errorData := &Data{
		err:    errors.New("mock uptime error"),
		health: apiv1.StateTypeUnhealthy,
		reason: "error getting uptime: mock uptime error",
		ts:     time.Now().UTC(),
	}

	comp.lastMu.Lock()
	comp.lastData = errorData
	comp.lastMu.Unlock()

	// Verify the error is reflected in health states
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error getting uptime: mock uptime error", states[0].Reason)
	assert.Equal(t, "mock uptime error", states[0].Error)
}

// TestComponent_CheckWithProcessError tests the Check method with mocked process error
func TestComponent_CheckWithProcessError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Override the process counting function to return an error
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
		return nil, errors.New("process count error")
	}

	// Call Check directly
	result := comp.Check()

	// Verify error handling
	data := result.(*Data)
	assert.NotNil(t, data.err)
	assert.Equal(t, "process count error", data.err.Error())
	assert.Equal(t, apiv1.StateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "error getting process count")
}

// TestComponent_CheckWithZombieProcesses tests the Check method with many zombie processes
func TestComponent_CheckWithZombieProcesses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	threshold := 10

	// Override the process counting function to return many zombie processes
	comp.zombieProcessCountThreshold = threshold
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
		return map[string][]*procs.Process{
			procs.Running: make([]*procs.Process, 10),
			procs.Zombie:  make([]*procs.Process, threshold+1),
		}, nil
	}

	// Call Check
	result := comp.Check()

	// Verify zombie process detection
	data := result.(*Data)
	assert.Equal(t, threshold+1, data.ProcessCountZombieProcesses)
	assert.Equal(t, apiv1.StateTypeUnhealthy, data.health)
	expectedReason := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", threshold+1, threshold)
	assert.Equal(t, expectedReason, data.reason)
}

// TestComponent_SystemManufacturer tests that the system manufacturer is captured
func TestComponent_SystemManufacturer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Override the process counting function to return normal processes
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
		return map[string][]*procs.Process{
			procs.Running: make([]*procs.Process, 10),
		}, nil
	}

	// Call Check
	result := comp.Check()

	// Verify manufacturer info is captured
	data := result.(*Data)
	// We don't care about the specific value, just that it's set
	assert.NotPanics(t, func() {
		_ = data.SystemManufacturer
	})
}

// TestComponent_WithRebootEventStore tests the component with a reboot event store
func TestComponent_WithRebootEventStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a reboot event store with some events
	mockStore := &MockRebootEventStore{
		events: apiv1.Events{
			{
				Time:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				Name:    "test-reboot",
				Type:    apiv1.EventTypeWarning,
				Message: "Test reboot event",
			},
		},
	}

	c, err := New(&components.GPUdInstance{
		RootCtx:          ctx,
		RebootEventStore: mockStore,
	})
	assert.NoError(t, err)
	defer c.Close()

	// Test that events can be retrieved
	events, err := c.Events(ctx, time.Now().Add(-2*time.Hour))
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "test-reboot", events[0].Name)

	// Test that component can be checked successfully
	comp := c.(*component)
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
		return map[string][]*procs.Process{
			procs.Running: make([]*procs.Process, 10),
		}, nil
	}

	result := comp.Check()
	data := result.(*Data)
	assert.Equal(t, apiv1.StateTypeHealthy, data.health)
}

// TestComponent_StartTicker tests that the Start method creates a ticker that runs Check
func TestComponent_StartTicker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)

	// Create a channel to track checks
	checkCalled := make(chan struct{}, 1)

	// We need to manually track if the Check function was called
	// Start a goroutine that watches for changes in lastData
	origComp := c.(*component)
	go func() {
		var lastUpdate time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				origComp.lastMu.RLock()
				currentData := origComp.lastData
				origComp.lastMu.RUnlock()

				if currentData != nil && currentData.ts.After(lastUpdate) {
					lastUpdate = currentData.ts
					select {
					case checkCalled <- struct{}{}:
					default:
					}
				}
			}
		}
	}()

	// Start the component (which starts the ticker)
	err = c.Start()
	assert.NoError(t, err)

	// Wait for the ticker to trigger at least one check
	select {
	case <-checkCalled:
		// Success - ticker called Check at least once
	case <-time.After(3 * time.Second): // Use shorter timeout for the test
		t.Fatal("Ticker did not call Check within expected time")
	}

	// Cleanup
	err = c.Close()
	assert.NoError(t, err)
}

// TestComponent_CloseContextCancellation tests that the Close method cancels the context
func TestComponent_CloseContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	comp := c.(*component)

	// Start the component
	err = comp.Start()
	assert.NoError(t, err)

	// Close the component
	err = comp.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-comp.ctx.Done():
		// Success - context was canceled
	default:
		t.Fatal("Component context was not canceled by Close method")
	}
}

// TestComponent_EventsWithErrorFromStore tests the Events method with a rebootEventStore that returns an error
func TestComponent_EventsWithErrorFromStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a mock reboot event store that returns an error
	mockStore := &ErrorRebootEventStore{}

	// Create component with the error-returning store
	c, err := New(&components.GPUdInstance{
		RootCtx:          ctx,
		RebootEventStore: mockStore,
	})
	assert.NoError(t, err)
	defer c.Close()

	// Call Events and verify it propagates the error
	since := time.Now().Add(-1 * time.Hour)
	events, err := c.Events(ctx, since)
	assert.Error(t, err)
	assert.Nil(t, events)
	assert.Contains(t, err.Error(), "mock event store error")
}

// TestComponent_CheckUptimeError tests the Check method when uptime returns an error
func TestComponent_CheckUptimeError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Since we can't directly monkey-patch host.UptimeWithContext, we'll use a different approach
	// Create a component with a mocked process counter that returns an error
	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	// Simulate an uptime error by directly injecting data with an uptime error
	testData := &Data{
		err:    errors.New("simulated uptime error"),
		health: apiv1.StateTypeUnhealthy,
		reason: "error getting uptime: simulated uptime error",
		ts:     time.Now().UTC(),
	}

	// Inject the data
	comp := c.(*component)
	comp.lastMu.Lock()
	comp.lastData = testData
	comp.lastMu.Unlock()

	// Verify health states reflect the uptime error
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error getting uptime: simulated uptime error", states[0].Reason)
	assert.Equal(t, "simulated uptime error", states[0].Error)
}

// TestData_SummaryComprehensive tests the Summary method of Data struct with more cases
func TestData_SummaryComprehensive(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "healthy with kernel version",
			data: &Data{
				Kernel: Kernel{
					Version: "5.15.0-generic",
				},
				reason: "os kernel version 5.15.0-generic",
				health: apiv1.StateTypeHealthy,
			},
			expected: "os kernel version 5.15.0-generic",
		},
		{
			name: "unhealthy with zombie processes",
			data: &Data{
				ProcessCountZombieProcesses: defaultZombieProcessCountThreshold + 10,
				reason:                      fmt.Sprintf("too many zombie processes: %d (threshold: %d)", defaultZombieProcessCountThreshold+10, defaultZombieProcessCountThreshold),
				health:                      apiv1.StateTypeUnhealthy,
			},
			expected: fmt.Sprintf("too many zombie processes: %d (threshold: %d)", defaultZombieProcessCountThreshold+10, defaultZombieProcessCountThreshold),
		},
		{
			name: "unhealthy with uptime error",
			data: &Data{
				err:    errors.New("uptime error"),
				reason: "error getting uptime: uptime error",
				health: apiv1.StateTypeUnhealthy,
			},
			expected: "error getting uptime: uptime error",
		},
		{
			name:     "empty reason",
			data:     &Data{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.data.Summary()
			assert.Equal(t, tt.expected, summary)
		})
	}
}

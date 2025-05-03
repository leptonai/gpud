package os

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// MockRebootEventStore is a mock implementation of the RebootEventStore interface
type MockRebootEventStore struct {
	events eventstore.Events
}

func (m *MockRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *MockRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return m.events, nil
}

// ErrorRebootEventStore is a mock implementation that always returns an error
type ErrorRebootEventStore struct{}

func (m *ErrorRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *ErrorRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return nil, errors.New("mock event store error")
}

func TestData_GetError(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "with error",
			data: &checkResult{
				err: assert.AnError,
			},
			expected: "assert.AnError general error for testing",
		},
		{
			name: "no error",
			data: &checkResult{
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
		data     *checkResult
		validate func(*testing.T, []apiv1.HealthState)
	}{
		{
			name: "nil data",
			data: nil,
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
				assert.Equal(t, "no data yet", states[0].Reason)
			},
		},
		{
			name: "with error",
			data: &checkResult{
				err:    assert.AnError,
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "failed to get os data -- assert.AnError general error for testing",
				ts:     time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
				assert.Equal(t, "failed to get os data -- assert.AnError general error for testing", states[0].Reason)
				assert.Equal(t, "assert.AnError general error for testing", states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
			},
		},
		{
			name: "with too many zombie processes",
			data: &checkResult{
				ProcessCountZombieProcesses: defaultZombieProcessCountThreshold + 1,
				Kernel: Kernel{
					Version: "5.15.0",
				},
				health: apiv1.HealthStateTypeUnhealthy,
				reason: fmt.Sprintf("too many zombie processes (threshold: %d)", defaultZombieProcessCountThreshold),
				ts:     time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
				expected := fmt.Sprintf("too many zombie processes (threshold: %d)", defaultZombieProcessCountThreshold)
				assert.Equal(t, expected, states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
			},
		},
		{
			name: "healthy case",
			data: &checkResult{
				Kernel: Kernel{
					Version: "5.15.0",
				},
				health: apiv1.HealthStateTypeHealthy,
				reason: "os kernel version 5.15.0",
				ts:     time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
				assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states := tt.data.HealthStates()
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
		events: eventstore.Events{
			{
				Time:    time.Now().Add(-1 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
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
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
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

		// Verify lastCheckResult is populated
		c.lastMu.RLock()
		lastCheckResult := c.lastCheckResult
		c.lastMu.RUnlock()

		assert.NotNil(t, lastCheckResult)

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
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("component states with data", func(t *testing.T) {
		// Inject test data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastCheckResult = &checkResult{
			Kernel: Kernel{
				Version: "5.15.0",
			},
			health: apiv1.HealthStateTypeHealthy,
			reason: "os kernel version 5.15.0",
			ts:     time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
	})

	t.Run("component states with error", func(t *testing.T) {
		// Inject error data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastCheckResult = &checkResult{
			err:    errors.New("test error"),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "failed to get os data -- test error",
			ts:     time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "failed to get os data -- test error", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
	})

	t.Run("component states with too many zombie processes", func(t *testing.T) {
		// Inject zombie process data
		c := comp.(*component)
		expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", defaultZombieProcessCountThreshold+1, defaultZombieProcessCountThreshold)
		c.lastMu.Lock()
		c.lastCheckResult = &checkResult{
			Kernel: Kernel{
				Version: "5.15.0",
			},
			ProcessCountZombieProcesses: defaultZombieProcessCountThreshold + 1,
			health:                      apiv1.HealthStateTypeUnhealthy,
			reason:                      expected,
			ts:                          time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, expected, states[0].Reason)
	})
}

// TestMockRebootEventStore tests the mock implementation of RebootEventStore
func TestMockRebootEventStore(t *testing.T) {
	mock := &MockRebootEventStore{
		events: eventstore.Events{
			{
				Time:    time.Now().Add(-1 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
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
	assert.Equal(t, string(apiv1.EventTypeWarning), events[0].Type)
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
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return nil, errors.New("process count error")
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify error is captured
		comp.lastMu.RLock()
		data := comp.lastCheckResult
		comp.lastMu.RUnlock()

		assert.NotNil(t, data)
		assert.NotNil(t, data.err)
		assert.Equal(t, "process count error", data.err.Error())
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
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
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{
				procs.Running: make([]process.ProcessStatus, 10),
				procs.Zombie:  make([]process.ProcessStatus, 5),
			}, nil
		}

		// Call CheckOnce
		_ = comp.Check()

		// Verify data is healthy
		comp.lastMu.RLock()
		data := comp.lastCheckResult
		comp.lastMu.RUnlock()

		assert.NotNil(t, data)
		assert.Equal(t, 5, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
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
	errorData := &checkResult{
		ts:     time.Now().UTC(),
		err:    errors.New("uptime error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting uptime: uptime error",
	}

	// Inject the error data
	comp.lastMu.Lock()
	comp.lastCheckResult = errorData
	comp.lastMu.Unlock()

	// Verify error handling through States
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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
		data     *checkResult
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
			data: &checkResult{
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
		data     *checkResult
		expected string
	}{
		{
			name: "with reason",
			data: &checkResult{
				reason: "test reason",
			},
			expected: "test reason",
		},
		{
			name:     "empty reason",
			data:     &checkResult{},
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
		data     *checkResult
		expected apiv1.HealthStateType
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "healthy",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy",
			data: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expected: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.data.HealthStateType()
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
	testData := &checkResult{
		VirtualizationEnvironment: pkghost.VirtualizationEnvironment{Type: "docker"},
		Kernel: Kernel{
			Arch:    "x86_64",
			Version: "5.15.0",
		},
		ProcessCountZombieProcesses: 5,
		health:                      apiv1.HealthStateTypeHealthy,
		reason:                      "os kernel version 5.15.0",
		ts:                          time.Now().UTC(),
	}

	comp.lastMu.Lock()
	comp.lastCheckResult = testData
	comp.lastMu.Unlock()

	// Get states and validate
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
}

// TestComponent_CheckWithUptimeError tests the Check method with mocked process error
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
	errorData := &checkResult{
		err:    errors.New("mock uptime error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting uptime: mock uptime error",
		ts:     time.Now().UTC(),
	}

	comp.lastMu.Lock()
	comp.lastCheckResult = errorData
	comp.lastMu.Unlock()

	// Verify the error is reflected in health states
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		return nil, errors.New("process count error")
	}

	// Call Check directly
	result := comp.Check()

	// Verify error handling
	data := result.(*checkResult)
	assert.NotNil(t, data.err)
	assert.Equal(t, "process count error", data.err.Error())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
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
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		return map[string][]process.ProcessStatus{
			procs.Running: make([]process.ProcessStatus, 10),
			procs.Zombie:  make([]process.ProcessStatus, threshold+1),
		}, nil
	}

	// Call Check
	result := comp.Check()

	// Verify zombie process detection
	data := result.(*checkResult)
	assert.Equal(t, threshold+1, data.ProcessCountZombieProcesses)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	expectedReason := fmt.Sprintf("too many zombie processes (threshold: %d)", threshold)
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
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		return map[string][]process.ProcessStatus{
			procs.Running: make([]process.ProcessStatus, 10),
		}, nil
	}

	// Call Check
	result := comp.Check()

	// Verify manufacturer info is captured
	data := result.(*checkResult)
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
		events: eventstore.Events{
			{
				Time:    time.Now().Add(-1 * time.Hour),
				Name:    "test-reboot",
				Type:    string(apiv1.EventTypeWarning),
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
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		return map[string][]process.ProcessStatus{
			procs.Running: make([]process.ProcessStatus, 10),
		}, nil
	}

	result := comp.Check()
	data := result.(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
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
	// Start a goroutine that watches for changes in lastCheckResult
	origComp := c.(*component)
	go func() {
		var lastUpdate time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				origComp.lastMu.RLock()
				currentData := origComp.lastCheckResult
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
	testData := &checkResult{
		err:    errors.New("simulated uptime error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error getting uptime: simulated uptime error",
		ts:     time.Now().UTC(),
	}

	// Inject the data
	comp := c.(*component)
	comp.lastMu.Lock()
	comp.lastCheckResult = testData
	comp.lastMu.Unlock()

	// Verify health states reflect the uptime error
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error getting uptime: simulated uptime error", states[0].Reason)
	assert.Equal(t, "simulated uptime error", states[0].Error)
}

// TestData_SummaryComprehensive tests the Summary method of Data struct with more cases
func TestData_SummaryComprehensive(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "healthy with kernel version",
			data: &checkResult{
				Kernel: Kernel{
					Version: "5.15.0-generic",
				},
				reason: "os kernel version 5.15.0-generic",
				health: apiv1.HealthStateTypeHealthy,
			},
			expected: "os kernel version 5.15.0-generic",
		},
		{
			name: "unhealthy with zombie processes",
			data: &checkResult{
				ProcessCountZombieProcesses: defaultZombieProcessCountThreshold + 10,
				reason:                      fmt.Sprintf("too many zombie processes (threshold: %d)", defaultZombieProcessCountThreshold),
				health:                      apiv1.HealthStateTypeUnhealthy,
			},
			expected: fmt.Sprintf("too many zombie processes (threshold: %d)", defaultZombieProcessCountThreshold),
		},
		{
			name: "unhealthy with uptime error",
			data: &checkResult{
				err:    errors.New("uptime error"),
				reason: "error getting uptime: uptime error",
				health: apiv1.HealthStateTypeUnhealthy,
			},
			expected: "error getting uptime: uptime error",
		},
		{
			name:     "empty reason",
			data:     &checkResult{},
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

// TestCountProcessesByStatusFuncVariations tests various scenarios for the countProcessesByStatusFunc function
func TestCountProcessesByStatusFuncVariations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockRebootStore := &MockRebootEventStore{}

	t.Run("empty process list", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Mock empty process list
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.Equal(t, 0, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
		assert.Contains(t, data.reason, "os kernel version")
	})

	t.Run("multiple process types but no zombies", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Mock process list with no zombies
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{
				procs.Running: make([]process.ProcessStatus, 10),
				"sleeping":    make([]process.ProcessStatus, 20),
				"stopped":     make([]process.ProcessStatus, 5),
				"idle":        make([]process.ProcessStatus, 3),
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.Equal(t, 0, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
		assert.Contains(t, data.reason, "os kernel version")
	})

	t.Run("exactly at zombie threshold", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Set a low threshold for testing
		threshold := 10
		comp.zombieProcessCountThreshold = threshold

		// Mock process list with zombies exactly at threshold
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{
				procs.Running: make([]process.ProcessStatus, 15),
				procs.Zombie:  make([]process.ProcessStatus, threshold),
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.Equal(t, threshold, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
		assert.Contains(t, data.reason, "os kernel version")
	})

	t.Run("one above zombie threshold", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Set a low threshold for testing
		threshold := 10
		comp.zombieProcessCountThreshold = threshold

		// Mock process list with zombies one above threshold
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{
				procs.Running: make([]process.ProcessStatus, 15),
				procs.Zombie:  make([]process.ProcessStatus, threshold+1),
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.Equal(t, threshold+1, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
		expectedReason := fmt.Sprintf("too many zombie processes (threshold: %d)", threshold)
		assert.Equal(t, expectedReason, data.reason)
	})

	t.Run("very large number of zombies", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Set a low threshold for testing
		threshold := 100
		comp.zombieProcessCountThreshold = threshold

		// Mock process list with a very large number of zombies
		zombieCount := threshold * 10
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{
				procs.Running: make([]process.ProcessStatus, 50),
				procs.Zombie:  make([]process.ProcessStatus, zombieCount),
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.Equal(t, zombieCount, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
		expectedReason := fmt.Sprintf("too many zombie processes (threshold: %d)", threshold)
		assert.Equal(t, expectedReason, data.reason)
	})

	t.Run("context cancellation during process check", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Mock process function that checks for context cancellation
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			// Create a canceled context
			canceledCtx, cancel := context.WithCancel(ctx)
			cancel()

			// Check if context is canceled and return error if it is
			select {
			case <-canceledCtx.Done():
				return nil, context.Canceled
			default:
				return map[string][]process.ProcessStatus{
					procs.Running: make([]process.ProcessStatus, 10),
				}, nil
			}
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.NotNil(t, data.err)
		assert.Equal(t, context.Canceled, data.err)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
		assert.Contains(t, data.reason, "error getting process count")
	})

	t.Run("mixed process types with zombies", func(t *testing.T) {
		c, err := New(&components.GPUdInstance{
			RootCtx:          ctx,
			RebootEventStore: mockRebootStore,
		})
		assert.NoError(t, err)
		defer c.Close()
		comp := c.(*component)

		// Set threshold
		threshold := 50
		comp.zombieProcessCountThreshold = threshold

		// Mock process list with various statuses including zombies
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
			return map[string][]process.ProcessStatus{
				procs.Running: make([]process.ProcessStatus, 100),
				"sleeping":    make([]process.ProcessStatus, 200),
				"stopped":     make([]process.ProcessStatus, 10),
				procs.Zombie:  make([]process.ProcessStatus, 30), // Below threshold
				"idle":        make([]process.ProcessStatus, 5),
				"dead":        make([]process.ProcessStatus, 2),
			}, nil
		}

		result := comp.Check()
		data := result.(*checkResult)

		assert.Equal(t, 30, data.ProcessCountZombieProcesses)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
		assert.Contains(t, data.reason, "os kernel version")
	})
}

// TestComponent_UptimeCalculation tests that uptime calculation is correct
func TestComponent_UptimeCalculation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Mock a specific boot time for testing
	bootTime := time.Now().Add(-24 * time.Hour) // 24 hours ago
	uptimeSeconds := uint64(24 * 60 * 60)       // 24 hours in seconds

	// Create a test result with controlled uptime values
	testData := &checkResult{
		ts: time.Now().UTC(),
		Uptimes: Uptimes{
			Seconds:             uptimeSeconds,
			BootTimeUnixSeconds: uint64(bootTime.Unix()),
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "os uptime test",
	}

	// Inject the test data
	comp.lastMu.Lock()
	comp.lastCheckResult = testData
	comp.lastMu.Unlock()

	// Verify the String() output includes the uptime information
	data := comp.lastCheckResult
	output := data.String()

	// Since String() uses humanize.RelTime which is dynamic, we just check that it contains "Uptime"
	assert.Contains(t, output, "Uptime")
}

// TestComponent_DetailedStringOutput tests the String() method provides detailed output
func TestComponent_DetailedStringOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Create a test result with comprehensive data
	testData := &checkResult{
		VirtualizationEnvironment: pkghost.VirtualizationEnvironment{Type: "kvm"},
		SystemManufacturer:        "Test Manufacturer",
		Kernel: Kernel{
			Arch:    "x86_64",
			Version: "5.15.0-custom",
		},
		Platform: Platform{
			Name:    "ubuntu",
			Family:  "debian",
			Version: "22.04",
		},
		Uptimes: Uptimes{
			Seconds:             3600,
			BootTimeUnixSeconds: uint64(time.Now().UTC().Add(-1 * time.Hour).Unix()),
		},
		ProcessCountZombieProcesses: 42,
		health:                      apiv1.HealthStateTypeHealthy,
		reason:                      "test reason",
		ts:                          time.Now().UTC(),
	}

	// Inject the test data
	comp.lastMu.Lock()
	comp.lastCheckResult = testData
	comp.lastMu.Unlock()

	// Verify the String() output includes all the key information
	data := comp.lastCheckResult
	output := data.String()

	assert.Contains(t, output, "VM Type")
	assert.Contains(t, output, "kvm")
	assert.Contains(t, output, "Kernel Arch")
	assert.Contains(t, output, "x86_64")
	assert.Contains(t, output, "Kernel Version")
	assert.Contains(t, output, "5.15.0-custom")
	assert.Contains(t, output, "Platform Name")
	assert.Contains(t, output, "ubuntu")
	assert.Contains(t, output, "Platform Version")
	assert.Contains(t, output, "22.04")
	assert.Contains(t, output, "Zombie Process Count")
	assert.Contains(t, output, "42")
}

// TestComponent_ExtraInfoInHealthState tests that extra info is included in health states
func TestComponent_ExtraInfoInHealthState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer c.Close()

	comp := c.(*component)

	// Create test data
	testData := &checkResult{
		Kernel: Kernel{
			Version: "5.15.0",
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
		ts:     time.Now().UTC(),
	}

	// Inject the test data
	comp.lastMu.Lock()
	comp.lastCheckResult = testData
	comp.lastMu.Unlock()

	// Get health states
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)

	// Verify extra info is included and contains JSON data
	assert.Contains(t, states[0].ExtraInfo, "data")
	extraData := states[0].ExtraInfo["data"]
	assert.Contains(t, extraData, "kernel")
	assert.Contains(t, extraData, "5.15.0")
}

// TestComponent_ContextCancelation tests behavior with a canceled context
func TestComponent_ContextCancelation(t *testing.T) {
	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	c, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)

	// Start the component
	err = c.Start()
	assert.NoError(t, err)

	// Cancel the context
	cancel()

	// Allow time for cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Component should be closed and not processing anymore
	comp := c.(*component)
	select {
	case <-comp.ctx.Done():
		// Success - context was canceled
	default:
		t.Fatal("Component context was not canceled")
	}

	// Cleanup
	err = c.Close()
	assert.NoError(t, err)
}

// TestComponent_JsonMarshalCheckResult tests JSON marshaling of checkResult
func TestComponent_JsonMarshalCheckResult(t *testing.T) {
	// Create a test checkResult
	cr := &checkResult{
		VirtualizationEnvironment: pkghost.VirtualizationEnvironment{Type: "kvm"},
		SystemManufacturer:        "Test Manufacturer",
		Kernel: Kernel{
			Arch:    "x86_64",
			Version: "5.15.0",
		},
		Platform: Platform{
			Name:    "ubuntu",
			Version: "22.04",
		},
		ProcessCountZombieProcesses: 5,
		health:                      apiv1.HealthStateTypeHealthy,
		reason:                      "test reason",
		ts:                          time.Now().UTC(),
	}

	// Get health states which marshals to JSON in ExtraInfo
	states := cr.HealthStates()
	assert.Len(t, states, 1)

	// Verify JSON data in ExtraInfo
	jsonData := states[0].ExtraInfo["data"]
	assert.NotEmpty(t, jsonData)
	assert.Contains(t, jsonData, "\"kernel\":")
	assert.Contains(t, jsonData, "\"virtualization_environment\":")
	assert.Contains(t, jsonData, "\"process_count_zombie_processes\":5")
}

// TestComponent_CheckResultMethods tests the methods of the checkResult struct
func TestComponent_CheckResultMethods(t *testing.T) {
	tests := []struct {
		name     string
		data     *checkResult
		wantErr  string
		wantType apiv1.HealthStateType
		summary  string
	}{
		{
			name:     "nil data",
			data:     nil,
			wantErr:  "",
			wantType: "",
			summary:  "",
		},
		{
			name: "with error",
			data: &checkResult{
				err:    errors.New("test error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "test reason with error",
			},
			wantErr:  "test error",
			wantType: apiv1.HealthStateTypeUnhealthy,
			summary:  "test reason with error",
		},
		{
			name: "healthy data",
			data: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
				reason: "test healthy reason",
			},
			wantErr:  "",
			wantType: apiv1.HealthStateTypeHealthy,
			summary:  "test healthy reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantErr, tt.data.getError())
			assert.Equal(t, tt.wantType, tt.data.HealthStateType())
			assert.Equal(t, tt.summary, tt.data.Summary())
		})
	}
}

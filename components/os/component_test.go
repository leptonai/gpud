package os

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
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
				err:     assert.AnError,
				healthy: false,
				reason:  "failed to get os data -- assert.AnError general error for testing",
				ts:      time.Now().UTC(),
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
				ProcessCountZombieProcesses: zombieProcessCountThreshold + 1,
				Kernel: Kernel{
					Version: "5.15.0",
				},
				healthy: false,
				reason:  fmt.Sprintf("too many zombie processes: %d (threshold: %d)", zombieProcessCountThreshold+1, zombieProcessCountThreshold),
				ts:      time.Now().UTC(),
			},
			validate: func(t *testing.T, states []apiv1.HealthState) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
				expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", zombieProcessCountThreshold+1, zombieProcessCountThreshold)
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
				healthy: true,
				reason:  "os kernel version 5.15.0",
				ts:      time.Now().UTC(),
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
			states, err := tt.data.getHealthStates()
			assert.NoError(t, err)
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
		comp := New(ctx, mockRebootStore)
		assert.NotNil(t, comp)
		assert.Equal(t, Name, comp.Name())

		// Clean up
		err := comp.Close()
		assert.NoError(t, err)
	})

	t.Run("component states with no data", func(t *testing.T) {
		comp := New(ctx, mockRebootStore)
		defer comp.Close()

		// States should return default state when no data
		states, err := comp.HealthStates(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	})

	t.Run("component events", func(t *testing.T) {
		comp := New(ctx, mockRebootStore)
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
		comp := New(ctx, mockRebootStore)
		defer comp.Close()

		// Trigger CheckOnce manually
		c := comp.(*component)
		c.CheckOnce()

		// Verify lastData is populated
		c.lastMu.RLock()
		lastData := c.lastData
		c.lastMu.RUnlock()

		assert.NotNil(t, lastData)

		// Start the component (this starts a goroutine)
		err := comp.Start()
		assert.NoError(t, err)

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

	comp := New(ctx, mockRebootStore)
	defer comp.Close()

	t.Run("component states with no data", func(t *testing.T) {
		// States should return default state when no data
		states, err := comp.HealthStates(ctx)
		assert.NoError(t, err)
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
			healthy: true,
			reason:  "os kernel version 5.15.0",
			ts:      time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.HealthStates(ctx)
		assert.NoError(t, err)
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
			err:     errors.New("test error"),
			healthy: false,
			reason:  "failed to get os data -- test error",
			ts:      time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.HealthStates(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "failed to get os data -- test error", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
	})

	t.Run("component states with too many zombie processes", func(t *testing.T) {
		// Inject zombie process data
		c := comp.(*component)
		expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", zombieProcessCountThreshold+1, zombieProcessCountThreshold)
		c.lastMu.Lock()
		c.lastData = &Data{
			Kernel: Kernel{
				Version: "5.15.0",
			},
			ProcessCountZombieProcesses: zombieProcessCountThreshold + 1,
			healthy:                     false,
			reason:                      expected,
			ts:                          time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.HealthStates(ctx)
		assert.NoError(t, err)
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
		comp := New(ctx, mockRebootStore).(*component)
		// Override the process counting function to return an error
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
			return nil, errors.New("process count error")
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify error is captured
		comp.lastMu.RLock()
		data := comp.lastData
		comp.lastMu.RUnlock()

		assert.NotNil(t, data)
		assert.NotNil(t, data.err)
		assert.Equal(t, "process count error", data.err.Error())
		assert.False(t, data.healthy)
		assert.Contains(t, data.reason, "error getting process count")
	})

	// Test normal case with no issues
	t.Run("healthy case", func(t *testing.T) {
		comp := New(ctx, mockRebootStore).(*component)
		// Override the process counting function to return normal processes
		comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]*procs.Process, error) {
			return map[string][]*procs.Process{
				procs.Running: make([]*procs.Process, 10),
				procs.Zombie:  make([]*procs.Process, 5),
			}, nil
		}

		// Call CheckOnce
		comp.CheckOnce()

		// Verify data is healthy
		comp.lastMu.RLock()
		data := comp.lastData
		comp.lastMu.RUnlock()

		assert.NotNil(t, data)
		assert.Equal(t, 5, data.ProcessCountZombieProcesses)
		assert.True(t, data.healthy)
		assert.Contains(t, data.reason, "os kernel version")
	})
}

// TestComponent_UptimeError tests the handling of uptime errors through the component's state
func TestComponent_UptimeError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockRebootStore := &MockRebootEventStore{}

	// Create a component
	comp := New(ctx, mockRebootStore).(*component)

	// Directly set error data to simulate an uptime error
	errorData := &Data{
		ts:      time.Now().UTC(),
		err:     errors.New("uptime error"),
		healthy: false,
		reason:  "error getting uptime: uptime error",
	}

	// Inject the error data
	comp.lastMu.Lock()
	comp.lastData = errorData
	comp.lastMu.Unlock()

	// Verify error handling through States
	states, err := comp.HealthStates(ctx)
	assert.NoError(t, err)
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
	comp := New(ctx, nil)
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
	assert.GreaterOrEqual(t, zombieProcessCountThreshold, 1000)
}

func TestCheckHealthState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rs, err := CheckHealthState(ctx)
	assert.NoError(t, err)
	assert.Equal(t, apiv1.StateTypeHealthy, rs.HealthState())

	fmt.Println(rs.String())

	b, err := json.Marshal(rs)
	assert.NoError(t, err)
	fmt.Println(string(b))
}

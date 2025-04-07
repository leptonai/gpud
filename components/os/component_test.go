package os

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// MockRebootEventStore is a mock implementation of the RebootEventStore interface
type MockRebootEventStore struct {
	events []components.Event
}

func (m *MockRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *MockRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) ([]components.Event, error) {
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

func TestData_GetReason(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "no os data",
		},
		{
			name: "error case",
			data: &Data{
				err: assert.AnError,
			},
			expected: "failed to get os data -- assert.AnError general error for testing",
		},
		{
			name: "too many zombie processes",
			data: &Data{
				ProcessCountZombieProcesses: zombieProcessCountThreshold + 1,
				Kernel: Kernel{
					Version: "5.15.0",
				},
			},
			expected: fmt.Sprintf("too many zombie processes: %d (threshold: %d)", zombieProcessCountThreshold+1, zombieProcessCountThreshold),
		},
		{
			name: "normal case",
			data: &Data{
				Kernel: Kernel{
					Version: "5.15.0",
				},
			},
			expected: "os kernel version 5.15.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.getReason()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestData_GetHealth(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		expectedHealth string
		expectedBool   bool
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedHealth: components.StateHealthy,
			expectedBool:   true,
		},
		{
			name: "error case",
			data: &Data{
				err: assert.AnError,
			},
			expectedHealth: components.StateUnhealthy,
			expectedBool:   false,
		},
		{
			name: "too many zombie processes",
			data: &Data{
				ProcessCountZombieProcesses: zombieProcessCountThreshold + 1,
				Kernel: Kernel{
					Version: "5.15.0",
				},
			},
			expectedHealth: components.StateUnhealthy,
			expectedBool:   false,
		},
		{
			name: "healthy case",
			data: &Data{
				Kernel: Kernel{
					Version: "5.15.0",
				},
			},
			expectedHealth: components.StateHealthy,
			expectedBool:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health, healthy := tt.data.getHealth()
			assert.Equal(t, tt.expectedHealth, health)
			assert.Equal(t, tt.expectedBool, healthy)
		})
	}
}

func TestData_GetStates(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		validate func(*testing.T, []components.State)
	}{
		{
			name: "nil data",
			data: nil,
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateHealthy, states[0].Health)
				assert.True(t, states[0].Healthy)
				assert.Equal(t, "no data yet", states[0].Reason)
			},
		},
		{
			name: "with error",
			data: &Data{
				err: assert.AnError,
				ts:  time.Now().UTC(),
			},
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateUnhealthy, states[0].Health)
				assert.False(t, states[0].Healthy)
				assert.Equal(t, "failed to get os data -- assert.AnError general error for testing", states[0].Reason)
				assert.Equal(t, "assert.AnError general error for testing", states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Equal(t, "json", states[0].ExtraInfo["encoding"])
			},
		},
		{
			name: "with too many zombie processes",
			data: &Data{
				ProcessCountZombieProcesses: zombieProcessCountThreshold + 1,
				Kernel: Kernel{
					Version: "5.15.0",
				},
				ts: time.Now().UTC(),
			},
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateUnhealthy, states[0].Health)
				assert.False(t, states[0].Healthy)
				expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", zombieProcessCountThreshold+1, zombieProcessCountThreshold)
				assert.Equal(t, expected, states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Equal(t, "json", states[0].ExtraInfo["encoding"])
			},
		},
		{
			name: "healthy case",
			data: &Data{
				Kernel: Kernel{
					Version: "5.15.0",
				},
				ts: time.Now().UTC(),
			},
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateHealthy, states[0].Health)
				assert.True(t, states[0].Healthy)
				assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Equal(t, "json", states[0].ExtraInfo["encoding"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getStates()
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
		events: []components.Event{
			{
				Time:    metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				Name:    "reboot",
				Type:    common.EventTypeWarning,
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
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
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
		assert.Equal(t, common.EventTypeWarning, events[0].Type)
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
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
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
			ts: time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
		assert.Equal(t, "os kernel version 5.15.0", states[0].Reason)
	})

	t.Run("component states with error", func(t *testing.T) {
		// Inject error data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastData = &Data{
			err: errors.New("test error"),
			ts:  time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateUnhealthy, states[0].Health)
		assert.False(t, states[0].Healthy)
		assert.Equal(t, "failed to get os data -- test error", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
	})

	t.Run("component states with too many zombie processes", func(t *testing.T) {
		// Inject zombie process data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastData = &Data{
			Kernel: Kernel{
				Version: "5.15.0",
			},
			ProcessCountZombieProcesses: zombieProcessCountThreshold + 1,
			ts:                          time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateUnhealthy, states[0].Health)
		assert.False(t, states[0].Healthy)
		expected := fmt.Sprintf("too many zombie processes: %d (threshold: %d)", zombieProcessCountThreshold+1, zombieProcessCountThreshold)
		assert.Equal(t, expected, states[0].Reason)
	})
}

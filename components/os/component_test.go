package os

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
			},
			expected: "too many zombie processes: 1001 (threshold: 1000)",
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
			name: "normal case",
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

func TestRecordRebootEvent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Create event store and bucket
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	// Test with a recent reboot time
	t.Run("recent reboot should record event", func(t *testing.T) {
		recentTime := time.Now().Add(-1 * time.Hour)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return recentTime, nil
		}

		err = recordRebootEvent(ctx, bucket, mockLastReboot)
		assert.NoError(t, err)

		// Check if the event was recorded
		events, err := bucket.Get(ctx, recentTime.Add(-1*time.Minute))
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, "reboot", events[0].Name)
		assert.Equal(t, common.EventTypeWarning, events[0].Type)
		assert.Equal(t, recentTime.Unix(), events[0].Time.Unix())
	})

	// Test with an old reboot time (beyond retention)
	t.Run("old reboot should not record event", func(t *testing.T) {
		oldTime := time.Now().Add(-2 * eventstore.DefaultRetention)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return oldTime, nil
		}

		err = recordRebootEvent(ctx, bucket, mockLastReboot)
		assert.NoError(t, err)

		// There should still only be 1 event (from the previous test)
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with error from lastReboot
	t.Run("error getting reboot time", func(t *testing.T) {
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return time.Time{}, errors.New("uptime command failed")
		}

		err = recordRebootEvent(ctx, bucket, mockLastReboot)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "uptime command failed")
	})

	// Test with duplicate event (same timestamp)
	t.Run("duplicate event should not be recorded", func(t *testing.T) {
		// Get the existing event
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		require.Len(t, events, 1)

		existingTime := events[0].Time.Time

		// Try to record with same timestamp
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return existingTime, nil
		}

		err = recordRebootEvent(ctx, bucket, mockLastReboot)
		assert.NoError(t, err)

		// Should still be only 1 event
		events, err = bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	// Test with close timestamp but not exact match
	t.Run("very close timestamps should not create duplicate", func(t *testing.T) {
		// Get the existing event
		events, err := bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		require.Len(t, events, 1)

		existingTime := events[0].Time.Time

		// Try with timestamp 30 seconds different (less than 1 minute threshold)
		closeTime := existingTime.Add(30 * time.Second)
		mockLastReboot := func(ctx context.Context) (time.Time, error) {
			return closeTime, nil
		}

		err = recordRebootEvent(ctx, bucket, mockLastReboot)
		assert.NoError(t, err)

		// Should still be only 1 event
		events, err = bucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})
}

func TestComponent(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	t.Run("component creation", func(t *testing.T) {
		comp, err := New(ctx, store)
		assert.NoError(t, err)
		assert.NotNil(t, comp)
		assert.Equal(t, Name, comp.Name())

		// Clean up
		err = comp.Close()
		assert.NoError(t, err)
	})

	t.Run("component states with no data", func(t *testing.T) {
		comp, err := New(ctx, store)
		require.NoError(t, err)
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
		comp, err := New(ctx, store)
		require.NoError(t, err)
		defer comp.Close()

		// Insert test event
		c := comp.(*component)
		testEvent := components.Event{
			Time:    metav1.Time{Time: time.Now()},
			Name:    "test_event",
			Type:    common.EventTypeInfo,
			Message: "Test event message",
		}
		err = c.eventBucket.Insert(ctx, testEvent)
		assert.NoError(t, err)

		// Get events
		since := time.Now().Add(-1 * time.Hour)
		events, err := comp.Events(ctx, since)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(events), 1)

		// Find our test event
		found := false
		for _, e := range events {
			if e.Name == "test_event" {
				found = true
				assert.Equal(t, common.EventTypeInfo, e.Type)
				assert.Equal(t, "Test event message", e.Message)
				break
			}
		}
		assert.True(t, found, "Test event should be found in events")
	})

	t.Run("component metrics", func(t *testing.T) {
		comp, err := New(ctx, store)
		require.NoError(t, err)
		defer comp.Close()

		metrics, err := comp.Metrics(ctx, time.Now().Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Nil(t, metrics) // OS component doesn't implement metrics yet
	})

	t.Run("component start and check once", func(t *testing.T) {
		comp, err := New(ctx, store)
		require.NoError(t, err)
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
		err = comp.Start()
		assert.NoError(t, err)

		// Allow time for at least one check
		time.Sleep(100 * time.Millisecond)
	})
}

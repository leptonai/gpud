package cpu

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
)

func TestDataGetReason(t *testing.T) {
	// Test with nil data
	var d *Data
	assert.Equal(t, "no cpu data found", d.getReason())

	// Test with valid data
	d = &Data{
		Info: Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "142",
			ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
		},
		Cores: Cores{
			Logical: 8,
		},
		Usage: Usage{
			UsedPercent:  "25.50",
			LoadAvg1Min:  "1.25",
			LoadAvg5Min:  "1.50",
			LoadAvg15Min: "1.75",
		},
	}
	assert.Contains(t, d.getReason(), "arch: x86_64")
	assert.Contains(t, d.getReason(), "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz")
}

func TestDataGetHealth(t *testing.T) {
	// Test with nil data
	var d *Data
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	d = &Data{
		err: fmt.Errorf("some error"),
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)

	// Test with valid data
	d = &Data{
		Info: Info{
			Arch:      "x86_64",
			ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
		},
		Cores: Cores{
			Logical: 8,
		},
		Usage: Usage{
			UsedPercent: "25.50",
		},
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		Info: Info{
			Arch:      "x86_64",
			ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
		},
		Cores: Cores{
			Logical: 8,
		},
		Usage: Usage{
			UsedPercent:  "25.50",
			LoadAvg1Min:  "1.25",
			LoadAvg5Min:  "1.50",
			LoadAvg15Min: "1.75",
		},
		ts: time.Now(),
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1) // Single state for the entire Data

	// Verify that the state name is correct
	assert.Equal(t, Name, states[0].Name)
	assert.Empty(t, states[0].Error, "Error should be empty for healthy state")
}

func TestNilDataGetStates(t *testing.T) {
	// Test with nil Data
	var d *Data
	states, err := d.getStates()

	assert.NoError(t, err)
	assert.Len(t, states, 1) // Should return a single state
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no cpu data found", states[0].Reason)
	assert.Empty(t, states[0].Error, "Error should be empty for nil data")
}

func TestDataJSONMarshaling(t *testing.T) {
	d := &Data{
		Info: Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "142",
			ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
		},
		Cores: Cores{
			Logical: 8,
		},
		Usage: Usage{
			UsedPercent:  "25.50",
			LoadAvg1Min:  "1.25",
			LoadAvg5Min:  "1.50",
			LoadAvg15Min: "1.75",
			usedPercent:  25.50, // This should not be marshaled
		},
		ts: time.Now(), // This should not be marshaled
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(d)
	assert.NoError(t, err)

	// Check JSON contains the expected fields
	jsonStr := string(jsonData)
	assert.Contains(t, jsonStr, `"arch":"x86_64"`)
	assert.Contains(t, jsonStr, `"cpu":"0"`)
	assert.Contains(t, jsonStr, `"family":"6"`)
	assert.Contains(t, jsonStr, `"model":"142"`)
	assert.Contains(t, jsonStr, `"model_name":"Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz"`)
	assert.Contains(t, jsonStr, `"logical":8`)
	assert.Contains(t, jsonStr, `"used_percent":"25.50"`)
	assert.Contains(t, jsonStr, `"load_avg_1min":"1.25"`)
	assert.Contains(t, jsonStr, `"load_avg_5min":"1.50"`)
	assert.Contains(t, jsonStr, `"load_avg_15min":"1.75"`)

	// Check that the private fields are not marshaled
	assert.NotContains(t, jsonStr, `usedPercent`)
	assert.NotContains(t, jsonStr, `ts`)
	assert.NotContains(t, jsonStr, `err`)

	// Unmarshal back to a new Data struct
	var newData Data
	err = json.Unmarshal(jsonData, &newData)
	assert.NoError(t, err)

	// Check the values were correctly unmarshaled
	assert.Equal(t, d.Info.Arch, newData.Info.Arch)
	assert.Equal(t, d.Info.CPU, newData.Info.CPU)
	assert.Equal(t, d.Info.Family, newData.Info.Family)
	assert.Equal(t, d.Info.Model, newData.Info.Model)
	assert.Equal(t, d.Info.ModelName, newData.Info.ModelName)
	assert.Equal(t, d.Cores.Logical, newData.Cores.Logical)
	assert.Equal(t, d.Usage.UsedPercent, newData.Usage.UsedPercent)
	assert.Equal(t, d.Usage.LoadAvg1Min, newData.Usage.LoadAvg1Min)
	assert.Equal(t, d.Usage.LoadAvg5Min, newData.Usage.LoadAvg5Min)
	assert.Equal(t, d.Usage.LoadAvg15Min, newData.Usage.LoadAvg15Min)

	// Check private fields weren't unmarshaled
	assert.Zero(t, newData.Usage.usedPercent)
	assert.True(t, newData.ts.IsZero())
}

func TestComponentEvents(t *testing.T) {
	// Create a mock event bucket
	now := time.Now()
	mockEvents := []components.Event{
		{
			Time:    metav1.Time{Time: now.Add(-time.Hour)},
			Name:    "cpu_event",
			Type:    common.EventTypeWarning,
			Message: "Test CPU event 1",
		},
		{
			Time:    metav1.Time{Time: now.Add(-30 * time.Minute)},
			Name:    "cpu_event",
			Type:    common.EventTypeInfo,
			Message: "Test CPU event 2",
		},
	}

	// Create a mock bucket that satisfies the eventstore.Bucket interface
	mockBucket := &mockEventBucket{events: mockEvents}

	// Create a test component with the mock event bucket
	comp := &component{
		eventBucket: mockBucket,
	}

	// Call Events method with a time from 2 hours ago
	ctx := context.Background()
	since := now.Add(-2 * time.Hour)
	events, err := comp.Events(ctx, since)

	// Verify results
	assert.NoError(t, err)
	assert.Equal(t, mockEvents, events)
}

// mockEventBucket implements the eventstore.Bucket interface for testing
type mockEventBucket struct {
	events []components.Event
}

func (m *mockEventBucket) Name() string {
	return "mock"
}

func (m *mockEventBucket) Insert(ctx context.Context, event components.Event) error {
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, event components.Event) (*components.Event, error) {
	return nil, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	return m.events, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*components.Event, error) {
	return nil, nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}

func (m *mockEventBucket) Close() {
	// No-op
}

func TestDataWithErrorGetStates(t *testing.T) {
	// Test with Data with error
	d := &Data{
		err: fmt.Errorf("test error"),
	}
	states, err := d.getStates()

	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Unhealthy", states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "failed to get CPU data")
	assert.Equal(t, "test error", states[0].Error)
}

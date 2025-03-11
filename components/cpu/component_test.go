package cpu

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInfoGetReason(t *testing.T) {
	// Test with nil info
	var i *Info
	assert.Equal(t, "no cpu info found", i.getReason())

	// Test with error
	i = &Info{err: assert.AnError}
	assert.Contains(t, i.getReason(), "failed to get CPU information")

	// Test with valid info
	i = &Info{
		Arch:      "x86_64",
		CPU:       "0",
		Family:    "6",
		Model:     "142",
		ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
	}
	assert.Contains(t, i.getReason(), "x86_64")
	assert.Contains(t, i.getReason(), "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz")
}

func TestInfoGetHealth(t *testing.T) {
	// Test with nil info
	var i *Info
	health, healthy := i.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	i = &Info{err: assert.AnError}
	health, healthy = i.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)

	// Test with valid info
	i = &Info{
		Arch:      "x86_64",
		ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
	}
	health, healthy = i.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

func TestCoresGetReason(t *testing.T) {
	// Test with nil cores
	var c *Cores
	assert.Equal(t, "no cpu cores found", c.getReason())

	// Test with error
	c = &Cores{err: assert.AnError}
	assert.Contains(t, c.getReason(), "failed to get CPU cores")

	// Test with valid cores
	c = &Cores{
		Physical: 4,
		Logical:  8,
	}
	assert.Contains(t, c.getReason(), "physical: 4 cores")
	assert.Contains(t, c.getReason(), "logical: 8 cores")
}

func TestCoresGetHealth(t *testing.T) {
	// Test with nil cores
	var c *Cores
	health, healthy := c.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	c = &Cores{err: assert.AnError}
	health, healthy = c.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)

	// Test with valid cores
	c = &Cores{
		Physical: 4,
		Logical:  8,
	}
	health, healthy = c.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

func TestUsageGetReason(t *testing.T) {
	// Test with nil usage
	var u *Usage
	assert.Equal(t, "no cpu usage found", u.getReason())

	// Test with error
	u = &Usage{err: assert.AnError}
	assert.Contains(t, u.getReason(), "failed to get CPU usage")

	// Test with valid usage
	u = &Usage{
		UsedPercent:  "25.50",
		LoadAvg1Min:  "1.25",
		LoadAvg5Min:  "1.50",
		LoadAvg15Min: "1.75",
	}
	assert.Contains(t, u.getReason(), "25.50%")
	assert.Contains(t, u.getReason(), "1.25")
	assert.Contains(t, u.getReason(), "1.50")
	assert.Contains(t, u.getReason(), "1.75")
}

func TestUsageGetHealth(t *testing.T) {
	// Test with nil usage
	var u *Usage
	health, healthy := u.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	u = &Usage{err: assert.AnError}
	health, healthy = u.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)

	// Test with normal load
	u = &Usage{
		UsedPercent:  "50.00",
		LoadAvg1Min:  "1.25",
		LoadAvg5Min:  "1.50",
		LoadAvg15Min: "1.75",
	}
	health, healthy = u.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		Info: &Info{
			Arch:      "x86_64",
			ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
		},
		Cores: &Cores{
			Physical: 4,
			Logical:  8,
		},
		Usage: &Usage{
			UsedPercent:  "25.50",
			LoadAvg1Min:  "1.25",
			LoadAvg5Min:  "1.50",
			LoadAvg15Min: "1.75",
		},
		ts: time.Now(),
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 3) // Info, Cores, Usage states

	// Verify that the state names are correct
	stateNames := []string{}
	for _, state := range states {
		stateNames = append(stateNames, state.Name)
	}
	assert.Contains(t, stateNames, "info")
	assert.Contains(t, stateNames, "cores")
	assert.Contains(t, stateNames, "usage")
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
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataWithNilFieldsGetStates(t *testing.T) {
	// Test with nil Info
	t.Run("nil Info", func(t *testing.T) {
		d := &Data{
			Info:  nil,
			Cores: &Cores{Physical: 4, Logical: 8},
			Usage: &Usage{UsedPercent: "25.50"},
			ts:    time.Now(),
		}
		states, err := d.getStates()

		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// Find the info state
		var infoState = findStateByName(states, "info")
		assert.NotNil(t, infoState)
		assert.Equal(t, "no cpu info found", infoState.Reason)
		assert.Equal(t, "Healthy", infoState.Health)
		assert.True(t, infoState.Healthy)
	})

	// Test with nil Cores
	t.Run("nil Cores", func(t *testing.T) {
		d := &Data{
			Info:  &Info{Arch: "x86_64"},
			Cores: nil,
			Usage: &Usage{UsedPercent: "25.50"},
			ts:    time.Now(),
		}
		states, err := d.getStates()

		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// Find the cores state
		var coresState = findStateByName(states, "cores")
		assert.NotNil(t, coresState)
		assert.Equal(t, "no cpu cores found", coresState.Reason)
		assert.Equal(t, "Healthy", coresState.Health)
		assert.True(t, coresState.Healthy)
	})

	// Test with nil Usage
	t.Run("nil Usage", func(t *testing.T) {
		d := &Data{
			Info:  &Info{Arch: "x86_64"},
			Cores: &Cores{Physical: 4, Logical: 8},
			Usage: nil,
			ts:    time.Now(),
		}
		states, err := d.getStates()

		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// Find the usage state
		var usageState = findStateByName(states, "usage")
		assert.NotNil(t, usageState)
		assert.Equal(t, "no cpu usage found", usageState.Reason)
		assert.Equal(t, "Healthy", usageState.Health)
		assert.True(t, usageState.Healthy)
	})
}

func TestDataWithErrorFieldsGetStates(t *testing.T) {
	// Test with Info with error
	t.Run("Info with error", func(t *testing.T) {
		d := &Data{
			Info:  &Info{err: fmt.Errorf("info error")},
			Cores: &Cores{Physical: 4, Logical: 8},
			Usage: &Usage{UsedPercent: "25.50"},
			ts:    time.Now(),
		}
		states, err := d.getStates()

		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// Find the info state
		var infoState = findStateByName(states, "info")
		assert.NotNil(t, infoState)
		assert.Contains(t, infoState.Reason, "failed to get CPU information")
		assert.Equal(t, "Unhealthy", infoState.Health)
		assert.False(t, infoState.Healthy)
	})

	// Test with Cores with error
	t.Run("Cores with error", func(t *testing.T) {
		d := &Data{
			Info:  &Info{Arch: "x86_64"},
			Cores: &Cores{err: fmt.Errorf("cores error")},
			Usage: &Usage{UsedPercent: "25.50"},
			ts:    time.Now(),
		}
		states, err := d.getStates()

		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// Find the cores state
		var coresState = findStateByName(states, "cores")
		assert.NotNil(t, coresState)
		assert.Contains(t, coresState.Reason, "failed to get CPU cores")
		assert.Equal(t, "Unhealthy", coresState.Health)
		assert.False(t, coresState.Healthy)
	})

	// Test with Usage with error
	t.Run("Usage with error", func(t *testing.T) {
		d := &Data{
			Info:  &Info{Arch: "x86_64"},
			Cores: &Cores{Physical: 4, Logical: 8},
			Usage: &Usage{err: fmt.Errorf("usage error")},
			ts:    time.Now(),
		}
		states, err := d.getStates()

		assert.NoError(t, err)
		assert.Len(t, states, 3)

		// Find the usage state
		var usageState = findStateByName(states, "usage")
		assert.NotNil(t, usageState)
		assert.Contains(t, usageState.Reason, "failed to get CPU usage")
		assert.Equal(t, "Unhealthy", usageState.Health)
		assert.False(t, usageState.Healthy)
	})
}

func TestDataJSONMarshaling(t *testing.T) {
	d := &Data{
		Info: &Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "142",
			ModelName: "Intel(R) Core(TM) i7-8565U CPU @ 1.80GHz",
		},
		Cores: &Cores{
			Physical: 4,
			Logical:  8,
		},
		Usage: &Usage{
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
	assert.Contains(t, jsonStr, `"physical":4`)
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
	assert.Equal(t, d.Cores.Physical, newData.Cores.Physical)
	assert.Equal(t, d.Cores.Logical, newData.Cores.Logical)
	assert.Equal(t, d.Usage.UsedPercent, newData.Usage.UsedPercent)
	assert.Equal(t, d.Usage.LoadAvg1Min, newData.Usage.LoadAvg1Min)
	assert.Equal(t, d.Usage.LoadAvg5Min, newData.Usage.LoadAvg5Min)
	assert.Equal(t, d.Usage.LoadAvg15Min, newData.Usage.LoadAvg15Min)

	// Check private fields weren't unmarshaled
	assert.Zero(t, newData.Usage.usedPercent)
	assert.True(t, newData.ts.IsZero())
	assert.Nil(t, newData.Info.err)
	assert.Nil(t, newData.Cores.err)
	assert.Nil(t, newData.Usage.err)
}

// Helper function to find a state by name
func findStateByName(states []components.State, name string) *components.State {
	for i := range states {
		if states[i].Name == name {
			return &states[i]
		}
	}
	return nil
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

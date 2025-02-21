package infiniband

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name        string
		output      *infiniband.IbstatOutput
		config      infiniband.ExpectedPortStates
		wantReason  string
		wantHealthy bool
		wantErr     bool
	}{
		{
			name:   "thresholds not set",
			output: &infiniband.IbstatOutput{},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			wantReason:  msgThresholdNotSetSkipped,
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "healthy state with matching ports and rate",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason:  msgNoIbIssueFound,
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "unhealthy state - not enough ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason:  "not enough LinkUp ports, only 1 LinkUp out of 1, expected at least 2 ports and 200 Gb/sec rate; some ports must be missing",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "unhealthy state - rate too low",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason:  "not enough LinkUp ports, only 0 LinkUp out of 2, expected at least 2 ports and 200 Gb/sec rate; some ports must be missing",
			wantHealthy: false,
			wantErr:     false,
		},
		{
			name: "unhealthy state - disabled ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason:  "not enough LinkUp ports, only 0 LinkUp out of 2, expected at least 2 ports and 200 Gb/sec rate; some ports might be down, 2 Disabled devices with Rate > 200 found (mlx5_0, mlx5_1)",
			wantHealthy: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, healthy, err := evaluate(tt.output, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealthy, healthy)
		})
	}
}

func TestDefaultExpectedPortStates(t *testing.T) {
	// Test default values
	defaults := GetDefaultExpectedPortStates()
	assert.Equal(t, 0, defaults.AtLeastPorts)
	assert.Equal(t, 0, defaults.AtLeastRate)

	// Test setting new values
	newStates := infiniband.ExpectedPortStates{
		AtLeastPorts: 2,
		AtLeastRate:  200,
	}
	SetDefaultExpectedPortStates(newStates)

	updated := GetDefaultExpectedPortStates()
	assert.Equal(t, newStates.AtLeastPorts, updated.AtLeastPorts)
	assert.Equal(t, newStates.AtLeastRate, updated.AtLeastRate)
}

func TestEvaluateWithTestData(t *testing.T) {
	// Read the test data file
	testDataPath := filepath.Join("testdata", "ibstat.47.0.h100.all.active.1")
	content, err := os.ReadFile(testDataPath)
	require.NoError(t, err, "Failed to read test data file")

	// Parse the test data
	cards, err := infiniband.ParseIBStat(string(content))
	require.NoError(t, err, "Failed to parse ibstat output")

	output := &infiniband.IbstatOutput{
		Raw:    string(content),
		Parsed: cards,
	}

	tests := []struct {
		name        string
		config      infiniband.ExpectedPortStates
		wantReason  string
		wantHealthy bool
		wantErr     bool
	}{
		{
			name: "healthy state - all H100 ports active at 400Gb/s",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 8,   // Number of 400Gb/s ports in the test data
				AtLeastRate:  400, // Expected rate for H100 cards
			},
			wantReason:  msgNoIbIssueFound,
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "healthy state - mixed rate ports",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports in test data
				AtLeastRate:  100, // Minimum rate that includes all ports
			},
			wantReason:  msgNoIbIssueFound,
			wantHealthy: true,
			wantErr:     false,
		},
		{
			name: "unhealthy state - not enough high-rate ports",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports
				AtLeastRate:  400, // Only 8 ports have this rate
			},
			wantReason:  "not enough LinkUp ports, only 8 LinkUp out of 12, expected at least 12 ports and 400 Gb/sec rate; some ports must be missing",
			wantHealthy: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, healthy, err := evaluate(output, tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealthy, healthy)
		})
	}
}

func TestComponentStatesWithTestData(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
	require.NoError(t, err)
	defer eventsStore.Close()

	c := &component{
		rootCtx:     ctx,
		cancel:      cancel,
		eventsStore: eventsStore,
		toolOverwrites: nvidia_common.ToolOverwrites{
			IbstatCommand: "cat " + filepath.Join("testdata", "ibstat.47.0.h100.all.active.1"),
		},
	}

	now := time.Now().UTC()
	states, err := c.getStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 8,   // Number of 400Gb/s ports in the test data
		AtLeastRate:  400, // Expected rate for H100 cards
	})
	require.NoError(t, err)
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, "ibstat", state.Name)
	assert.True(t, state.Healthy)
	assert.Equal(t, components.StateHealthy, state.Health)
	assert.Equal(t, msgNoIbIssueFound, state.Reason)
	assert.Nil(t, state.SuggestedActions)

	// Test with different thresholds that should result in unhealthy state
	lastEvent, err := c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12, // More ports than available
		AtLeastRate:  400,
	})
	require.NoError(t, err)
	require.NotNil(t, lastEvent)

	states, err = c.getStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12, // More ports than available
		AtLeastRate:  400,
	})
	require.NoError(t, err)
	require.Len(t, states, 1)

	state = states[0]
	assert.Equal(t, "ibstat", state.Name)
	assert.False(t, state.Healthy)
	assert.Equal(t, components.StateUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "not enough LinkUp ports")
	assert.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []common.RepairActionType{common.RepairActionTypeHardwareInspection}, state.SuggestedActions.RepairActions)
}

func TestComponentGetStatesWithThresholds(t *testing.T) {
	tests := []struct {
		name       string
		thresholds infiniband.ExpectedPortStates
		wantState  components.State
		wantErr    bool
	}{
		{
			name: "thresholds not set - should skip check",
			thresholds: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			wantState: components.State{
				Name:    "ibstat",
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  msgThresholdNotSetSkipped,
			},
			wantErr: false,
		},
		{
			name: "thresholds set but no ibstat command",
			thresholds: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			},
			wantState: components.State{
				Name:    "ibstat",
				Health:  components.StateUnhealthy,
				Healthy: false,
				Reason:  "ibstat threshold set but ibstat not found",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
			require.NoError(t, err)
			defer eventsStore.Close()

			c := &component{
				rootCtx:     ctx,
				cancel:      cancel,
				eventsStore: eventsStore,
			}

			now := time.Now().UTC()
			states, err := c.getStates(ctx, now, tt.thresholds)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, states, 1)
			assert.Equal(t, tt.wantState.Name, states[0].Name)
			assert.Equal(t, tt.wantState.Health, states[0].Health)
			assert.Equal(t, tt.wantState.Healthy, states[0].Healthy)
			assert.Contains(t, states[0].Reason, tt.wantState.Reason)
		})
	}
}

func TestComponentStatesNoIbstatCommand(t *testing.T) {
	testCases := []struct {
		name          string
		ibstatCommand string
		wantReason    string
	}{
		{
			name:          "empty command",
			ibstatCommand: "",
			wantReason:    "ibstat threshold set but ibstat not found",
		},
		{
			name:          "non-existent command",
			ibstatCommand: "/non/existent/path/to/ibstat",
			wantReason:    "ibstat threshold set but ibstat not found",
		},
		{
			name:          "invalid command",
			ibstatCommand: "invalid_command",
			wantReason:    "ibstat threshold set but ibstat not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
			require.NoError(t, err)
			defer eventsStore.Close()

			c := &component{
				rootCtx:     ctx,
				cancel:      cancel,
				eventsStore: eventsStore,
				toolOverwrites: nvidia_common.ToolOverwrites{
					IbstatCommand: tc.ibstatCommand,
				},
			}

			now := time.Now().UTC()
			lastEvent, err := c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			})
			if err != nil {
				t.Fatalf("failed to check ibstat once: %v", err)
			}
			require.NotNil(t, lastEvent)

			states, err := c.getStates(ctx, now, infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			})

			require.NoError(t, err)
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, "ibstat", state.Name)
			assert.Equal(t, components.StateUnhealthy, state.Health)
			assert.False(t, state.Healthy)
			assert.Contains(t, state.Reason, tc.wantReason)
		})
	}
}

func TestCheckIbstatOnce(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
	require.NoError(t, err)
	defer eventsStore.Close()

	c := &component{
		rootCtx:        ctx,
		cancel:         cancel,
		eventsStore:    eventsStore,
		toolOverwrites: nvidia_common.ToolOverwrites{},
	}

	now := time.Now().UTC()

	// Test case 1: No thresholds set
	lastEvent, err := c.checkIbstatOnce(now, infiniband.ExpectedPortStates{})
	assert.NoError(t, err, "should not error when no thresholds are set")
	assert.Nil(t, lastEvent)

	// Test case 2: With thresholds but no ibstat command
	lastEvent, err = c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err, "should not error when ibstat command is not found")
	assert.NotNil(t, lastEvent)

	// Test case 3: With mock ibstat command returning healthy state
	c.toolOverwrites.IbstatCommand = "cat testdata/ibstat.47.0.h100.all.active.1"
	lastEvent, err = c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	})
	assert.NoError(t, err, "should not error with healthy state")
	assert.NotNil(t, lastEvent)

	// Test case 4: With mock ibstat command returning unhealthy state
	lastEvent, err = c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12,
		AtLeastRate:  400,
	})
	assert.NoError(t, err, "should not error with unhealthy state")
	assert.NotNil(t, lastEvent)

	// Test case 5: Duplicate event should not be inserted
	lastEvent, err = c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12,
		AtLeastRate:  400,
	})
	assert.NoError(t, err, "should not error with duplicate event")
	assert.NotNil(t, lastEvent)

	// Test case 6: Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	c.rootCtx = canceledCtx
	lastEvent, err = c.checkIbstatOnce(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.Error(t, err, "should error with canceled context")
	assert.Nil(t, lastEvent)
}

func TestGetStates(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
	require.NoError(t, err)
	defer eventsStore.Close()

	c := &component{
		rootCtx:        ctx,
		cancel:         cancel,
		eventsStore:    eventsStore,
		toolOverwrites: nvidia_common.ToolOverwrites{},
	}

	now := time.Now().UTC()

	// Test case 1: No thresholds set
	states, err := c.getStates(ctx, now, infiniband.ExpectedPortStates{})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "ibstat", states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, msgThresholdNotSetSkipped, states[0].Reason)

	// Test case 2: Empty events store with thresholds
	states, err = c.getStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "ibstat", states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "ibstat threshold set but ibstat not found")

	// Test case 3: With an event in store
	testEvent := components.Event{
		Time:    metav1.Time{Time: now},
		Name:    "ibstat",
		Type:    common.EventTypeWarning,
		Message: "test message",
		ExtraInfo: map[string]string{
			"state_healthy": "false",
			"state_health":  components.StateUnhealthy,
		},
		SuggestedActions: &common.SuggestedActions{
			RepairActions: []common.RepairActionType{
				common.RepairActionTypeHardwareInspection,
			},
			Descriptions: []string{
				"test action",
			},
		},
	}
	err = c.eventsStore.Insert(ctx, testEvent)
	assert.NoError(t, err)

	// Set lastEvent to make sure we get the unhealthy state
	c.lastEventMu.Lock()
	c.lastEvent = &testEvent
	c.lastEventMu.Unlock()

	states, err = c.getStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "ibstat", states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, testEvent.Message, states[0].Reason)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, testEvent.SuggestedActions.RepairActions, states[0].SuggestedActions.RepairActions)
	assert.Equal(t, testEvent.SuggestedActions.Descriptions, states[0].SuggestedActions.Descriptions)

	// Test case 4: With recent event (within 10 seconds)
	recentEvent := components.Event{
		Time:    metav1.Time{Time: now.Add(-5 * time.Second)},
		Name:    "ibstat",
		Type:    common.EventTypeWarning,
		Message: "recent test message",
		ExtraInfo: map[string]string{
			"state_healthy": "false",
			"state_health":  components.StateUnhealthy,
		},
	}
	c.lastEventMu.Lock()
	c.lastEvent = &recentEvent
	c.lastEventMu.Unlock()

	states, err = c.getStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, recentEvent.Message, states[0].Reason)

	// Test case 5: With canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel context immediately

	cNew := &component{
		rootCtx:        canceledCtx,
		cancel:         func() {}, // Empty cancel func since we already canceled
		eventsStore:    eventsStore,
		toolOverwrites: nvidia_common.ToolOverwrites{},
	}

	_, err = cNew.getStates(canceledCtx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")

	// Test case 6: With mock ibstat command
	c.toolOverwrites.IbstatCommand = "cat testdata/ibstat.47.0.h100.all.active.1"
	states, err = c.getStates(ctx, now.Add(20*time.Second), infiniband.ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, msgNoIbIssueFound, states[0].Reason)

	// Test case 7: With invalid ibstat command
	c.toolOverwrites.IbstatCommand = "invalid_command"
	states, err = c.getStates(ctx, now.Add(50*time.Second), infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "ibstat threshold set but ibstat not found")
}

// Add test for Events method
func TestEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
	require.NoError(t, err)
	defer eventsStore.Close()

	c := &component{
		rootCtx:     ctx,
		cancel:      cancel,
		eventsStore: eventsStore,
		toolOverwrites: nvidia_common.ToolOverwrites{
			IbstatCommand: "cat testdata/ibstat.47.0.h100.all.active.1",
		},
	}

	now := time.Now().UTC()

	// Test with no events
	events, err := c.Events(ctx, now.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)

	// Test with an event
	testEvent := components.Event{
		Time:    metav1.Time{Time: now},
		Name:    "ibstat",
		Type:    common.EventTypeWarning,
		Message: "test message",
	}
	err = c.eventsStore.Insert(ctx, testEvent)
	assert.NoError(t, err)

	events, err = c.Events(ctx, now.Add(-1*time.Hour))
	assert.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, testEvent.Message, events[0].Message)

	// Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	_, err = c.Events(canceledCtx, now)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

// Add test for Metrics method
func TestMetrics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := &component{}

	metrics, err := c.Metrics(ctx, time.Now())
	assert.NoError(t, err)
	assert.Empty(t, metrics)
}

// Add test for Close method
func TestClose(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx := context.Background()
	eventsStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName("test_ibstat"), 3*24*time.Hour)
	require.NoError(t, err)

	c := &component{
		rootCtx:     ctx,
		cancel:      func() {},
		eventsStore: eventsStore,
	}

	err = c.Close()
	assert.NoError(t, err)
}

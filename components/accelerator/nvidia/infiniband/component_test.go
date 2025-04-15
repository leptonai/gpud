package infiniband

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
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
			wantReason:  "only 1 ports (>= 200 Gb/s) are active, expect at least 2",
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
			wantReason:  "only 0 ports (>= 200 Gb/s) are active, expect at least 2",
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
			wantReason:  "only 0 ports (>= 200 Gb/s) are active, expect at least 2; 2 device(s) found Disabled (mlx5_0, mlx5_1)",
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
			wantReason:  "only 8 ports (>= 400 Gb/s) are active, expect at least 12",
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

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	c := &component{
		rootCtx:     ctx,
		cancel:      cancel,
		eventBucket: bucket,
		toolOverwrites: nvidia_common.ToolOverwrites{
			IbstatCommand: "cat " + filepath.Join("testdata", "ibstat.47.0.h100.all.active.1"),
		},
	}

	now := time.Now().UTC()
	states, err := c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 8,   // Number of 400Gb/s ports in the test data
		AtLeastRate:  400, // Expected rate for H100 cards
	})
	require.NoError(t, err)
	require.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, "ibstat", state.Name)
	assert.Equal(t, apiv1.StateTypeHealthy, state.Health)
	assert.Equal(t, msgNoIbIssueFound, state.Reason)
	assert.Nil(t, state.SuggestedActions)

	// Test with different thresholds that should result in unhealthy state
	lastEvent, err := c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12, // More ports than available
		AtLeastRate:  400,
	})
	require.NoError(t, err)
	require.NotNil(t, lastEvent)

	states, err = c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12, // More ports than available
		AtLeastRate:  400,
	})
	require.NoError(t, err)
	require.Len(t, states, 1)

	state = states[0]
	assert.Equal(t, "ibstat", state.Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "only 8 ports (>= 400 Gb/s) are active, expect at least 12")
	assert.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, state.SuggestedActions.RepairActions)
}

func TestComponentGetStatesWithThresholds(t *testing.T) {
	tests := []struct {
		name       string
		thresholds infiniband.ExpectedPortStates
		wantState  apiv1.HealthState
		wantErr    bool
	}{
		{
			name: "thresholds not set - should skip check",
			thresholds: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			wantState: apiv1.HealthState{
				Name:   "ibstat",
				Health: apiv1.StateTypeHealthy,
				Reason: msgThresholdNotSetSkipped,
			},
			wantErr: false,
		},
		{
			name: "thresholds set but no ibstat command",
			thresholds: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			},
			wantState: apiv1.HealthState{
				Name:   "ibstat",
				Health: apiv1.StateTypeUnhealthy,
				Reason: "ibstat threshold set but ibstat not found",
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

			store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
			assert.NoError(t, err)
			bucket, err := store.Bucket("test_events")
			assert.NoError(t, err)
			defer bucket.Close()

			c := &component{
				rootCtx:     ctx,
				cancel:      cancel,
				eventBucket: bucket,
			}

			now := time.Now().UTC()
			states, err := c.getHealthStates(ctx, now, tt.thresholds)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, states, 1)
			assert.Equal(t, tt.wantState.Name, states[0].Name)
			assert.Equal(t, tt.wantState.Health, states[0].Health)
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

			store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
			assert.NoError(t, err)
			bucket, err := store.Bucket("test_events")
			assert.NoError(t, err)
			defer bucket.Close()

			c := &component{
				rootCtx:     ctx,
				cancel:      cancel,
				eventBucket: bucket,
				toolOverwrites: nvidia_common.ToolOverwrites{
					IbstatCommand: tc.ibstatCommand,
				},
			}

			now := time.Now().UTC()
			lastEvent, err := c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			})
			if err != nil {
				t.Fatalf("failed to check ibstat once: %v", err)
			}
			require.NotNil(t, lastEvent)

			states, err := c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			})

			require.NoError(t, err)
			require.Len(t, states, 1)

			state := states[0]
			assert.Equal(t, "ibstat", state.Name)
			assert.Equal(t, apiv1.StateTypeUnhealthy, state.Health)
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

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	c := &component{
		rootCtx:        ctx,
		cancel:         cancel,
		eventBucket:    bucket,
		toolOverwrites: nvidia_common.ToolOverwrites{},
	}

	now := time.Now().UTC()

	// Test case 1: No thresholds set
	lastEvent, err := c.checkOnceIbstat(now, infiniband.ExpectedPortStates{})
	assert.NoError(t, err, "should not error when no thresholds are set")
	assert.Nil(t, lastEvent)

	// Test case 2: With thresholds but no ibstat command
	lastEvent, err = c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err, "should not error when ibstat command is not found")
	assert.NotNil(t, lastEvent)

	// Test case 3: With mock ibstat command returning healthy state
	c.toolOverwrites.IbstatCommand = "cat testdata/ibstat.47.0.h100.all.active.1"
	lastEvent, err = c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	})
	assert.NoError(t, err, "should not error with healthy state")
	assert.NotNil(t, lastEvent)

	// Test case 4: With mock ibstat command returning unhealthy state
	lastEvent, err = c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12,
		AtLeastRate:  400,
	})
	assert.NoError(t, err, "should not error with unhealthy state")
	assert.NotNil(t, lastEvent)

	// Test case 5: Duplicate event should not be inserted
	lastEvent, err = c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
		AtLeastPorts: 12,
		AtLeastRate:  400,
	})
	assert.NoError(t, err, "should not error with duplicate event")
	assert.NotNil(t, lastEvent)

	// Test case 6: Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	c.rootCtx = canceledCtx
	lastEvent, err = c.checkOnceIbstat(now, infiniband.ExpectedPortStates{
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

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	c := &component{
		rootCtx:        ctx,
		cancel:         cancel,
		eventBucket:    bucket,
		toolOverwrites: nvidia_common.ToolOverwrites{},
	}

	now := time.Now().UTC()

	// Test case 1: No thresholds set
	states, err := c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "ibstat", states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, msgThresholdNotSetSkipped, states[0].Reason)

	// Test case 2: Empty events store with thresholds
	states, err = c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "ibstat", states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "ibstat threshold set but ibstat not found")

	// Test case 3: With an event in store
	testEvent := apiv1.Event{
		Time:    metav1.Time{Time: now},
		Name:    "ibstat",
		Type:    apiv1.EventTypeWarning,
		Message: "test message",
		DeprecatedExtraInfo: map[string]string{
			"state_healthy": "false",
			"state_health":  string(apiv1.StateTypeUnhealthy),
		},
		DeprecatedSuggestedActions: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
			DeprecatedDescriptions: []string{
				"test action",
			},
		},
	}
	err = c.eventBucket.Insert(ctx, testEvent)
	assert.NoError(t, err)

	// Set lastEvent to make sure we get the unhealthy state
	c.lastEventMu.Lock()
	c.lastEvent = &testEvent
	c.lastEventMu.Unlock()

	states, err = c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "ibstat", states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, testEvent.Message, states[0].Reason)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, testEvent.DeprecatedSuggestedActions.RepairActions, states[0].SuggestedActions.RepairActions)
	assert.Equal(t, testEvent.DeprecatedSuggestedActions.DeprecatedDescriptions, states[0].SuggestedActions.DeprecatedDescriptions)

	// Test case 4: With recent event (within 10 seconds)
	recentEvent := apiv1.Event{
		Time:    metav1.Time{Time: now.Add(-5 * time.Second)},
		Name:    "ibstat",
		Type:    apiv1.EventTypeWarning,
		Message: "recent test message",
		DeprecatedExtraInfo: map[string]string{
			"state_healthy": "false",
			"state_health":  string(apiv1.StateTypeUnhealthy),
		},
	}
	c.lastEventMu.Lock()
	c.lastEvent = &recentEvent
	c.lastEventMu.Unlock()

	states, err = c.getHealthStates(ctx, now, infiniband.ExpectedPortStates{
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
		eventBucket:    bucket,
		toolOverwrites: nvidia_common.ToolOverwrites{},
	}

	_, err = cNew.getHealthStates(canceledCtx, now, infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")

	// Test case 6: With mock ibstat command
	c.toolOverwrites.IbstatCommand = "cat testdata/ibstat.47.0.h100.all.active.1"
	states, err = c.getHealthStates(ctx, now.Add(20*time.Second), infiniband.ExpectedPortStates{
		AtLeastPorts: 8,
		AtLeastRate:  400,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, msgNoIbIssueFound, states[0].Reason)

	// Test case 7: With invalid ibstat command
	c.toolOverwrites.IbstatCommand = "invalid_command"
	states, err = c.getHealthStates(ctx, now.Add(50*time.Second), infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	})
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "ibstat threshold set but ibstat not found")
}

// Add test for Events method
func TestEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	c := &component{
		rootCtx:     ctx,
		cancel:      cancel,
		eventBucket: bucket,
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
	testEvent := apiv1.Event{
		Time:    metav1.Time{Time: now},
		Name:    "ibstat",
		Type:    apiv1.EventTypeWarning,
		Message: "test message",
	}
	err = c.eventBucket.Insert(ctx, testEvent)
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

// Add test for Close method
func TestClose(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx := context.Background()
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	kmsgSyncer, err := kmsg.NewSyncer(ctx, func(line string) (string, string) {
		return line, "test message"
	}, bucket)
	assert.NoError(t, err)

	c := &component{
		rootCtx:     ctx,
		cancel:      func() {},
		kmsgSyncer:  kmsgSyncer,
		eventBucket: bucket,
	}

	err = c.Close()
	assert.NoError(t, err)
}

// MockEventBucket implements the events_db.Store interface for testing
type MockEventBucket struct {
	events apiv1.Events
	mu     sync.Mutex
}

func NewMockEventBucket() *MockEventBucket {
	return &MockEventBucket{
		events: apiv1.Events{},
	}
}

func (m *MockEventBucket) Name() string {
	return "mock"
}

func (m *MockEventBucket) Insert(ctx context.Context, event apiv1.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result apiv1.Events
	for _, event := range m.events {
		if !event.Time.Time.Before(since) {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *MockEventBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.events {
		if e.Name == event.Name && e.Type == event.Type && e.Message == event.Message {
			return &m.events[i], nil
		}
	}
	return nil, nil
}

func (m *MockEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.events) == 0 {
		return nil, nil
	}

	latest := m.events[0]
	for _, e := range m.events[1:] {
		if e.Time.Time.After(latest.Time.Time) {
			latest = e
		}
	}
	return &latest, nil
}

func (m *MockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var newEvents apiv1.Events
	var purgedCount int

	for _, event := range m.events {
		if event.Time.Time.Unix() >= beforeTimestamp {
			newEvents = append(newEvents, event)
		} else {
			purgedCount++
		}
	}

	m.events = newEvents
	return purgedCount, nil
}

func (m *MockEventBucket) Close() {
	// No-op for mock
}

func (m *MockEventBucket) GetEvents() apiv1.Events {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(apiv1.Events, len(m.events))
	copy(result, m.events)
	return result
}

// TestLogLineProcessor tests the Match function with sample log lines
func TestLogLineProcessor(t *testing.T) {
	t.Parallel()

	// Test direct matching of log lines
	tests := []struct {
		name         string
		logLine      string
		expectMatch  bool
		expectedName string
		expectedMsg  string
	}{
		{
			name:         "PCI power insufficient",
			logLine:      "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W).",
			expectMatch:  true,
			expectedName: "pci_power_insufficient",
			expectedMsg:  "Insufficient power on MLX5 PCIe slot",
		},
		{
			name:         "Port module high temperature",
			logLine:      "mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature",
			expectMatch:  true,
			expectedName: "port_module_high_temperature",
			expectedMsg:  "Overheated MLX5 adapter",
		},
		{
			name:         "No match",
			logLine:      "Some unrelated log line",
			expectMatch:  false,
			expectedName: "",
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, msg := Match(tt.logLine)
			if tt.expectMatch {
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedMsg, msg)
			} else {
				assert.Empty(t, name)
				assert.Empty(t, msg)
			}
		})
	}

	// Test with events store
	mockStore := NewMockEventBucket()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a direct log processor test using our processor function
	// We'll test the match function integration with the mock store
	now := time.Now().UTC()

	// Create events using the Match function
	for _, tt := range tests {
		if tt.expectMatch {
			eventName, eventMessage := Match(tt.logLine)
			event := apiv1.Event{
				Time:                metav1.Time{Time: now},
				Name:                eventName,
				Type:                apiv1.EventTypeWarning,
				Message:             eventMessage,
				DeprecatedExtraInfo: map[string]string{"log_line": tt.logLine},
			}
			err := mockStore.Insert(ctx, event)
			require.NoError(t, err)
		}
	}

	// Verify events were properly stored
	events := mockStore.GetEvents()
	matchingTests := 0
	for _, tt := range tests {
		if tt.expectMatch {
			matchingTests++
		}
	}
	require.Len(t, events, matchingTests)

	for _, event := range events {
		assert.Equal(t, apiv1.EventTypeWarning, event.Type)
		assert.NotEmpty(t, event.Name)
		assert.NotEmpty(t, event.Message)
		assert.Contains(t, event.DeprecatedExtraInfo, "log_line")
	}
}

// TestNewWithLogLineProcessor tests the New function with focus on the log line processor
func TestNewWithLogLineProcessor(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test successful creation
	comp, err := New(ctx, store, nvidia_common.ToolOverwrites{})
	require.NoError(t, err)
	defer comp.Close()

	c, ok := comp.(*component)
	require.True(t, ok)
	require.NotNil(t, c.kmsgSyncer)
}

// TestIntegrationWithLogLineProcessor tests that the component can process kmsg events
func TestIntegrationWithLogLineProcessor(t *testing.T) {
	// We are not using t.Parallel() here because we need to mock some global functions

	mockEventBucket := NewMockEventBucket()

	// Create a component with mock store
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp := &component{
		rootCtx:     ctx,
		cancel:      cancel,
		eventBucket: mockEventBucket,
	}

	// Directly test the Match function on a sample log line
	logLine := "mlx5_core 0000:5c:00.0: mlx5_pcie_event:299:(pid 268269): Detected insufficient power on the PCIe slot (27W)."
	eventName, eventMessage := Match(logLine)
	assert.Equal(t, "pci_power_insufficient", eventName)
	assert.Equal(t, "Insufficient power on MLX5 PCIe slot", eventMessage)

	// Manually create an event and insert it
	now := time.Now().UTC()
	event := apiv1.Event{
		Time:    metav1.Time{Time: now},
		Name:    eventName,
		Type:    apiv1.EventTypeWarning,
		Message: eventMessage,
		DeprecatedExtraInfo: map[string]string{
			"log_line": logLine,
		},
	}
	err := mockEventBucket.Insert(ctx, event)
	require.NoError(t, err)

	// Now test the Events method and verify our event exists in the results
	// Note: Events() may also generate an additional "ibstat" event
	events, err := comp.Events(ctx, now.Add(-10*time.Second))
	require.NoError(t, err)

	// Find our manually created event in the results
	var foundManualEvent bool
	for _, e := range events {
		if e.Name == eventName && e.Message == eventMessage {
			foundManualEvent = true
			break
		}
	}
	assert.True(t, foundManualEvent, "Our manually created event should be in the results")

	// Now add another event with a different timestamp and verify filtering works
	olderEvent := apiv1.Event{
		Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
		Name:    "port_module_high_temperature",
		Type:    apiv1.EventTypeWarning,
		Message: "Overheated MLX5 adapter",
		DeprecatedExtraInfo: map[string]string{
			"log_line": "mlx5_port_module_event:1131:(pid 0): Port module event[error]: module 0, Cable error, High Temperature",
		},
	}
	err = mockEventBucket.Insert(ctx, olderEvent)
	require.NoError(t, err)

	// Test filtering by time
	recentEvents, err := comp.Events(ctx, now.Add(-30*time.Second))
	require.NoError(t, err)

	// Instead of checking exact count, verify that our recent event is included
	var foundRecentEvent bool
	for _, e := range recentEvents {
		if e.Name == eventName && e.Message == eventMessage {
			foundRecentEvent = true
			break
		}
	}
	assert.True(t, foundRecentEvent, "Our recent event should be in the filtered results")

	// Get all events
	allEvents, err := comp.Events(ctx, now.Add(-2*time.Minute))
	require.NoError(t, err)

	// Verify both our manually created events are in the results
	var foundManualEvents int
	for _, e := range allEvents {
		if (e.Name == eventName && e.Message == eventMessage) ||
			(e.Name == "port_module_high_temperature" && e.Message == "Overheated MLX5 adapter") {
			foundManualEvents++
		}
	}
	assert.Equal(t, 2, foundManualEvents, "Both our manually created events should be in the results")
}

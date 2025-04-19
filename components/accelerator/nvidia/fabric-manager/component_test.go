package fabricmanager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)

	bucket, err := store.Bucket(Name)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := newWatcher([][]string{
		{"tail", "testdata/fabricmanager.log"},
		{"sleep 1"},
	})
	require.NoError(t, err)
	llp := newLogLineProcessor(ctx, w, Match, bucket)

	comp := &component{
		ctx:    ctx,
		cancel: cancel,

		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },

		eventBucket:      bucket,
		logLineProcessor: llp,
	}
	defer comp.Close()

	_ = comp.Check()

	time.Sleep(5 * time.Second)

	events, err := comp.Events(ctx, time.Time{})
	require.NoError(t, err)
	assert.Len(t, events, 1)

	expectedEvent := apiv1.Event{
		Time:    metav1.Time{Time: time.Date(2025, 2, 27, 15, 10, 2, 0, time.UTC)},
		Name:    "fabricmanager_nvswitch_non_fatal_error",
		Type:    "Warning",
		Message: "NVSwitch non-fatal error detected",
		DeprecatedExtraInfo: map[string]string{
			"log_line": "[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
		},
	}

	assert.Equal(t, expectedEvent.Name, events[0].Name)
	assert.Equal(t, expectedEvent.Type, events[0].Type)
	assert.Equal(t, expectedEvent.Message, events[0].Message)
	assert.Equal(t, expectedEvent.DeprecatedExtraInfo["log_line"], events[0].DeprecatedExtraInfo["log_line"])

	comp.checkFMExistsFunc = func() bool { return false }
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "fabric manager found and active", states[0].Reason)
}

// mockWatcher implements the watcher interface for testing
type mockWatcher struct {
	ch chan logLine
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{
		ch: make(chan logLine),
	}
}

func (w *mockWatcher) watch() <-chan logLine {
	return w.ch
}

func (w *mockWatcher) close() {
	close(w.ch)
}

// mockMatchFunc implements the matchFunc interface for testing
func mockMatchFunc(line string) (eventName string, message string) {
	if line == "test-error-line" {
		return "test-error", "This is a test error"
	}
	return "", ""
}

func TestEventsWithNoProcessor(t *testing.T) {
	t.Parallel()

	// Create a component with no logLineProcessor
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		checkFMExistsFunc: func() bool { return false },
		checkFMActiveFunc: func() bool { return false },
	}

	// Call Events
	events, err := comp.Events(context.Background(), time.Now().Add(-1*time.Hour))

	// Verify results
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestEventsWithProcessor(t *testing.T) {
	t.Parallel()

	// Setup SQLite database
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a mock watcher
	mockW := newMockWatcher()

	// Create events store
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)

	// Create a processor
	llp := newLogLineProcessor(ctx, mockW, mockMatchFunc, bucket)

	// Create component with processor
	comp := &component{
		ctx:    ctx,
		cancel: cancel,

		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },

		eventBucket:      bucket,
		logLineProcessor: llp,
	}

	// Insert a test event directly into the store
	testEvent := apiv1.Event{
		Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
		Name:    "test-error",
		Message: "This is a test error",
		Type:    "Warning",
		DeprecatedExtraInfo: map[string]string{
			"log_line": "test-error-line",
		},
	}
	err = bucket.Insert(ctx, testEvent)
	require.NoError(t, err)

	// Call Events
	events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))

	// Verify results
	assert.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 1)
	assert.Equal(t, "test-error", events[0].Name)
	assert.Equal(t, "This is a test error", events[0].Message)
}

func TestStatesWhenFabricManagerDoesNotExist(t *testing.T) {
	t.Parallel()

	// Create a component where fabric manager doesn't exist
	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		checkFMExistsFunc: func() bool { return false },
		checkFMActiveFunc: func() bool { return false },
	}

	_ = comp.Check()

	// Call States
	states := comp.LastHealthStates()

	// Verify results
	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "nv-fabricmanager executable not found", states[0].Reason)
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	comp := &component{}
	assert.Equal(t, Name, comp.Name())
}

func TestComponentStart(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	comp := &component{
		ctx:               ctx,
		cancel:            cancel,
		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },
	}
	defer comp.Close()

	err := comp.Start()
	assert.NoError(t, err)

	// Allow time for the goroutine to do first check
	time.Sleep(100 * time.Millisecond)

	// Verify lastData was updated
	comp.lastMu.RLock()
	assert.NotNil(t, comp.lastData)
	comp.lastMu.RUnlock()
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	// Setup mock components
	ctx, cancel := context.WithCancel(context.Background())
	mockW := newMockWatcher()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)

	llp := newLogLineProcessor(ctx, mockW, mockMatchFunc, bucket)

	comp := &component{
		ctx:              ctx,
		cancel:           cancel,
		logLineProcessor: llp,
		eventBucket:      bucket,
	}

	// Test Close
	err = comp.Close()
	assert.NoError(t, err)
}

func TestStatesWhenFabricManagerExistsButNotActive(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return false },
	}

	_ = comp.Check()

	states := comp.LastHealthStates()

	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "nv-fabricmanager found but fabric manager service is not active", states[0].Reason)
}

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test nil Data
	var d *Data
	assert.Equal(t, "", d.getError())

	// Test nil error
	d = &Data{}
	assert.Equal(t, "", d.getError())

	// Test with error
	testErr := assert.AnError
	d = &Data{err: testErr}
	assert.Equal(t, testErr.Error(), d.getError())
}

func TestDataGetLastHealthStates(t *testing.T) {
	t.Parallel()

	// Test nil Data
	var d *Data
	states := d.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test unhealthy state
	d = &Data{
		health: apiv1.StateTypeUnhealthy,
		reason: "test unhealthy reason",
		err:    assert.AnError,
	}
	states = d.getLastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test unhealthy reason", states[0].Reason)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Test creating component with nil eventstore
	instance := &components.GPUdInstance{
		RootCtx: context.Background(),
	}
	comp, err := New(instance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

func TestDataString(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	assert.Equal(t, "", d.String())

	// Test with active fabric manager
	d = &Data{
		FabricManagerActive: true,
	}
	assert.Equal(t, "fabric manager is active", d.String())

	// Test with inactive fabric manager
	d = &Data{
		FabricManagerActive: false,
	}
	assert.Equal(t, "fabric manager is not active", d.String())
}

func TestDataSummary(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	assert.Equal(t, "", d.Summary())

	// Test with reason
	d = &Data{
		reason: "test reason",
	}
	assert.Equal(t, "test reason", d.Summary())
}

func TestDataHealthState(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	assert.Equal(t, apiv1.HealthStateType(""), d.HealthState())

	// Test with health state
	d = &Data{
		health: apiv1.StateTypeHealthy,
	}
	assert.Equal(t, apiv1.StateTypeHealthy, d.HealthState())
}

func TestStatesWhenFabricManagerExistsAndActive(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},

		checkFMExistsFunc: func() bool { return true },
		checkFMActiveFunc: func() bool { return true },
	}

	result := comp.Check()
	assert.NotNil(t, result)

	// Type assertion to access Data methods
	data, ok := result.(*Data)
	assert.True(t, ok)
	assert.True(t, data.FabricManagerActive)
	assert.Equal(t, apiv1.StateTypeHealthy, data.health)
	assert.Equal(t, "fabric manager found and active", data.reason)

	states := comp.LastHealthStates()
	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "fabric manager found and active", states[0].Reason)
}

// This test mocks checkFMExists and checkFMActive to test all branches in Check method
func TestCheckAllBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		fmExists       bool
		fmActive       bool
		expectedData   *Data
		expectedState  apiv1.HealthStateType
		expectedReason string
	}{
		{
			name:           "FM doesn't exist",
			fmExists:       false,
			fmActive:       false,
			expectedState:  apiv1.StateTypeHealthy,
			expectedReason: "nv-fabricmanager executable not found",
		},
		{
			name:           "FM exists but not active",
			fmExists:       true,
			fmActive:       false,
			expectedState:  apiv1.StateTypeUnhealthy,
			expectedReason: "nv-fabricmanager found but fabric manager service is not active",
		},
		{
			name:           "FM exists and active",
			fmExists:       true,
			fmActive:       true,
			expectedState:  apiv1.StateTypeHealthy,
			expectedReason: "fabric manager found and active",
		},
	}

	for _, tc := range tests {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			comp := &component{
				ctx:               context.Background(),
				cancel:            func() {},
				checkFMExistsFunc: func() bool { return tc.fmExists },
				checkFMActiveFunc: func() bool { return tc.fmActive },
			}

			result := comp.Check()
			data, ok := result.(*Data)
			assert.True(t, ok)

			if tc.fmExists && tc.fmActive {
				assert.True(t, data.FabricManagerActive)
			} else {
				assert.False(t, data.FabricManagerActive)
			}

			assert.Equal(t, tc.expectedState, data.health)
			assert.Equal(t, tc.expectedReason, data.reason)
		})
	}
}

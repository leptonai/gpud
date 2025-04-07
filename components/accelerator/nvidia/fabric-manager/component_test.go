package fabricmanager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	comp.CheckOnce()

	time.Sleep(5 * time.Second)

	events, err := comp.Events(ctx, time.Time{})
	require.NoError(t, err)
	assert.Len(t, events, 1)

	expectedEvent := components.Event{
		Time:    metav1.Time{Time: time.Date(2025, 2, 27, 15, 10, 2, 0, time.UTC)},
		Name:    "fabricmanager_nvswitch_non_fatal_error",
		Type:    "Warning",
		Message: "NVSwitch non-fatal error detected",
		ExtraInfo: map[string]string{
			"log_line": "[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
		},
	}

	assert.Equal(t, expectedEvent.Name, events[0].Name)
	assert.Equal(t, expectedEvent.Type, events[0].Type)
	assert.Equal(t, expectedEvent.Message, events[0].Message)
	assert.Equal(t, expectedEvent.ExtraInfo["log_line"], events[0].ExtraInfo["log_line"])

	comp.checkFMExistsFunc = func() bool { return false }
	states, err := comp.States(ctx)
	require.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
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
	testEvent := components.Event{
		Time:    metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
		Name:    "test-error",
		Message: "This is a test error",
		Type:    "Warning",
		ExtraInfo: map[string]string{
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

	comp.CheckOnce()

	// Call States
	states, err := comp.States(context.Background())

	// Verify results
	assert.NoError(t, err)
	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
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

	comp.CheckOnce()

	states, err := comp.States(context.Background())

	assert.NoError(t, err)
	require.NotNil(t, states)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
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

func TestDataGetStates(t *testing.T) {
	t.Parallel()

	// Test nil Data
	var d *Data
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test unhealthy state
	d = &Data{
		healthy: false,
		reason:  "test unhealthy reason",
		err:     assert.AnError,
	}
	states, err = d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, "test unhealthy reason", states[0].Reason)
	assert.Equal(t, assert.AnError.Error(), states[0].Error)
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	comp := &component{
		ctx:    context.Background(),
		cancel: func() {},
	}

	metrics, err := comp.Metrics(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, metrics)
}

package hwslowdown

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	"github.com/leptonai/gpud/pkg/common"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentWithNoPoller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defaultPoller := nvidia_query.GetDefaultPoller()
	_, err := New(ctx, nvidia_common.Config{}, nil)

	if defaultPoller != nil {
		// expects no error
		assert.NoError(t, err)
	} else {
		// expects error
		assert.Equal(t, err, nvidia_query.ErrDefaultPollerNotSet)
	}
}

func TestComponentStates(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	testCases := []struct {
		name               string
		window             time.Duration
		thresholdPerMinute float64
		insertedEvent      []components.Event
		expectedStates     int
		expectHealthy      bool
	}{
		{
			name:               "single event within window",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-5 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "multiple events within window but below threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-5 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "events above threshold",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "events above threshold with multiple GPUs",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				// GPU 0-3 events at -4 minutes
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-3",
					},
				},
				// GPU 0-3 events at -3 minutes
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-3",
					},
				},
				// GPU 0-3 events at -2 minutes
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-3",
					},
				},
				// GPU 0-3 events at -1 minutes
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-3",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "events outside window",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []components.Event{
				{
					Time:    metav1.Time{Time: now.Add(-10 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: now.Add(-8 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "no events",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent:      []components.Event{},
			expectedStates:     1,
			expectHealthy:      true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
			assert.NoError(t, err)
			bucket, err := store.Bucket("test_events")
			assert.NoError(t, err)
			defer bucket.Close()

			if len(tc.insertedEvent) > 0 {
				for _, event := range tc.insertedEvent {
					err := bucket.Insert(ctx, event)
					assert.NoError(t, err)
				}
			}

			c := &component{
				stateHWSlowdownEvaluationWindow:                  tc.window,
				stateHWSlowdownEventsThresholdFrequencyPerMinute: tc.thresholdPerMinute,
				eventBucket: bucket,
			}

			states, err := c.States(ctx)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStates, len(states))

			if len(states) > 0 {
				assert.Equal(t, tc.expectHealthy, states[0].Healthy)
			}
		})
	}
}

func TestComponentRegisterCollectors(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	reg := prometheus.NewRegistry()
	c := &component{}

	err := c.RegisterCollectors(reg, dbRW, dbRO, "test_metrics")
	assert.NoError(t, err)
	assert.Equal(t, reg, c.gatherer)
}

func TestComponentStatesEdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		window             time.Duration
		thresholdPerMinute float64
		setupStore         func(bucket eventstore.Bucket, ctx context.Context) error
		expectError        bool
		expectedStates     int
		expectHealthy      bool
	}{
		{
			name:               "zero evaluation window",
			window:             0,
			thresholdPerMinute: 0.6,
			setupStore:         func(bucket eventstore.Bucket, ctx context.Context) error { return nil },
			expectError:        false,
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "negative evaluation window",
			window:             -10 * time.Minute,
			thresholdPerMinute: 0.6,
			setupStore:         func(bucket eventstore.Bucket, ctx context.Context) error { return nil },
			expectError:        false,
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "zero threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: 0,
			setupStore: func(bucket eventstore.Bucket, ctx context.Context) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				}
				return bucket.Insert(ctx, event)
			},
			expectError:    false,
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "negative threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: -0.6,
			setupStore: func(bucket eventstore.Bucket, ctx context.Context) error {
				event := components.Event{
					Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"gpu_uuid": "gpu-0",
					},
				}
				return bucket.Insert(ctx, event)
			},
			expectError:    false,
			expectedStates: 1,
			expectHealthy:  false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
			assert.NoError(t, err)
			bucket, err := store.Bucket("test_events")
			assert.NoError(t, err)
			defer bucket.Close()

			err = tc.setupStore(bucket, ctx)
			assert.NoError(t, err)

			c := &component{
				stateHWSlowdownEvaluationWindow:                  tc.window,
				stateHWSlowdownEventsThresholdFrequencyPerMinute: tc.thresholdPerMinute,
				eventBucket: bucket,
			}

			states, err := c.States(ctx)
			if tc.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStates, len(states))
			if len(states) > 0 {
				assert.Equal(t, tc.expectHealthy, states[0].Healthy)
			}
		})
	}
}

func TestComponentName(t *testing.T) {
	t.Parallel()
	c := &component{}
	assert.Equal(t, nvidia_hw_slowdown_id.Name, c.Name())
}

func TestComponentStart(t *testing.T) {
	t.Parallel()
	c := &component{}
	assert.NoError(t, c.Start())
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	// Insert test events
	testEvents := []components.Event{
		{
			Time:    metav1.Time{Time: time.Now().UTC().Add(-5 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-0",
			},
		},
		{
			Time:    metav1.Time{Time: time.Now().UTC().Add(-3 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid": "gpu-1",
			},
		},
	}

	for _, event := range testEvents {
		err := bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	c := &component{
		eventBucket: bucket,
	}

	// Test getting events since a specific time
	since := time.Now().UTC().Add(-10 * time.Minute)
	events, err := c.Events(ctx, since)
	assert.NoError(t, err)
	assert.Equal(t, len(testEvents), len(events))

	// Test getting events with more recent time
	since = time.Now().UTC().Add(-4 * time.Minute)
	events, err = c.Events(ctx, since)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(events))
}

func TestComponentMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	reg := prometheus.NewRegistry()
	c := &component{}

	err := c.RegisterCollectors(reg, dbRW, dbRO, "test_metrics")
	assert.NoError(t, err)
	assert.Equal(t, reg, c.gatherer)

	since := time.Now().UTC().Add(-10 * time.Minute)
	metrics, err := c.Metrics(ctx, since)

	// Since we don't have a mock for nvidia_query_metrics_clock functions,
	// we expect an error or empty metrics
	if err != nil {
		assert.Error(t, err)
	} else {
		assert.Empty(t, metrics)
	}
}

func TestGetDataSourceInfo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		events        []components.Event
		expectedParts []string
	}{
		{
			name:          "empty events",
			events:        []components.Event{},
			expectedParts: []string{},
		},
		{
			name: "events with no data_source field",
			events: []components.Event{
				{
					Time:      metav1.Time{Time: time.Now()},
					Name:      "hw_slowdown",
					Type:      common.EventTypeWarning,
					Message:   "HW Slowdown detected",
					ExtraInfo: map[string]string{},
				},
				{
					Time:      metav1.Time{Time: time.Now()},
					Name:      "hw_slowdown",
					Type:      common.EventTypeWarning,
					Message:   "HW Slowdown detected",
					ExtraInfo: map[string]string{"gpu_uuid": "gpu-0"},
				},
			},
			expectedParts: []string{},
		},
		{
			name: "events with nil extraInfo",
			events: []components.Event{
				{
					Time:      metav1.Time{Time: time.Now()},
					Name:      "hw_slowdown",
					Type:      common.EventTypeWarning,
					Message:   "HW Slowdown detected",
					ExtraInfo: nil,
				},
				{
					Time:      metav1.Time{Time: time.Now()},
					Name:      "hw_slowdown",
					Type:      common.EventTypeWarning,
					Message:   "HW Slowdown detected",
					ExtraInfo: nil,
				},
			},
			expectedParts: []string{},
		},
		{
			name: "mixed events with and without data_sources",
			events: []components.Event{
				{
					Time:      metav1.Time{Time: time.Now()},
					Name:      "hw_slowdown",
					Type:      common.EventTypeWarning,
					Message:   "HW Slowdown detected",
					ExtraInfo: map[string]string{},
				},
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "nvidia-smi",
						"gpu_uuid":    "gpu-0",
					},
				},
				{
					Time:      metav1.Time{Time: time.Now()},
					Name:      "hw_slowdown",
					Type:      common.EventTypeWarning,
					Message:   "HW Slowdown detected",
					ExtraInfo: nil,
				},
			},
			expectedParts: []string{"nvidia-smi: 1"},
		},
		{
			name: "events with single data_source",
			events: []components.Event{
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "nvidia-smi",
						"gpu_uuid":    "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "nvidia-smi",
						"gpu_uuid":    "gpu-1",
					},
				},
			},
			expectedParts: []string{"nvidia-smi: 2"},
		},
		{
			name: "events with multiple data_sources",
			events: []components.Event{
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "nvidia-smi",
						"gpu_uuid":    "gpu-0",
					},
				},
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "dcgm",
						"gpu_uuid":    "gpu-1",
					},
				},
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "nvml",
						"gpu_uuid":    "gpu-2",
					},
				},
				{
					Time:    metav1.Time{Time: time.Now()},
					Name:    "hw_slowdown",
					Type:    common.EventTypeWarning,
					Message: "HW Slowdown detected",
					ExtraInfo: map[string]string{
						"data_source": "nvidia-smi",
						"gpu_uuid":    "gpu-3",
					},
				},
			},
			expectedParts: []string{"nvidia-smi: 2", "dcgm: 1", "nvml: 1"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := summarizeDataSources(tc.events)

			if len(tc.expectedParts) == 0 {
				assert.Empty(t, result)
				return
			}

			// Check for each expected part in the result
			for _, part := range tc.expectedParts {
				assert.Contains(t, result, part)
			}

			// Verify the count of commas matches expected parts - 1 (if more than one part)
			if len(tc.expectedParts) > 1 {
				commaCount := strings.Count(result, ", ")
				assert.Equal(t, len(tc.expectedParts)-1, commaCount, "Expected %d commas in result: %s", len(tc.expectedParts)-1, result)
			}
		})
	}
}

// Test specifically for the state message with data sources
func TestStatesWithDataSources(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	// Setup test events with data sources
	testEvents := []components.Event{
		{
			Time:    metav1.Time{Time: now.Add(-4 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid":    "gpu-0",
				"data_source": "nvidia-smi",
			},
		},
		{
			Time:    metav1.Time{Time: now.Add(-3 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid":    "gpu-1",
				"data_source": "dcgm",
			},
		},
		{
			Time:    metav1.Time{Time: now.Add(-2 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid":    "gpu-0",
				"data_source": "nvidia-smi",
			},
		},
		{
			Time:    metav1.Time{Time: now.Add(-1 * time.Minute)},
			Name:    "hw_slowdown",
			Type:    common.EventTypeWarning,
			Message: "HW Slowdown detected",
			ExtraInfo: map[string]string{
				"gpu_uuid":    "gpu-1",
				"data_source": "dcgm",
			},
		},
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	assert.NoError(t, err)
	bucket, err := store.Bucket("test_events")
	assert.NoError(t, err)
	defer bucket.Close()

	// Insert test events
	for _, event := range testEvents {
		err := bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Create component with specific thresholds to ensure unhealthy state
	c := &component{
		stateHWSlowdownEvaluationWindow:                  5 * time.Minute,
		stateHWSlowdownEventsThresholdFrequencyPerMinute: 0.1, // Low threshold to ensure unhealthy state
		eventBucket: bucket,
	}

	// Get states
	states, err := c.States(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(states))
	assert.False(t, states[0].Healthy)

	// Verify data sources are included in the reason
	assert.Contains(t, states[0].Reason, "event source counts")
	assert.Contains(t, states[0].Reason, "nvidia-smi")
	assert.Contains(t, states[0].Reason, "dcgm")
}

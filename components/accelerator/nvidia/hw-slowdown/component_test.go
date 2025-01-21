package hwslowdown

import (
	"context"
	"testing"
	"time"

	nvidia_hw_slowdown_state "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/state"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentStates(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()

	testCases := []struct {
		name               string
		window             time.Duration
		thresholdPerMinute float64
		insertedEvent      []nvidia_hw_slowdown_state.Event
		expectedStates     int
		expectHealthy      bool
	}{
		{
			name:               "single event within window",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent:      []nvidia_hw_slowdown_state.Event{{Timestamp: now.Add(-5 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-0"}}},
			expectedStates:     1,
			expectHealthy:      true,
		},
		{
			name:               "multiple events within window but below threshold",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []nvidia_hw_slowdown_state.Event{
				{Timestamp: now.Add(-5 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-0"}},
				{Timestamp: now.Add(-3 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-1"}},
				{Timestamp: now.Add(-1 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-2"}},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "events above threshold",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []nvidia_hw_slowdown_state.Event{
				{Timestamp: now.Add(-4 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-0"}},
				{Timestamp: now.Add(-3 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-1"}},
				{Timestamp: now.Add(-2 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-2"}},
				{Timestamp: now.Add(-1 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-3"}},
			},
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "events above threshold with multiple GPUs",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []nvidia_hw_slowdown_state.Event{
				{Timestamp: now.Add(-4 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-4 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-1", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-4 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-2", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-4 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-3", Reasons: []string{"reason"}},

				{Timestamp: now.Add(-3 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-3 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-1", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-3 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-2", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-3 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-3", Reasons: []string{"reason"}},

				{Timestamp: now.Add(-2 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-2 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-1", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-2 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-2", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-2 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-3", Reasons: []string{"reason"}},

				{Timestamp: now.Add(-1 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-1 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-1", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-1 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-2", Reasons: []string{"reason"}},
				{Timestamp: now.Add(-1 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-3", Reasons: []string{"reason"}},
			},
			expectedStates: 1,
			expectHealthy:  false,
		},
		{
			name:               "events outside window",
			window:             5 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent: []nvidia_hw_slowdown_state.Event{
				{Timestamp: now.Add(-10 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-0"}},
				{Timestamp: now.Add(-8 * time.Minute).Unix(), DataSource: "nvml", GPUUUID: "gpu-0", Reasons: []string{"reason-1"}},
			},
			expectedStates: 1,
			expectHealthy:  true,
		},
		{
			name:               "no events",
			window:             10 * time.Minute,
			thresholdPerMinute: 0.6,
			insertedEvent:      []nvidia_hw_slowdown_state.Event{},
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

			if err := nvidia_hw_slowdown_state.CreateTable(ctx, dbRW); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			if len(tc.insertedEvent) > 0 {
				for _, event := range tc.insertedEvent {
					if err := nvidia_hw_slowdown_state.InsertEvent(ctx, dbRW, event); err != nil {
						t.Fatalf("failed to insert event: %v", err)
					}
				}
			}

			c := &component{
				stateHWSlowdownEvaluationWindow:                  tc.window,
				stateHWSlowdownEventsThresholdFrequencyPerMinute: tc.thresholdPerMinute,
				readEvents: func(ctx context.Context, since time.Time) ([]nvidia_hw_slowdown_state.Event, error) {
					return nvidia_hw_slowdown_state.ReadEvents(
						ctx,
						dbRO,
						nvidia_hw_slowdown_state.WithSince(now.Add(-tc.window)),
						nvidia_hw_slowdown_state.WithDedupDataSource(true),
					)
				},
			}

			states, err := c.States(ctx)
			if err != nil {
				t.Fatalf("failed to get states: %v", err)
			}

			if len(states) != tc.expectedStates {
				t.Fatalf("expected %d states, got %d", tc.expectedStates, len(states))
			}

			if len(states) > 0 && states[0].Healthy != tc.expectHealthy {
				t.Errorf("expected healthy=%v, got %v", tc.expectHealthy, states[0].Healthy)
			}
		})
	}
}

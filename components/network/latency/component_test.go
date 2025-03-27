package latency

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/gpud-metrics/state"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestDataGetReason(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	assert.Equal(t, "no network latency data", d.getReason(DefaultGlobalMillisecondThreshold))

	// Test data with error
	d = &Data{
		err: errors.New("test error"),
	}
	assert.Equal(t, "failed to get network latency data -- test error", d.getReason(DefaultGlobalMillisecondThreshold))

	// Test data with latencies
	d = &Data{
		EgressLatencies: []latency.Latency{
			{
				RegionName:          "us-west-2",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
				LatencyMilliseconds: 50,
			},
			{
				RegionName:          "us-east-1",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
				LatencyMilliseconds: 100,
			},
		},
	}

	reason := d.getReason(DefaultGlobalMillisecondThreshold)
	assert.Contains(t, reason, "latency to us-west-2 edge derp server")
	assert.Contains(t, reason, "latency to us-east-1 edge derp server")

	// Test with zero threshold
	assert.Equal(t, "no issue", d.getReason(0))
}

func TestDataGetHealth(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	health, healthy := d.getHealth(DefaultGlobalMillisecondThreshold)
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test data with error
	d = &Data{
		err: errors.New("test error"),
	}
	health, healthy = d.getHealth(DefaultGlobalMillisecondThreshold)
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test data with latencies below threshold
	d = &Data{
		EgressLatencies: []latency.Latency{
			{
				RegionName:          "us-west-2",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
				LatencyMilliseconds: 50,
			},
			{
				RegionName:          "us-east-1",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 100 * time.Millisecond},
				LatencyMilliseconds: 100,
			},
		},
	}
	health, healthy = d.getHealth(DefaultGlobalMillisecondThreshold)
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)

	// Test data with latencies above threshold
	d = &Data{
		EgressLatencies: []latency.Latency{
			{
				RegionName:          "us-west-2",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 5000 * time.Millisecond},
				LatencyMilliseconds: 8000,
			},
			{
				RegionName:          "us-east-1",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 8000 * time.Millisecond},
				LatencyMilliseconds: 8000,
			},
		},
	}
	health, healthy = d.getHealth(DefaultGlobalMillisecondThreshold)
	assert.Equal(t, components.StateUnhealthy, health)
	assert.False(t, healthy)

	// Test with zero threshold
	health, healthy = d.getHealth(0)
	assert.Equal(t, components.StateHealthy, health)
	assert.True(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	states, err := d.getStates(DefaultGlobalMillisecondThreshold)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "network-latency", states[0].Name)
	assert.Equal(t, "no network latency data", states[0].Reason)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)

	// Test data with error
	d = &Data{
		err: errors.New("test error"),
	}
	states, err = d.getStates(DefaultGlobalMillisecondThreshold)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "network-latency", states[0].Name)
	assert.Equal(t, "failed to get network latency data -- test error", states[0].Reason)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)

	// Test data with latencies
	d = &Data{
		EgressLatencies: []latency.Latency{
			{
				RegionName:          "us-west-2",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
				LatencyMilliseconds: 50,
			},
		},
		ts: time.Now(),
	}
	states, err = d.getStates(DefaultGlobalMillisecondThreshold)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, "network-latency", states[0].Name)
	assert.Contains(t, states[0].Reason, "latency to us-west-2 edge derp server")
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Contains(t, states[0].ExtraInfo, "data")
	assert.Contains(t, states[0].ExtraInfo, "encoding")
}

// setupTestComponent creates a component with properly initialized metrics
func setupTestComponent(t *testing.T, ctx context.Context) (*component, *sql.DB, *sql.DB, func()) {
	// Create test databases
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)

	// Create metrics table
	err := state.CreateTableMetrics(ctx, dbRW, "test_metrics")
	require.NoError(t, err)

	// Create new component
	comp := New(ctx).(*component)

	// Create prometheus registry and register collectors
	reg := prometheus.NewRegistry()
	err = comp.RegisterCollectors(reg, dbRW, dbRO, "test_metrics")
	require.NoError(t, err)

	return comp, dbRW, dbRO, cleanup
}

// TestComponentWithMockMeasureLatencies tests the component using a mocked measureLatencies function
func TestComponentWithMockMeasureLatencies(t *testing.T) {
	ctx := context.Background()

	// Create test cases with different latency scenarios
	testCases := []struct {
		name               string
		mockLatencies      latency.Latencies
		mockError          error
		expectedHealthy    bool
		customThreshold    int64
		shouldUseThreshold bool
	}{
		{
			name: "healthy latencies",
			mockLatencies: latency.Latencies{
				{
					RegionName:          "us-west-2",
					Provider:            "aws",
					Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
					LatencyMilliseconds: 50,
				},
			},
			expectedHealthy: true,
		},
		{
			name: "unhealthy latencies",
			mockLatencies: latency.Latencies{
				{
					RegionName:          "us-west-2",
					Provider:            "aws",
					Latency:             metav1.Duration{Duration: 8000 * time.Millisecond},
					LatencyMilliseconds: 8000,
				},
			},
			expectedHealthy: false,
		},
		{
			name:            "error in measurement",
			mockError:       errors.New("network error"),
			expectedHealthy: false,
		},
		{
			name: "custom threshold - healthy",
			mockLatencies: latency.Latencies{
				{
					RegionName:          "us-west-2",
					Provider:            "aws",
					Latency:             metav1.Duration{Duration: 1500 * time.Millisecond},
					LatencyMilliseconds: 1500,
				},
			},
			customThreshold:    2000,
			shouldUseThreshold: true,
			expectedHealthy:    true,
		},
		{
			name: "custom threshold - unhealthy",
			mockLatencies: latency.Latencies{
				{
					RegionName:          "us-west-2",
					Provider:            "aws",
					Latency:             metav1.Duration{Duration: 1500 * time.Millisecond},
					LatencyMilliseconds: 1500,
				},
			},
			customThreshold:    1000,
			shouldUseThreshold: true,
			expectedHealthy:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a properly initialized component with metrics
			comp, _, _, cleanup := setupTestComponent(t, ctx)
			defer cleanup()

			// Mock the measureLatencies function
			comp.measureLatencies = func(context.Context) (latency.Latencies, error) {
				return tc.mockLatencies, tc.mockError
			}

			// Set custom threshold if needed
			if tc.shouldUseThreshold {
				comp.globalMillisecondThreshold = tc.customThreshold
			}

			// Run the check with our mocked function
			comp.CheckOnce()

			// Get states and verify
			states, err := comp.States(ctx)
			require.NoError(t, err)
			require.Len(t, states, 1)

			// Verify the expected health state
			assert.Equal(t, tc.expectedHealthy, states[0].Healthy)

			// Verify error is captured correctly if there was one
			if tc.mockError != nil {
				assert.Contains(t, states[0].Reason, tc.mockError.Error())
			}
		})
	}
}

// TestComponentName tests the Name method
func TestComponentName(t *testing.T) {
	comp := New(context.Background())
	assert.Equal(t, Name, comp.Name())
}

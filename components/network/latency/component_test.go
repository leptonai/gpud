package latency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	latencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
)

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.getError())

	// Test data with nil error
	cr = &checkResult{}
	assert.Equal(t, "", cr.getError())

	// Test data with error
	cr = &checkResult{
		err: errors.New("test error"),
	}
	assert.Equal(t, "test error", cr.getError())
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	assert.NoError(t, err)
	defer comp.Close()
	assert.Equal(t, Name, comp.Name())
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	assert.NoError(t, err)
	defer comp.Close()

	events, err := comp.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	assert.NoError(t, err)
	assert.NoError(t, comp.Close())
}

func TestComponentStatesNoData(t *testing.T) {
	t.Parallel()

	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	assert.NoError(t, err)

	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestComponentStartAndCheckOnce(t *testing.T) {
	t.Parallel()

	mockLatencies := []latency.Latency{
		{
			RegionName:          "us-west-2",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
			LatencyMilliseconds: 50,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with mock latency function
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		getEgressLatenciesFunc: func(_ context.Context, _ ...latencyedge.OpOption) (latency.Latencies, error) {
			return mockLatencies, nil
		},
		globalMillisecondThreshold: DefaultGlobalMillisecondThreshold,
	}

	// Test start and cancel
	err := comp.Start()
	assert.NoError(t, err)

	// Test check once with success
	_ = comp.Check()

	// Check states
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestComponentStartAndCheckOnceWithError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with mock latency function
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		getEgressLatenciesFunc: func(_ context.Context, _ ...latencyedge.OpOption) (latency.Latencies, error) {
			return nil, errors.New("test error")
		},
		globalMillisecondThreshold: DefaultGlobalMillisecondThreshold,
	}

	// Test start and cancel
	err := comp.Start()
	assert.NoError(t, err)

	_ = comp.Check()

	// Check states when there's an error
	states := comp.LastHealthStates()
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "error measuring egress latencies")
}

func TestCheckHealthState(t *testing.T) {
	if os.Getenv("TEST_NETWORK_LATENCY") != "true" {
		t.Skip("skipping network latency check")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(&components.GPUdInstance{
		RootCtx: ctx,
	})
	assert.NoError(t, err)
	defer comp.Close()

	rs := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rs.HealthStateType())

	fmt.Println(rs.String())

	b, err := json.Marshal(rs)
	assert.NoError(t, err)
	fmt.Println(string(b))
}

func TestCheckResultString(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.String())

	// Test data with latencies
	cr = &checkResult{
		EgressLatencies: []latency.Latency{
			{
				RegionName:          "us-west-2",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
				LatencyMilliseconds: 50,
			},
			{
				RegionName:          "eu-west-1",
				Provider:            "aws",
				Latency:             metav1.Duration{Duration: 120 * time.Millisecond},
				LatencyMilliseconds: 120,
			},
		},
	}
	result := cr.String()
	assert.Contains(t, result, "us-west-2 (aws)")
	assert.Contains(t, result, "eu-west-1 (aws)")
	assert.Contains(t, result, "50ms")
	assert.Contains(t, result, "120ms")
}

func TestCheckResultSummary(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.Summary())

	// Test data with reason
	cr = &checkResult{
		reason: "test reason",
	}
	assert.Equal(t, "test reason", cr.Summary())
}

func TestHealthStateType(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())

	// Test data with health state
	cr = &checkResult{
		health: apiv1.HealthStateTypeHealthy,
	}
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

	// Test data with unhealthy state
	cr = &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
	}
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
}

func TestComponentCheckWithThresholdExceeded(t *testing.T) {
	t.Parallel()

	// Create latencies with one exceeding threshold
	mockLatencies := []latency.Latency{
		{
			RegionName:          "us-west-2",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 50 * time.Millisecond},
			LatencyMilliseconds: 50,
		},
		{
			RegionName:          "eu-west-1",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 8000 * time.Millisecond},
			LatencyMilliseconds: 8000,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with mock latency function and low threshold
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		getEgressLatenciesFunc: func(_ context.Context, _ ...latencyedge.OpOption) (latency.Latencies, error) {
			return mockLatencies, nil
		},
		globalMillisecondThreshold: 7000, // DefaultGlobalMillisecondThreshold
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify result is unhealthy due to threshold exceeded
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "exceeded threshold")
	assert.Contains(t, cr.reason, "eu-west-1")
}

func TestComponentCheckWithAllLatenciesExceedingThreshold(t *testing.T) {
	t.Parallel()

	// Create latencies all exceeding threshold
	mockLatencies := []latency.Latency{
		{
			RegionName:          "us-west-2",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 7500 * time.Millisecond},
			LatencyMilliseconds: 7500,
		},
		{
			RegionName:          "eu-west-1",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 8000 * time.Millisecond},
			LatencyMilliseconds: 8000,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with mock latency function
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		getEgressLatenciesFunc: func(_ context.Context, _ ...latencyedge.OpOption) (latency.Latencies, error) {
			return mockLatencies, nil
		},
		globalMillisecondThreshold: 7000, // DefaultGlobalMillisecondThreshold
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify result is unhealthy due to all thresholds exceeded
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "exceeded threshold")
	assert.Contains(t, cr.reason, "us-west-2")
	assert.Contains(t, cr.reason, "eu-west-1")
}

func TestComponentCheckWithDisabledThreshold(t *testing.T) {
	t.Parallel()

	// Create latencies all exceeding what would be the threshold
	mockLatencies := []latency.Latency{
		{
			RegionName:          "us-west-2",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 7500 * time.Millisecond},
			LatencyMilliseconds: 7500,
		},
		{
			RegionName:          "eu-west-1",
			Provider:            "aws",
			Latency:             metav1.Duration{Duration: 8000 * time.Millisecond},
			LatencyMilliseconds: 8000,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with mock latency function but disabled threshold
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		getEgressLatenciesFunc: func(_ context.Context, _ ...latencyedge.OpOption) (latency.Latencies, error) {
			return mockLatencies, nil
		},
		globalMillisecondThreshold: 0, // Disabled
	}

	result := comp.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify result is healthy since threshold is disabled
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Contains(t, cr.reason, "no issue found")
}

package latency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	latency_edge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
)

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test nil data
	var d *Data
	assert.Equal(t, "", d.getError())

	// Test data with nil error
	d = &Data{}
	assert.Equal(t, "", d.getError())

	// Test data with error
	d = &Data{
		err: errors.New("test error"),
	}
	assert.Equal(t, "test error", d.getError())
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	c := New(context.Background())
	assert.Equal(t, Name, c.Name())
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	c := New(context.Background())
	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Empty(t, events)
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	c := New(context.Background())
	err := c.Close()
	assert.NoError(t, err)
}

func TestComponentStatesNoData(t *testing.T) {
	t.Parallel()

	c := New(context.Background())
	states, err := c.HealthStates(context.Background())
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
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
		getEgressLatenciesFunc: func(_ context.Context, _ ...latency_edge.OpOption) (latency.Latencies, error) {
			return mockLatencies, nil
		},
		globalMillisecondThreshold: DefaultGlobalMillisecondThreshold,
	}

	// Test start and cancel
	err := comp.Start()
	assert.NoError(t, err)

	// Test check once with success
	comp.CheckOnce()

	// Check states
	states, err := comp.HealthStates(context.Background())
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
}

func TestComponentStartAndCheckOnceWithError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create component with mock latency function
	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		getEgressLatenciesFunc: func(_ context.Context, _ ...latency_edge.OpOption) (latency.Latencies, error) {
			return nil, errors.New("test error")
		},
		globalMillisecondThreshold: DefaultGlobalMillisecondThreshold,
	}

	// Test start and cancel
	err := comp.Start()
	assert.NoError(t, err)

	comp.CheckOnce()

	// Check states when there's an error
	states, err := comp.HealthStates(context.Background())
	assert.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "error measuring egress latencies")
}

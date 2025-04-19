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
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rs.HealthState())

	fmt.Println(rs.String())

	b, err := json.Marshal(rs)
	assert.NoError(t, err)
	fmt.Println(string(b))
}

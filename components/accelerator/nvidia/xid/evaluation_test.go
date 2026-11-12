package xid

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestCachedStateWithoutObservationIsNotEvaluated(t *testing.T) {
	c := &component{currState: apiv1.HealthState{Health: apiv1.HealthStateTypeHealthy}}
	state := c.LastHealthStates()[0]
	require.NotNil(t, state.Evaluated)
	require.False(t, *state.Evaluated)
	require.Equal(t, apiv1.NotEvaluatedNoData, state.NotEvaluatedReason)
	// Known faults survive missing evidence coverage.
	c.currState.Health = apiv1.HealthStateTypeUnhealthy
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, c.LastHealthStates()[0].Health)
}

func TestNativeStreamCoverage(t *testing.T) {
	for _, withNVSentinel := range []bool{false, true} {
		name := "native only"
		if withNVSentinel {
			name = "NVSentinel remains available"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := &component{ctx: ctx, currState: apiv1.HealthState{Health: apiv1.HealthStateTypeHealthy, Evaluation: apiv1.Evaluated()}}
			if withNVSentinel {
				c.nvsSource = nvsentineltest.NewFakeSource()
			}
			messages := make(chan kernelRecord)
			done := make(chan struct{})
			go func() { defer close(done); c.start(messages, time.Hour) }()
			require.Eventually(t, c.monitoring.Load, time.Second, time.Millisecond)
			require.True(t, *c.LastHealthStates()[0].Evaluated)
			close(messages)
			if !withNVSentinel {
				require.Eventually(t, func() bool { return !c.monitoring.Load() }, time.Second, time.Millisecond)
				state := c.LastHealthStates()[0]
				require.False(t, *state.Evaluated)
				require.Equal(t, apiv1.NotEvaluatedNoData, state.NotEvaluatedReason)
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("stream did not stop")
			}
			require.False(t, *c.LastHealthStates()[0].Evaluated)
		})
	}
}

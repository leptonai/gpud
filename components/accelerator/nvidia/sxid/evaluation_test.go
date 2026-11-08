package sxid

import (
	"testing"

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

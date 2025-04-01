package latency

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/netutil/latency"
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

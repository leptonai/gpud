package netstat

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/netutil"
)

func resetMetrics() {
	metricTCPRetransSegmentsTotal.Reset()
	metricTCPExtSegmentRetransmitsTotal.Reset()
	metricUDPInErrorsTotal.Reset()
	metricUDPRcvbufErrorsTotal.Reset()
	metricUDPSndbufErrorsTotal.Reset()
}

func TestComponentMetadata(t *testing.T) {
	t.Parallel()

	comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
	require.NoError(t, err)
	defer comp.Close()

	assert.Equal(t, Name, comp.Name())
	assert.Equal(t, []string{"network", Name}, comp.Tags())
	assert.Equal(t, runtime.GOOS == "linux", comp.IsSupported())
}

func TestComponentLastHealthStatesNoData(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		now: func() time.Time {
			return time.Unix(0, 0).UTC()
		},
	}

	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
}

func TestComponentCheckSuccess(t *testing.T) {
	resetMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Unix(1735689600, 0).UTC() // 2025-01-01T00:00:00Z

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		now: func() time.Time {
			return now
		},
		collectCounters: func() (netutil.NetStatCounters, error) {
			return netutil.NetStatCounters{
				TCPRetransSegments:       42,
				TcpExtSegmentRetransmits: 7,
				UDPInErrors:              3,
				UDPRcvbufErrors:          9,
				UDPSndbufErrors:          11,
			}, nil
		},
	}
	defer comp.Close()

	result := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "collected netstat counters")

	assert.InDelta(t, 42, testutil.ToFloat64(metricTCPRetransSegmentsTotal.With(prometheus.Labels{})), 0.001)
	assert.InDelta(t, 7, testutil.ToFloat64(metricTCPExtSegmentRetransmitsTotal.With(prometheus.Labels{})), 0.001)
	assert.InDelta(t, 3, testutil.ToFloat64(metricUDPInErrorsTotal.With(prometheus.Labels{})), 0.001)
	assert.InDelta(t, 9, testutil.ToFloat64(metricUDPRcvbufErrorsTotal.With(prometheus.Labels{})), 0.001)
	assert.InDelta(t, 11, testutil.ToFloat64(metricUDPSndbufErrorsTotal.With(prometheus.Labels{})), 0.001)

	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	state := states[0]
	assert.Equal(t, Name, state.Component)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	assert.Equal(t, metav1.NewTime(now), state.Time)
	assert.Empty(t, state.Error)
}

func TestComponentCheckErrorDoesNotFlipHealth(t *testing.T) {
	resetMetrics()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
		now: func() time.Time {
			return time.Unix(1735689600, 0).UTC()
		},
		collectCounters: func() (netutil.NetStatCounters, error) {
			return netutil.NetStatCounters{}, errors.New("read failure")
		},
	}
	defer comp.Close()

	result := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "failed to collect netstat counters")

	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Contains(t, states[0].Reason, "failed to collect")
	assert.Contains(t, states[0].Error, "read failure")

	assert.InDelta(t, 0, testutil.ToFloat64(metricTCPRetransSegmentsTotal.With(prometheus.Labels{})), 0.001)
	assert.InDelta(t, 0, testutil.ToFloat64(metricTCPExtSegmentRetransmitsTotal.With(prometheus.Labels{})), 0.001)
}

func TestComponentStartAndEvents(t *testing.T) {
	t.Parallel()

	comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
	require.NoError(t, err)
	defer comp.Close()

	// Start() should return nil (no background work)
	err = comp.Start()
	assert.NoError(t, err)

	// Events() should return nil
	events, err := comp.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	// Verify context is not done before Close
	select {
	case <-comp.ctx.Done():
		t.Fatal("context should not be done before Close")
	default:
		// expected
	}

	// Close should cancel context
	err := comp.Close()
	assert.NoError(t, err)

	// Verify context is done after Close
	select {
	case <-comp.ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after Close")
	}
}

func TestCheckResultMethods(t *testing.T) {
	t.Parallel()

	t.Run("ComponentName", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})

	t.Run("String with data", func(t *testing.T) {
		cr := &checkResult{
			counters: netutil.NetStatCounters{
				TCPRetransSegments:       100,
				TcpExtSegmentRetransmits: 50,
				UDPInErrors:              10,
				UDPRcvbufErrors:          5,
				UDPSndbufErrors:          2,
			},
		}
		str := cr.String()
		assert.Contains(t, str, "tcp_retrans_segments_total=100")
		assert.Contains(t, str, "tcp_ext_segment_retransmits_total=50")
		assert.Contains(t, str, "udp_in_errors_total=10")
		assert.Contains(t, str, "udp_rcvbuf_errors_total=5")
		assert.Contains(t, str, "udp_sndbuf_errors_total=2")
	})

	t.Run("String with nil receiver", func(t *testing.T) {
		var cr *checkResult
		assert.Empty(t, cr.String())
	})

	t.Run("Summary with data", func(t *testing.T) {
		cr := &checkResult{
			reason: "test reason",
		}
		assert.Equal(t, "test reason", cr.Summary())
	})

	t.Run("Summary with nil receiver", func(t *testing.T) {
		var cr *checkResult
		assert.Empty(t, cr.Summary())
	})

	t.Run("HealthStateType with data", func(t *testing.T) {
		cr := &checkResult{
			health: apiv1.HealthStateTypeHealthy,
		}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("HealthStateType with nil receiver", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("HealthStates with nil receiver", func(t *testing.T) {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("HealthStates with error", func(t *testing.T) {
		now := time.Unix(1735689600, 0).UTC()
		cr := &checkResult{
			ts:     now,
			health: apiv1.HealthStateTypeHealthy,
			reason: "test reason",
			err:    errors.New("test error"),
		}
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "test reason", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
		assert.Equal(t, metav1.NewTime(now), states[0].Time)
	})
}

func TestSetMetricsAllCounters(t *testing.T) {
	resetMetrics()

	counters := netutil.NetStatCounters{
		TCPRetransSegments:       1000,
		TcpExtSegmentRetransmits: 2000,
		UDPInErrors:              3000,
		UDPRcvbufErrors:          5000,
		UDPSndbufErrors:          6000,
	}

	setMetrics(counters)

	labels := prometheus.Labels{}
	assert.InDelta(t, 1000, testutil.ToFloat64(metricTCPRetransSegmentsTotal.With(labels)), 0.001)
	assert.InDelta(t, 2000, testutil.ToFloat64(metricTCPExtSegmentRetransmitsTotal.With(labels)), 0.001)
	assert.InDelta(t, 3000, testutil.ToFloat64(metricUDPInErrorsTotal.With(labels)), 0.001)
	assert.InDelta(t, 5000, testutil.ToFloat64(metricUDPRcvbufErrorsTotal.With(labels)), 0.001)
	assert.InDelta(t, 6000, testutil.ToFloat64(metricUDPSndbufErrorsTotal.With(labels)), 0.001)
}

func TestCheckResultGetError(t *testing.T) {
	t.Parallel()

	t.Run("with error", func(t *testing.T) {
		cr := &checkResult{
			err: errors.New("test error message"),
		}
		assert.Equal(t, "test error message", cr.getError())
	})

	t.Run("without error", func(t *testing.T) {
		cr := &checkResult{}
		assert.Empty(t, cr.getError())
	})

	t.Run("nil receiver", func(t *testing.T) {
		var cr *checkResult
		assert.Empty(t, cr.getError())
	})
}

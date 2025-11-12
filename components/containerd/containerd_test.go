package containerd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestParseContainerdVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "with v prefix",
			input: "containerd github.com/containerd/containerd v1.7.20 abc",
			want:  "v1.7.20",
		},
		{
			name:  "without v prefix",
			input: "containerd containerd.io 1.7.20 abc",
			want:  "1.7.20",
		},
		{
			name:    "invalid output",
			input:   "containerd version",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerdVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComponentRecordsContainerdVersion(t *testing.T) {
	metricVersion.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &component{
		ctx:                          ctx,
		cancel:                       cancel,
		checkDependencyInstalledFunc: func() bool { return true },
		getContainerdVersionFunc:     func() (string, error) { return "v1.7.20", nil },
		getTimeNowFunc: func() time.Time {
			return time.Unix(0, 0)
		},
	}

	res := c.Check().(*checkResult)
	require.Equal(t, "v1.7.20", res.ContainerdVersion)
	metric := metricVersion.WithLabelValues("v1.7.20")
	assert.Equal(t, 1.0, gaugeValue(t, metric))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, res.health)
}

func TestComponentContainerdVersionErrorDoesNotAffectHealth(t *testing.T) {
	metricVersion.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &component{
		ctx:                          ctx,
		cancel:                       cancel,
		checkDependencyInstalledFunc: func() bool { return true },
		getContainerdVersionFunc: func() (string, error) {
			return "", errors.New("boom")
		},
		getTimeNowFunc: func() time.Time {
			return time.Unix(0, 0)
		},
	}

	res := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, res.health)
	assert.Empty(t, res.ContainerdVersion)
	metric := metricVersion.WithLabelValues("v1.7.20")
	assert.Equal(t, 0.0, gaugeValue(t, metric))

}

func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	metric := &io_prometheus_client.Metric{}
	require.NoError(t, g.Write(metric))
	gauge := metric.GetGauge()
	if gauge == nil {
		return 0
	}
	return gauge.GetValue()
}

package kubelet

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestParseKubeletVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "standard", input: "Kubernetes v1.33.4", want: "v1.33.4"},
		{name: "no prefix", input: "Kubernetes 1.30.1", want: "1.30.1"},
		{name: "invalid", input: "version", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKubeletVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComponentRecordsKubeletVersion(t *testing.T) {
	metricVersion.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &component{
		ctx:                       ctx,
		cancel:                    cancel,
		checkKubeletInstalledFunc: func() bool { return true },
		getKubeletVersionFunc:     func() (string, error) { return "v1.33.4", nil },
		checkKubeletRunning:       func() bool { return false },
	}

	res := c.Check().(*checkResult)
	require.Equal(t, "v1.33.4", res.KubeletVersion)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, res.health)
	assert.Equal(t, 1.0, gaugeValue(t, metricVersion.WithLabelValues("v1.33.4")))
}

func TestComponentKubeletVersionErrorDoesNotAffectHealth(t *testing.T) {
	metricVersion.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &component{
		ctx:                       ctx,
		cancel:                    cancel,
		checkKubeletInstalledFunc: func() bool { return true },
		getKubeletVersionFunc: func() (string, error) {
			return "", errors.New("boom")
		},
		checkKubeletRunning: func() bool { return false },
	}

	res := c.Check().(*checkResult)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, res.health)
	assert.Empty(t, res.KubeletVersion)
	assert.Equal(t, 0.0, gaugeValue(t, metricVersion.WithLabelValues("v1.33.4")))
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

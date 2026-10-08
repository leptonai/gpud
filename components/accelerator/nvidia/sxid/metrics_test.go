package sxid

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestRecordSXIDErrsMetric(t *testing.T) {
	t.Parallel()

	c := metricSXIDErrs.With(prometheus.Labels{
		"sxid":       "12028",
		"device_pci": "PCI:0000:05:00.0",
	})

	before := counterValue(t, c)
	recordSXIDErrsMetric(12028, "PCI:0000:05:00.0")
	after := counterValue(t, c)

	if after != before+1 {
		t.Errorf("counter = %v, want %v", after, before+1)
	}
}

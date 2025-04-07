// Package gpudmetrics implements metrics collection and reporting.
package gpudmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	componentsRegistered = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "components",
			Name:      "registered",
			Help:      "current registered components",
		},
		[]string{"component"},
	)
)

func init() {
	prometheus.MustRegister(componentsRegistered)
}

func SetRegistered(componentName string) {
	componentsRegistered.With(prometheus.Labels{"component": componentName}).Set(1.0)
}

func ReadRegisteredTotal(gatherer prometheus.Gatherer) (int64, error) {
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		return 0, err
	}

	total := int64(0)
	for _, mf := range metricFamilies {
		if mf.GetName() == "gpud_components_registered" {
			for _, m := range mf.GetMetric() {
				total += int64(m.GetGauge().GetValue())
			}
		}
	}
	return total, nil
}

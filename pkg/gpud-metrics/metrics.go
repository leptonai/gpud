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
	componentsHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "components",
			Name:      "healthy",
			Help:      "current healthy components",
		},
		[]string{"component"},
	)
	componentsUnhealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "components",
			Name:      "unhealthy",
			Help:      "current unhealthy components",
		},
		[]string{"component"},
	)
)

func Register(reg *prometheus.Registry) error {
	if err := reg.Register(componentsRegistered); err != nil {
		return err
	}
	if err := reg.Register(componentsHealthy); err != nil {
		return err
	}
	if err := reg.Register(componentsUnhealthy); err != nil {
		return err
	}
	return nil
}

func SetRegistered(componentName string) {
	componentsRegistered.With(prometheus.Labels{"component": componentName}).Set(1.0)
}

func SetHealthy(componentName string) {
	componentsHealthy.With(prometheus.Labels{"component": componentName}).Set(1.0)
	componentsUnhealthy.With(prometheus.Labels{"component": componentName}).Set(0.0)
}

func SetUnhealthy(componentName string) {
	componentsHealthy.With(prometheus.Labels{"component": componentName}).Set(0.0)
	componentsUnhealthy.With(prometheus.Labels{"component": componentName}).Set(1.0)
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

func ReadHealthyTotal(gatherer prometheus.Gatherer) (int64, error) {
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		return 0, err
	}

	total := int64(0)
	for _, mf := range metricFamilies {
		if mf.GetName() == "gpud_components_healthy" {
			for _, m := range mf.GetMetric() {
				total += int64(m.GetGauge().GetValue())
			}
		}
	}
	return total, nil
}

func ReadUnhealthyTotal(gatherer prometheus.Gatherer) (int64, error) {
	metricFamilies, err := gatherer.Gather()
	if err != nil {
		return 0, err
	}

	total := int64(0)
	for _, mf := range metricFamilies {
		if mf.GetName() == "gpud_components_unhealthy" {
			for _, m := range mf.GetMetric() {
				total += int64(m.GetGauge().GetValue())
			}
		}
	}
	return total, nil
}

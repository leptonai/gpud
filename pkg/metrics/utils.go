package metrics

import (
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func ConvertToLeptonMetrics(ms Metrics) apiv1.GPUdComponentMetrics {
	aggregated := make(map[string][]apiv1.Metric)
	for _, m := range ms {
		if m.Component == "" {
			continue
		}
		if _, ok := aggregated[m.Component]; !ok {
			aggregated[m.Component] = make([]apiv1.Metric, 0)
		}

		mt := apiv1.Metric{
			UnixSeconds: m.UnixMilliseconds,
			Name:        m.Name,
			Labels:      m.Labels,
			Value:       m.Value,
		}

		aggregated[m.Component] = append(aggregated[m.Component], mt)
	}

	converted := make(apiv1.GPUdComponentMetrics, 0, len(ms))
	for component, ms := range aggregated {
		sort.Slice(ms, func(i, j int) bool {
			return ms[i].UnixSeconds < ms[j].UnixSeconds
		})
		converted = append(converted, apiv1.ComponentMetrics{
			Component: component,
			Metrics:   ms,
		})
	}
	return converted
}

type HealthStateSetter interface {
	Set(healthState apiv1.HealthStateType)
}

var _ HealthStateSetter = &healthStateSetter{}

type healthStateSetter struct {
	setHealthy   func(healthy bool)
	setUnhealthy func(unhealthy bool)
	setDegraded  func(degraded bool)
}

func (s *healthStateSetter) Set(healthState apiv1.HealthStateType) {
	s.setHealthy(healthState == apiv1.HealthStateTypeHealthy)
	s.setUnhealthy(healthState == apiv1.HealthStateTypeUnhealthy)
	s.setDegraded(healthState == apiv1.HealthStateTypeDegraded)
}

// to prevent duplicate metrics registration
var (
	registeredHealthStateMetricsMu sync.Mutex
	registeredHealthStateMetrics   = make(map[string]HealthStateSetter)
)

func RegisterHealthStateMetrics(componentName string) (HealthStateSetter, error) {
	registeredHealthStateMetricsMu.Lock()
	defer registeredHealthStateMetricsMu.Unlock()

	if v, ok := registeredHealthStateMetrics[componentName]; ok {
		return v, nil
	}

	setter, err := RegisterHealthStateMetricsWithRegisterer(defaultRegisterer, componentName)
	if err != nil {
		return nil, err
	}

	registeredHealthStateMetrics[componentName] = setter
	return setter, nil
}

// RegisterHealthStateMetricsWithRegisterer registers the health state metrics for the given component.
// Returns the function to update the health state metrics.
func RegisterHealthStateMetricsWithRegisterer(reg prometheus.Registerer, componentName string) (HealthStateSetter, error) {
	metricStateHealthy := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: NormalizeComponentNameToMetricSubsystem(componentName),
			Name:      "health_state_healthy",
			Help:      "last known healthy state of the component (set to 1 if healthy)",
		},
		[]string{MetricComponentLabelKey},
	).MustCurryWith(
		prometheus.Labels{
			MetricComponentLabelKey: componentName,
		},
	)
	if err := reg.Register(metricStateHealthy); err != nil {
		return nil, err
	}
	setHealthy := func(healthy bool) {
		if healthy {
			metricStateHealthy.With(prometheus.Labels{}).Set(1)
		} else {
			metricStateHealthy.With(prometheus.Labels{}).Set(0)
		}
	}

	metricStateUnhealthy := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: NormalizeComponentNameToMetricSubsystem(componentName),
			Name:      "health_state_unhealthy",
			Help:      "last known unhealthy state of the component (set to 1 if unhealthy)",
		},
		[]string{MetricComponentLabelKey},
	).MustCurryWith(
		prometheus.Labels{
			MetricComponentLabelKey: componentName,
		},
	)
	if err := reg.Register(metricStateUnhealthy); err != nil {
		return nil, err
	}
	setUnhealthy := func(unhealthy bool) {
		if unhealthy {
			metricStateUnhealthy.With(prometheus.Labels{}).Set(1)
		} else {
			metricStateUnhealthy.With(prometheus.Labels{}).Set(0)
		}
	}

	metricStateDegraded := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: NormalizeComponentNameToMetricSubsystem(componentName),
			Name:      "health_state_degraded",
			Help:      "last known degraded state of the component (set to 1 if degraded)",
		},
		[]string{MetricComponentLabelKey},
	).MustCurryWith(
		prometheus.Labels{
			MetricComponentLabelKey: componentName,
		},
	)
	if err := reg.Register(metricStateDegraded); err != nil {
		return nil, err
	}
	setDegraded := func(degraded bool) {
		if degraded {
			metricStateDegraded.With(prometheus.Labels{}).Set(1)
		} else {
			metricStateDegraded.With(prometheus.Labels{}).Set(0)
		}
	}

	return &healthStateSetter{
		setHealthy:   setHealthy,
		setUnhealthy: setUnhealthy,
		setDegraded:  setDegraded,
	}, nil
}

func NormalizeComponentNameToMetricSubsystem(componentName string) string {
	s := strings.ToLower(componentName)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

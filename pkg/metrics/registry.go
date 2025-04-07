package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	defaultRegistry                         = prometheus.NewRegistry()
	defaultRegisterer prometheus.Registerer = defaultRegistry
	defaultGatherer   prometheus.Gatherer   = defaultRegistry
)

// DefaultRegisterer returns the default registerer.
func DefaultRegisterer() prometheus.Registerer {
	return defaultRegisterer
}

// DefaultGatherer returns the default gatherer.
func DefaultGatherer() prometheus.Gatherer {
	return defaultGatherer
}

func MustRegister(collectors ...prometheus.Collector) {
	defaultRegisterer.MustRegister(collectors...)
}

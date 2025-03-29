package metrics

import (
	"sort"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	components_metrics_state "github.com/leptonai/gpud/pkg/gpud-metrics/state"
)

func ConvertToLeptonMetrics(ms Metrics) v1.LeptonMetrics {
	aggregated := make(map[string][]components.Metric)
	for _, m := range ms {
		if m.Component == "" {
			continue
		}
		if _, ok := aggregated[m.Component]; !ok {
			aggregated[m.Component] = make([]components.Metric, 0)
		}

		aggregated[m.Component] = append(aggregated[m.Component], components.Metric{
			Metric: components_metrics_state.Metric{
				UnixSeconds:         m.UnixMilliseconds,
				MetricName:          m.Name,
				MetricSecondaryName: m.Label,
				Value:               m.Value,
			},
		})
	}

	converted := make(v1.LeptonMetrics, 0, len(ms))
	for component, ms := range aggregated {
		sort.Slice(ms, func(i, j int) bool {
			return ms[i].Metric.UnixSeconds < ms[j].Metric.UnixSeconds
		})
		converted = append(converted, v1.LeptonComponentMetrics{
			Component: component,
			Metrics:   ms,
		})
	}
	return converted
}

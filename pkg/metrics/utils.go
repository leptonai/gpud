package metrics

import (
	"sort"

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

			Name:  m.Name,
			Value: m.Value,

			DeprecatedMetricName:          m.Name,
			DeprecatedMetricSecondaryName: m.LabelValue,
		}
		if m.LabelName != "" {
			mt.Labels = map[string]string{
				m.LabelName: m.LabelValue,
			}
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

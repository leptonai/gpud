package scraper

import (
	"context"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

var _ pkgmetrics.Scraper = &promScraper{}

func NewPrometheusScraper(gatherer prometheus.Gatherer) (pkgmetrics.Scraper, error) {
	return &promScraper{
		gatherer: gatherer,
	}, nil
}

type promScraper struct {
	gatherer prometheus.Gatherer
}

func (s *promScraper) Scrape(_ context.Context) (pkgmetrics.Metrics, error) {
	if s == nil || s.gatherer == nil {
		return nil, nil
	}

	gathered, err := s.gatherer.Gather()
	if err != nil {
		return nil, err
	}

	log.Logger.Infow("scraping prometheus metrics")
	now := time.Now().UTC().UnixMilli()

	ms := make(pkgmetrics.Metrics, 0, len(gathered))
	for _, metricFamily := range gathered {
		for _, mtRaw := range metricFamily.GetMetric() {
			m := pkgmetrics.Metric{
				UnixMilliseconds: now,
				Name:             metricFamily.GetName(),
			}

			for _, label := range mtRaw.GetLabel() {
				labelName := label.GetName()
				if labelName == pkgmetrics.MetricComponentLabelKey {
					m.Component = label.GetValue()
					continue
				}
				if strings.HasPrefix(labelName, pkgmetrics.MetricLabelNamePrefix) {
					m.LabelName = strings.TrimPrefix(labelName, pkgmetrics.MetricLabelNamePrefix)
					m.LabelValue = label.GetValue()
				}
			}
			if m.Component == "" {
				continue
			}

			// for now, only support counter and gauge
			switch {
			case mtRaw.GetCounter() != nil:
				m.Value = mtRaw.GetCounter().GetValue()
			case mtRaw.GetGauge() != nil:
				m.Value = mtRaw.GetGauge().GetValue()
			}

			ms = append(ms, m)
		}
	}

	return ms, nil
}

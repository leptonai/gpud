package session

import (
	"context"
	"errors"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

func (s *Session) getMetrics(ctx context.Context, payload Request) (apiv1.GPUdComponentMetrics, error) {
	if payload.Method != "metrics" {
		return nil, errors.New("mismatch method")
	}
	allComponents := s.components
	if len(payload.Components) > 0 {
		allComponents = payload.Components
	}

	now := time.Now().UTC()
	metricsSince := now.Add(-DefaultQuerySince)
	if payload.Since > 0 {
		metricsSince = now.Add(-payload.Since)
	}

	var metricBuf = make(chan apiv1.ComponentMetrics, len(allComponents))
	localCtx, done := context.WithTimeout(ctx, time.Minute)
	defer done()
	for _, componentName := range allComponents {
		go func(name string) {
			metricBuf <- s.getMetricsFromComponent(localCtx, name, metricsSince)
		}(componentName)
	}
	var retMetrics apiv1.GPUdComponentMetrics
	for currMetric := range metricBuf {
		retMetrics = append(retMetrics, currMetric)
		if len(retMetrics) == len(allComponents) {
			close(metricBuf)
			break
		}
	}
	return retMetrics, nil
}

func (s *Session) getMetricsFromComponent(ctx context.Context, componentName string, since time.Time) apiv1.ComponentMetrics {
	component := s.componentsRegistry.Get(componentName)
	if component == nil {
		log.Logger.Errorw("failed to get component",
			"operation", "GetEvents",
			"component", componentName,
			"error", errdefs.ErrNotFound,
		)
		return apiv1.ComponentMetrics{
			Component: componentName,
		}
	}
	currMetrics := apiv1.ComponentMetrics{
		Component: componentName,
	}
	metricsData, err := s.metricsStore.Read(ctx, pkgmetrics.WithSince(since), pkgmetrics.WithComponents(componentName))
	if err != nil {
		log.Logger.Errorw("failed to invoke component metrics",
			"operation", "GetEvents",
			"component", componentName,
			"error", err,
		)
		return currMetrics
	}

	for _, data := range metricsData {
		currMetrics.Metrics = append(currMetrics.Metrics, apiv1.Metric{
			UnixSeconds: data.UnixMilliseconds,
			Name:        data.Name,
			Labels:      data.Labels,
			Value:       data.Value,
		})
	}
	return currMetrics
}

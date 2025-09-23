package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

func TestSession_getMetrics(t *testing.T) {
	t.Run("mismatch method returns error", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		session := createMockSession(registry)

		ctx := context.Background()
		payload := Request{
			Method: "not_metrics",
		}

		result, err := session.getMetrics(ctx, payload)

		assert.Error(t, err)
		assert.Equal(t, "mismatch method", err.Error())
		assert.Nil(t, result)
	})

	t.Run("uses default components when none specified", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore
		session.components = []string{"comp1", "comp2"}

		ctx := context.Background()
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)

		metricsData := pkgmetrics.Metrics{
			{Name: "metric1", Value: 42.0, UnixMilliseconds: 1000, Component: "comp1"},
			{Name: "metric2", Value: 84.0, UnixMilliseconds: 2000, Component: "comp2"},
		}

		registry.On("Get", "comp1").Return(comp1)
		registry.On("Get", "comp2").Return(comp2)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData, nil)

		payload := Request{
			Method: "metrics",
		}

		result, err := session.getMetrics(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 2)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("uses specified components", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore
		session.components = []string{"default1", "default2"}

		ctx := context.Background()
		comp := new(mockComponent)

		metricsData := pkgmetrics.Metrics{
			{Name: "metric", Value: 100.0, UnixMilliseconds: 1000, Component: "specified"},
		}

		registry.On("Get", "specified").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData, nil)

		payload := Request{
			Method:     "metrics",
			Components: []string{"specified"},
		}

		result, err := session.getMetrics(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "specified", result[0].Component)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("uses custom since duration", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		ctx := context.Background()
		comp := new(mockComponent)

		metricsData := pkgmetrics.Metrics{
			{Name: "metric", Value: 50.0, UnixMilliseconds: 1000, Component: "comp"},
		}

		registry.On("Get", "comp").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData, nil)

		payload := Request{
			Method:     "metrics",
			Components: []string{"comp"},
			Since:      2 * time.Hour,
		}

		result, err := session.getMetrics(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 1)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("uses default since when not specified", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore
		session.components = []string{"comp"}

		ctx := context.Background()
		comp := new(mockComponent)

		metricsData := pkgmetrics.Metrics{
			{Name: "metric", Value: 75.0, UnixMilliseconds: 1000, Component: "comp"},
		}

		registry.On("Get", "comp").Return(comp)
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData, nil)

		payload := Request{
			Method: "metrics",
			Since:  0, // Will use DefaultQuerySince
		}

		result, err := session.getMetrics(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 1)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("handles multiple components concurrently", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		ctx := context.Background()
		comp1 := new(mockComponent)
		comp2 := new(mockComponent)
		comp3 := new(mockComponent)

		metricsData1 := pkgmetrics.Metrics{{Name: "m1", Value: 1.0, UnixMilliseconds: 1000, Component: "comp1"}}
		metricsData2 := pkgmetrics.Metrics{{Name: "m2", Value: 2.0, UnixMilliseconds: 2000, Component: "comp2"}}
		metricsData3 := pkgmetrics.Metrics{{Name: "m3", Value: 3.0, UnixMilliseconds: 3000, Component: "comp3"}}

		registry.On("Get", "comp1").Return(comp1)
		registry.On("Get", "comp2").Return(comp2)
		registry.On("Get", "comp3").Return(comp3)

		// Since metrics are fetched concurrently, we can't predict which component will be fetched in which call
		// So we use a generic matcher that returns the appropriate metrics based on the call
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData1, nil).Once()
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData2, nil).Once()
		metricsStore.On("Read", mock.Anything, mock.Anything).Return(metricsData3, nil).Once()

		payload := Request{
			Method:     "metrics",
			Components: []string{"comp1", "comp2", "comp3"},
		}

		result, err := session.getMetrics(ctx, payload)

		assert.NoError(t, err)
		assert.Len(t, result, 3)

		// Verify all components are present (order not guaranteed)
		componentNames := make(map[string]bool)
		for _, metric := range result {
			componentNames[metric.Component] = true
		}
		assert.True(t, componentNames["comp1"])
		assert.True(t, componentNames["comp2"])
		assert.True(t, componentNames["comp3"])

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})
}

func TestSession_getMetricsFromComponent(t *testing.T) {
	t.Run("component not found", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		ctx := context.Background()
		since := time.Now().Add(-time.Hour)

		registry.On("Get", "nonexistent").Return(nil)

		result := session.getMetricsFromComponent(ctx, "nonexistent", since)

		assert.Equal(t, "nonexistent", result.Component)
		assert.Empty(t, result.Metrics)

		registry.AssertExpectations(t)
	})

	t.Run("successful metrics retrieval", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		ctx := context.Background()
		since := time.Now().Add(-time.Hour)

		comp := new(mockComponent)
		metricsData := pkgmetrics.Metrics{
			{Name: "cpu_usage", Value: 45.5, UnixMilliseconds: 1000000, Labels: map[string]string{"core": "0"}},
			{Name: "mem_usage", Value: 75.2, UnixMilliseconds: 2000000, Labels: map[string]string{"type": "rss"}},
		}

		registry.On("Get", "test-comp").Return(comp)
		metricsStore.On("Read", ctx, mock.Anything).Return(metricsData, nil)

		result := session.getMetricsFromComponent(ctx, "test-comp", since)

		assert.Equal(t, "test-comp", result.Component)
		assert.Len(t, result.Metrics, 2)
		assert.Equal(t, "cpu_usage", result.Metrics[0].Name)
		assert.Equal(t, 45.5, result.Metrics[0].Value)
		assert.Equal(t, int64(1000000), result.Metrics[0].UnixSeconds)
		assert.Equal(t, "mem_usage", result.Metrics[1].Name)
		assert.Equal(t, 75.2, result.Metrics[1].Value)
		assert.Equal(t, int64(2000000), result.Metrics[1].UnixSeconds)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("metrics store error", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		ctx := context.Background()
		since := time.Now().Add(-time.Hour)

		comp := new(mockComponent)
		emptyMetrics := pkgmetrics.Metrics{}

		registry.On("Get", "error-comp").Return(comp)
		metricsStore.On("Read", ctx, mock.Anything).Return(emptyMetrics, errors.New("store error"))

		result := session.getMetricsFromComponent(ctx, "error-comp", since)

		assert.Equal(t, "error-comp", result.Component)
		assert.Empty(t, result.Metrics)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})

	t.Run("handles empty metrics", func(t *testing.T) {
		registry := new(mockComponentRegistry)
		metricsStore := new(mockMetricsStore)
		session := createMockSession(registry)
		session.metricsStore = metricsStore

		ctx := context.Background()
		since := time.Now().Add(-time.Hour)

		comp := new(mockComponent)
		emptyMetrics := pkgmetrics.Metrics{}

		registry.On("Get", "empty-comp").Return(comp)
		metricsStore.On("Read", ctx, mock.Anything).Return(emptyMetrics, nil)

		result := session.getMetricsFromComponent(ctx, "empty-comp", since)

		assert.Equal(t, "empty-comp", result.Component)
		assert.Empty(t, result.Metrics)

		registry.AssertExpectations(t)
		metricsStore.AssertExpectations(t)
	})
}

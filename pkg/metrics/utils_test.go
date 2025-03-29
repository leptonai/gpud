package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/leptonai/gpud/api/v1"
)

func TestConvertToLeptonMetrics_Empty(t *testing.T) {
	ms := Metrics{}
	result := ConvertToLeptonMetrics(ms)
	assert.Empty(t, result, "empty metrics should return empty result")
}

func TestConvertToLeptonMetrics_EmptyComponent(t *testing.T) {
	// Metrics with empty component should be skipped
	ms := Metrics{
		{
			UnixMilliseconds: 1000,
			Component:        "", // Empty component
			Name:             "metric1",
			Value:            10.5,
		},
		{
			UnixMilliseconds: 2000,
			Component:        "component1",
			Name:             "metric2",
			Value:            20.5,
		},
	}

	result := ConvertToLeptonMetrics(ms)
	require.Len(t, result, 1, "should have only one component metrics")
	assert.Equal(t, "component1", result[0].Component)
	require.Len(t, result[0].Metrics, 1, "should have one metric")
	assert.Equal(t, "metric2", result[0].Metrics[0].Metric.MetricName)
	assert.Equal(t, int64(2000), result[0].Metrics[0].Metric.UnixSeconds)
	assert.Equal(t, 20.5, result[0].Metrics[0].Metric.Value)
}

func TestConvertToLeptonMetrics_SingleComponent(t *testing.T) {
	now := time.Now().UnixMilli()

	ms := Metrics{
		{
			UnixMilliseconds: now,
			Component:        "component1",
			Name:             "metric1",
			Value:            10.5,
		},
		{
			UnixMilliseconds: now + 1000,
			Component:        "component1",
			Name:             "metric2",
			Label:            "gpu0",
			Value:            20.5,
		},
	}

	result := ConvertToLeptonMetrics(ms)
	require.Len(t, result, 1, "should have one component metrics")
	assert.Equal(t, "component1", result[0].Component)
	require.Len(t, result[0].Metrics, 2, "should have two metrics")

	// Check first metric
	assert.Equal(t, "metric1", result[0].Metrics[0].Metric.MetricName)
	assert.Equal(t, "", result[0].Metrics[0].Metric.MetricSecondaryName)
	assert.Equal(t, now, result[0].Metrics[0].Metric.UnixSeconds)
	assert.Equal(t, 10.5, result[0].Metrics[0].Metric.Value)

	// Check second metric
	assert.Equal(t, "metric2", result[0].Metrics[1].Metric.MetricName)
	assert.Equal(t, "gpu0", result[0].Metrics[1].Metric.MetricSecondaryName)
	assert.Equal(t, now+1000, result[0].Metrics[1].Metric.UnixSeconds)
	assert.Equal(t, 20.5, result[0].Metrics[1].Metric.Value)
}

func TestConvertToLeptonMetrics_MultipleComponents(t *testing.T) {
	now := time.Now().UnixMilli()

	ms := Metrics{
		{
			UnixMilliseconds: now,
			Component:        "component1",
			Name:             "metric1",
			Value:            10.5,
		},
		{
			UnixMilliseconds: now + 1000,
			Component:        "component2",
			Name:             "metric2",
			Label:            "gpu0",
			Value:            20.5,
		},
		{
			UnixMilliseconds: now + 2000,
			Component:        "component1",
			Name:             "metric3",
			Label:            "gpu1",
			Value:            30.5,
		},
	}

	result := ConvertToLeptonMetrics(ms)
	require.Len(t, result, 2, "should have two component metrics")

	// Components may be in any order, so find them by name
	var comp1, comp2 v1.LeptonComponentMetrics
	for _, c := range result {
		if c.Component == "component1" {
			comp1 = c
		} else if c.Component == "component2" {
			comp2 = c
		}
	}

	// Check component1
	assert.Equal(t, "component1", comp1.Component)
	require.Len(t, comp1.Metrics, 2, "component1 should have two metrics")

	// Check metrics are sorted by timestamp
	assert.Equal(t, now, comp1.Metrics[0].Metric.UnixSeconds)
	assert.Equal(t, "metric1", comp1.Metrics[0].Metric.MetricName)
	assert.Equal(t, 10.5, comp1.Metrics[0].Metric.Value)

	assert.Equal(t, now+2000, comp1.Metrics[1].Metric.UnixSeconds)
	assert.Equal(t, "metric3", comp1.Metrics[1].Metric.MetricName)
	assert.Equal(t, "gpu1", comp1.Metrics[1].Metric.MetricSecondaryName)
	assert.Equal(t, 30.5, comp1.Metrics[1].Metric.Value)

	// Check component2
	assert.Equal(t, "component2", comp2.Component)
	require.Len(t, comp2.Metrics, 1, "component2 should have one metric")
	assert.Equal(t, "metric2", comp2.Metrics[0].Metric.MetricName)
	assert.Equal(t, "gpu0", comp2.Metrics[0].Metric.MetricSecondaryName)
	assert.Equal(t, now+1000, comp2.Metrics[0].Metric.UnixSeconds)
	assert.Equal(t, 20.5, comp2.Metrics[0].Metric.Value)
}

func TestConvertToLeptonMetrics_SortingByTimestamp(t *testing.T) {
	// Test that metrics within a component are sorted by timestamp
	now := time.Now().UnixMilli()

	// Create metrics with unsorted timestamps
	ms := Metrics{
		{
			UnixMilliseconds: now + 2000, // Third timestamp
			Component:        "component1",
			Name:             "metric3",
			Value:            30.5,
		},
		{
			UnixMilliseconds: now, // First timestamp
			Component:        "component1",
			Name:             "metric1",
			Value:            10.5,
		},
		{
			UnixMilliseconds: now + 1000, // Second timestamp
			Component:        "component1",
			Name:             "metric2",
			Value:            20.5,
		},
	}

	result := ConvertToLeptonMetrics(ms)
	require.Len(t, result, 1, "should have one component metrics")
	require.Len(t, result[0].Metrics, 3, "should have three metrics")

	// Verify metrics are sorted by timestamp
	assert.Equal(t, now, result[0].Metrics[0].Metric.UnixSeconds, "first metric should have earliest timestamp")
	assert.Equal(t, now+1000, result[0].Metrics[1].Metric.UnixSeconds, "second metric should have middle timestamp")
	assert.Equal(t, now+2000, result[0].Metrics[2].Metric.UnixSeconds, "third metric should have latest timestamp")
}

package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
	assert.Equal(t, int64(2000), result[0].Metrics[0].UnixSeconds)
	assert.Equal(t, 20.5, result[0].Metrics[0].Value)
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
			Value:            20.5,
			Labels:           map[string]string{"label": "gpu0"},
		},
	}

	result := ConvertToLeptonMetrics(ms)
	require.Len(t, result, 1, "should have one component metrics")
	assert.Equal(t, "component1", result[0].Component)
	require.Len(t, result[0].Metrics, 2, "should have two metrics")

	// Check first metric
	assert.Equal(t, "metric1", result[0].Metrics[0].Name)
	assert.Nil(t, result[0].Metrics[0].Labels)
	assert.Equal(t, now, result[0].Metrics[0].UnixSeconds)
	assert.Equal(t, 10.5, result[0].Metrics[0].Value)

	// Check second metric
	assert.Equal(t, "metric2", result[0].Metrics[1].Name)
	assert.Equal(t, "gpu0", result[0].Metrics[1].Labels["label"])
	assert.Equal(t, now+1000, result[0].Metrics[1].UnixSeconds)
	assert.Equal(t, 20.5, result[0].Metrics[1].Value)
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
			Value:            20.5,
			Labels:           map[string]string{"label": "gpu0"},
		},
		{
			UnixMilliseconds: now + 2000,
			Component:        "component1",
			Name:             "metric3",
			Value:            30.5,
			Labels:           map[string]string{"label": "gpu1"},
		},
	}

	result := ConvertToLeptonMetrics(ms)
	require.Len(t, result, 2, "should have two component metrics")

	// Components may be in any order, so find them by name
	var comp1, comp2 v1.ComponentMetrics
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
	assert.Equal(t, now, comp1.Metrics[0].UnixSeconds)
	assert.Equal(t, "metric1", comp1.Metrics[0].Name)
	assert.Equal(t, 10.5, comp1.Metrics[0].Value)

	assert.Equal(t, now+2000, comp1.Metrics[1].UnixSeconds)
	assert.Equal(t, "metric3", comp1.Metrics[1].Name)
	assert.Equal(t, "gpu1", comp1.Metrics[1].Labels["label"])
	assert.Equal(t, 30.5, comp1.Metrics[1].Value)

	// Check component2
	assert.Equal(t, "component2", comp2.Component)
	require.Len(t, comp2.Metrics, 1, "component2 should have one metric")
	assert.Equal(t, "metric2", comp2.Metrics[0].Name)
	assert.Equal(t, "gpu0", comp2.Metrics[0].Labels["label"])
	assert.Equal(t, now+1000, comp2.Metrics[0].UnixSeconds)
	assert.Equal(t, 20.5, comp2.Metrics[0].Value)
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
	assert.Equal(t, now, result[0].Metrics[0].UnixSeconds, "first metric should have earliest timestamp")
	assert.Equal(t, now+1000, result[0].Metrics[1].UnixSeconds, "second metric should have middle timestamp")
	assert.Equal(t, now+2000, result[0].Metrics[2].UnixSeconds, "third metric should have latest timestamp")
}

func TestNormalizeComponentNameToMetricSubsystem(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Lowercase string with no special characters",
			input:    "component",
			expected: "component",
		},
		{
			name:     "Uppercase string",
			input:    "COMPONENT",
			expected: "component",
		},
		{
			name:     "Mixed case string",
			input:    "ComPoNeNt",
			expected: "component",
		},
		{
			name:     "String with hyphens",
			input:    "nvidia-gpu",
			expected: "nvidia_gpu",
		},
		{
			name:     "String with dots",
			input:    "nvidia.gpu",
			expected: "nvidia_gpu",
		},
		{
			name:     "String with slashes",
			input:    "nvidia/gpu",
			expected: "nvidia_gpu",
		},
		{
			name:     "String with all special characters",
			input:    "Com-Po.Ne/Nt",
			expected: "com_po_ne_nt",
		},
		{
			name:     "Complex component path",
			input:    "accelerator/nvidia/clock-speed",
			expected: "accelerator_nvidia_clock_speed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := NormalizeComponentNameToMetricSubsystem(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_registerHealthStateMetrics(t *testing.T) {
	// Create a mock registry using testutil helper
	registry := prometheus.NewRegistry()
	componentName := "test-component"

	// Call registerHealthStateMetrics
	setter, err := RegisterHealthStateMetricsWithRegisterer(registry, componentName)
	require.NoError(t, err)
	require.NotNil(t, setter)

	// Test setter functionality
	setter.Set(v1.HealthStateTypeHealthy)
	setter.Set(v1.HealthStateTypeUnhealthy)
	setter.Set(v1.HealthStateTypeDegraded)

	// Call the setter multiple times to verify it works
	setter.Set(v1.HealthStateTypeHealthy)
	setter.Set(v1.HealthStateTypeUnhealthy)
	setter.Set(v1.HealthStateTypeDegraded)

	// Success if no panics occurred
}

func Test_registerHealthStateMetrics_RegistrationError(t *testing.T) {
	registry := prometheus.NewRegistry()
	componentName := "test-component"

	// First registration should succeed
	_, err := RegisterHealthStateMetricsWithRegisterer(registry, componentName)
	require.NoError(t, err)

	// Second registration with same name should fail
	_, err = RegisterHealthStateMetricsWithRegisterer(registry, componentName)
	require.Error(t, err)
}

func Test_registerHealthStateMetrics_MultipleComponents(t *testing.T) {
	registry := prometheus.NewRegistry()

	// Register for two different components
	component1 := "component1"
	setter1, err := RegisterHealthStateMetricsWithRegisterer(registry, component1)
	require.NoError(t, err)
	require.NotNil(t, setter1)

	component2 := "component2"
	setter2, err := RegisterHealthStateMetricsWithRegisterer(registry, component2)
	require.NoError(t, err)
	require.NotNil(t, setter2)

	// Test setters for both components
	setter1.Set(v1.HealthStateTypeHealthy)
	setter2.Set(v1.HealthStateTypeUnhealthy)

	// Success if no panics occurred
}

func Test_registerHealthStateMetrics_ErrorOnSecondMetric(t *testing.T) {
	// Create a test registry with a collector that will
	// only allow one registration and then return an error
	registry := prometheus.NewRegistry()
	componentName := "test-component"

	// Create a custom registerer that will allow only the first registration
	customReg := &limitedRegisterer{
		Registry:                registry,
		maxRegistrationsAllowed: 1,
	}

	// Now when we call registerHealthStateMetrics, it should fail on the second metric
	_, err := RegisterHealthStateMetricsWithRegisterer(customReg, componentName)
	require.Error(t, err)
}

func Test_registerHealthStateMetrics_ErrorOnThirdMetric(t *testing.T) {
	// Create a test registry with a collector that will
	// only allow two registrations and then return an error
	registry := prometheus.NewRegistry()
	componentName := "test-component"

	// Create a custom registerer that will allow only the first and second registrations
	customReg := &limitedRegisterer{
		Registry:                registry,
		maxRegistrationsAllowed: 2,
	}

	// Now when we call registerHealthStateMetrics, it should fail on the third metric
	_, err := RegisterHealthStateMetricsWithRegisterer(customReg, componentName)
	require.Error(t, err)
}

// limitedRegisterer is a custom registerer that allows only a fixed number of registrations
type limitedRegisterer struct {
	Registry                *prometheus.Registry
	registrationCount       int
	maxRegistrationsAllowed int
}

func (r *limitedRegisterer) Register(c prometheus.Collector) error {
	r.registrationCount++
	if r.registrationCount > r.maxRegistrationsAllowed {
		return fmt.Errorf("registration limit reached")
	}
	return r.Registry.Register(c)
}

func (r *limitedRegisterer) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		if err := r.Register(c); err != nil {
			panic(err)
		}
	}
}

func (r *limitedRegisterer) Unregister(c prometheus.Collector) bool {
	return r.Registry.Unregister(c)
}

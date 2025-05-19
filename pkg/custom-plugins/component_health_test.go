package customplugins

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// TestHealthStateSetterInitialization tests the initialization of the healthStateSetter in NewInitFunc
func TestHealthStateSetterInitialization(t *testing.T) {
	spec := &Spec{
		PluginName: "test-plugin",
		Timeout: metav1.Duration{
			Duration: time.Second * 10,
		},
	}

	initFunc := spec.NewInitFunc()
	require.NotNil(t, initFunc)

	rootCtx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx: rootCtx,
	}

	// Call the init function to create a component
	comp, err := initFunc(gpudInstance)
	require.NoError(t, err)
	require.NotNil(t, comp)

	// Assert the component is of the expected type
	c, ok := comp.(*component)
	require.True(t, ok)

	// Verify the healthStateSetter was initialized
	assert.NotNil(t, c.healthStateSetter)
}

// TestComponentCheck_ValidHealthStateMetrics tests that valid health state metrics are passed to RegisterHealthStateMetrics
func TestComponentCheck_ValidHealthStateMetrics(t *testing.T) {
	// This test is a more realistic test using the actual pkgmetrics package,
	// rather than mocks

	// Create a spec and register health state metrics
	spec := &Spec{
		PluginName: "test-plugin",
	}

	// Register metrics with a custom registry
	registry := prometheus.NewRegistry()
	healthStateSetter, err := pkgmetrics.RegisterHealthStateMetricsWithRegisterer(registry, spec.ComponentName())
	require.NoError(t, err)
	require.NotNil(t, healthStateSetter)

	// Create a component with the real healthStateSetter
	c := &component{
		ctx:               context.Background(),
		spec:              spec,
		healthStateSetter: healthStateSetter,
	}

	// Run a check which should set some health metrics
	result := c.Check()
	require.NotNil(t, result)

	// Verify metrics were registered by gathering them from the registry
	metrics, err := registry.Gather()
	require.NoError(t, err)

	// There should be three metrics for healthy, unhealthy, and degraded
	foundMetrics := 0
	for _, m := range metrics {
		if m.GetName() == pkgmetrics.NormalizeComponentNameToMetricSubsystem(spec.ComponentName())+"_health_state_healthy" {
			foundMetrics++
			// Since the component should be healthy, the value should be 1
			require.Equal(t, float64(1), m.GetMetric()[0].GetGauge().GetValue())
		}
		if m.GetName() == pkgmetrics.NormalizeComponentNameToMetricSubsystem(spec.ComponentName())+"_health_state_unhealthy" {
			foundMetrics++
			// Since the component should not be unhealthy, the value should be 0
			require.Equal(t, float64(0), m.GetMetric()[0].GetGauge().GetValue())
		}
		if m.GetName() == pkgmetrics.NormalizeComponentNameToMetricSubsystem(spec.ComponentName())+"_health_state_degraded" {
			foundMetrics++
			// Since the component should not be degraded, the value should be 0
			require.Equal(t, float64(0), m.GetMetric()[0].GetGauge().GetValue())
		}
	}
	require.Equal(t, 3, foundMetrics, "Expected to find three health state metrics")
}

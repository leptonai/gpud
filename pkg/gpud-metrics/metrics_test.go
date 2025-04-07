package gpudmetrics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

// SimpleComponent is a basic implementation of components.Component for testing
type SimpleComponent struct {
	name      string
	states    []components.State
	statesErr error
}

func NewSimpleComponent(name string, healthy bool) *SimpleComponent {
	state := components.State{Name: name, Healthy: healthy}
	return &SimpleComponent{
		name:   name,
		states: []components.State{state},
	}
}

func (c *SimpleComponent) Name() string {
	return c.name
}

func (c *SimpleComponent) Start() error {
	return nil
}

func (c *SimpleComponent) States(ctx context.Context) ([]components.State, error) {
	return c.states, c.statesErr
}

func (c *SimpleComponent) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *SimpleComponent) Close() error {
	return nil
}

// SetHealthStatus changes the component's health state
func (c *SimpleComponent) SetHealthStatus(healthy bool) {
	for i := range c.states {
		c.states[i].Healthy = healthy
	}
}

// SetError sets an error to be returned by States
func (c *SimpleComponent) SetError(err error) {
	c.statesErr = err
}

// ErrorGatherer implements prometheus.Gatherer but always returns an error
type ErrorGatherer struct{}

func (g *ErrorGatherer) Gather() ([]*dto.MetricFamily, error) {
	return nil, fmt.Errorf("simulated gather error")
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name        string
		registerFn  func() error
		expectError bool
	}{
		{
			name: "first registration succeeds",
			registerFn: func() error {
				registry := prometheus.NewRegistry()
				return Register(registry)
			},
			expectError: false,
		},
		{
			name: "duplicate registration fails",
			registerFn: func() error {
				registry := prometheus.NewRegistry()
				if err := Register(registry); err != nil {
					return err
				}
				return Register(registry)
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.registerFn()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSetAndReadFunctions tests all the Set and Read functions
func TestSetAndReadFunctions(t *testing.T) {
	registry := prometheus.NewRegistry()
	err := Register(registry)
	require.NoError(t, err)

	// Test component registration with unique component names
	SetRegistered("test_set_1")
	SetRegistered("test_set_2")
	SetRegistered("test_set_3")

	total, err := ReadRegisteredTotal(registry)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), total, "Should have 3 registered components")

	// Test healthy/unhealthy behavior
	SetHealthy("test_set_1")
	SetHealthy("test_set_2")
	SetUnhealthy("test_set_3")

	healthyTotal, err := ReadHealthyTotal(registry)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), healthyTotal, "Should have 2 healthy components")

	unhealthyTotal, err := ReadUnhealthyTotal(registry)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), unhealthyTotal, "Should have 1 unhealthy component")

	// Test setting component from healthy to unhealthy
	SetUnhealthy("test_set_1")

	healthyTotal, err = ReadHealthyTotal(registry)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), healthyTotal, "Should have 1 healthy component")

	unhealthyTotal, err = ReadUnhealthyTotal(registry)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), unhealthyTotal, "Should have 2 unhealthy components")
}

func TestReadFunctionsWithErrorGatherer(t *testing.T) {
	errorGatherer := &ErrorGatherer{}

	// Test all read functions with error gatherer
	_, err := ReadRegisteredTotal(errorGatherer)
	assert.Error(t, err)

	_, err = ReadHealthyTotal(errorGatherer)
	assert.Error(t, err)

	_, err = ReadUnhealthyTotal(errorGatherer)
	assert.Error(t, err)
}

// TestRegisterNilRegistry tests that Register panics with a nil registry
// This behavior is controlled by the prometheus library, not our code
func TestRegisterNilRegistry(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r, "Expected a panic when registering with nil registry")
	}()

	// This should panic
	_ = Register(nil)

	// If we get here, the test has failed
	t.Fatal("Expected panic did not occur")
}

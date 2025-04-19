package components

import (
	"context"
	"fmt"
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockComponent implements the Component interface for testing
type mockComponent struct {
	name string
}

func newMockComponent(name string) Component {
	return &mockComponent{name: name}
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Start() error {
	return nil
}

func (m *mockComponent) Check() CheckResult {
	return &mockCheckResult{}
}

func (m *mockComponent) LastHealthStates() apiv1.HealthStates {
	return apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy},
	}
}

func (m *mockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return apiv1.Events{}, nil
}

func (m *mockComponent) Close() error {
	return nil
}

// mockCheckResult implements the CheckResult interface for testing
type mockCheckResult struct{}

func (r *mockCheckResult) String() string {
	return "mock check result"
}

func (r *mockCheckResult) Summary() string {
	return "mock summary"
}

func (r *mockCheckResult) HealthState() apiv1.HealthStateType {
	return apiv1.HealthStateTypeHealthy
}

// Mock function that returns a component without error
func mockInitFuncSuccess(instance *GPUdInstance) (Component, error) {
	return newMockComponent("test-component"), nil
}

// Mock function that returns an error
func mockInitFuncError(instance *GPUdInstance) (Component, error) {
	return nil, fmt.Errorf("mock init error")
}

func TestHasRegistered(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// When registry is empty, should return false
	assert.False(t, reg.hasRegistered("test-component"))

	// Register a component
	comp := newMockComponent("test-component")
	reg.mu.Lock()
	reg.components["test-component"] = comp
	reg.mu.Unlock()

	// Should now return true for the registered component
	assert.True(t, reg.hasRegistered("test-component"))

	// Should still return false for unregistered components
	assert.False(t, reg.hasRegistered("unknown-component"))
}

func TestRegisterInitFunc(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Test registering a component successfully
	err := reg.registerInit(mockInitFuncSuccess)
	assert.NoError(t, err)
	assert.True(t, reg.hasRegistered("test-component"))

	// Test registering a component that already exists
	err = reg.registerInit(mockInitFuncSuccess)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Test registering a component with an initialization function that returns an error
	err = reg.registerInit(mockInitFuncError)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock init error")

	// The component should not be registered if the init function fails
	assert.False(t, reg.hasRegistered("error-component"))
}

func TestAll(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Test empty registry
	components := reg.All()
	assert.Empty(t, components, "Expected empty slice for empty registry")

	// Register multiple components with different names
	componentA := newMockComponent("component-a")
	componentC := newMockComponent("component-c")
	componentB := newMockComponent("component-b")

	// Add components directly to the registry
	reg.mu.Lock()
	reg.components["component-a"] = componentA
	reg.components["component-c"] = componentC
	reg.components["component-b"] = componentB
	reg.mu.Unlock()

	// Test that All returns all components in alphabetical order
	components = reg.All()
	assert.Len(t, components, 3, "Expected 3 components")

	// Verify components are sorted alphabetically by name
	assert.Equal(t, "component-a", components[0].Name())
	assert.Equal(t, "component-b", components[1].Name())
	assert.Equal(t, "component-c", components[2].Name())
}

func TestMustRegister(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})

	// Test successful registration
	require.NotPanics(t, func() {
		r.MustRegister(func(instance *GPUdInstance) (Component, error) {
			return newMockComponent("must-register-component"), nil
		})
	})

	// Test that the component was registered
	component := r.Get("must-register-component")
	assert.NotNil(t, component)
	assert.Equal(t, "must-register-component", component.Name())

	// Test panic on registration error
	require.Panics(t, func() {
		r.MustRegister(func(instance *GPUdInstance) (Component, error) {
			return nil, fmt.Errorf("initialization error")
		})
	})

	// Test panic on duplicate registration
	require.Panics(t, func() {
		r.MustRegister(func(instance *GPUdInstance) (Component, error) {
			return newMockComponent("must-register-component"), nil
		})
	})
}

func TestGet(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Test getting a component that doesn't exist
	component := r.Get("non-existent")
	assert.Nil(t, component, "Expected nil for non-existent component")

	// Register a component
	expectedComponent := newMockComponent("get-test-component")
	reg.mu.Lock()
	reg.components["get-test-component"] = expectedComponent
	reg.mu.Unlock()

	// Test getting a component that exists
	component = r.Get("get-test-component")
	assert.NotNil(t, component, "Expected non-nil for existing component")
	assert.Equal(t, "get-test-component", component.Name())
	assert.Equal(t, expectedComponent, component)
}

func TestListAll(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Test empty registry
	components := reg.listAll()
	assert.Empty(t, components, "Expected empty slice for empty registry")

	// Register components
	compA := newMockComponent("comp-a")
	compB := newMockComponent("comp-b")
	reg.mu.Lock()
	reg.components["comp-a"] = compA
	reg.components["comp-b"] = compB
	reg.mu.Unlock()

	// Test that listAll returns all components (order not guaranteed)
	components = reg.listAll()
	assert.Len(t, components, 2, "Expected 2 components")

	// Check that both components are present (regardless of order)
	names := make(map[string]bool)
	for _, comp := range components {
		names[comp.Name()] = true
	}
	assert.True(t, names["comp-a"])
	assert.True(t, names["comp-b"])
}

func TestNewRegistry(t *testing.T) {
	// Create a test instance
	instance := &GPUdInstance{RootCtx: context.Background()}

	// Create a new registry
	r := NewRegistry(instance)

	// Assert that the registry is not nil
	assert.NotNil(t, r)

	// Assert that the registry is of type *registry
	_, ok := r.(*registry)
	assert.True(t, ok)

	// Cast to registry to check internal state
	reg := r.(*registry)

	// Check that the gpudInstance is set correctly
	assert.Equal(t, instance, reg.gpudInstance)

	// Check that the components map is initialized and empty
	assert.NotNil(t, reg.components)
	assert.Empty(t, reg.components)
}

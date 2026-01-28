package components

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// mockComponent implements the Component interface for testing
type mockComponent struct {
	name string
	tags []string
}

func newMockComponent(name string) Component {
	return &mockComponent{name: name}
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Tags() []string {
	return m.tags
}

func (m *mockComponent) IsSupported() bool {
	return true
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

func (r *mockCheckResult) ComponentName() string {
	return "mock-component"
}

func (r *mockCheckResult) String() string {
	return "mock check result"
}

func (r *mockCheckResult) Summary() string {
	return "mock summary"
}

func (r *mockCheckResult) HealthStateType() apiv1.HealthStateType {
	return apiv1.HealthStateTypeHealthy
}

func (r *mockCheckResult) HealthStates() apiv1.HealthStates {
	return apiv1.HealthStates{
		{Health: apiv1.HealthStateTypeHealthy},
	}
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
	_, err := reg.Register(mockInitFuncSuccess)
	assert.NoError(t, err)
	assert.True(t, reg.hasRegistered("test-component"))

	// Test registering a component that already exists
	_, err = reg.Register(mockInitFuncSuccess)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Test registering a component with an initialization function that returns an error
	_, err = reg.Register(mockInitFuncError)
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

func TestDeregister(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Test deregistering a component that doesn't exist
	component := r.Deregister("non-existent")
	assert.Nil(t, component, "Expected nil for deregistering non-existent component")

	// Register components for testing
	compA := newMockComponent("comp-a")
	compB := newMockComponent("comp-b")
	reg.mu.Lock()
	reg.components["comp-a"] = compA
	reg.components["comp-b"] = compB
	reg.mu.Unlock()

	// Test deregistering an existing component
	component = r.Deregister("comp-a")
	assert.NotNil(t, component, "Expected non-nil for deregistering existing component")
	assert.Equal(t, "comp-a", component.Name())

	// Verify component was removed
	assert.Nil(t, r.Get("comp-a"), "Component should be removed after deregistering")
	assert.NotNil(t, r.Get("comp-b"), "Other component should remain registered")

	// Test deregistering the same component again (should be safe)
	component = r.Deregister("comp-a")
	assert.Nil(t, component, "Expected nil for deregistering already removed component")
}

func TestRegisterMetrics(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})

	// Create a test component registration function
	compName := "metrics-test-component"
	initFunc := func(instance *GPUdInstance) (Component, error) {
		return newMockComponent(compName), nil
	}

	// Register the component
	_, err := r.(*registry).Register(initFunc)
	assert.NoError(t, err)

	// Verify the component was registered in metrics
	// This is a basic verification that the code path is executed
	// A more thorough test would need to mock the metrics package
	assert.True(t, r.(*registry).hasRegistered(compName))
}

func TestConcurrentRegistryOperations(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})

	// Number of concurrent operations
	concurrency := 10

	// Use wait groups to coordinate goroutines
	var wg sync.WaitGroup
	wg.Add(concurrency * 3) // register, get, and deregister operations

	// Create a unique component for each goroutine
	for i := 0; i < concurrency; i++ {
		compName := fmt.Sprintf("concurrent-comp-%d", i)

		// Test concurrent registration
		go func(name string) {
			defer wg.Done()
			initFunc := func(instance *GPUdInstance) (Component, error) {
				return newMockComponent(name), nil
			}
			_, err := r.(*registry).Register(initFunc)
			assert.NoError(t, err)
		}(compName)

		// Test concurrent get operations
		go func(name string) {
			defer wg.Done()
			// Try getting the component multiple times
			for j := 0; j < 5; j++ {
				comp := r.Get(name)
				if comp != nil {
					assert.Equal(t, name, comp.Name())
				}
				// Small sleep to increase likelihood of concurrent access
				time.Sleep(time.Millisecond)
			}
		}(compName)

		// Test concurrent deregistration
		go func(name string) {
			defer wg.Done()
			// Wait a bit to ensure the component has time to be registered
			time.Sleep(5 * time.Millisecond)
			comp := r.Deregister(name)
			if comp != nil {
				assert.Equal(t, name, comp.Name())
			}
		}(compName)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Verify final state
	// Some components may or may not be registered depending on the race between
	// registration and deregistration
	components := r.All()
	t.Logf("Final number of components: %d", len(components))
}

func TestRegistryErrorCases(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Test case 1: Init function returns error
	errorMsg := "initialization failed error"
	initFuncWithError := func(instance *GPUdInstance) (Component, error) {
		return nil, fmt.Errorf("%s", errorMsg)
	}

	_, err := reg.Register(initFuncWithError)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), errorMsg)

	// Test case 2: Duplicate component registration
	compName := "duplicate-component"
	initFunc := func(instance *GPUdInstance) (Component, error) {
		return newMockComponent(compName), nil
	}

	// First registration should succeed
	_, err = reg.Register(initFunc)
	assert.NoError(t, err)
	assert.True(t, reg.hasRegistered(compName))

	// Second registration with same name should fail
	_, err = reg.Register(initFunc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Test case 3: Check that MustRegister panics with appropriate error message
	panicked := false
	panicMsg := ""

	defer func() {
		if r := recover(); r != nil {
			panicked = true
			panicMsg = fmt.Sprintf("%v", r)
		}
	}()

	r.MustRegister(initFunc)

	// Verify panic occurred with expected message
	assert.True(t, panicked, "MustRegister should have panicked")
	assert.Contains(t, panicMsg, "already registered")
}

func TestRegisterAlreadyRegisteredError(t *testing.T) {
	// Create a new registry
	r := NewRegistry(&GPUdInstance{
		RootCtx: context.Background(),
	})
	reg := r.(*registry)

	// Create a test component name and init function
	compName := "already-registered-test-component"
	initFunc := func(instance *GPUdInstance) (Component, error) {
		return newMockComponent(compName), nil
	}

	// First registration should succeed
	comp1, err := reg.Register(initFunc)
	assert.NoError(t, err)
	assert.NotNil(t, comp1)
	assert.Equal(t, compName, comp1.Name())
	assert.True(t, reg.hasRegistered(compName))

	// Second registration with same name should fail with ErrAlreadyRegistered
	comp2, err := reg.Register(initFunc)
	assert.Error(t, err)
	assert.Nil(t, comp2)

	// Verify that the error is wrapped with ErrAlreadyRegistered using errors.Is
	assert.True(t, errors.Is(err, ErrAlreadyRegistered),
		"Expected error to be or wrap ErrAlreadyRegistered, got: %v", err)

	// Also verify the error message contains component name
	assert.Contains(t, err.Error(), compName)
	assert.Contains(t, err.Error(), "already registered")
}

// Test FailureInjector struct
func TestFailureInjector(t *testing.T) {
	// Test empty FailureInjector
	emptyInjector := &FailureInjector{}
	assert.Empty(t, emptyInjector.GPUUUIDsWithRowRemappingPending)
	assert.Empty(t, emptyInjector.GPUUUIDsWithRowRemappingFailed)

	// Test FailureInjector with values
	pendingUUIDs := []string{"GPU-pending-1", "GPU-pending-2"}
	failedUUIDs := []string{"GPU-failed-1", "GPU-failed-2"}

	injector := &FailureInjector{
		GPUUUIDsWithRowRemappingPending: pendingUUIDs,
		GPUUUIDsWithRowRemappingFailed:  failedUUIDs,
	}

	assert.Equal(t, pendingUUIDs, injector.GPUUUIDsWithRowRemappingPending)
	assert.Equal(t, failedUUIDs, injector.GPUUUIDsWithRowRemappingFailed)
	assert.Len(t, injector.GPUUUIDsWithRowRemappingPending, 2)
	assert.Len(t, injector.GPUUUIDsWithRowRemappingFailed, 2)
}

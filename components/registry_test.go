package components

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mock function that returns a component without error
func mockInitFuncSuccess(instance GPUdInstance) (Component, error) {
	return newMockComponent("test-component"), nil
}

// Mock function that returns an error
func mockInitFuncError(instance GPUdInstance) (Component, error) {
	return nil, fmt.Errorf("mock init error")
}

func TestHasRegistered(t *testing.T) {
	// Create a new registry
	reg := NewRegistry(GPUdInstance{
		RootCtx: context.Background(),
	})

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
	reg := NewRegistry(GPUdInstance{
		RootCtx: context.Background(),
	})

	// Test registering a component successfully
	err := reg.registerInit("test-component", mockInitFuncSuccess)
	assert.NoError(t, err)
	assert.True(t, reg.hasRegistered("test-component"))

	// Test registering a component that already exists
	err = reg.registerInit("test-component", mockInitFuncSuccess)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Test registering a component with an initialization function that returns an error
	err = reg.registerInit("error-component", mockInitFuncError)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock init error")

	// The component should not be registered if the init function fails
	assert.False(t, reg.hasRegistered("error-component"))
}

func TestAll(t *testing.T) {
	// Create a new registry
	reg := NewRegistry(GPUdInstance{
		RootCtx: context.Background(),
	})

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

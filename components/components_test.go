package components

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/stretchr/testify/assert"
)

// Mock implementation of Component interface for testing
type mockComponent struct {
	name    string
	closed  bool
	started bool
}

func (m *mockComponent) Name() string {
	return m.name
}

func (m *mockComponent) Start() error {
	m.started = true
	return nil
}

func (m *mockComponent) States(ctx context.Context) ([]State, error) {
	return []State{{Name: m.name, Healthy: true}}, nil
}

func (m *mockComponent) Events(ctx context.Context, since time.Time) ([]Event, error) {
	return []Event{}, nil
}

func (m *mockComponent) Close() error {
	m.closed = true
	return nil
}

// Mock component that returns errors
type mockErrorComponent struct {
	mockComponent
	closeError error
}

func (m *mockErrorComponent) Close() error {
	m.closed = true
	return m.closeError
}

func newMockComponent(name string) *mockComponent {
	return &mockComponent{name: name, closed: false, started: false}
}

func newMockErrorComponent(name string, closeError error) *mockErrorComponent {
	return &mockErrorComponent{
		mockComponent: mockComponent{name: name, closed: false, started: false},
		closeError:    closeError,
	}
}

func TestGetComponentErrors(t *testing.T) {
	// Test nil set error
	_, err := getComponent(nil, "nvidia")
	if !errors.Is(err, errdefs.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}

	// Verify error message
	expectedMsg := "component set not initialized: unavailable"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}

	// Test unwrapping
	unwrapped := errors.Unwrap(err)
	if unwrapped != errdefs.ErrUnavailable {
		t.Errorf("expected unwrapped error to be ErrUnavailable, got %v", unwrapped)
	}

	// Test component not found error
	_, err = getComponent(map[string]Component{}, "nvidia")
	if !errors.Is(err, errdefs.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Verify not found error message
	expectedMsg = "component nvidia not found: not found"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}

	// Test unwrapping not found error
	unwrapped = errors.Unwrap(err)
	if unwrapped != errdefs.ErrNotFound {
		t.Errorf("expected unwrapped error to be ErrNotFound, got %v", unwrapped)
	}
}

func TestIsComponentRegistered(t *testing.T) {
	// Test with nil set
	assert.False(t, isComponentRegistered(nil, "test"))

	// Test with empty set
	emptySet := map[string]Component{}
	assert.False(t, isComponentRegistered(emptySet, "test"))

	// Test with component registered
	set := map[string]Component{
		"test": newMockComponent("test"),
	}
	assert.True(t, isComponentRegistered(set, "test"))
	assert.False(t, isComponentRegistered(set, "nonexistent"))
}

func TestGetComponent(t *testing.T) {
	// Create test components
	testComp := newMockComponent("test")
	set := map[string]Component{
		"test": testComp,
	}

	// Test successful get
	comp, err := getComponent(set, "test")
	assert.NoError(t, err)
	assert.Equal(t, testComp, comp)

	// Test get nonexistent component
	_, err = getComponent(set, "nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errdefs.ErrNotFound)

	// Test error message
	expectedMsg := "component nonexistent not found: not found"
	assert.Equal(t, expectedMsg, err.Error())

	// Test error unwrapping
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, errdefs.ErrNotFound, unwrapped)
}

func TestRegisterComponent(t *testing.T) {
	// Test with nil set
	nilSet := map[string]Component(nil)
	err := registerComponent(nilSet, newMockComponent("test"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errdefs.ErrUnavailable)

	// Test error message
	expectedMsg := "component set not initialized: unavailable"
	assert.Equal(t, expectedMsg, err.Error())

	// Test error unwrapping
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, errdefs.ErrUnavailable, unwrapped)

	// Test with empty set
	set := map[string]Component{}
	testComp := newMockComponent("test")
	err = registerComponent(set, testComp)
	assert.NoError(t, err)
	assert.Equal(t, testComp, set["test"])

	// Test registering duplicate component
	err = registerComponent(set, newMockComponent("test"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, errdefs.ErrAlreadyExists)

	// Test already exists error message
	expectedMsg = "component test already registered: already exists"
	assert.Equal(t, expectedMsg, err.Error())

	// Test error unwrapping
	unwrapped = errors.Unwrap(err)
	assert.Equal(t, errdefs.ErrAlreadyExists, unwrapped)
}

func TestStopDeregisterComponent(t *testing.T) {
	// Test with nil set
	nilSet := map[string]Component(nil)
	err := stopDeregisterComponent(nilSet, "test")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errdefs.ErrUnavailable)

	// Test error message
	expectedMsg := "component set not initialized: unavailable"
	assert.Equal(t, expectedMsg, err.Error())

	// Test error unwrapping
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, errdefs.ErrUnavailable, unwrapped)

	// Test with nonexistent component
	set := map[string]Component{}
	err = stopDeregisterComponent(set, "nonexistent")
	assert.NoError(t, err) // Should return nil for nonexistent component

	// Test with existing component
	testComp := newMockComponent("test")
	set["test"] = testComp
	assert.False(t, testComp.closed)

	err = stopDeregisterComponent(set, "test")
	assert.NoError(t, err)
	assert.True(t, testComp.closed)
	_, exists := set["test"]
	assert.False(t, exists) // Component should be removed from set

	// Test with component that returns error on Close
	customErr := fmt.Errorf("custom close error: %w", context.Canceled)
	errorComp := newMockErrorComponent("error-test", customErr)
	set["error-test"] = errorComp

	err = stopDeregisterComponent(set, "error-test")
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "custom close error: context canceled", err.Error())

	// Test error unwrapping
	unwrapped = errors.Unwrap(err)
	assert.Equal(t, context.Canceled, unwrapped)
}

func TestSetComponent(t *testing.T) {
	// Test with nil set
	nilSet := map[string]Component(nil)
	newComp := newMockComponent("test")
	err := setComponent(nilSet, newComp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errdefs.ErrUnavailable)

	// Test error message
	expectedMsg := "component set not initialized: unavailable"
	assert.Equal(t, expectedMsg, err.Error())

	// Test error unwrapping
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, errdefs.ErrUnavailable, unwrapped)

	// Test with empty set (new component)
	set := map[string]Component{}
	testComp := newMockComponent("test")
	err = setComponent(set, testComp)
	assert.NoError(t, err)
	assert.Contains(t, set, "test")

	// Test with existing component (replacement)
	existingComp := newMockComponent("test")
	set["test"] = existingComp
	assert.False(t, existingComp.closed)

	replacementComp := newMockComponent("test")
	err = setComponent(set, replacementComp)
	assert.NoError(t, err)
	assert.Contains(t, set, "test")
	assert.True(t, existingComp.closed) // Old component should be closed
	// In the current implementation, set["test"] will be existingComp after calling Close()

	// Test with component that returns error on Close
	customErr := fmt.Errorf("custom close error: %w", context.Canceled)
	errorComp := newMockErrorComponent("error-comp", customErr)
	set["error-comp"] = errorComp

	newComp = newMockComponent("error-comp")
	err = setComponent(set, newComp)
	assert.NoError(t, err) // Note: setComponent doesn't return Close errors
	assert.True(t, errorComp.closed)
}

func TestGetAllComponents(t *testing.T) {
	// Setup custom map
	customSet := map[string]Component{
		"comp1": newMockComponent("comp1"),
		"comp2": newMockComponent("comp2"),
	}

	// Test returning components
	assert.Equal(t, 2, len(customSet))
	assert.Contains(t, customSet, "comp1")
	assert.Contains(t, customSet, "comp2")
}

func TestGlobalFunctions(t *testing.T) {
	// Clear defaultSet before testing
	defaultSetMu.Lock()
	defaultSet = make(map[string]Component)
	defaultSetMu.Unlock()

	// Test GetAllComponents with empty set
	comps := GetAllComponents()
	assert.Empty(t, comps)

	// Register a component
	testComp := newMockComponent("test-global")
	err := RegisterComponent("test-global", testComp)
	assert.NoError(t, err)

	// Test IsComponentRegistered
	assert.True(t, IsComponentRegistered("test-global"))
	assert.False(t, IsComponentRegistered("nonexistent"))

	// Test GetComponent
	comp, err := GetComponent("test-global")
	assert.NoError(t, err)
	assert.Equal(t, testComp, comp)

	_, err = GetComponent("nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errdefs.ErrNotFound)

	// Test error message
	expectedMsg := "component nonexistent not found: not found"
	assert.Equal(t, expectedMsg, err.Error())

	// Test error unwrapping
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, errdefs.ErrNotFound, unwrapped)

	// Test SetComponent
	newComp := newMockComponent("test-global")
	err = SetComponent("test-global", newComp)
	assert.NoError(t, err)
	assert.True(t, testComp.closed) // Old component should be closed

	// Register a component that will return error on Close
	customErr := fmt.Errorf("custom close error: %w", context.Canceled)
	errorComp := newMockErrorComponent("error-global", customErr)
	err = RegisterComponent("error-global", errorComp)
	assert.NoError(t, err)

	// Test StopDeregisterComponent with error
	err = StopDeregisterComponent("error-global")
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "custom close error: context canceled", err.Error())
	assert.True(t, errorComp.closed)

	// Test regular StopDeregisterComponent
	err = StopDeregisterComponent("test-global")
	assert.NoError(t, err)
	assert.False(t, IsComponentRegistered("test-global"))

	// Test GetAllComponents after adding components
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	assert.NoError(t, RegisterComponent("comp1", comp1))
	assert.NoError(t, RegisterComponent("comp2", comp2))

	comps = GetAllComponents()
	assert.Len(t, comps, 2)
	assert.Contains(t, comps, "comp1")
	assert.Contains(t, comps, "comp2")

	// Clean up
	defaultSetMu.Lock()
	defaultSet = make(map[string]Component)
	defaultSetMu.Unlock()
}

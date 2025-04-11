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
	if !errors.Is(err, errdefs.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Verify error message
	expectedMsg := "component nvidia not found: not found"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}

	// Test unwrapping
	unwrapped := errors.Unwrap(err)
	if unwrapped != errdefs.ErrNotFound {
		t.Errorf("expected unwrapped error to be ErrNotFound, got %v", unwrapped)
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

	// Register a component that will return error on Close
	customErr := fmt.Errorf("custom close error: %w", context.Canceled)
	errorComp := newMockErrorComponent("error-global", customErr)
	err = RegisterComponent("error-global", errorComp)
	assert.NoError(t, err)

	// Test GetAllComponents after adding components
	comp1 := newMockComponent("comp1")
	comp2 := newMockComponent("comp2")
	assert.NoError(t, RegisterComponent("comp1", comp1))
	assert.NoError(t, RegisterComponent("comp2", comp2))

	comps = GetAllComponents()
	assert.Len(t, comps, 4)
	assert.Contains(t, comps, "comp1")
	assert.Contains(t, comps, "comp2")

	// Clean up
	defaultSetMu.Lock()
	defaultSet = make(map[string]Component)
	defaultSetMu.Unlock()
}

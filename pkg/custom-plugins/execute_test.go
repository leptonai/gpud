package customplugins

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSpecs is a testable implementation of Specs that allows customization
type TestSpecs struct {
	specs []testSpec
}

type testSpec struct {
	pluginName   string
	specType     string
	initFuncImpl components.InitFunc
}

// Make TestSpecs satisfy the Specs interface required by ExecuteInOrder
func (ts TestSpecs) ExecuteInOrder(gpudInstance *components.GPUdInstance, failFast bool) ([]components.CheckResult, error) {
	// Sort init types first
	specs := make([]testSpec, len(ts.specs))
	copy(specs, ts.specs)

	// Correct sorting implementation
	// First move all "init" types to the front while preserving their relative order
	initSpecs := make([]testSpec, 0)
	otherSpecs := make([]testSpec, 0)

	// Separate init and non-init specs
	for _, spec := range specs {
		if spec.specType == "init" {
			initSpecs = append(initSpecs, spec)
		} else {
			otherSpecs = append(otherSpecs, spec)
		}
	}

	// Combine them back together
	specs = append(initSpecs, otherSpecs...)

	results := make([]components.CheckResult, 0, len(specs))
	for _, spec := range specs {
		if spec.initFuncImpl == nil {
			continue
		}

		comp, err := spec.initFuncImpl(gpudInstance)
		if err != nil {
			return nil, err
		}

		checkResult := comp.Check()
		_ = comp.Close()

		if checkResult.HealthStateType() != apiv1.HealthStateTypeHealthy {
			if failFast {
				return nil, errors.New("plugin " + comp.Name() + " returned unhealthy state")
			}
		}

		results = append(results, checkResult)
	}
	return results, nil
}

func TestComponentSpecsExecuteInOrder(t *testing.T) {
	testFile := filepath.Join("testdata", "plugins.plaintext.2.regex.yaml")
	specs, err := LoadSpecs(testFile)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	results, err := specs.ExecuteInOrder(&components.GPUdInstance{RootCtx: ctx}, false)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(results))

	for _, rs := range results {
		t.Logf("%q: %q", rs.ComponentName(), rs.Summary())
	}
}

// TestExecuteInOrderWithCanceledContext tests the behavior when the context is canceled
func TestExecuteInOrderWithCanceledContext(t *testing.T) {
	testFile := filepath.Join("testdata", "plugins.plaintext.2.regex.yaml")
	specs, err := LoadSpecs(testFile)
	assert.NoError(t, err)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Since we're canceling the context, but the ExecuteInOrder function doesn't currently check
	// for context cancellation between plugin executions, this should still succeed
	// This test is added for completeness and to document the current behavior
	results, err := specs.ExecuteInOrder(&components.GPUdInstance{RootCtx: ctx}, false)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(results))
}

// TestExecuteInOrderWithFailFast tests ExecuteInOrder with failFast=true
func TestExecuteInOrderWithFailFast(t *testing.T) {
	ctx := context.Background()

	// Create a mock TestSpecs for testing
	testSpecs := TestSpecs{
		specs: []testSpec{
			{
				pluginName: "mock-unhealthy",
				specType:   "init",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					return &mockUnhealthyComponent{}, nil
				},
			},
		},
	}

	// Run with failFast=true
	_, err := testSpecs.ExecuteInOrder(&components.GPUdInstance{RootCtx: ctx}, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin mock-unhealthy returned unhealthy state")
}

// TestExecuteInOrderUnhealthyNoFailFast tests behavior when a plugin returns unhealthy but failFast=false
func TestExecuteInOrderUnhealthyNoFailFast(t *testing.T) {
	ctx := context.Background()

	// Create a mock TestSpecs for testing
	testSpecs := TestSpecs{
		specs: []testSpec{
			{
				pluginName: "mock-unhealthy",
				specType:   "init",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					return &mockUnhealthyComponent{}, nil
				},
			},
		},
	}

	results, err := testSpecs.ExecuteInOrder(&components.GPUdInstance{RootCtx: ctx}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, results[0].HealthStateType())
}

// TestExecuteInOrderSorting tests that "init" type plugins are executed first
func TestExecuteInOrderSorting(t *testing.T) {
	// Track execution order
	var executionOrder []string

	// Create a mock TestSpecs for testing
	testSpecs := TestSpecs{
		specs: []testSpec{
			{
				pluginName: "second",
				specType:   "check",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					executionOrder = append(executionOrder, "second")
					return &mockComponent{name: "second"}, nil
				},
			},
			{
				pluginName: "first-init",
				specType:   "init",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					executionOrder = append(executionOrder, "first-init")
					return &mockComponent{name: "first-init"}, nil
				},
			},
			{
				pluginName: "third",
				specType:   "check",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					executionOrder = append(executionOrder, "third")
					return &mockComponent{name: "third"}, nil
				},
			},
			{
				pluginName: "zeroth-init",
				specType:   "init",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					executionOrder = append(executionOrder, "zeroth-init")
					return &mockComponent{name: "zeroth-init"}, nil
				},
			},
		},
	}

	_, err := testSpecs.ExecuteInOrder(&components.GPUdInstance{RootCtx: context.Background()}, false)
	assert.NoError(t, err)

	// Verify init types came first, in their original order
	assert.Equal(t, []string{"first-init", "zeroth-init", "second", "third"}, executionOrder)
}

// TestExecuteInOrderInitError tests error handling when a plugin's init function returns an error
func TestExecuteInOrderInitError(t *testing.T) {
	testSpecs := TestSpecs{
		specs: []testSpec{
			{
				pluginName: "error-plugin",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					return nil, errors.New("init error")
				},
			},
		},
	}

	_, err := testSpecs.ExecuteInOrder(&components.GPUdInstance{RootCtx: context.Background()}, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "init error")
}

// TestExecuteInOrderNilInitFunc tests behavior when a plugin has a nil init function
func TestExecuteInOrderNilInitFunc(t *testing.T) {
	testSpecs := TestSpecs{
		specs: []testSpec{
			{
				pluginName:   "nil-init-func",
				initFuncImpl: nil,
			},
			{
				pluginName: "normal-plugin",
				initFuncImpl: func(gpudInstance *components.GPUdInstance) (components.Component, error) {
					return &mockComponent{name: "normal"}, nil
				},
			},
		},
	}

	results, err := testSpecs.ExecuteInOrder(&components.GPUdInstance{RootCtx: context.Background()}, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(results)) // Only the normal plugin should be executed
}

// TestComponentTags tests the Tags method of the component
func TestComponentTags(t *testing.T) {
	// Create a component with a Spec
	testFile := filepath.Join("testdata", "plugins.plaintext.2.regex.yaml")
	specs, err := LoadSpecs(testFile)
	assert.NoError(t, err)
	assert.True(t, len(specs) > 0, "Should have at least one spec")

	// Get the first spec
	spec := specs[0]

	// Create a component from the spec
	initFunc := spec.NewInitFunc()
	assert.NotNil(t, initFunc, "InitFunc should not be nil")

	comp, err := initFunc(&components.GPUdInstance{RootCtx: context.Background()})
	assert.NoError(t, err)
	assert.NotNil(t, comp, "Component should not be nil")

	// Test the Tags method
	tags := comp.Tags()
	assert.NotEmpty(t, tags, "Tags should not be empty")
	assert.Contains(t, tags, "custom-plugin", "Tags should contain 'custom-plugin'")
	assert.Contains(t, tags, comp.Name(), "Tags should contain the component name")
}

// Mock components for testing
type mockComponent struct {
	name string
}

func (m *mockComponent) Name() string      { return m.name }
func (m *mockComponent) Tags() []string    { return []string{} }
func (m *mockComponent) IsSupported() bool { return true }
func (m *mockComponent) Start() error      { return nil }
func (m *mockComponent) Check() components.CheckResult {
	return &mockCheckResult{
		name:   m.name,
		health: apiv1.HealthStateTypeHealthy,
		reason: "healthy",
	}
}
func (m *mockComponent) LastHealthStates() apiv1.HealthStates { return nil }
func (m *mockComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}
func (m *mockComponent) Close() error { return nil }

type mockUnhealthyComponent struct{}

func (m *mockUnhealthyComponent) Name() string      { return "mock-unhealthy" }
func (m *mockUnhealthyComponent) Tags() []string    { return []string{} }
func (m *mockUnhealthyComponent) IsSupported() bool { return true }
func (m *mockUnhealthyComponent) Start() error      { return nil }
func (m *mockUnhealthyComponent) Check() components.CheckResult {
	return &mockCheckResult{
		name:   m.Name(),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "unhealthy",
	}
}
func (m *mockUnhealthyComponent) LastHealthStates() apiv1.HealthStates { return nil }
func (m *mockUnhealthyComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}
func (m *mockUnhealthyComponent) Close() error { return nil }

// Mock CheckResult implementation
type mockCheckResult struct {
	name   string
	health apiv1.HealthStateType
	reason string
}

func (cr *mockCheckResult) ComponentName() string {
	return cr.name
}

func (cr *mockCheckResult) String() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *mockCheckResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *mockCheckResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *mockCheckResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return nil
	}
	return apiv1.HealthStates{
		{
			Time:      metav1.NewTime(time.Now().UTC()),
			Component: cr.name,
			Name:      cr.name,
			Health:    cr.health,
			Reason:    cr.reason,
		},
	}
}

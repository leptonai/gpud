package library

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/file"
)

// createTestComponent creates a test component with the given options
func createTestComponent() *component {
	ctx, cancel := context.WithCancel(context.Background())
	return &component{
		ctx:         ctx,
		cancel:      cancel,
		libraries:   map[string][]string{},
		findLibrary: file.FindLibrary,
		searchOpts:  []file.OpOption{file.WithSearchDirs("/usr/lib", "/lib")},
	}
}

func TestName(t *testing.T) {
	c := createTestComponent()
	assert.Equal(t, Name, c.Name())
}

func TestTags(t *testing.T) {
	c := createTestComponent()

	expectedTags := []string{
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags)
}

func TestCheck(t *testing.T) {
	// Mock component with custom findLibrary function
	comp := createTestComponent()
	comp.libraries = map[string][]string{
		"lib1.so": {"lib1alt.so"},
		"lib2.so": {},
	}

	// Case 1: One library not found
	comp.findLibrary = func(name string, opts ...file.OpOption) (string, error) {
		if name == "lib1.so" {
			return "/usr/lib/lib1.so", nil
		}
		return "", file.ErrLibraryNotFound
	}

	result := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "lib2.so")

	// Case 2: Error finding library
	comp.findLibrary = func(name string, opts ...file.OpOption) (string, error) {
		return "", errors.New("some error")
	}

	result = comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())

	// Case 3: All libraries found
	comp.findLibrary = func(name string, opts ...file.OpOption) (string, error) {
		if name == "lib1.so" {
			return "/usr/lib/lib1.so", nil
		}
		return "/usr/lib/lib2.so", nil
	}

	result = comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Equal(t, "all libraries exist", result.Summary())
}

func TestEvents(t *testing.T) {
	comp := createTestComponent()
	events, err := comp.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestLastHealthStates(t *testing.T) {
	comp := createTestComponent()

	// Initial state with no data
	states := comp.LastHealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)

	// Set data and check again
	comp.lastCheckResult = &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "test reason",
	}

	states = comp.LastHealthStates()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
}

func TestStartAndClose(t *testing.T) {
	comp := createTestComponent()

	err := comp.Start()
	assert.NoError(t, err)

	// Sleep briefly to allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	err = comp.Close()
	assert.NoError(t, err)

	// Check that context is canceled
	select {
	case <-comp.ctx.Done():
		// This is good, context is canceled
	default:
		assert.Fail(t, "context not canceled after Close()")
	}
}

func TestDataMethods(t *testing.T) {
	cr := &checkResult{
		ResolvedLibraries: []string{"/lib/lib1.so", "/usr/lib/lib2.so"},
		health:            apiv1.HealthStateTypeHealthy,
		reason:            "all libraries exist",
	}

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

	assert.Equal(t, "all libraries exist", cr.Summary())

	// Test String() outputs something
	str := cr.String()
	assert.NotEmpty(t, str)

	// Test nil data handling
	var nilData *checkResult
	assert.Empty(t, nilData.String())
	assert.Empty(t, nilData.Summary())
	assert.Empty(t, nilData.HealthStateType())
	assert.Empty(t, nilData.getError())
}

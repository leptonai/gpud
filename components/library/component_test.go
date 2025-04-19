package library

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	if c.Name() != Name {
		t.Errorf("expected component name %q, got %q", Name, c.Name())
	}
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
	if result.HealthState() != apiv1.HealthStateTypeUnhealthy {
		t.Errorf("expected unhealthy state, got %s", result.HealthState())
	}
	if !contains(result.Summary(), "lib2.so") {
		t.Errorf("expected summary to contain lib2.so, got %q", result.Summary())
	}

	// Case 2: Error finding library
	comp.findLibrary = func(name string, opts ...file.OpOption) (string, error) {
		return "", errors.New("some error")
	}

	result = comp.Check()
	if result.HealthState() != apiv1.HealthStateTypeUnhealthy {
		t.Errorf("expected unhealthy state, got %s", result.HealthState())
	}

	// Case 3: All libraries found
	comp.findLibrary = func(name string, opts ...file.OpOption) (string, error) {
		if name == "lib1.so" {
			return "/usr/lib/lib1.so", nil
		}
		return "/usr/lib/lib2.so", nil
	}

	result = comp.Check()
	if result.HealthState() != apiv1.HealthStateTypeHealthy {
		t.Errorf("expected healthy state, got %s", result.HealthState())
	}
	if result.Summary() != "all libraries exist" {
		t.Errorf("expected summary 'all libraries exist', got %q", result.Summary())
	}
}

func TestEvents(t *testing.T) {
	comp := createTestComponent()
	events, err := comp.Events(context.Background(), time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
}

func TestLastHealthStates(t *testing.T) {
	comp := createTestComponent()

	// Initial state with no data
	states := comp.LastHealthStates()
	if len(states) != 1 {
		t.Fatalf("expected 1 health state, got %d", len(states))
	}
	if states[0].Health != apiv1.HealthStateTypeHealthy {
		t.Errorf("expected healthy state, got %s", states[0].Health)
	}

	// Set data and check again
	comp.lastData = &Data{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "test reason",
	}

	states = comp.LastHealthStates()
	if states[0].Health != apiv1.HealthStateTypeUnhealthy {
		t.Errorf("expected unhealthy state, got %s", states[0].Health)
	}
	if states[0].Reason != "test reason" {
		t.Errorf("expected reason 'test reason', got %q", states[0].Reason)
	}
}

func TestStartAndClose(t *testing.T) {
	comp := createTestComponent()

	err := comp.Start()
	if err != nil {
		t.Errorf("failed to start component: %v", err)
	}

	// Sleep briefly to allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	err = comp.Close()
	if err != nil {
		t.Errorf("failed to close component: %v", err)
	}

	// Check that context is canceled
	select {
	case <-comp.ctx.Done():
		// This is good, context is canceled
	default:
		t.Error("context not canceled after Close()")
	}
}

func TestDataMethods(t *testing.T) {
	d := &Data{
		ResolvedLibraries: []string{"/lib/lib1.so", "/usr/lib/lib2.so"},
		health:            apiv1.HealthStateTypeHealthy,
		reason:            "all libraries exist",
	}

	if d.HealthState() != apiv1.HealthStateTypeHealthy {
		t.Errorf("expected healthy state, got %s", d.HealthState())
	}

	if d.Summary() != "all libraries exist" {
		t.Errorf("expected summary 'all libraries exist', got %q", d.Summary())
	}

	// Test String() outputs something
	str := d.String()
	if str == "" {
		t.Error("String() returned empty string")
	}

	// Test nil data handling
	var nilData *Data
	if nilData.String() != "" {
		t.Error("String() on nil Data should return empty string")
	}
	if nilData.Summary() != "" {
		t.Error("Summary() on nil Data should return empty string")
	}
	if nilData.HealthState() != "" {
		t.Error("HealthState() on nil Data should return empty string")
	}
	if nilData.getError() != "" {
		t.Error("getError() on nil Data should return empty string")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && s != substr && strings.Contains(s, substr)
}

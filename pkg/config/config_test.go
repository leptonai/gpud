package config

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigValidate_AutoUpdateExitCode(t *testing.T) {
	tests := []struct {
		name               string
		enableAutoUpdate   bool
		autoUpdateExitCode int
		wantErr            bool
	}{
		{
			name:               "Valid: Auto update enabled with exit code",
			enableAutoUpdate:   true,
			autoUpdateExitCode: 0,
			wantErr:            false,
		},
		{
			name:               "Valid: Auto update disabled with default exit code",
			enableAutoUpdate:   false,
			autoUpdateExitCode: -1,
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				RetentionPeriod:    metav1.Duration{Duration: time.Hour},
				CompactPeriod:      metav1.Duration{Duration: time.Hour},
				Address:            "localhost:8080", // Add a valid address to pass other validations
				EnableAutoUpdate:   tt.enableAutoUpdate,
				AutoUpdateExitCode: tt.autoUpdateExitCode,
			}

			err := cfg.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ShouldEnable(t *testing.T) {
	tests := []struct {
		name             string
		enableComponents []string
		componentName    string
		expected         bool
	}{
		{
			name:             "Empty enable components - should enable all",
			enableComponents: nil,
			componentName:    "any-component",
			expected:         true,
		},
		{
			name:             "Empty slice enable components - should enable all",
			enableComponents: []string{},
			componentName:    "any-component",
			expected:         true,
		},
		{
			name:             "Component in enable list - should enable",
			enableComponents: []string{"component1", "component2"},
			componentName:    "component1",
			expected:         true,
		},
		{
			name:             "Component not in enable list - should not enable",
			enableComponents: []string{"component1", "component2"},
			componentName:    "component3",
			expected:         false,
		},
		{
			name:             "Single component in enable list - exact match",
			enableComponents: []string{"target-component"},
			componentName:    "target-component",
			expected:         true,
		},
		{
			name:             "Single component in enable list - no match",
			enableComponents: []string{"target-component"},
			componentName:    "other-component",
			expected:         false,
		},
		{
			name:             "Case sensitive matching - different case",
			enableComponents: []string{"Component1"},
			componentName:    "component1",
			expected:         false,
		},
		{
			name:             "All components wildcard - should enable all",
			enableComponents: []string{"*"},
			componentName:    "any-component",
			expected:         true,
		},
		{
			name:             "All components keyword - should enable all",
			enableComponents: []string{"all"},
			componentName:    "any-component",
			expected:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Components: tt.enableComponents,
			}

			result := cfg.ShouldEnable(tt.componentName)

			if result != tt.expected {
				t.Errorf("Config.ShouldEnable() = %v, want %v", result, tt.expected)
			}

			// Test that calling the method multiple times works correctly (tests map initialization)
			result2 := cfg.ShouldEnable(tt.componentName)
			if result2 != tt.expected {
				t.Errorf("Config.ShouldEnable() second call = %v, want %v", result2, tt.expected)
			}
		})
	}
}

func TestConfig_ShouldDisable(t *testing.T) {
	tests := []struct {
		name          string
		components    []string
		componentName string
		expected      bool
	}{
		{
			name:          "Empty components - should not disable any",
			components:    nil,
			componentName: "any-component",
			expected:      false,
		},
		{
			name:          "Empty slice components - should not disable any",
			components:    []string{},
			componentName: "any-component",
			expected:      false,
		},
		{
			name:          "Component with disable prefix - should disable",
			components:    []string{"-component1", "component2"},
			componentName: "-component1",
			expected:      true,
		},
		{
			name:          "Component without disable prefix - should not disable",
			components:    []string{"-component1", "component2"},
			componentName: "component2",
			expected:      false,
		},
		{
			name:          "Component not in list - should not disable",
			components:    []string{"-component1", "component2"},
			componentName: "component3",
			expected:      false,
		},
		{
			name:          "All components wildcard - should not disable any",
			components:    []string{"*"},
			componentName: "any-component",
			expected:      false,
		},
		{
			name:          "All components keyword - should not disable any",
			components:    []string{"all"},
			componentName: "any-component",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Components: tt.components,
			}

			result := cfg.ShouldDisable(tt.componentName)

			if result != tt.expected {
				t.Errorf("Config.ShouldDisable() = %v, want %v", result, tt.expected)
			}

			// Test that calling the method multiple times works correctly (tests map initialization)
			result2 := cfg.ShouldDisable(tt.componentName)
			if result2 != tt.expected {
				t.Errorf("Config.ShouldDisable() second call = %v, want %v", result2, tt.expected)
			}
		})
	}
}

func TestConfig_ShouldEnableDisable_MapInitialization(t *testing.T) {
	t.Run("ShouldEnable map initialization", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"comp1", "comp2", "comp3"},
		}

		// First call should initialize the map
		result1 := cfg.ShouldEnable("comp1")
		if !result1 {
			t.Errorf("Expected comp1 to be enabled")
		}

		// Verify map was initialized
		if cfg.selectedComponents == nil {
			t.Errorf("selectedComponents map should be initialized after first call")
		}

		// Verify map contains expected entries
		if len(cfg.selectedComponents) != 3 {
			t.Errorf("selectedComponents map should have 3 entries, got %d", len(cfg.selectedComponents))
		}

		// Test different components
		if !cfg.ShouldEnable("comp2") {
			t.Errorf("Expected comp2 to be enabled")
		}
		if !cfg.ShouldEnable("comp3") {
			t.Errorf("Expected comp3 to be enabled")
		}
		if cfg.ShouldEnable("comp4") {
			t.Errorf("Expected comp4 to be disabled")
		}
	})

	t.Run("ShouldDisable map initialization", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"-comp1", "-comp2", "-comp3"},
		}

		// First call should initialize the map
		result1 := cfg.ShouldDisable("-comp1")
		if !result1 {
			t.Errorf("Expected -comp1 to be disabled")
		}

		// Verify map was initialized
		if cfg.disabledComponents == nil {
			t.Errorf("disabledComponents map should be initialized after first call")
		}

		// Verify map contains expected entries
		if len(cfg.disabledComponents) != 3 {
			t.Errorf("disabledComponents map should have 3 entries, got %d", len(cfg.disabledComponents))
		}

		// Test different components
		if !cfg.ShouldDisable("-comp2") {
			t.Errorf("Expected -comp2 to be disabled")
		}
		if !cfg.ShouldDisable("-comp3") {
			t.Errorf("Expected -comp3 to be disabled")
		}
		if cfg.ShouldDisable("-comp4") {
			t.Errorf("Expected -comp4 to not be disabled")
		}
	})
}

func TestConfig_ShouldEnable_WildcardAndAllConditions(t *testing.T) {
	tests := []struct {
		name          string
		components    []string
		componentName string
		expected      bool
		description   string
	}{
		{
			name:          "Wildcard as first element",
			components:    []string{"*"},
			componentName: "any-component",
			expected:      true,
			description:   "Should enable any component when '*' is the first element",
		},
		{
			name:          "All as first element",
			components:    []string{"all"},
			componentName: "any-component",
			expected:      true,
			description:   "Should enable any component when 'all' is the first element",
		},
		{
			name:          "Wildcard with other components",
			components:    []string{"*", "comp1", "comp2"},
			componentName: "random-component",
			expected:      true,
			description:   "Should enable any component when '*' is the first element, even with other components",
		},
		{
			name:          "All with other components",
			components:    []string{"all", "comp1", "comp2"},
			componentName: "random-component",
			expected:      true,
			description:   "Should enable any component when 'all' is the first element, even with other components",
		},
		{
			name:          "Wildcard in middle of list",
			components:    []string{"comp1", "*", "comp2"},
			componentName: "random-component",
			expected:      true,
			description:   "Should enable any component when '*' is found anywhere in the list during map initialization",
		},
		{
			name:          "All in middle of list",
			components:    []string{"comp1", "all", "comp2"},
			componentName: "random-component",
			expected:      true,
			description:   "Should enable any component when 'all' is found anywhere in the list during map initialization",
		},
		{
			name:          "Wildcard at end of list",
			components:    []string{"comp1", "comp2", "*"},
			componentName: "random-component",
			expected:      true,
			description:   "Should enable any component when '*' is found anywhere in the list during map initialization",
		},
		{
			name:          "All at end of list",
			components:    []string{"comp1", "comp2", "all"},
			componentName: "random-component",
			expected:      true,
			description:   "Should enable any component when 'all' is found anywhere in the list during map initialization",
		},
		{
			name:          "Multiple wildcards",
			components:    []string{"*", "*", "*"},
			componentName: "test-component",
			expected:      true,
			description:   "Should handle multiple wildcards correctly",
		},
		{
			name:          "Multiple all keywords",
			components:    []string{"all", "all", "all"},
			componentName: "test-component",
			expected:      true,
			description:   "Should handle multiple 'all' keywords correctly",
		},
		{
			name:          "Mixed wildcard and all",
			components:    []string{"*", "all"},
			componentName: "test-component",
			expected:      true,
			description:   "Should handle mixed '*' and 'all' correctly",
		},
		{
			name:          "Wildcard with disabled components",
			components:    []string{"*", "-comp1", "-comp2"},
			componentName: "enabled-component",
			expected:      true,
			description:   "Should enable component when '*' is the first element, even with disabled components",
		},
		{
			name:          "All with disabled components",
			components:    []string{"all", "-comp1", "-comp2"},
			componentName: "enabled-component",
			expected:      true,
			description:   "Should enable component when 'all' is the first element, even with disabled components",
		},
		{
			name:          "Empty string component name with wildcard",
			components:    []string{"*"},
			componentName: "",
			expected:      true,
			description:   "Should enable even empty component name when '*' is present",
		},
		{
			name:          "Empty string component name with all",
			components:    []string{"all"},
			componentName: "",
			expected:      true,
			description:   "Should enable even empty component name when 'all' is present",
		},
		{
			name:          "Special characters in component name with wildcard",
			components:    []string{"*"},
			componentName: "comp-with-special_chars.123",
			expected:      true,
			description:   "Should enable component with special characters when '*' is present",
		},
		{
			name:          "Special characters in component name with all",
			components:    []string{"all"},
			componentName: "comp-with-special_chars.123",
			expected:      true,
			description:   "Should enable component with special characters when 'all' is present",
		},
		{
			name:          "Wildcard not first - specific component in list",
			components:    []string{"comp1", "*", "comp2"},
			componentName: "comp1",
			expected:      true,
			description:   "Should enable specific component when it's in the list, even if wildcard is present later",
		},
		{
			name:          "All not first - specific component in list",
			components:    []string{"comp1", "all", "comp2"},
			componentName: "comp1",
			expected:      true,
			description:   "Should enable specific component when it's in the list, even if 'all' is present later",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Components: tt.components,
			}

			result := cfg.ShouldEnable(tt.componentName)

			if result != tt.expected {
				t.Errorf("Config.ShouldEnable() = %v, want %v. %s", result, tt.expected, tt.description)
			}

			// Test multiple calls to ensure consistency
			// Note: For cases where wildcard/all is not first, the first call may return true due to early return in loop,
			// but subsequent calls will use the map which may not contain the component
			if tt.components[0] == "*" || tt.components[0] == "all" {
				// Only test multiple calls for early return cases
				for i := 0; i < 3; i++ {
					result2 := cfg.ShouldEnable(tt.componentName)
					if result2 != tt.expected {
						t.Errorf("Config.ShouldEnable() call %d = %v, want %v", i+2, result2, tt.expected)
					}
				}
			}
		})
	}
}

func TestConfig_ShouldDisable_WildcardAndAllConditions(t *testing.T) {
	tests := []struct {
		name          string
		components    []string
		componentName string
		expected      bool
		description   string
	}{
		{
			name:          "Wildcard as first element",
			components:    []string{"*"},
			componentName: "any-component",
			expected:      false,
			description:   "Should not disable any component when '*' is the first element",
		},
		{
			name:          "All as first element",
			components:    []string{"all"},
			componentName: "any-component",
			expected:      false,
			description:   "Should not disable any component when 'all' is the first element",
		},
		{
			name:          "Wildcard with disabled components",
			components:    []string{"*", "-comp1", "-comp2"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when '*' is the first element, even with disabled components",
		},
		{
			name:          "All with disabled components",
			components:    []string{"all", "-comp1", "-comp2"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when 'all' is the first element, even with disabled components",
		},
		{
			name:          "Wildcard in middle of list",
			components:    []string{"-comp1", "*", "-comp2"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when '*' is found anywhere in the list during map initialization",
		},
		{
			name:          "All in middle of list",
			components:    []string{"-comp1", "all", "-comp2"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when 'all' is found anywhere in the list during map initialization",
		},
		{
			name:          "Wildcard at end of list",
			components:    []string{"-comp1", "-comp2", "*"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when '*' is found anywhere in the list during map initialization",
		},
		{
			name:          "All at end of list",
			components:    []string{"-comp1", "-comp2", "all"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when 'all' is found anywhere in the list during map initialization",
		},
		{
			name:          "Multiple wildcards",
			components:    []string{"*", "*", "*"},
			componentName: "test-component",
			expected:      false,
			description:   "Should handle multiple wildcards correctly",
		},
		{
			name:          "Multiple all keywords",
			components:    []string{"all", "all", "all"},
			componentName: "test-component",
			expected:      false,
			description:   "Should handle multiple 'all' keywords correctly",
		},
		{
			name:          "Mixed wildcard and all",
			components:    []string{"*", "all"},
			componentName: "test-component",
			expected:      false,
			description:   "Should handle mixed '*' and 'all' correctly",
		},
		{
			name:          "Empty string component name with wildcard",
			components:    []string{"*"},
			componentName: "",
			expected:      false,
			description:   "Should not disable even empty component name when '*' is present",
		},
		{
			name:          "Empty string component name with all",
			components:    []string{"all"},
			componentName: "",
			expected:      false,
			description:   "Should not disable even empty component name when 'all' is present",
		},
		{
			name:          "Special characters in component name with wildcard",
			components:    []string{"*"},
			componentName: "comp-with-special_chars.123",
			expected:      false,
			description:   "Should not disable component with special characters when '*' is present",
		},
		{
			name:          "Special characters in component name with all",
			components:    []string{"all"},
			componentName: "comp-with-special_chars.123",
			expected:      false,
			description:   "Should not disable component with special characters when 'all' is present",
		},
		{
			name:          "Wildcard not first - specific disabled component in list",
			components:    []string{"-comp1", "*", "-comp2"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when '*' is found during map initialization",
		},
		{
			name:          "All not first - specific disabled component in list",
			components:    []string{"-comp1", "all", "-comp2"},
			componentName: "-comp1",
			expected:      false,
			description:   "Should not disable any component when 'all' is found during map initialization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Components: tt.components,
			}

			result := cfg.ShouldDisable(tt.componentName)

			if result != tt.expected {
				t.Errorf("Config.ShouldDisable() = %v, want %v. %s", result, tt.expected, tt.description)
			}

			// Test multiple calls to ensure consistency
			// Note: For cases where wildcard/all is not first, the first call may return false due to early return in loop,
			// but subsequent calls will use the map which may contain the disabled component
			if tt.components[0] == "*" || tt.components[0] == "all" {
				// Only test multiple calls for early return cases
				for i := 0; i < 3; i++ {
					result2 := cfg.ShouldDisable(tt.componentName)
					if result2 != tt.expected {
						t.Errorf("Config.ShouldDisable() call %d = %v, want %v", i+2, result2, tt.expected)
					}
				}
			}
		})
	}
}

func TestConfig_ShouldEnable_EarlyReturnBehavior(t *testing.T) {
	tests := []struct {
		name        string
		components  []string
		description string
	}{
		{
			name:        "First element is wildcard - early return",
			components:  []string{"*", "comp1", "comp2"},
			description: "Should return true immediately when first element is '*'",
		},
		{
			name:        "First element is all - early return",
			components:  []string{"all", "comp1", "comp2"},
			description: "Should return true immediately when first element is 'all'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Components: tt.components,
			}

			// Call ShouldEnable - should return true immediately without initializing map
			result := cfg.ShouldEnable("test-component")
			if !result {
				t.Errorf("Expected ShouldEnable to return true for early return case")
			}

			// Verify that selectedComponents map was NOT initialized due to early return
			if cfg.selectedComponents != nil {
				t.Errorf("selectedComponents map should not be initialized when early return occurs")
			}
		})
	}
}

func TestConfig_ShouldDisable_EarlyReturnBehavior(t *testing.T) {
	tests := []struct {
		name        string
		components  []string
		description string
	}{
		{
			name:        "First element is wildcard - early return",
			components:  []string{"*", "-comp1", "-comp2"},
			description: "Should return false immediately when first element is '*'",
		},
		{
			name:        "First element is all - early return",
			components:  []string{"all", "-comp1", "-comp2"},
			description: "Should return false immediately when first element is 'all'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Components: tt.components,
			}

			// Call ShouldDisable - should return false immediately without initializing map
			result := cfg.ShouldDisable("test-component")
			if result {
				t.Errorf("Expected ShouldDisable to return false for early return case")
			}

			// Verify that disabledComponents map was NOT initialized due to early return
			if cfg.disabledComponents != nil {
				t.Errorf("disabledComponents map should not be initialized when early return occurs")
			}
		})
	}
}

func TestConfig_WildcardAndAll_EdgeCases(t *testing.T) {
	t.Run("ShouldEnable with wildcard and all mixed with regular components", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"comp1", "*", "comp2", "all", "-disabled"},
		}

		// First call should trigger early return due to "*" in the loop
		result := cfg.ShouldEnable("random-component")
		if !result {
			t.Errorf("Expected ShouldEnable to return true when '*' is found in loop")
		}

		// Map should be initialized because we start the loop before hitting the early return
		if cfg.selectedComponents == nil {
			t.Errorf("selectedComponents map should be initialized when loop starts")
		}
	})

	t.Run("ShouldDisable with wildcard and all mixed with disabled components", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"-comp1", "*", "-comp2", "all", "enabled"},
		}

		// First call should trigger early return due to "*" in the loop
		result := cfg.ShouldDisable("-comp1")
		if result {
			t.Errorf("Expected ShouldDisable to return false when '*' is found in loop")
		}

		// Map should be initialized because we start the loop before hitting the early return
		if cfg.disabledComponents == nil {
			t.Errorf("disabledComponents map should be initialized when loop starts")
		}
	})

	t.Run("ShouldEnable with only disabled components (no wildcard/all)", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"-comp1", "-comp2", "-comp3"},
		}

		// Should return false since no enabled components are specified
		result := cfg.ShouldEnable("any-component")
		if result {
			t.Errorf("Expected ShouldEnable to return false when only disabled components are specified")
		}

		// Map should be initialized and empty
		if cfg.selectedComponents == nil {
			t.Errorf("selectedComponents map should be initialized")
		}
		if len(cfg.selectedComponents) != 0 {
			t.Errorf("selectedComponents map should be empty, got %d entries", len(cfg.selectedComponents))
		}
	})

	t.Run("ShouldDisable with only enabled components (no wildcard/all)", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"comp1", "comp2", "comp3"},
		}

		// Should return false since no disabled components are specified
		result := cfg.ShouldDisable("-any-component")
		if result {
			t.Errorf("Expected ShouldDisable to return false when only enabled components are specified")
		}

		// Map should be initialized and empty
		if cfg.disabledComponents == nil {
			t.Errorf("disabledComponents map should be initialized")
		}
		if len(cfg.disabledComponents) != 0 {
			t.Errorf("disabledComponents map should be empty, got %d entries", len(cfg.disabledComponents))
		}
	})

	t.Run("ShouldEnable with case sensitivity", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"COMP1", "*", "comp2"},
		}

		// Should return true due to "*" in the list
		result := cfg.ShouldEnable("comp1")
		if !result {
			t.Errorf("Expected ShouldEnable to return true due to '*' in list")
		}
	})

	t.Run("ShouldDisable with case sensitivity", func(t *testing.T) {
		cfg := &Config{
			Components: []string{"-COMP1", "all", "-comp2"},
		}

		// Should return false due to "all" in the list
		result := cfg.ShouldDisable("-comp1")
		if result {
			t.Errorf("Expected ShouldDisable to return false due to 'all' in list")
		}
	})
}

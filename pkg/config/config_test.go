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
		{
			name:               "Invalid: Auto update disabled with non-default exit code",
			enableAutoUpdate:   false,
			autoUpdateExitCode: 0,
			wantErr:            true,
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

			if tt.wantErr && err != ErrInvalidAutoUpdateExitCode {
				t.Errorf("Config.Validate() error = %v, want %v", err, ErrInvalidAutoUpdateExitCode)
			}
		})
	}
}

func TestConfigValidate_EnableDisableComponents(t *testing.T) {
	tests := []struct {
		name              string
		enableComponents  []string
		disableComponents []string
		wantErr           bool
		expectedErr       error
	}{
		{
			name:              "Valid: Only enable components set",
			enableComponents:  []string{"component1", "component2"},
			disableComponents: nil,
			wantErr:           false,
			expectedErr:       nil,
		},
		{
			name:              "Valid: Only disable components set",
			enableComponents:  nil,
			disableComponents: []string{"component1", "component2"},
			wantErr:           false,
			expectedErr:       nil,
		},
		{
			name:              "Valid: Both empty",
			enableComponents:  nil,
			disableComponents: nil,
			wantErr:           false,
			expectedErr:       nil,
		},
		{
			name:              "Valid: Both empty slices",
			enableComponents:  []string{},
			disableComponents: []string{},
			wantErr:           false,
			expectedErr:       nil,
		},
		{
			name:              "Invalid: Both enable and disable components set",
			enableComponents:  []string{"component1"},
			disableComponents: []string{"component2"},
			wantErr:           true,
			expectedErr:       ErrInvalidEnableDisableComponents,
		},
		{
			name:              "Invalid: Both enable and disable components set with same component",
			enableComponents:  []string{"component1"},
			disableComponents: []string{"component1"},
			wantErr:           true,
			expectedErr:       ErrInvalidEnableDisableComponents,
		},
		{
			name:              "Invalid: Multiple components in both lists",
			enableComponents:  []string{"component1", "component2"},
			disableComponents: []string{"component3", "component4"},
			wantErr:           true,
			expectedErr:       ErrInvalidEnableDisableComponents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				RetentionPeriod:    metav1.Duration{Duration: time.Hour},
				CompactPeriod:      metav1.Duration{Duration: time.Hour},
				Address:            "localhost:8080", // Add a valid address to pass other validations
				EnableAutoUpdate:   true,
				AutoUpdateExitCode: -1, // Set to valid values to avoid other validation errors
				EnableComponents:   tt.enableComponents,
				DisableComponents:  tt.disableComponents,
			}

			err := cfg.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != tt.expectedErr {
				t.Errorf("Config.Validate() error = %v, want %v", err, tt.expectedErr)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				EnableComponents: tt.enableComponents,
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
		name              string
		disableComponents []string
		componentName     string
		expected          bool
	}{
		{
			name:              "Empty disable components - should not disable any",
			disableComponents: nil,
			componentName:     "any-component",
			expected:          false,
		},
		{
			name:              "Empty slice disable components - should not disable any",
			disableComponents: []string{},
			componentName:     "any-component",
			expected:          false,
		},
		{
			name:              "Component in disable list - should disable",
			disableComponents: []string{"component1", "component2"},
			componentName:     "component1",
			expected:          true,
		},
		{
			name:              "Component not in disable list - should not disable",
			disableComponents: []string{"component1", "component2"},
			componentName:     "component3",
			expected:          false,
		},
		{
			name:              "Single component in disable list - exact match",
			disableComponents: []string{"target-component"},
			componentName:     "target-component",
			expected:          true,
		},
		{
			name:              "Single component in disable list - no match",
			disableComponents: []string{"target-component"},
			componentName:     "other-component",
			expected:          false,
		},
		{
			name:              "Case sensitive matching - different case",
			disableComponents: []string{"Component1"},
			componentName:     "component1",
			expected:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				DisableComponents: tt.disableComponents,
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
			EnableComponents: []string{"comp1", "comp2", "comp3"},
		}

		// First call should initialize the map
		result1 := cfg.ShouldEnable("comp1")
		if !result1 {
			t.Errorf("Expected comp1 to be enabled")
		}

		// Verify map was initialized
		if cfg.enableComponents == nil {
			t.Errorf("enableComponents map should be initialized after first call")
		}

		// Verify map contains expected entries
		if len(cfg.enableComponents) != 3 {
			t.Errorf("enableComponents map should have 3 entries, got %d", len(cfg.enableComponents))
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
			DisableComponents: []string{"comp1", "comp2", "comp3"},
		}

		// First call should initialize the map
		result1 := cfg.ShouldDisable("comp1")
		if !result1 {
			t.Errorf("Expected comp1 to be disabled")
		}

		// Verify map was initialized
		if cfg.disableComponents == nil {
			t.Errorf("disableComponents map should be initialized after first call")
		}

		// Verify map contains expected entries
		if len(cfg.disableComponents) != 3 {
			t.Errorf("disableComponents map should have 3 entries, got %d", len(cfg.disableComponents))
		}

		// Test different components
		if !cfg.ShouldDisable("comp2") {
			t.Errorf("Expected comp2 to be disabled")
		}
		if !cfg.ShouldDisable("comp3") {
			t.Errorf("Expected comp3 to be disabled")
		}
		if cfg.ShouldDisable("comp4") {
			t.Errorf("Expected comp4 to not be disabled")
		}
	})
}

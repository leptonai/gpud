package plugins

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLoad(t *testing.T) {
	// Get the path to the test data file
	testFile := filepath.Join("testdata", "plugins.1.yaml")

	// Load the plugins
	plugins, err := Load(testFile)

	// Assert no error occurred
	assert.NoError(t, err)

	// Assert we loaded one plugin
	assert.Len(t, plugins, 1)

	// Check the plugin data
	plugin := plugins[0]
	assert.Equal(t, "nvidia-smi", plugin.Name)
	assert.Equal(t, "bnZpZGlhLXNtaQo=", plugin.StateJob.Script)
	assert.Equal(t, "bnZpZGlhLXNtaQo=", plugin.EventJob.Script)
	assert.True(t, plugin.DryRun)
	assert.Equal(t, metav1.Duration{Duration: 10 * time.Second}, plugin.Timeout)
	assert.Equal(t, metav1.Duration{Duration: 10 * time.Second}, plugin.Interval)

	// Check the decoded scripts - can access these as we're in the same package
	assert.Equal(t, "nvidia-smi\n", plugin.StateJob.scriptDecoded)
	assert.Equal(t, "nvidia-smi\n", plugin.EventJob.scriptDecoded)
}

func TestDecode(t *testing.T) {
	// Create a plugin with base64 encoded scripts
	plugin := Plugin{
		Name: "test",
		StateJob: &Job{
			Script: "c3RhdGUgc2NyaXB0",
		},
		EventJob: &Job{
			Script: "ZXZlbnQgc2NyaXB0",
		},
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	// Decode the plugin
	err := plugin.Validate()
	assert.NoError(t, err)

	// Check that the scripts were decoded correctly
	assert.Equal(t, "state script", plugin.StateJob.scriptDecoded)
	assert.Equal(t, "event script", plugin.EventJob.scriptDecoded)
}

func TestDecodeWithInvalidBase64(t *testing.T) {
	// Create a plugin with invalid base64 encoded scripts
	plugin := Plugin{
		StateJob: &Job{Script: "invalid base64"},
	}

	// Decode the plugin
	err := plugin.Validate()
	assert.Error(t, err)

	assert.Empty(t, plugin.StateJob.scriptDecoded)
}

func TestLoadWithInvalidPath(t *testing.T) {
	// Try to load plugins from a non-existent file
	plugins, err := Load("non-existent-file")

	// Assert an error occurred
	assert.Error(t, err)
	assert.Nil(t, plugins)
}

func TestValidate(t *testing.T) {
	// Test cases for Validate()
	testCases := []struct {
		name          string
		plugin        Plugin
		expectedError error
	}{
		{
			name: "valid plugin",
			plugin: Plugin{
				Name: "test-plugin",
				StateJob: &Job{
					Script: "c3RhdGUgc2NyaXB0",
				},
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			},
			expectedError: nil,
		},
		{
			name: "missing component name",
			plugin: Plugin{
				Name: "",
				StateJob: &Job{
					Script: "c3RhdGUgc2NyaXB0",
				},
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			},
			expectedError: ErrComponentNameRequired,
		},
		{
			name: "missing state script",
			plugin: Plugin{
				Name:    "test-plugin",
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			},
			expectedError: ErrStateScriptRequired,
		},
		{
			name: "missing timeout",
			plugin: Plugin{
				Name: "test-plugin",
				StateJob: &Job{
					Script: "c3RhdGUgc2NyaXB0",
				},
			},
			expectedError: ErrTimeoutRequired,
		},
		{
			name: "invalid base64 in state script",
			plugin: Plugin{
				Name: "test-plugin",
				StateJob: &Job{
					Script: "invalid base64",
				},
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			},
			expectedError: nil, // The exact error is not defined, just check for non-nil
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.plugin.Validate()

			if tc.expectedError == nil && tc.name != "invalid base64 in state script" {
				assert.NoError(t, err)
			} else if tc.name == "invalid base64 in state script" {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tc.expectedError, err)
			}
		})
	}
}

func TestComponentName(t *testing.T) {
	// Test when component name is already set
	plugin := Plugin{
		Name:          "test-plugin",
		componentName: "custom-component-name",
	}
	assert.Equal(t, "custom-component-name", plugin.ComponentName())

	// Test when component name is derived from Name
	plugin = Plugin{
		Name: "test plugin",
	}
	assert.Equal(t, "plugin-test-plugin", plugin.ComponentName())

	// Check that the componentName field is cached
	assert.Equal(t, "plugin-test-plugin", plugin.componentName)
}

func TestRun(t *testing.T) {
	plugin := Plugin{
		Name: "test-plugin",
		StateJob: &Job{
			Script: "c3RhdGUgc2NyaXB0",
		},
	}

	// Test that Run method returns nil (current implementation)
	err := plugin.CheckOnce(context.Background())
	assert.NoError(t, err)
}

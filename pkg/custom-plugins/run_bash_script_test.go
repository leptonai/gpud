package customplugins

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/process"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRunBashScriptValidation(t *testing.T) {
	testCases := []struct {
		name        string
		script      RunBashScript
		expectError bool
	}{
		{
			name: "valid plaintext script",
			script: RunBashScript{
				ContentType: "plaintext",
				Script:      "echo 'Valid plaintext'",
			},
			expectError: false,
		},
		{
			name: "valid base64 script",
			script: RunBashScript{
				ContentType: "base64",
				Script:      "ZWNobyAnVmFsaWQgYmFzZTY0Jw==", // "echo 'Valid base64'"
			},
			expectError: false,
		},
		{
			name: "empty script",
			script: RunBashScript{
				ContentType: "plaintext",
				Script:      "",
			},
			expectError: true,
		},
		{
			name: "invalid base64",
			script: RunBashScript{
				ContentType: "base64",
				Script:      "this is not valid base64!",
			},
			expectError: true,
		},
		{
			name: "unsupported content type",
			script: RunBashScript{
				ContentType: "json",
				Script:      `{"command": "echo 'hello'"}`,
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.script.Validate()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDecodeWithInvalidBase64(t *testing.T) {
	// Create a plugin with invalid base64 encoded scripts
	plugin := Spec{
		StatePlugin: &Plugin{
			Steps: []PluginStep{
				{
					Name: "test-plugin",
					RunBashScript: &RunBashScript{
						ContentType: "base64",
						Script:      "invalid base64",
					},
				},
			},
		},
	}

	// Decode the plugin
	err := plugin.Validate()
	assert.Error(t, err)

	_, err = plugin.StatePlugin.Steps[0].RunBashScript.decode()
	assert.Error(t, err)
}

func TestDecodeWithPlaintext(t *testing.T) {
	// Create a plugin with plaintext scripts
	plugin := Spec{
		Name: "test-plaintext",
		StatePlugin: &Plugin{
			Steps: []PluginStep{
				{
					Name: "test-plaintext",
					RunBashScript: &RunBashScript{
						ContentType: "plaintext",
						Script:      "echo 'Hello, World!'",
					},
				},
			},
		},
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	// Validate the plugin
	err := plugin.Validate()
	assert.NoError(t, err)

	// Test decoding the plaintext scripts
	stateScript, err := plugin.StatePlugin.Steps[0].RunBashScript.decode()
	assert.NoError(t, err)
	assert.Equal(t, "echo 'Hello, World!'", stateScript)
}

func TestDecodeWithEmptyScript(t *testing.T) {
	// Create a RunBashScript with empty script
	emptyScript := RunBashScript{
		ContentType: "plaintext",
		Script:      "",
	}

	// Test decoding the empty script
	decoded, err := emptyScript.decode()
	assert.NoError(t, err)
	assert.Equal(t, "", decoded, "Empty script should decode to empty string")

	// Test with base64 content type as well
	emptyBase64Script := RunBashScript{
		ContentType: "base64",
		Script:      "",
	}
	decoded, err = emptyBase64Script.decode()
	assert.NoError(t, err)
	assert.Equal(t, "", decoded, "Empty script should decode to empty string regardless of content type")
}

func TestRunBashScript(t *testing.T) {
	simpleScript := RunBashScript{
		ContentType: "plaintext",
		Script:      "echo 'Hello, World!'",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, exitCode, err := simpleScript.executeBash(ctx, process.NewExclusiveRunner())
	assert.NoError(t, err)
	assert.Equal(t, "Hello, World!\n", string(out), "Script should output 'Hello, World!'")
	assert.Equal(t, int32(0), exitCode, "Script should exit with code 0")
}

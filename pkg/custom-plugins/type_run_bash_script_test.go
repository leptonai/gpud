package customplugins

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
		Type: SpecTypeComponent,
		StatePlugin: &Plugin{
			Steps: []Step{
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
		PluginName: "test-plaintext",
		Type:       SpecTypeComponent,
		StatePlugin: &Plugin{
			Steps: []Step{
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

// Mock implementation of process.Runner for testing
type MockRunner struct {
	mock.Mock
}

func (m *MockRunner) RunUntilCompletion(ctx context.Context, script string) ([]byte, int32, error) {
	args := m.Called(ctx, script)
	return args.Get(0).([]byte), int32(args.Int(1)), args.Error(2)
}

func TestExecuteBash_WithDecodeError(t *testing.T) {
	// Script with invalid content type should cause decode error
	scriptWithDecodeError := RunBashScript{
		ContentType: "invalid-type",
		Script:      "echo 'Hello'",
	}

	ctx := context.Background()
	mockRunner := new(MockRunner)

	// Mock should not be called because decode will fail first
	out, exitCode, err := scriptWithDecodeError.executeBash(ctx, mockRunner)

	assert.Error(t, err, "Should fail with decode error")
	assert.Empty(t, out, "Output should be empty")
	assert.Equal(t, int32(0), exitCode, "Exit code should be 0")

	// Verify that the mock runner was not called
	mockRunner.AssertNotCalled(t, "RunUntilCompletion")
}

func TestExecuteBash_WithRunnerError(t *testing.T) {
	script := RunBashScript{
		ContentType: "plaintext",
		Script:      "echo 'Test'",
	}

	ctx := context.Background()
	mockRunner := new(MockRunner)
	expectedError := errors.New("runner error")

	// Set up the mock to return an error
	mockRunner.On("RunUntilCompletion", ctx, "echo 'Test'").Return([]byte{}, 1, expectedError)

	out, exitCode, err := script.executeBash(ctx, mockRunner)

	assert.Equal(t, expectedError, err, "Should return the error from runner")
	assert.Empty(t, out, "Output should be empty")
	assert.Equal(t, int32(1), exitCode, "Should return non-zero exit code")

	mockRunner.AssertExpectations(t)
}

func TestExecuteBash_WithCanceledContext(t *testing.T) {
	script := RunBashScript{
		ContentType: "plaintext",
		Script:      "echo 'Test with canceled context'",
	}

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockRunner := new(MockRunner)

	// Setup mock to simulate context cancellation behavior
	contextErr := context.Canceled
	mockRunner.On("RunUntilCompletion", ctx, "echo 'Test with canceled context'").
		Return([]byte{}, 1, contextErr)

	out, exitCode, err := script.executeBash(ctx, mockRunner)

	assert.Equal(t, contextErr, err, "Should return context cancellation error")
	assert.Empty(t, out, "Output should be empty")
	assert.Equal(t, int32(1), exitCode, "Should return non-zero exit code")

	mockRunner.AssertExpectations(t)
}

func TestExecuteBash_WithBase64Script(t *testing.T) {
	// Base64 encoded "echo 'Base64 test'"
	base64Script := RunBashScript{
		ContentType: "base64",
		Script:      "ZWNobyAnQmFzZTY0IHRlc3Qn",
	}

	ctx := context.Background()
	mockRunner := new(MockRunner)

	// The decoded script should be passed to the runner
	mockRunner.On("RunUntilCompletion", ctx, "echo 'Base64 test'").
		Return([]byte("Base64 test\n"), 0, nil)

	out, exitCode, err := base64Script.executeBash(ctx, mockRunner)

	assert.NoError(t, err, "Should not return error")
	assert.Equal(t, "Base64 test\n", string(out), "Should return expected output")
	assert.Equal(t, int32(0), exitCode, "Should return zero exit code")

	mockRunner.AssertExpectations(t)
}

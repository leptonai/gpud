package customplugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPluginStepValidation(t *testing.T) {
	testCases := []struct {
		name        string
		step        Step
		expectError bool
	}{
		{
			name: "valid step",
			step: Step{
				Name: "valid-step",
				RunBashScript: &RunBashScript{
					ContentType: "plaintext",
					Script:      "echo 'Valid step'",
				},
			},
			expectError: false,
		},
		{
			name: "missing name",
			step: Step{
				RunBashScript: &RunBashScript{
					ContentType: "plaintext",
					Script:      "echo 'Missing name'",
				},
			},
			expectError: true,
		},
		{
			name: "missing script",
			step: Step{
				Name: "missing-script",
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

package customplugins

import (
	"testing"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestReadHealthState(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedState  *apiv1.HealthState
		expectedErrMsg string
	}{
		{
			name:          "valid health state",
			input:         StateOutputPrefix + `{"health": "Healthy", "reason": "Everything is fine"}`,
			expectedState: &apiv1.HealthState{Health: "Healthy", Reason: "Everything is fine"},
		},
		{
			name:          "valid health state with whitespace",
			input:         "  " + StateOutputPrefix + `{"health": "Healthy", "reason": "Everything is fine"}  `,
			expectedState: &apiv1.HealthState{Health: "Healthy", Reason: "Everything is fine"},
		},
		{
			name:          "valid health state with detailed info",
			input:         StateOutputPrefix + `{"health": "Unhealthy", "reason": "GPU 0 is not responding", "extra_info": {"error": "timeout"}}`,
			expectedState: &apiv1.HealthState{Health: "Unhealthy", Reason: "GPU 0 is not responding", DeprecatedExtraInfo: map[string]string{"error": "timeout"}},
		},
		{
			name:           "missing prefix",
			input:          `{"health": "Healthy", "reason": "Everything is fine"}`,
			expectedErrMsg: "input does not start with " + StateOutputPrefix,
		},
		{
			name:           "wrong prefix",
			input:          "WRONG_PREFIX:" + `{"health": "Healthy", "reason": "Everything is fine"}`,
			expectedErrMsg: "input does not start with " + StateOutputPrefix,
		},
		{
			name:           "invalid json",
			input:          StateOutputPrefix + `{"health": "Healthy", "reason": "Broken JSON`,
			expectedErrMsg: "failed to unmarshal health state",
		},
		{
			name:           "empty input",
			input:          "",
			expectedErrMsg: "input does not start with " + StateOutputPrefix,
		},
		{
			name:           "prefix only",
			input:          StateOutputPrefix,
			expectedErrMsg: "failed to unmarshal health state",
		},
		{
			name:          "prefix with empty json",
			input:         StateOutputPrefix + "{}",
			expectedState: &apiv1.HealthState{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := ReadHealthState([]byte(tt.input))

			if tt.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				assert.Nil(t, state)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, state)
				assert.Equal(t, tt.expectedState.Health, state.Health)
				assert.Equal(t, tt.expectedState.Reason, state.Reason)

				if tt.expectedState.DeprecatedExtraInfo != nil {
					assert.Equal(t, tt.expectedState.DeprecatedExtraInfo, state.DeprecatedExtraInfo)
				}
			}
		})
	}
}

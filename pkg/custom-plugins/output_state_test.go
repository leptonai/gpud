package customplugins

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestReadHealthState(t *testing.T) {
	tests := []struct {
		name                 string
		input                string
		expectedHealthType   apiv1.HealthStateType
		expectedHealthReason string
		expectedErrMsg       string
	}{
		{
			name:               "health state type prefix",
			input:              HealthStateOutputPrefixType + "Healthy",
			expectedHealthType: "Healthy",
		},
		{
			name:                 "health state reason prefix",
			input:                HealthStateOutputPrefixReason + "Everything is fine",
			expectedHealthReason: "Everything is fine",
		},
		{
			name:               "both prefixes",
			input:              HealthStateOutputPrefixType + "Healthy" + " " + HealthStateOutputPrefixReason + "Everything is fine",
			expectedHealthType: "Healthy" + " " + HealthStateOutputPrefixReason + "Everything is fine",
		},
		{
			name:               "newlines in input",
			input:              HealthStateOutputPrefixType + "Healthy\n" + HealthStateOutputPrefixReason + "Everything is fine",
			expectedHealthType: "Healthy\n" + HealthStateOutputPrefixReason + "Everything is fine",
		},
		{
			name:  "no prefix",
			input: "Some random text",
		},
		{
			name:               "health type with json",
			input:              HealthStateOutputPrefixType + `{"health": "Healthy", "reason": "Everything is fine"}`,
			expectedHealthType: `{"health": "Healthy", "reason": "Everything is fine"}`,
		},
		{
			name:               "health type with whitespace",
			input:              "  " + HealthStateOutputPrefixType + "Healthy  ",
			expectedHealthType: "Healthy",
		},
		{
			name:                 "health reason with whitespace",
			input:                "  " + HealthStateOutputPrefixReason + "Everything is fine  ",
			expectedHealthReason: "Everything is fine",
		},
		{
			name:  "empty input",
			input: "",
		},
		{
			name:               "prefix only for type",
			input:              HealthStateOutputPrefixType,
			expectedHealthType: "",
		},
		{
			name:                 "prefix only for reason",
			input:                HealthStateOutputPrefixReason,
			expectedHealthReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthType, healthReason, err := readHealthStateLine(tt.input)

			if tt.expectedErrMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedHealthType, healthType)
				assert.Equal(t, tt.expectedHealthReason, healthReason)
			}
		})
	}
}

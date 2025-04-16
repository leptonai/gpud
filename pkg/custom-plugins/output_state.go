package customplugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// StateOutputPrefix defines the prefix for the line,
// where each plugin is expected to emit its state data in JSON format.
// The JSON data schema for the state is defined in api/v1/types.go.
const StateOutputPrefix = "GPUD_CUSTOM_PLUGIN_HEALTH_STATE:"

// ReadHealthState reads the health state from the input.
// The input is expected to be a line that starts with StateOutputPrefix.
// The input is expected to be a JSON object that matches the HealthState type in api/v1/types.go.
func ReadHealthState(input []byte) (*apiv1.HealthState, error) {
	input = bytes.TrimSpace(input)
	if !strings.HasPrefix(string(input), StateOutputPrefix) {
		return nil, fmt.Errorf("input does not start with %s", StateOutputPrefix)
	}

	trimmed := strings.TrimPrefix(string(input), StateOutputPrefix)
	var s apiv1.HealthState
	if err := json.Unmarshal([]byte(trimmed), &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health state: %w", err)
	}

	return &s, nil
}

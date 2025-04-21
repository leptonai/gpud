package customplugins

import (
	"fmt"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// HealthStateOutputPrefixType defines the prefix for the line,
// where each plugin is expected to emit its state data in JSON format.
// The JSON data schema for the state is defined in api/v1/types.go.
const (
	HealthStateOutputPrefixType   = "GPUD_HEALTH_STATE_TYPE:"
	HealthStateOutputPrefixReason = "GPUD_HEALTH_STATE_REASON:"
)

// readHealthStateLine reads the health state from the input.
func readHealthStateLine(lineInput string) (apiv1.HealthStateType, string, error) {
	healthStateType := apiv1.HealthStateType("")
	healthStateReason := ""

	lineInput = strings.TrimSpace(lineInput)
	if strings.HasPrefix(lineInput, HealthStateOutputPrefixType) {
		trimmed := strings.TrimPrefix(lineInput, HealthStateOutputPrefixType)
		healthStateType = apiv1.HealthStateType(strings.TrimSpace(trimmed))
	}
	if strings.HasPrefix(lineInput, HealthStateOutputPrefixReason) {
		trimmed := strings.TrimPrefix(lineInput, HealthStateOutputPrefixReason)
		healthStateReason = strings.TrimSpace(trimmed)
	}

	return healthStateType, healthStateReason, nil
}

// ReadHealthStateFromLines reads the health state from the lines.
func ReadHealthStateFromLines(lines []string) (apiv1.HealthStateType, string, error) {
	healthStateType := apiv1.HealthStateType("")
	healthStateReason := ""

	for _, line := range lines {
		st, rs, err := readHealthStateLine(line)
		if err != nil {
			return "", "", fmt.Errorf("failed to read health state from line %q: %w", line, err)
		}
		if st != "" {
			healthStateType = st
		}
		if rs != "" {
			healthStateReason = rs
		}
	}
	return healthStateType, healthStateReason, nil
}

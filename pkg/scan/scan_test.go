package scan

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/components"
)

// mockCheckResult implements the components.CheckResult interface for testing
type mockCheckResult struct {
	componentName   string
	summary         string
	healthStateType apiv1.HealthStateType
	stringOutput    string
}

func (m *mockCheckResult) ComponentName() string {
	return m.componentName
}

func (m *mockCheckResult) String() string {
	return m.stringOutput
}

func (m *mockCheckResult) Summary() string {
	return m.summary
}

func (m *mockCheckResult) HealthStateType() apiv1.HealthStateType {
	return m.healthStateType
}

func (m *mockCheckResult) HealthStates() apiv1.HealthStates {
	return apiv1.HealthStates{
		{
			Component: m.componentName,
			Health:    m.healthStateType,
			Reason:    m.summary,
		},
	}
}

func TestScan(t *testing.T) {
	if os.Getenv("TEST_GPUD_SCAN") != "true" {
		t.Skip("skipping scan test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := Scan(ctx); err != nil {
		t.Logf("error scanning: %+v", err)
	}
}

func TestPrintSummary(t *testing.T) {
	tests := []struct {
		name           string
		checkResult    components.CheckResult
		expectedOutput string
	}{
		{
			name: "healthy result",
			checkResult: &mockCheckResult{
				componentName:   "test-component",
				summary:         "All systems operational",
				healthStateType: apiv1.HealthStateTypeHealthy,
				stringOutput:    "Component details go here",
			},
			expectedOutput: fmt.Sprintf(
				"%s %s\n%s\n",
				cmdcommon.CheckMark,
				"All systems operational",
				"Component details go here",
			),
		},
		{
			name: "unhealthy result",
			checkResult: &mockCheckResult{
				componentName:   "test-component",
				summary:         "System error detected",
				healthStateType: apiv1.HealthStateTypeUnhealthy,
				stringOutput:    "Error details go here",
			},
			expectedOutput: fmt.Sprintf(
				"%s %s\n%s\n",
				cmdcommon.WarningSign,
				"System error detected",
				"Error details go here",
			),
		},
		{
			name: "degraded result",
			checkResult: &mockCheckResult{
				componentName:   "test-component",
				summary:         "Performance degraded",
				healthStateType: apiv1.HealthStateTypeDegraded,
				stringOutput:    "Degradation details go here",
			},
			expectedOutput: fmt.Sprintf(
				"%s %s\n%s\n",
				cmdcommon.WarningSign,
				"Performance degraded",
				"Degradation details go here",
			),
		},
		{
			name: "initializing result",
			checkResult: &mockCheckResult{
				componentName:   "test-component",
				summary:         "System initializing",
				healthStateType: apiv1.HealthStateTypeInitializing,
				stringOutput:    "Initialization details go here",
			},
			expectedOutput: fmt.Sprintf(
				"%s %s\n%s\n",
				cmdcommon.WarningSign,
				"System initializing",
				"Initialization details go here",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printSummary(tt.checkResult)

			// Restore stdout and get the output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			output := buf.String()

			// Verify the output
			assert.Equal(t, tt.expectedOutput, output)
		})
	}
}

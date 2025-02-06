package fd

import (
	"strings"
	"testing"
)

func TestCalculateUsedPercent(t *testing.T) {
	tests := []struct {
		name     string
		usage    uint64
		limit    uint64
		expected float64
	}{
		{"Zero usage", 0, 100, 0},
		{"Half usage", 50, 100, 50},
		{"Full usage", 100, 100, 100},
		{"Over usage", 150, 100, 150},
		{"Zero limit", 50, 0, 0},
		{"Large numbers", 1000000, 10000000, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcUsagePct(tt.usage, tt.limit)
			if result != tt.expected {
				t.Errorf("calculateUsedPercent(%d, %d) = %f; want %f", tt.usage, tt.limit, result, tt.expected)
			}
		})
	}
}

func TestOutputStates(t *testing.T) {
	tests := []struct {
		name           string
		output         Output
		wantHealthy    bool
		wantReasonPart string
	}{
		{
			name: "Healthy state",
			output: Output{
				AllocatedFileHandles:                 50,
				AllocatedFileHandlesPercent:          "50.00",
				UsedPercent:                          "50.00",
				Usage:                                50,
				ThresholdAllocatedFileHandlesPercent: "50.00",
				ThresholdRunningPIDsPercent:          "50.00",
				RunningPIDs:                          50,
				ThresholdRunningPIDs:                 100,
				ThresholdAllocatedFileHandles:        100,
				FileHandlesSupported:                 true,
				FDLimitSupported:                     true,
			},
			wantHealthy:    true,
			wantReasonPart: "current file descriptors: 50",
		},
		{
			name: "Unhealthy - threshold allocated file handles > 80%",
			output: Output{
				ThresholdAllocatedFileHandlesPercent: "81.00",
			},
			wantHealthy:    true,
			wantReasonPart: ErrFileHandlesAllocationExceedsWarning,
		},
		{
			name: "Unhealthy - with errors",
			output: Output{
				Errors: []string{"test error"},
			},
			wantHealthy:    false,
			wantReasonPart: "test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.output.States()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(states) != 1 {
				t.Fatalf("expected 1 state, got %d", len(states))
			}
			state := states[0]
			if state.Healthy != tt.wantHealthy {
				t.Errorf("Healthy = %v, want %v", state.Healthy, tt.wantHealthy)
			}
			if !strings.Contains(state.Reason, tt.wantReasonPart) {
				t.Errorf("Reason = %q, want to contain %q", state.Reason, tt.wantReasonPart)
			}
		})
	}
}

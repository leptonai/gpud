package fd

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateUsagePct(t *testing.T) {
	assert.Equal(t, 50.0, calcUsagePct(50, 100))
	assert.Equal(t, 0.0, calcUsagePct(0, 100))
	assert.Equal(t, 100.0, calcUsagePct(100, 100))
	assert.Equal(t, 0.0, calcUsagePct(50, 0))
}

func TestDataGetReason(t *testing.T) {
	// Test with nil data
	var d *Data
	assert.Equal(t, "no file descriptors data", d.getReason())

	// Test with error
	d = &Data{err: assert.AnError}
	assert.Contains(t, d.getReason(), "failed to get file descriptors data")

	// Test with valid data
	d = &Data{
		Usage:                                500,
		ThresholdAllocatedFileHandles:        1000,
		ThresholdAllocatedFileHandlesPercent: "50.00",
	}
	assert.Contains(t, d.getReason(), "current file descriptors: 500")
	assert.Contains(t, d.getReason(), "threshold: 1000")

	// Test with warning threshold
	d = &Data{
		Usage:                                500,
		ThresholdAllocatedFileHandles:        1000,
		ThresholdAllocatedFileHandlesPercent: "85.00", // Above warning threshold
	}
	assert.Contains(t, d.getReason(), ErrFileHandlesAllocationExceedsWarning)
}

func TestDataGetHealth(t *testing.T) {
	// Test with nil data
	var d *Data
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	d = &Data{err: assert.AnError}
	health, healthy = d.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)

	// Test with valid data below threshold
	d = &Data{
		ThresholdAllocatedFileHandlesPercent: "50.00",
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with valid data above threshold
	d = &Data{
		ThresholdAllocatedFileHandlesPercent: "85.00", // Above warning threshold
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Degraded", health)
	assert.False(t, healthy)
}

func TestDataGetThresholdAllocatedFileHandlesPercent(t *testing.T) {
	// Test with valid data
	d := Data{
		ThresholdAllocatedFileHandlesPercent: "75.50",
	}
	value, err := d.getThresholdAllocatedFileHandlesPercent()
	assert.NoError(t, err)
	assert.Equal(t, 75.5, value)

	// Test with invalid data
	d = Data{
		ThresholdAllocatedFileHandlesPercent: "invalid",
	}
	_, err = d.getThresholdAllocatedFileHandlesPercent()
	assert.Error(t, err)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		Usage:                                500,
		ThresholdAllocatedFileHandles:        1000,
		ThresholdAllocatedFileHandlesPercent: "50.00",
		ts:                                   time.Now(),
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "file_descriptors", states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.NotEmpty(t, states[0].ExtraInfo)
}

func TestDataGetReasonEdgeCases(t *testing.T) {
	testCases := []struct {
		name       string
		data       *Data
		contains   []string
		notcontain string
	}{
		{
			name: "zero usage",
			data: &Data{
				Usage:                                0,
				ThresholdAllocatedFileHandles:        1000,
				ThresholdAllocatedFileHandlesPercent: "0.00",
			},
			contains: []string{"current file descriptors: 0", "threshold: 1000"},
		},
		{
			name: "high usage but below threshold",
			data: &Data{
				Usage:                                700,
				ThresholdAllocatedFileHandles:        1000,
				ThresholdAllocatedFileHandlesPercent: "70.00",
			},
			contains:   []string{"current file descriptors: 700", "threshold: 1000"},
			notcontain: ErrFileHandlesAllocationExceedsWarning,
		},
		{
			name: "very high usage",
			data: &Data{
				Usage:                                950,
				ThresholdAllocatedFileHandles:        1000,
				ThresholdAllocatedFileHandlesPercent: "95.00",
			},
			contains: []string{"current file descriptors: 950", ErrFileHandlesAllocationExceedsWarning},
		},
		{
			name: "various error messages",
			data: &Data{
				err: fmt.Errorf("failed to read /proc/sys/fs/file-nr"),
			},
			contains: []string{"failed to get file descriptors data", "failed to read /proc/sys/fs/file-nr"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reason := tc.data.getReason()
			for _, substr := range tc.contains {
				assert.Contains(t, reason, substr)
			}
			if tc.notcontain != "" {
				assert.NotContains(t, reason, tc.notcontain)
			}
		})
	}
}

func TestDataGetHealthEdgeCases(t *testing.T) {
	testCases := []struct {
		name          string
		data          *Data
		expectedState string
		expectHealthy bool
	}{
		{
			name: "exactly at warning threshold",
			data: &Data{
				ThresholdAllocatedFileHandlesPercent: "80.00", // Exactly at warning threshold
			},
			expectedState: "Healthy",
			expectHealthy: true,
		},
		{
			name: "slightly above warning threshold",
			data: &Data{
				ThresholdAllocatedFileHandlesPercent: "80.01", // Just above warning threshold
			},
			expectedState: "Degraded",
			expectHealthy: false,
		},
		{
			name: "at 100% capacity",
			data: &Data{
				ThresholdAllocatedFileHandlesPercent: "100.00",
			},
			expectedState: "Degraded",
			expectHealthy: false,
		},
		{
			name: "beyond 100% capacity",
			data: &Data{
				ThresholdAllocatedFileHandlesPercent: "120.00", // Over 100%
			},
			expectedState: "Degraded",
			expectHealthy: false,
		},
		{
			name: "with complex error",
			data: &Data{
				err: fmt.Errorf("multiple errors: %w: %v",
					fmt.Errorf("primary error"),
					fmt.Errorf("secondary error")),
			},
			expectedState: "Unhealthy",
			expectHealthy: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			health, healthy := tc.data.getHealth()
			assert.Equal(t, tc.expectedState, health)
			assert.Equal(t, tc.expectHealthy, healthy)
		})
	}
}

func TestDataGetStatesWithError(t *testing.T) {
	testCases := []struct {
		name           string
		data           *Data
		expectedHealth string
		expectError    bool
	}{
		{
			name: "with error",
			data: &Data{
				Usage:                                500,
				ThresholdAllocatedFileHandles:        1000,
				ThresholdAllocatedFileHandlesPercent: "50.00",
				err:                                  errors.New("failed to read file descriptors"),
			},
			expectedHealth: "Unhealthy",
			expectError:    true,
		},
		{
			name:           "nil data",
			data:           nil,
			expectedHealth: "Healthy",
			expectError:    false,
		},
		{
			name: "invalid percentage format",
			data: &Data{
				Usage:                                500,
				ThresholdAllocatedFileHandles:        1000,
				ThresholdAllocatedFileHandlesPercent: "invalid", // Invalid percentage
			},
			expectedHealth: "Healthy", // Should still be healthy since the percentage parsing error is handled internally
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states, err := tc.data.getStates()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Len(t, states, 1)
			assert.Equal(t, "file_descriptors", states[0].Name)
			assert.Equal(t, tc.expectedHealth, states[0].Health)

			// For all non-nil data cases, check ExtraInfo
			if tc.data != nil {
				assert.NotNil(t, states[0].ExtraInfo)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Contains(t, states[0].ExtraInfo, "encoding")

				// Verify we can unmarshal the JSON data
				var decodedData Data
				err := json.Unmarshal([]byte(states[0].ExtraInfo["data"]), &decodedData)
				assert.NoError(t, err)
			}
		})
	}
}

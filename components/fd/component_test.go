package fd

import (
	"context"
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

func TestDataGetReasonWithContextErrors(t *testing.T) {
	// Test with context deadline exceeded error
	d := &Data{err: context.DeadlineExceeded}
	reason := d.getReason()
	assert.Contains(t, reason, "check failed with context deadline exceeded -- transient error, please retry")

	// Test with context canceled error
	d = &Data{err: context.Canceled}
	reason = d.getReason()
	assert.Contains(t, reason, "check failed with context canceled -- transient error, please retry")

	// Test with other error types
	customErr := fmt.Errorf("custom error")
	d = &Data{err: customErr}
	reason = d.getReason()
	assert.Contains(t, reason, "failed to get file descriptors data -- custom error")
}

func TestDataGetReasonWithUsedPercent(t *testing.T) {
	d := &Data{
		Usage:                                500,
		ThresholdAllocatedFileHandles:        1000,
		ThresholdAllocatedFileHandlesPercent: "50.00",
		UsedPercent:                          "30.00",
	}
	reason := d.getReason()
	assert.Contains(t, reason, "current file descriptors: 500")
	assert.Contains(t, reason, "threshold: 1000")
	assert.Contains(t, reason, "used_percent: 50.00")
}

func TestDataGetHealthWithContextErrors(t *testing.T) {
	// Test with context deadline exceeded error
	d := &Data{err: context.DeadlineExceeded}
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with context canceled error
	d = &Data{err: context.Canceled}
	health, healthy = d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with other errors
	d = &Data{err: fmt.Errorf("some other error")}
	health, healthy = d.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)
}

func TestDataGetHealthDegradedState(t *testing.T) {
	// Test threshold exactly at warning level
	d := &Data{
		ThresholdAllocatedFileHandlesPercent: fmt.Sprintf("%.2f", WarningFileHandlesAllocationPercent),
	}
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test threshold just above warning level
	d = &Data{
		ThresholdAllocatedFileHandlesPercent: fmt.Sprintf("%.2f", WarningFileHandlesAllocationPercent+0.01),
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Degraded", health)
	assert.False(t, healthy)

	// Test with error and threshold above warning
	// Error condition should take precedence
	d = &Data{
		err:                                  fmt.Errorf("some error"),
		ThresholdAllocatedFileHandlesPercent: "85.00",
	}
	health, healthy = d.getHealth()
	assert.Equal(t, "Degraded", health)
	assert.False(t, healthy)
}

func TestDataGetHealthWithInvalidThreshold(t *testing.T) {
	// Test with invalid percentage format
	d := &Data{
		ThresholdAllocatedFileHandlesPercent: "invalid",
	}
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)
}

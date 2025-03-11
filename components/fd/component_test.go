package fd

import (
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

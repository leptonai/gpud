package xid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultThresholds(t *testing.T) {
	// Save original value and restore it after the test to avoid polluting other tests
	originalRebootThreshold := GetDefaultRebootThreshold()
	original := GetDefaultThresholds()
	t.Cleanup(func() {
		SetDefaultRebootThreshold(originalRebootThreshold)
		SetDefaultThresholds(original)
	})

	// Test default values
	assert.Equal(t, DefaultRebootThreshold, GetDefaultRebootThreshold())
	defaultThresholds := GetDefaultThresholds()
	assert.Equal(t, 1000, defaultThresholds.ThresholdOverrides[94].RebootThreshold)

	// Test setting a global reboot threshold separately from per-XID overrides.
	SetDefaultRebootThreshold(4)
	assert.Equal(t, 4, GetDefaultRebootThreshold())

	// Test setting new override values.
	newThresholds := Thresholds{
		ThresholdOverrides: map[int]ThresholdOverride{
			95: {RebootThreshold: 5},
		},
	}
	SetDefaultThresholds(newThresholds)

	updatedThresholds := GetDefaultThresholds()
	assert.Equal(t, 5, updatedThresholds.ThresholdOverrides[95].RebootThreshold)
	assert.Equal(t, 1000, updatedThresholds.ThresholdOverrides[94].RebootThreshold)
}

func TestRebootThresholdForXID(t *testing.T) {
	threshold := Thresholds{
		ThresholdOverrides: map[int]ThresholdOverride{
			94: {RebootThreshold: 1000},
		},
	}

	assert.Equal(t, 1000, rebootThresholdForXID(94, DefaultRebootThreshold, threshold))
	assert.Equal(t, DefaultRebootThreshold, rebootThresholdForXID(95, DefaultRebootThreshold, threshold))
}

func TestDefaultLookbackPeriod(t *testing.T) {
	// Save original value and restore it after the test to avoid polluting other tests.
	original := GetLookbackPeriod()
	t.Cleanup(func() {
		SetLookbackPeriod(original)
	})

	// Test setting and getting lookback period.
	newLookbackPeriod := 6 * time.Hour
	SetLookbackPeriod(newLookbackPeriod)
	assert.Equal(t, newLookbackPeriod, GetLookbackPeriod())
}

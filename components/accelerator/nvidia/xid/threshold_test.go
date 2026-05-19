package xid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultThresholds(t *testing.T) {
	// Save original value and restore it after the test to avoid polluting other tests
	original := GetDefaultThresholds()
	t.Cleanup(func() {
		SetDefaultThresholds(original)
	})

	// Test default values
	defaultThresholds := GetDefaultThresholds()
	assert.Equal(t, DefaultRebootThreshold, defaultThresholds.Threshold)
	assert.Equal(t, 1000, defaultThresholds.ThresholdOverrides[94].RebootThreshold)

	// Test setting new values
	newThresholds := Thresholds{
		Threshold: 4,
	}
	SetDefaultThresholds(newThresholds)

	updatedThresholds := GetDefaultThresholds()
	assert.Equal(t, newThresholds.Threshold, updatedThresholds.Threshold)
	assert.Equal(t, 1000, updatedThresholds.ThresholdOverrides[94].RebootThreshold)
}

func TestRebootThresholdForXID(t *testing.T) {
	threshold := Thresholds{
		Threshold: DefaultRebootThreshold,
		ThresholdOverrides: map[int]ThresholdOverride{
			94: {RebootThreshold: 1000},
		},
	}

	assert.Equal(t, 1000, rebootThresholdForXID(94, threshold))
	assert.Equal(t, DefaultRebootThreshold, rebootThresholdForXID(95, threshold))
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

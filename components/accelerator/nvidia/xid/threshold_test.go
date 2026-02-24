package xid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExpectedPortStates(t *testing.T) {
	// Save original value and restore it after the test to avoid polluting other tests
	original := GetDefaultRebootThreshold()
	t.Cleanup(func() {
		SetDefaultRebootThreshold(original)
	})

	// Test default values
	defaultRebootThreshold := GetDefaultRebootThreshold()
	assert.Equal(t, DefaultRebootThreshold, defaultRebootThreshold.Threshold)

	// Test setting new values
	newRebootThreshold := RebootThreshold{
		Threshold: 4,
	}
	SetDefaultRebootThreshold(newRebootThreshold)

	updatedRebootThreshold := GetDefaultRebootThreshold()
	assert.Equal(t, newRebootThreshold.Threshold, updatedRebootThreshold.Threshold)
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

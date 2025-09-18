package xid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExpectedPortStates(t *testing.T) {
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

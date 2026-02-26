package sxid

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultLookbackPeriod(t *testing.T) {
	// Save original value and restore it after the test to avoid polluting other tests.
	original := GetLookbackPeriod()
	t.Cleanup(func() {
		SetLookbackPeriod(original)
	})

	// Test setting and getting lookback period.
	newLookbackPeriod := 12 * time.Hour
	SetLookbackPeriod(newLookbackPeriod)
	assert.Equal(t, newLookbackPeriod, GetLookbackPeriod())
}

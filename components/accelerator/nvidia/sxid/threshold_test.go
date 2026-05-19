package sxid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultThresholds(t *testing.T) {
	original := GetDefaultThresholds()
	t.Cleanup(func() {
		SetDefaultThresholds(original)
	})

	assert.Equal(t, 2, DefaultRebootThreshold)
	assert.Equal(t, 2, rebootThresholdForSXID(11005, DefaultRebootThreshold, Thresholds{}))

	thresholds := Thresholds{
		Overrides: map[int]ThresholdOverride{
			11004: {RebootThreshold: 7},
		},
	}
	SetDefaultThresholds(thresholds)

	updated := GetDefaultThresholds()
	assert.Equal(t, 7, updated.Overrides[11004].RebootThreshold)

	updated.Overrides[11004] = ThresholdOverride{RebootThreshold: 2}
	assert.Equal(t, 7, GetDefaultThresholds().Overrides[11004].RebootThreshold)
}

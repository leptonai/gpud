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

	thresholds := Thresholds{
		ThresholdOverrides: map[int]ThresholdOverride{
			11004: {RebootThreshold: 7},
		},
	}
	SetDefaultThresholds(thresholds)

	updated := GetDefaultThresholds()
	assert.Equal(t, 7, updated.ThresholdOverrides[11004].RebootThreshold)

	updated.ThresholdOverrides[11004] = ThresholdOverride{RebootThreshold: 2}
	assert.Equal(t, 7, GetDefaultThresholds().ThresholdOverrides[11004].RebootThreshold)
}

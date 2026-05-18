package sxid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultThresholdOverrides(t *testing.T) {
	original := GetDefaultThresholdOverrides()
	t.Cleanup(func() {
		SetDefaultThresholdOverrides(original)
	})

	overrides := map[int]ThresholdOverride{
		11004: {RebootThreshold: 7},
	}
	SetDefaultThresholdOverrides(overrides)

	updated := GetDefaultThresholdOverrides()
	assert.Equal(t, 7, updated[11004].RebootThreshold)

	updated[11004] = ThresholdOverride{RebootThreshold: 2}
	assert.Equal(t, 7, GetDefaultThresholdOverrides()[11004].RebootThreshold)
}

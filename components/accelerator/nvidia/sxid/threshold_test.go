package sxid

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRebootThresholdOverrides(t *testing.T) {
	original := GetDefaultRebootThresholdOverrides()
	t.Cleanup(func() {
		SetDefaultRebootThresholdOverrides(original)
	})

	overrides := map[int]RebootThresholdOverride{
		11004: {RebootThreshold: 7},
	}
	SetDefaultRebootThresholdOverrides(overrides)

	updated := GetDefaultRebootThresholdOverrides()
	assert.Equal(t, 7, updated[11004].RebootThreshold)

	updated[11004] = RebootThresholdOverride{RebootThreshold: 2}
	assert.Equal(t, 7, GetDefaultRebootThresholdOverrides()[11004].RebootThreshold)
}

package temperature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaultMarginThreshold(t *testing.T) {
	original := GetDefaultThresholdS()
	defer SetDefaultMarginThreshold(original)

	SetDefaultMarginThreshold(Thresholds{CelsiusSlowdownMargin: 7})
	assert.Equal(t, int32(7), GetDefaultThresholdS().CelsiusSlowdownMargin)

	SetDefaultMarginThreshold(Thresholds{CelsiusSlowdownMargin: -3})
	assert.Equal(t, int32(0), GetDefaultThresholdS().CelsiusSlowdownMargin)
}

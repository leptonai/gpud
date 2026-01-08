package temperature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaultMarginThreshold(t *testing.T) {
	original := GetDefaultMarginThreshold()
	defer SetDefaultMarginThreshold(original)

	SetDefaultMarginThreshold(MarginThreshold{DegradedCelsius: 7})
	assert.Equal(t, int32(7), GetDefaultMarginThreshold().DegradedCelsius)

	SetDefaultMarginThreshold(MarginThreshold{DegradedCelsius: -3})
	assert.Equal(t, int32(0), GetDefaultMarginThreshold().DegradedCelsius)
}

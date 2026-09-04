package persistencemode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExpectedMode(t *testing.T) {
	// default preserves the historical behavior
	assert.Equal(t, ExpectedModeEnabled, GetDefaultExpectedMode())

	SetDefaultExpectedMode(ExpectedModeAny)
	assert.Equal(t, ExpectedModeAny, GetDefaultExpectedMode())

	SetDefaultExpectedMode(ExpectedModeEnabled)
	assert.Equal(t, ExpectedModeEnabled, GetDefaultExpectedMode())
}

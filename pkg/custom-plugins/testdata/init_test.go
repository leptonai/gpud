package testdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	specs := ExampleSpecs()
	assert.NotNil(t, specs)
}

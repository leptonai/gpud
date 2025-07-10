package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyOpts(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{})

		assert.NoError(t, err)
		assert.False(t, op.debug)
	})

	t.Run("with debug", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithDebug(true)})

		assert.NoError(t, err)
		assert.True(t, op.debug)
	})
}

func TestWithDebug(t *testing.T) {
	opt := WithDebug(true)
	op := &Op{}
	opt(op)

	assert.True(t, op.debug)
}

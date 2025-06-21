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
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.False(t, op.debug)
	})

	t.Run("with ibstat command", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithIbstatCommand("/custom/ibstat")})

		assert.NoError(t, err)
		assert.Equal(t, "/custom/ibstat", op.ibstatCommand)
		assert.False(t, op.debug)
	})

	t.Run("with debug", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithDebug(true)})

		assert.NoError(t, err)
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.True(t, op.debug)
	})

	t.Run("with multiple options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithIbstatCommand("/custom/ibstat"),
			WithDebug(true),
		})

		assert.NoError(t, err)
		assert.Equal(t, "/custom/ibstat", op.ibstatCommand)
		assert.True(t, op.debug)
	})
}

func TestWithIbstatCommand(t *testing.T) {
	opt := WithIbstatCommand("/test/ibstat")
	op := &Op{}
	opt(op)

	assert.Equal(t, "/test/ibstat", op.ibstatCommand)
}

func TestWithDebug(t *testing.T) {
	opt := WithDebug(true)
	op := &Op{}
	opt(op)

	assert.True(t, op.debug)
}

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
		assert.Equal(t, "ibstatus", op.ibstatusCommand)
		assert.False(t, op.debug)
	})

	t.Run("with ibstat command", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithIbstatCommand("/custom/ibstat")})

		assert.NoError(t, err)
		assert.Equal(t, "/custom/ibstat", op.ibstatCommand)
		assert.Equal(t, "ibstatus", op.ibstatusCommand)
		assert.False(t, op.debug)
	})

	t.Run("with ibstatus command", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithIbstatusCommand("/custom/ibstatus")})

		assert.NoError(t, err)
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.Equal(t, "/custom/ibstatus", op.ibstatusCommand)
		assert.False(t, op.debug)
	})

	t.Run("with debug", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{WithDebug(true)})

		assert.NoError(t, err)
		assert.Equal(t, "ibstat", op.ibstatCommand)
		assert.Equal(t, "ibstatus", op.ibstatusCommand)
		assert.True(t, op.debug)
	})

	t.Run("with multiple options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{
			WithIbstatCommand("/custom/ibstat"),
			WithIbstatusCommand("/custom/ibstatus"),
			WithDebug(true),
		})

		assert.NoError(t, err)
		assert.Equal(t, "/custom/ibstat", op.ibstatCommand)
		assert.Equal(t, "/custom/ibstatus", op.ibstatusCommand)
		assert.True(t, op.debug)
	})
}

func TestWithIbstatCommand(t *testing.T) {
	opt := WithIbstatCommand("/test/ibstat")
	op := &Op{}
	opt(op)

	assert.Equal(t, "/test/ibstat", op.ibstatCommand)
}

func TestWithIbstatusCommand(t *testing.T) {
	opt := WithIbstatusCommand("/test/ibstatus")
	op := &Op{}
	opt(op)

	assert.Equal(t, "/test/ibstatus", op.ibstatusCommand)
}

func TestWithDebug(t *testing.T) {
	opt := WithDebug(true)
	op := &Op{}
	opt(op)

	assert.True(t, op.debug)
}

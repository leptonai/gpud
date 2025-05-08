package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpApplyOpts(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.ApplyOpts([]OpOption{})

		assert.NoError(t, err)
		assert.Equal(t, "ibstat", op.IbstatCommand)
		assert.Equal(t, "ibstatus", op.IbstatusCommand)
	})

	t.Run("with custom ibstat", func(t *testing.T) {
		op := &Op{}
		customPath := "/custom/path/to/ibstat"
		err := op.ApplyOpts([]OpOption{WithIbstatCommand(customPath)})

		assert.NoError(t, err)
		assert.Equal(t, customPath, op.IbstatCommand)
		assert.Equal(t, "ibstatus", op.IbstatusCommand) // Default value
	})

	t.Run("with custom ibstatus", func(t *testing.T) {
		op := &Op{}
		customPath := "/custom/path/to/ibstatus"
		err := op.ApplyOpts([]OpOption{WithIbstatusCommand(customPath)})

		assert.NoError(t, err)
		assert.Equal(t, "ibstat", op.IbstatCommand) // Default value
		assert.Equal(t, customPath, op.IbstatusCommand)
	})

	t.Run("with both custom commands", func(t *testing.T) {
		op := &Op{}
		customIbstat := "/custom/path/to/ibstat"
		customIbstatus := "/custom/path/to/ibstatus"

		err := op.ApplyOpts([]OpOption{
			WithIbstatCommand(customIbstat),
			WithIbstatusCommand(customIbstatus),
		})

		assert.NoError(t, err)
		assert.Equal(t, customIbstat, op.IbstatCommand)
		assert.Equal(t, customIbstatus, op.IbstatusCommand)
	})

	t.Run("multiple options applied in order", func(t *testing.T) {
		op := &Op{}
		firstIbstat := "/first/path/ibstat"
		secondIbstat := "/second/path/ibstat"

		err := op.ApplyOpts([]OpOption{
			WithIbstatCommand(firstIbstat),
			WithIbstatCommand(secondIbstat),
		})

		assert.NoError(t, err)
		assert.Equal(t, secondIbstat, op.IbstatCommand) // Last one wins
		assert.Equal(t, "ibstatus", op.IbstatusCommand)
	})
}

func TestWithIbstatCommand(t *testing.T) {
	customPath := "/custom/ibstat"
	op := &Op{}

	option := WithIbstatCommand(customPath)
	option(op)

	assert.Equal(t, customPath, op.IbstatCommand)
}

func TestWithIbstatusCommand(t *testing.T) {
	customPath := "/custom/ibstatus"
	op := &Op{}

	option := WithIbstatusCommand(customPath)
	option(op)

	assert.Equal(t, customPath, op.IbstatusCommand)
}

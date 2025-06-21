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
	})

	t.Run("with custom ibstat", func(t *testing.T) {
		op := &Op{}
		customPath := "/custom/path/to/ibstat"
		err := op.ApplyOpts([]OpOption{WithIbstatCommand(customPath)})

		assert.NoError(t, err)
		assert.Equal(t, customPath, op.IbstatCommand)
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
	})
}

func TestWithIbstatCommand(t *testing.T) {
	customPath := "/custom/ibstat"
	op := &Op{}

	option := WithIbstatCommand(customPath)
	option(op)

	assert.Equal(t, customPath, op.IbstatCommand)
}

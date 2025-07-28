package scan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
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

func TestWithFailureInjector(t *testing.T) {
	tests := []struct {
		name     string
		injector *components.FailureInjector
	}{
		{
			name:     "nil injector",
			injector: nil,
		},
		{
			name: "empty injector",
			injector: &components.FailureInjector{
				GPUUUIDsWithRowRemappingPending: []string{},
				GPUUUIDsWithRowRemappingFailed:  []string{},
			},
		},
		{
			name: "injector with UUIDs",
			injector: &components.FailureInjector{
				GPUUUIDsWithRowRemappingPending: []string{"GPU-12345678-1234-1234-1234-123456789012"},
				GPUUUIDsWithRowRemappingFailed:  []string{"GPU-87654321-4321-4321-4321-210987654321"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithFailureInjector(tt.injector)
			option(op)

			assert.Equal(t, tt.injector, op.failureInjector)
		})
	}
}

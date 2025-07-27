package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
)

func TestOpApplyOpts(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.ApplyOpts([]OpOption{})

		assert.NoError(t, err)
	})
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

			assert.Equal(t, tt.injector, op.FailureInjector)
		})
	}
}

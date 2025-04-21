package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithIbstatCommand(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"set custom path", "/usr/local/bin/ibstat", "/usr/local/bin/ibstat"},
		{"set empty path", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithIbstatCommand(tt.path)
			opt(op)
			assert.Equal(t, tt.expected, op.IbstatCommand)
		})
	}
}

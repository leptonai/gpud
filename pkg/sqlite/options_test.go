package sqlite

import "testing"

func TestWithReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected bool
	}{
		{
			name:     "set read-only true",
			value:    true,
			expected: true,
		},
		{
			name:     "set read-only false",
			value:    false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			opt := WithReadOnly(tt.value)
			opt(op)

			if op.readOnly != tt.expected {
				t.Errorf("WithReadOnly(%v) = %v, want %v", tt.value, op.readOnly, tt.expected)
			}
		})
	}
}

func TestOp_applyOpts(t *testing.T) {
	t.Run("apply multiple options", func(t *testing.T) {
		op := &Op{}
		opts := []OpOption{
			WithReadOnly(true),
			WithReadOnly(false),
			WithReadOnly(true),
		}

		err := op.applyOpts(opts)
		if err != nil {
			t.Errorf("applyOpts() unexpected error = %v", err)
		}

		// Last option should take effect
		if !op.readOnly {
			t.Error("applyOpts() failed to apply last option")
		}
	})

	t.Run("apply no options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts(nil)
		if err != nil {
			t.Errorf("applyOpts(nil) unexpected error = %v", err)
		}

		// Should maintain default values
		if op.readOnly {
			t.Error("applyOpts(nil) modified default values")
		}
	})

	t.Run("apply empty options", func(t *testing.T) {
		op := &Op{}
		err := op.applyOpts([]OpOption{})
		if err != nil {
			t.Errorf("applyOpts([]) unexpected error = %v", err)
		}

		// Should maintain default values
		if op.readOnly {
			t.Error("applyOpts([]) modified default values")
		}
	})
}

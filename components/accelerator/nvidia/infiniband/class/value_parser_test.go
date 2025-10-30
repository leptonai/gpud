package class

import (
	"errors"
	"testing"
)

func TestNewValueParser(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "valid number",
			input: "123",
		},
		{
			name:  "string value",
			input: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := newValueParser(tt.input)
			if vp == nil {
				t.Fatal("newValueParser returned nil")
			}
			if vp.v != tt.input {
				t.Errorf("expected value %q, got %q", tt.input, vp.v)
			}
			if vp.err != nil {
				t.Errorf("expected nil error, got %v", vp.err)
			}
		})
	}
}

func TestValueParserPUInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *uint64
		wantErr bool
	}{
		{
			name:    "positive integer",
			input:   "123",
			want:    uint64Ptr(123),
			wantErr: false,
		},
		{
			name:    "zero",
			input:   "0",
			want:    uint64Ptr(0),
			wantErr: false,
		},
		{
			name:    "large positive integer",
			input:   "18446744073709551615", // max uint64
			want:    uint64Ptr(18446744073709551615),
			wantErr: false,
		},
		{
			name:    "hex number",
			input:   "0xFF",
			want:    uint64Ptr(255),
			wantErr: false,
		},
		{
			name:    "negative integer",
			input:   "-1",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid string",
			input:   "not a number",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "overflow",
			input:   "18446744073709551616", // max uint64 + 1
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := newValueParser(tt.input)
			got := vp.PUInt64()
			err := vp.Err()

			if (err != nil) != tt.wantErr {
				t.Errorf("PUInt64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil && got != nil {
				t.Errorf("PUInt64() = %v, want nil", *got)
			} else if tt.want != nil && got == nil {
				t.Errorf("PUInt64() = nil, want %v", *tt.want)
			} else if tt.want != nil && got != nil && *got != *tt.want {
				t.Errorf("PUInt64() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestValueParserErrorPropagation(t *testing.T) {
	// Test that once an error is set, subsequent operations return nil
	vp := newValueParser("invalid")

	// First call should set an error
	pUInt64 := vp.PUInt64()
	firstErr := vp.Err()
	if firstErr == nil {
		t.Fatal("expected error after parsing invalid input")
	}
	if pUInt64 != nil {
		t.Errorf("PUInt64() after error = %v, want nil", *pUInt64)
	}

	// Subsequent calls should return nil without changing the error
	pUInt64Again := vp.PUInt64()
	if pUInt64Again != nil {
		t.Errorf("PUInt64() after error = %v, want nil", *pUInt64Again)
	}

	// Error should remain the same
	if !errors.Is(vp.Err(), firstErr) {
		t.Errorf("error changed from %v to %v", firstErr, vp.Err())
	}
}

// Helper function
func uint64Ptr(v uint64) *uint64 {
	return &v
}

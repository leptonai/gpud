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

func TestValueParserInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "positive integer",
			input:   "123",
			want:    123,
			wantErr: false,
		},
		{
			name:    "negative integer",
			input:   "-456",
			want:    -456,
			wantErr: false,
		},
		{
			name:    "zero",
			input:   "0",
			want:    0,
			wantErr: false,
		},
		{
			name:    "hex number",
			input:   "0x1F",
			want:    31,
			wantErr: false,
		},
		{
			name:    "octal number",
			input:   "0755",
			want:    493,
			wantErr: false,
		},
		{
			name:    "invalid string",
			input:   "not a number",
			want:    0,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "float string",
			input:   "123.45",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := newValueParser(tt.input)
			got := vp.Int()
			err := vp.Err()

			if (err != nil) != tt.wantErr {
				t.Errorf("Int() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Int() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValueParserPInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *int64
		wantErr bool
	}{
		{
			name:    "positive integer",
			input:   "123",
			want:    int64Ptr(123),
			wantErr: false,
		},
		{
			name:    "negative integer",
			input:   "-456",
			want:    int64Ptr(-456),
			wantErr: false,
		},
		{
			name:    "large positive integer",
			input:   "9223372036854775807", // max int64
			want:    int64Ptr(9223372036854775807),
			wantErr: false,
		},
		{
			name:    "large negative integer",
			input:   "-9223372036854775808", // min int64
			want:    int64Ptr(-9223372036854775808),
			wantErr: false,
		},
		{
			name:    "invalid string",
			input:   "invalid",
			want:    int64Ptr(0), // PInt64 returns pointer to 0 when int64() fails
			wantErr: true,
		},
		{
			name:    "overflow",
			input:   "9223372036854775808", // max int64 + 1
			want:    int64Ptr(0),           // PInt64 returns pointer to 0 when int64() fails
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := newValueParser(tt.input)
			got := vp.PInt64()
			err := vp.Err()

			if (err != nil) != tt.wantErr {
				t.Errorf("PInt64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want == nil && got != nil {
				t.Errorf("PInt64() = %v, want nil", *got)
			} else if tt.want != nil && got == nil {
				t.Errorf("PInt64() = nil, want %v", *tt.want)
			} else if tt.want != nil && got != nil && *got != *tt.want {
				t.Errorf("PInt64() = %v, want %v", *got, *tt.want)
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
	// Test that once an error is set, subsequent operations return default values
	vp := newValueParser("invalid")

	// First call should set an error
	_ = vp.Int()
	firstErr := vp.Err()
	if firstErr == nil {
		t.Fatal("expected error after parsing invalid input")
	}

	// Subsequent calls should return nil/0 without changing the error
	pInt64 := vp.PInt64()
	if pInt64 != nil {
		t.Errorf("PInt64() after error = %v, want nil", *pInt64)
	}

	pUInt64 := vp.PUInt64()
	if pUInt64 != nil {
		t.Errorf("PUInt64() after error = %v, want nil", *pUInt64)
	}

	// Error should remain the same
	if !errors.Is(vp.Err(), firstErr) {
		t.Errorf("error changed from %v to %v", firstErr, vp.Err())
	}
}

func TestValueParserMultipleOperations(t *testing.T) {
	// Test calling multiple methods on the same parser
	vp := newValueParser("42")

	intVal := vp.Int()
	if intVal != 42 {
		t.Errorf("Int() = %v, want 42", intVal)
	}

	// Calling other methods after Int() should still work
	pInt64 := vp.PInt64()
	if pInt64 == nil || *pInt64 != 42 {
		t.Errorf("PInt64() = %v, want 42", pInt64)
	}

	pUInt64 := vp.PUInt64()
	if pUInt64 == nil || *pUInt64 != 42 {
		t.Errorf("PUInt64() = %v, want 42", pUInt64)
	}

	if vp.Err() != nil {
		t.Errorf("unexpected error: %v", vp.Err())
	}
}

func TestValueParserInt64Internal(t *testing.T) {
	// Test the internal int64() method through Int()
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{
			name:    "binary number",
			input:   "0b1010",
			want:    10,
			wantErr: false,
		},
		{
			name:    "scientific notation",
			input:   "1e3",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := newValueParser(tt.input)
			got := vp.Int()
			err := vp.Err()

			if (err != nil) != tt.wantErr {
				t.Errorf("int64() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("int64() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper functions
func int64Ptr(v int64) *int64 {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

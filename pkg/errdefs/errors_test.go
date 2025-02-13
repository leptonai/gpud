package errdefs

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wrapMsg  string
		checkFn  func(error) bool
		expected bool
	}{
		{
			name:     "direct invalid argument",
			err:      ErrInvalidArgument,
			checkFn:  IsInvalidArgument,
			expected: true,
		},
		{
			name:     "wrapped invalid argument",
			err:      fmt.Errorf("wrap: %w", ErrInvalidArgument),
			checkFn:  IsInvalidArgument,
			expected: true,
		},
		{
			name:     "direct not found",
			err:      ErrNotFound,
			checkFn:  IsNotFound,
			expected: true,
		},
		{
			name:     "wrapped not found",
			err:      fmt.Errorf("wrap: %w", ErrNotFound),
			checkFn:  IsNotFound,
			expected: true,
		},
		{
			name:     "direct already exists",
			err:      ErrAlreadyExists,
			checkFn:  IsAlreadyExists,
			expected: true,
		},
		{
			name:     "wrapped already exists",
			err:      fmt.Errorf("wrap: %w", ErrAlreadyExists),
			checkFn:  IsAlreadyExists,
			expected: true,
		},
		{
			name:     "direct failed precondition",
			err:      ErrFailedPrecondition,
			checkFn:  IsFailedPrecondition,
			expected: true,
		},
		{
			name:     "wrapped failed precondition",
			err:      fmt.Errorf("wrap: %w", ErrFailedPrecondition),
			checkFn:  IsFailedPrecondition,
			expected: true,
		},
		{
			name:     "direct unavailable",
			err:      ErrUnavailable,
			checkFn:  IsUnavailable,
			expected: true,
		},
		{
			name:     "wrapped unavailable",
			err:      fmt.Errorf("wrap: %w", ErrUnavailable),
			checkFn:  IsUnavailable,
			expected: true,
		},
		{
			name:     "direct not implemented",
			err:      ErrNotImplemented,
			checkFn:  IsNotImplemented,
			expected: true,
		},
		{
			name:     "wrapped not implemented",
			err:      fmt.Errorf("wrap: %w", ErrNotImplemented),
			checkFn:  IsNotImplemented,
			expected: true,
		},
		{
			name:     "direct context canceled",
			err:      context.Canceled,
			checkFn:  IsCanceled,
			expected: true,
		},
		{
			name:     "wrapped context canceled",
			err:      fmt.Errorf("wrap: %w", context.Canceled),
			checkFn:  IsCanceled,
			expected: true,
		},
		{
			name:     "direct deadline exceeded",
			err:      context.DeadlineExceeded,
			checkFn:  IsDeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped deadline exceeded",
			err:      fmt.Errorf("wrap: %w", context.DeadlineExceeded),
			checkFn:  IsDeadlineExceeded,
			expected: true,
		},
		// Negative test cases
		{
			name:     "different error type",
			err:      errors.New("some other error"),
			checkFn:  IsInvalidArgument,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			checkFn:  IsInvalidArgument,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.checkFn(tt.err)
			if got != tt.expected {
				t.Errorf("expected %v, got %v for error: %v", tt.expected, got, tt.err)
			}
		})
	}
}

func TestErrorWrapping(t *testing.T) {
	baseErr := errors.New("base error")
	tests := []struct {
		name       string
		err        error
		wrappedBy  error
		shouldWrap bool
	}{
		{
			name:       "invalid argument wrapping",
			err:        fmt.Errorf("wrap invalid argument: %w", ErrInvalidArgument),
			wrappedBy:  ErrInvalidArgument,
			shouldWrap: true,
		},
		{
			name:       "not found wrapping",
			err:        fmt.Errorf("wrap not found: %w", ErrNotFound),
			wrappedBy:  ErrNotFound,
			shouldWrap: true,
		},
		{
			name:       "already exists wrapping",
			err:        fmt.Errorf("wrap already exists: %w", ErrAlreadyExists),
			wrappedBy:  ErrAlreadyExists,
			shouldWrap: true,
		},
		{
			name:       "failed precondition wrapping",
			err:        fmt.Errorf("wrap failed precondition: %w", ErrFailedPrecondition),
			wrappedBy:  ErrFailedPrecondition,
			shouldWrap: true,
		},
		{
			name:       "unavailable wrapping",
			err:        fmt.Errorf("wrap unavailable: %w", ErrUnavailable),
			wrappedBy:  ErrUnavailable,
			shouldWrap: true,
		},
		{
			name:       "not implemented wrapping",
			err:        fmt.Errorf("wrap not implemented: %w", ErrNotImplemented),
			wrappedBy:  ErrNotImplemented,
			shouldWrap: true,
		},
		{
			name:       "different error types",
			err:        baseErr,
			wrappedBy:  ErrInvalidArgument,
			shouldWrap: false,
		},
		{
			name:       "nil error",
			err:        nil,
			wrappedBy:  ErrInvalidArgument,
			shouldWrap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := errors.Is(tt.err, tt.wrappedBy)
			if got != tt.shouldWrap {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.wrappedBy, got, tt.shouldWrap)
			}
		})
	}
}

func TestUnknownError(t *testing.T) {
	err := ErrUnknown
	if err.Error() != "unknown" {
		t.Errorf("ErrUnknown.Error() = %v, want 'unknown'", err.Error())
	}
}

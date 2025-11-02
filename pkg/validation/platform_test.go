package validation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name            string
		req             PlatformRequirements
		expectedErr     error
		expectedMsgFunc func(PlatformRequirements) string
	}{
		{
			name: "meets both requirements - no error",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: nil,
		},
		{
			name: "exactly meets requirements - no error",
			req: PlatformRequirements{
				LogicalCPUCores:  3,
				TotalMemoryBytes: 3 * 1024 * 1024 * 1024, // 3 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: nil,
		},
		{
			name: "insufficient CPU cores only",
			req: PlatformRequirements{
				LogicalCPUCores:  2,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientCPU,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: %d (minimum %d)", ErrInsufficientCPU, p.LogicalCPUCores, p.MinimumCPUCores)
			},
		},
		{
			name: "insufficient memory only",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientMemory,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: %s (minimum %s)", ErrInsufficientMemory, p.FormatMemoryHumanized(), p.FormatMinimumMemoryHumanized())
			},
		},
		{
			name: "insufficient both CPU and memory",
			req: PlatformRequirements{
				LogicalCPUCores:  2,
				TotalMemoryBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientResources,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: CPU cores: %d (minimum %d), memory: %s (minimum %s)",
					ErrInsufficientResources,
					p.LogicalCPUCores, p.MinimumCPUCores,
					p.FormatMemoryHumanized(), p.FormatMinimumMemoryHumanized())
			},
		},
		{
			name: "zero CPU cores",
			req: PlatformRequirements{
				LogicalCPUCores:  0,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientCPU,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: %d (minimum %d)", ErrInsufficientCPU, p.LogicalCPUCores, p.MinimumCPUCores)
			},
		},
		{
			name: "zero memory",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 0,
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientMemory,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: %s (minimum %s)", ErrInsufficientMemory, p.FormatMemoryHumanized(), p.FormatMinimumMemoryHumanized())
			},
		},
		{
			name: "high CPU but low memory",
			req: PlatformRequirements{
				LogicalCPUCores:  16,
				TotalMemoryBytes: 1 * 1024 * 1024 * 1024, // 1 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientMemory,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: %s (minimum %s)", ErrInsufficientMemory, p.FormatMemoryHumanized(), p.FormatMinimumMemoryHumanized())
			},
		},
		{
			name: "low CPU but high memory",
			req: PlatformRequirements{
				LogicalCPUCores:  1,
				TotalMemoryBytes: 16 * 1024 * 1024 * 1024, // 16 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectedErr: ErrInsufficientCPU,
			expectedMsgFunc: func(p PlatformRequirements) string {
				return fmt.Sprintf("%s: %d (minimum %d)", ErrInsufficientCPU, p.LogicalCPUCores, p.MinimumCPUCores)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Check()

			if tt.expectedErr == nil {
				require.NoError(t, err, "Check() expected no error")
				return
			}

			require.Error(t, err, "Check() expected error")
			require.ErrorIs(t, err, tt.expectedErr, "Check() error mismatch")

			if tt.expectedMsgFunc != nil {
				expectedMsg := tt.expectedMsgFunc(tt.req)
				assert.Equal(t, expectedMsg, err.Error(), "Check() error message mismatch")
			}
		})
	}
}

func TestGetPlatformRequirements(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := GetPlatformRequirements(ctx)
	require.NoError(t, err, "GetPlatformRequirements returned unexpected error")

	assert.Equal(t, MinimumLogicalCPUCores, req.MinimumCPUCores, "MinimumCPUCores mismatch")
	assert.EqualValues(t, MinimumMemoryBytes, req.MinimumMemory, "MinimumMemory mismatch")

	// Verify that observed values are reasonable
	assert.GreaterOrEqual(t, req.LogicalCPUCores, 1, "LogicalCPUCores should be at least 1")
	assert.GreaterOrEqual(t, req.TotalMemoryBytes, uint64(1024*1024), "TotalMemoryBytes should be at least 1 MiB")

	if checkErr := req.Check(); checkErr != nil {
		require.Truef(t,
			errors.Is(checkErr, ErrInsufficientCPU) ||
				errors.Is(checkErr, ErrInsufficientMemory) ||
				errors.Is(checkErr, ErrInsufficientResources),
			"Check() returned unexpected error: %v", checkErr,
		)

		t.Logf("Platform does not meet minimum requirements: %v", checkErr)
	}
}

func TestGetPlatformRequirementsWithNilContext(t *testing.T) {
	// GetPlatformRequirements handles context.TODO() gracefully
	// This test verifies that behavior works correctly
	req, err := GetPlatformRequirements(context.TODO())
	require.NoError(t, err, "GetPlatformRequirements returned unexpected error")

	assert.Equal(t, MinimumLogicalCPUCores, req.MinimumCPUCores, "MinimumCPUCores mismatch")
	assert.EqualValues(t, MinimumMemoryBytes, req.MinimumMemory, "MinimumMemory mismatch")

	if checkErr := req.Check(); checkErr != nil {
		require.Truef(t,
			errors.Is(checkErr, ErrInsufficientCPU) ||
				errors.Is(checkErr, ErrInsufficientMemory) ||
				errors.Is(checkErr, ErrInsufficientResources),
			"Check() returned unexpected error: %v", checkErr,
		)
		t.Logf("Platform does not meet minimum requirements: %v", checkErr)
	}
}

func TestFormatMemoryHumanized(t *testing.T) {
	tests := []struct {
		name     string
		bytes    uint64
		expected string
	}{
		{
			name:     "3 GiB",
			bytes:    3 * 1024 * 1024 * 1024,
			expected: "3.2 GB", // humanize uses decimal GB (1000-based), so 3 GiB = 3.2 GB
		},
		{
			name:     "2.5 GiB",
			bytes:    (5 * 1024 * 1024 * 1024) / 2,
			expected: "2.7 GB", // 2.5 GiB = 2.7 GB in decimal
		},
		{
			name:     "1024 MiB",
			bytes:    1024 * 1024 * 1024,
			expected: "1.1 GB", // 1 GiB = 1.1 GB in decimal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := PlatformRequirements{
				TotalMemoryBytes: tt.bytes,
			}
			result := req.FormatMemoryHumanized()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatMinimumMemoryHumanized(t *testing.T) {
	req := PlatformRequirements{
		MinimumMemory: 3 * 1024 * 1024 * 1024, // 3 GiB
	}

	result := req.FormatMinimumMemoryHumanized()
	expected := "3.2 GB" // humanize uses decimal GB, so 3 GiB = 3.2 GB

	assert.Equal(t, expected, result)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 3, MinimumLogicalCPUCores)

	expectedMemory := uint64(3 * 1024 * 1024 * 1024)
	assert.EqualValues(t, expectedMemory, MinimumMemoryBytes)
}

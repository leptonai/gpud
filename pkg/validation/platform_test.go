package validation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMeetsMinimum(t *testing.T) {
	tests := []struct {
		name     string
		req      PlatformRequirements
		expected bool
	}{
		{
			name: "meets both requirements",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expected: true,
		},
		{
			name: "exactly meets requirements",
			req: PlatformRequirements{
				LogicalCPUCores:  3,
				TotalMemoryBytes: 3 * 1024 * 1024 * 1024, // 3 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expected: true,
		},
		{
			name: "insufficient CPU cores",
			req: PlatformRequirements{
				LogicalCPUCores:  2,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expected: false,
		},
		{
			name: "insufficient memory",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expected: false,
		},
		{
			name: "insufficient both",
			req: PlatformRequirements{
				LogicalCPUCores:  2,
				TotalMemoryBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.req.MeetsMinimum()
			if result != tt.expected {
				t.Errorf("MeetsMinimum() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name           string
		req            PlatformRequirements
		expectErr      bool
		expectedErr    error
		errMsgContains string
	}{
		{
			name: "meets both requirements - no error",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr: false,
		},
		{
			name: "exactly meets requirements - no error",
			req: PlatformRequirements{
				LogicalCPUCores:  3,
				TotalMemoryBytes: 3 * 1024 * 1024 * 1024, // 3 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr: false,
		},
		{
			name: "insufficient CPU cores only",
			req: PlatformRequirements{
				LogicalCPUCores:  2,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientCPU,
			errMsgContains: "insufficient CPU cores",
		},
		{
			name: "insufficient memory only",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientMemory,
			errMsgContains: "insufficient memory",
		},
		{
			name: "insufficient both CPU and memory",
			req: PlatformRequirements{
				LogicalCPUCores:  2,
				TotalMemoryBytes: 2 * 1024 * 1024 * 1024, // 2 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientResources,
			errMsgContains: "insufficient",
		},
		{
			name: "zero CPU cores",
			req: PlatformRequirements{
				LogicalCPUCores:  0,
				TotalMemoryBytes: 4 * 1024 * 1024 * 1024, // 4 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientCPU,
			errMsgContains: "insufficient CPU cores",
		},
		{
			name: "zero memory",
			req: PlatformRequirements{
				LogicalCPUCores:  4,
				TotalMemoryBytes: 0,
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientMemory,
			errMsgContains: "insufficient memory",
		},
		{
			name: "high CPU but low memory",
			req: PlatformRequirements{
				LogicalCPUCores:  16,
				TotalMemoryBytes: 1 * 1024 * 1024 * 1024, // 1 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientMemory,
			errMsgContains: "insufficient memory",
		},
		{
			name: "low CPU but high memory",
			req: PlatformRequirements{
				LogicalCPUCores:  1,
				TotalMemoryBytes: 16 * 1024 * 1024 * 1024, // 16 GiB
				MinimumCPUCores:  3,
				MinimumMemory:    3 * 1024 * 1024 * 1024, // 3 GiB
			},
			expectErr:      true,
			expectedErr:    ErrInsufficientCPU,
			errMsgContains: "insufficient CPU cores",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.check()

			if tt.expectErr {
				if err == nil {
					t.Errorf("check() expected error but got nil")
					return
				}

				// Verify the error wraps the expected sentinel error
				if !errors.Is(err, tt.expectedErr) {
					t.Errorf("check() error = %v, want error wrapping %v", err, tt.expectedErr)
				}

				// Verify the error message contains expected text
				if tt.errMsgContains != "" {
					errMsg := err.Error()
					if len(errMsg) == 0 || !contains(errMsg, tt.errMsgContains) {
						t.Errorf("check() error message = %q, want to contain %q", errMsg, tt.errMsgContains)
					}
				}
			} else {
				if err != nil {
					t.Errorf("check() expected no error but got: %v", err)
				}
			}
		})
	}
}

// contains checks if a string contains a substring (helper function for tests)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMinimumResourceCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := MinimumResourceCheck(ctx)

	// Note: This may return an error if the actual system doesn't meet minimum requirements
	// We verify the consistency between err and MeetsMinimum()
	if err == nil {
		// System meets requirements
		if !req.MeetsMinimum() {
			t.Errorf("MinimumResourceCheck returned no error but MeetsMinimum() = false")
		}
		t.Logf("Platform meets minimum requirements: CPU cores=%d (min=%d), Memory=%s (min=%s)",
			req.LogicalCPUCores, req.MinimumCPUCores,
			req.FormatMemoryHumanized(), req.FormatMinimumMemoryHumanized())
	} else {
		// System doesn't meet requirements
		if req.MeetsMinimum() {
			t.Errorf("MinimumResourceCheck returned error but MeetsMinimum() = true")
		}
		// Verify it's one of our expected errors
		if !errors.Is(err, ErrInsufficientCPU) &&
			!errors.Is(err, ErrInsufficientMemory) &&
			!errors.Is(err, ErrInsufficientResources) {
			t.Fatalf("MinimumResourceCheck returned unexpected error type: %v", err)
		}
		t.Logf("Platform does NOT meet minimum requirements: %v", err)
	}

	// Verify that minimum thresholds are populated
	if req.MinimumCPUCores != MinimumLogicalCPUCores {
		t.Errorf("MinimumCPUCores = %d, want %d", req.MinimumCPUCores, MinimumLogicalCPUCores)
	}

	if req.MinimumMemory != MinimumMemoryBytes {
		t.Errorf("MinimumMemory = %d, want %d", req.MinimumMemory, MinimumMemoryBytes)
	}

	// Verify that observed values are reasonable
	if req.LogicalCPUCores < 1 {
		t.Errorf("LogicalCPUCores = %d, expected at least 1", req.LogicalCPUCores)
	}

	if req.TotalMemoryBytes < 1024*1024 { // At least 1 MiB
		t.Errorf("TotalMemoryBytes = %d, expected at least 1 MiB", req.TotalMemoryBytes)
	}
}

func TestMinimumResourceCheckWithNilContext(t *testing.T) {
	// MinimumResourceCheck handles context.TODO() gracefully
	// This test verifies that behavior works correctly
	req, err := MinimumResourceCheck(context.TODO())

	// Note: This may return an error if the actual system doesn't meet minimum requirements
	// We check both req and err are valid regardless
	if err == nil {
		// System meets requirements
		if !req.MeetsMinimum() {
			t.Errorf("MinimumResourceCheck returned no error but MeetsMinimum() = false")
		}
	} else {
		// System doesn't meet requirements
		if req.MeetsMinimum() {
			t.Errorf("MinimumResourceCheck returned error but MeetsMinimum() = true")
		}
		// Verify it's one of our expected errors
		if !errors.Is(err, ErrInsufficientCPU) &&
			!errors.Is(err, ErrInsufficientMemory) &&
			!errors.Is(err, ErrInsufficientResources) {
			t.Errorf("MinimumResourceCheck returned unexpected error type: %v", err)
		}
	}

	// Verify that minimum thresholds are populated
	if req.MinimumCPUCores != MinimumLogicalCPUCores {
		t.Errorf("MinimumCPUCores = %d, want %d", req.MinimumCPUCores, MinimumLogicalCPUCores)
	}

	if req.MinimumMemory != MinimumMemoryBytes {
		t.Errorf("MinimumMemory = %d, want %d", req.MinimumMemory, MinimumMemoryBytes)
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
			if result != tt.expected {
				t.Errorf("FormatMemoryHumanized() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatMinimumMemoryHumanized(t *testing.T) {
	req := PlatformRequirements{
		MinimumMemory: 3 * 1024 * 1024 * 1024, // 3 GiB
	}

	result := req.FormatMinimumMemoryHumanized()
	expected := "3.2 GB" // humanize uses decimal GB, so 3 GiB = 3.2 GB

	if result != expected {
		t.Errorf("FormatMinimumMemoryHumanized() = %q, want %q", result, expected)
	}
}

func TestConstants(t *testing.T) {
	if MinimumLogicalCPUCores != 3 {
		t.Errorf("MinimumLogicalCPUCores = %d, want 3", MinimumLogicalCPUCores)
	}

	expectedMemory := uint64(3 * 1024 * 1024 * 1024)
	if MinimumMemoryBytes != expectedMemory {
		t.Errorf("MinimumMemoryBytes = %d, want %d", MinimumMemoryBytes, expectedMemory)
	}
}

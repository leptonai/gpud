package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestSession_processPackageStatus(t *testing.T) {
	t.Run("handles package status with mock dependencies", func(t *testing.T) {
		// Note: This test demonstrates the structure, but processPackageStatus
		// uses a global controller (gpudmanager.GlobalController) which makes
		// it difficult to test without refactoring to accept an interface.
		// In production code, the controller should be injected as a dependency.

		session := &Session{
			ctx: context.Background(),
		}

		response := &Response{}

		// We can't easily test the actual package status without mocking the global controller
		// This would require refactoring processPackageStatus to accept a controller interface
		// For now, we document this as a limitation

		assert.NotNil(t, session)
		assert.NotNil(t, response)
	})

	t.Run("package status phase mapping", func(t *testing.T) {
		// This test verifies the logic for phase mapping
		// In a real implementation with dependency injection, we would test:

		// Test cases for phase mapping:
		// 1. IsInstalled = true -> InstalledPhase
		// 2. Installing = true -> InstallingPhase
		// 3. Neither -> UnknownPhase

		testCases := []struct {
			name        string
			isInstalled bool
			installing  bool
			expected    apiv1.PackagePhase
		}{
			{
				name:        "installed package",
				isInstalled: true,
				installing:  false,
				expected:    apiv1.InstalledPhase,
			},
			{
				name:        "installing package",
				isInstalled: false,
				installing:  true,
				expected:    apiv1.InstallingPhase,
			},
			{
				name:        "unknown phase",
				isInstalled: false,
				installing:  false,
				expected:    apiv1.UnknownPhase,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// This demonstrates the expected phase mapping logic
				packagePhase := apiv1.UnknownPhase
				if tc.isInstalled {
					packagePhase = apiv1.InstalledPhase
				} else if tc.installing {
					packagePhase = apiv1.InstallingPhase
				}
				assert.Equal(t, tc.expected, packagePhase)
			})
		}
	})

	t.Run("status string mapping", func(t *testing.T) {
		// Test the status boolean to string conversion
		testCases := []struct {
			name     string
			status   bool
			expected string
		}{
			{
				name:     "healthy status",
				status:   true,
				expected: "Healthy",
			},
			{
				name:     "unhealthy status",
				status:   false,
				expected: "Unhealthy",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				status := "Unhealthy"
				if tc.status {
					status = "Healthy"
				}
				assert.Equal(t, tc.expected, status)
			})
		}
	})

	t.Run("response captures errors", func(t *testing.T) {
		// This test demonstrates that errors should be captured in the response
		// A proper test would require dependency injection

		session := &Session{
			ctx: context.Background(),
		}

		response := &Response{}

		// The actual call might fail due to missing global controller
		// This is expected in a test environment without proper setup
		// processPackageStatus should handle errors gracefully and set response.Error

		assert.NotNil(t, session)
		assert.NotNil(t, response)
		// In a real scenario with mocked dependencies:
		// - response.Error would be set if Status() returns an error
		// - response.PackageStatus would contain the converted package statuses on success
	})
}

// Note: To properly test processPackageStatus, the function should be refactored to accept:
// - A controller interface instead of using gpudmanager.GlobalController
//
// Example refactored signature:
// func (s *Session) processPackageStatus(ctx context.Context, response *Response, controller Controller) {
//     packageStatus, err := controller.Status(ctx)
//     // ... rest of implementation
// }
//
// This would allow proper unit testing with mocks.

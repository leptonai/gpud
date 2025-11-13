package tailscale

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

// mockComponent creates a component with mock functions for testing
func mockComponent(
	ctx context.Context,
	isInstalled bool,
	isActive bool,
	activeError error,
) *component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:    cctx,
		cancel: cancel,
		checkTailscaledInstalled: func() bool {
			return isInstalled
		},
		checkServiceActiveFunc: func() (bool, error) {
			return isActive, activeError
		},
	}
	// Initialize lastCheckResult for tests that don't call CheckOnce
	if isInstalled && (!isActive || activeError != nil) {
		c.lastCheckResult = &checkResult{
			TailscaledServiceActive: isActive,
			health:                  apiv1.HealthStateTypeUnhealthy,
			err:                     activeError,
			reason:                  "tailscaled installed but tailscaled service is not active or failed to check",
		}
	} else if isInstalled && isActive {
		c.lastCheckResult = &checkResult{
			TailscaledServiceActive: true,
			health:                  apiv1.HealthStateTypeHealthy,
			reason:                  "tailscaled service is active/running",
		}
	} else {
		c.lastCheckResult = &checkResult{
			TailscaledServiceActive: false,
			health:                  apiv1.HealthStateTypeHealthy,
			reason:                  "tailscaled is not installed",
		}
	}
	return c
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,
	}
	c, err := New(gpudInstance)

	assert.NoError(t, err, "New should not return an error")
	assert.NotNil(t, c, "New should return a non-nil component")
	assert.Equal(t, Name, c.Name(), "Component name should match")

	// Type assertion to access internal fields
	tc, ok := c.(*component)
	require.True(t, ok, "Component should be of type *component")

	assert.NotNil(t, tc.ctx, "Context should be set")
	assert.NotNil(t, tc.cancel, "Cancel function should be set")
	assert.NotNil(t, tc.checkTailscaledInstalled, "checkTailscaledInstalledFunc should be set")
	assert.NotNil(t, tc.checkServiceActiveFunc, "checkServiceActive function should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, true, true, nil)

	assert.Equal(t, Name, c.Name(), "Component name should be 'tailscale'")
}

func TestTags(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, true, true, nil)

	expectedTags := []string{
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 1, "Component should return exactly 1 tag")
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := mockComponent(ctx, true, true, nil)

	err := c.Start()
	assert.NoError(t, err, "Start should not return an error")

	// Verify the background goroutine started by checking if CheckOnce updates lastCheckResult
	time.Sleep(100 * time.Millisecond) // Give some time for the goroutine to run

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult, "lastCheckResult should be updated after Start")
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, true, true, nil)

	err := c.Close()
	assert.NoError(t, err, "Close should not return an error")

	// Verify context is canceled
	select {
	case <-c.ctx.Done():
		// Context is canceled as expected
	default:
		t.Error("Context should be canceled after Close")
	}
}

func TestCheckOnce(t *testing.T) {
	testCases := []struct {
		name         string
		isInstalled  bool
		isActive     bool
		activeError  error
		expectActive bool
		expectError  bool
		expectReason string
	}{
		{
			name:         "tailscaled installed and active",
			isInstalled:  true,
			isActive:     true,
			activeError:  nil,
			expectActive: true,
			expectError:  false,
			expectReason: "tailscaled service is active/running",
		},
		{
			name:         "tailscaled installed but not active",
			isInstalled:  true,
			isActive:     false,
			activeError:  nil,
			expectActive: false,
			expectError:  false,
			expectReason: "tailscaled installed but tailscaled service is not active or failed to check",
		},
		{
			name:         "tailscaled installed but error checking active status",
			isInstalled:  true,
			isActive:     false,
			activeError:  errors.New("test error"),
			expectActive: false,
			expectError:  true,
			expectReason: "tailscaled installed but tailscaled service is not active or failed to check",
		},
		{
			name:         "tailscaled not installed",
			isInstalled:  false,
			isActive:     false,
			activeError:  nil,
			expectActive: false,
			expectError:  false,
			expectReason: "tailscaled is not installed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := mockComponent(ctx, tc.isInstalled, tc.isActive, tc.activeError)

			_ = c.Check()

			c.lastMu.RLock()
			lastCheckResult := c.lastCheckResult
			c.lastMu.RUnlock()

			assert.NotNil(t, lastCheckResult, "lastCheckResult should be set after CheckOnce")
			assert.Equal(t, tc.expectActive, lastCheckResult.TailscaledServiceActive,
				"TailscaledServiceActive should match expected value")
			assert.Equal(t, tc.expectReason, lastCheckResult.reason,
				"Reason should match expected value")

			if tc.expectError {
				if tc.activeError != nil {
					assert.Equal(t, tc.activeError, lastCheckResult.err, "Error should match expected error")
				} else {
					assert.NotNil(t, lastCheckResult.err, "Error should be set when expected")
				}
			} else {
				assert.Nil(t, lastCheckResult.err, "Error should not be set when not expected")
			}
		})
	}
}

func TestStates(t *testing.T) {
	testCases := []struct {
		name           string
		isInstalled    bool
		isActive       bool
		activeError    error
		expectedHealth apiv1.HealthStateType
		expectedStatus bool
	}{
		{
			name:           "tailscaled installed and active",
			isInstalled:    true,
			isActive:       true,
			activeError:    nil,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedStatus: true,
		},
		{
			name:           "tailscaled installed but not active",
			isInstalled:    true,
			isActive:       false,
			activeError:    nil,
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedStatus: false,
		},
		{
			name:           "tailscaled installed but error checking status",
			isInstalled:    true,
			isActive:       false,
			activeError:    errors.New("test error"),
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedStatus: false,
		},
		{
			name:           "tailscaled not installed",
			isInstalled:    false,
			isActive:       false,
			activeError:    nil,
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedStatus: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := mockComponent(ctx, tc.isInstalled, tc.isActive, tc.activeError)

			// Run CheckOnce to populate lastCheckResult
			_ = c.Check()

			// Get the states
			states := c.LastHealthStates()
			assert.Len(t, states, 1, "States should return exactly one state")

			state := states[0]
			assert.Equal(t, Name, state.Name, "State name should match component name")
			assert.Equal(t, tc.expectedHealth, state.Health, "Health status should match expected")
		})
	}
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, true, true, nil)

	events, err := c.Events(ctx, time.Now().Add(-time.Hour))

	assert.NoError(t, err, "Events should not return an error")
	assert.Nil(t, events, "Events should return nil")
}

func TestDataGetStates(t *testing.T) {
	testCases := []struct {
		name            string
		data            *checkResult
		expectedReason  string
		expectedHealth  apiv1.HealthStateType
		expectedHealthy bool
	}{
		{
			name: "active service",
			data: &checkResult{
				TailscaledServiceActive: true,
				health:                  apiv1.HealthStateTypeHealthy,
				reason:                  "tailscaled service is active/running",
			},
			expectedReason:  "tailscaled service is active/running",
			expectedHealth:  apiv1.HealthStateTypeHealthy,
			expectedHealthy: true,
		},
		{
			name: "inactive service",
			data: &checkResult{
				TailscaledServiceActive: false,
				health:                  apiv1.HealthStateTypeUnhealthy,
				reason:                  "tailscaled installed but tailscaled service is not active or failed to check",
			},
			expectedReason:  "tailscaled installed but tailscaled service is not active or failed to check",
			expectedHealth:  apiv1.HealthStateTypeUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "error state",
			data: &checkResult{
				err:    errors.New("test error"),
				health: apiv1.HealthStateTypeUnhealthy,
				reason: "tailscaled installed but tailscaled service is not active or failed to check",
			},
			expectedReason:  "tailscaled installed but tailscaled service is not active or failed to check",
			expectedHealth:  apiv1.HealthStateTypeUnhealthy,
			expectedHealthy: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states := tc.data.HealthStates()

			assert.Len(t, states, 1, "getStates should return exactly one state")

			state := states[0]
			assert.Equal(t, Name, state.Name, "State name should match component name")
			assert.Equal(t, tc.expectedReason, state.Reason, "State reason should match expected")
			assert.Equal(t, tc.expectedHealth, state.Health, "State health should match expected")

			// Check that Error field is set correctly
			if tc.data != nil && tc.data.err != nil {
				assert.Equal(t, tc.data.err.Error(), state.Error, "State error should match Data.err")
			} else {
				assert.Empty(t, state.Error, "State error should be empty when there's no error")
			}
		})
	}
}

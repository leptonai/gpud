package tailscale

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		checkDependencyInstalled: func() bool {
			return isInstalled
		},
		checkServiceActive: func(ctx context.Context) (bool, error) {
			return isActive, activeError
		},
	}
	return c
}

func TestNew(t *testing.T) {
	ctx := context.Background()
	c := New(ctx)

	assert.NotNil(t, c, "New should return a non-nil component")
	assert.Equal(t, Name, c.Name(), "Component name should match")

	// Type assertion to access internal fields
	tc, ok := c.(*component)
	require.True(t, ok, "Component should be of type *component")

	assert.NotNil(t, tc.ctx, "Context should be set")
	assert.NotNil(t, tc.cancel, "Cancel function should be set")
	assert.NotNil(t, tc.checkDependencyInstalled, "checkDependencyInstalled function should be set")
	assert.NotNil(t, tc.checkServiceActive, "checkServiceActive function should be set")
}

func TestName(t *testing.T) {
	ctx := context.Background()
	c := mockComponent(ctx, true, true, nil)

	assert.Equal(t, Name, c.Name(), "Component name should be 'tailscale'")
}

func TestStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := mockComponent(ctx, true, true, nil)

	err := c.Start()
	assert.NoError(t, err, "Start should not return an error")

	// Verify the background goroutine started by checking if CheckOnce updates lastData
	time.Sleep(100 * time.Millisecond) // Give some time for the goroutine to run

	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()

	assert.NotNil(t, lastData, "lastData should be updated after Start")
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
	}{
		{
			name:         "tailscaled installed and active",
			isInstalled:  true,
			isActive:     true,
			activeError:  nil,
			expectActive: true,
			expectError:  false,
		},
		{
			name:         "tailscaled installed but not active",
			isInstalled:  true,
			isActive:     false,
			activeError:  nil,
			expectActive: false,
			expectError:  true,
		},
		{
			name:         "tailscaled installed but error checking active status",
			isInstalled:  true,
			isActive:     false,
			activeError:  errors.New("test error"),
			expectActive: false,
			expectError:  true,
		},
		{
			name:         "tailscaled not installed",
			isInstalled:  false,
			isActive:     false,
			activeError:  nil,
			expectActive: false,
			expectError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := mockComponent(ctx, tc.isInstalled, tc.isActive, tc.activeError)

			c.CheckOnce()

			c.lastMu.RLock()
			lastData := c.lastData
			c.lastMu.RUnlock()

			assert.NotNil(t, lastData, "lastData should be set after CheckOnce")
			assert.Equal(t, tc.expectActive, lastData.TailscaledServiceActive,
				"TailscaledServiceActive should match expected value")

			if tc.expectError {
				assert.NotNil(t, lastData.err, "Error should be set when expected")
			} else if tc.isInstalled {
				assert.Nil(t, lastData.err, "Error should not be set when not expected")
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
		expectedHealth string
		expectedStatus bool
	}{
		{
			name:           "tailscaled installed and active",
			isInstalled:    true,
			isActive:       true,
			activeError:    nil,
			expectedHealth: components.StateHealthy,
			expectedStatus: true,
		},
		{
			name:           "tailscaled installed but not active",
			isInstalled:    true,
			isActive:       false,
			activeError:    nil,
			expectedHealth: components.StateUnhealthy,
			expectedStatus: false,
		},
		{
			name:           "tailscaled installed but error checking status",
			isInstalled:    true,
			isActive:       false,
			activeError:    errors.New("test error"),
			expectedHealth: components.StateUnhealthy,
			expectedStatus: false,
		},
		{
			name:           "tailscaled not installed",
			isInstalled:    false,
			isActive:       false,
			activeError:    nil,
			expectedHealth: components.StateHealthy,
			expectedStatus: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := mockComponent(ctx, tc.isInstalled, tc.isActive, tc.activeError)

			// Run CheckOnce to populate lastData
			c.CheckOnce()

			// Get the states
			states, err := c.States(ctx)

			assert.NoError(t, err, "States should not return an error")
			assert.Len(t, states, 1, "States should return exactly one state")

			state := states[0]
			assert.Equal(t, Name, state.Name, "State name should match component name")
			assert.Equal(t, tc.expectedHealth, state.Health, "Health status should match expected")
			assert.Equal(t, tc.expectedStatus, state.Healthy, "Healthy boolean should match expected")
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

func TestDataGetReason(t *testing.T) {
	testCases := []struct {
		name           string
		data           *Data
		expectedReason string
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedReason: "no tailscaled check yet",
		},
		{
			name: "error present",
			data: &Data{
				err: errors.New("test error"),
			},
			expectedReason: "tailscaled check failed -- test error",
		},
		{
			name: "service active",
			data: &Data{
				TailscaledServiceActive: true,
			},
			expectedReason: "tailscaled service is active/running",
		},
		{
			name: "service not active",
			data: &Data{
				TailscaledServiceActive: false,
			},
			expectedReason: "tailscaled service is not active",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reason := tc.data.getReason()
			assert.Equal(t, tc.expectedReason, reason, "Reason should match expected")
		})
	}
}

func TestDataGetHealth(t *testing.T) {
	testCases := []struct {
		name            string
		data            *Data
		expectedHealth  string
		expectedHealthy bool
	}{
		{
			name:            "nil data",
			data:            nil,
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
		{
			name: "error present",
			data: &Data{
				err: errors.New("test error"),
			},
			expectedHealth:  components.StateUnhealthy,
			expectedHealthy: false,
		},
		{
			name: "no error",
			data: &Data{
				TailscaledServiceActive: true,
			},
			expectedHealth:  components.StateHealthy,
			expectedHealthy: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			health, healthy := tc.data.getHealth()
			assert.Equal(t, tc.expectedHealth, health, "Health string should match expected")
			assert.Equal(t, tc.expectedHealthy, healthy, "Healthy boolean should match expected")
		})
	}
}

func TestDataGetStates(t *testing.T) {
	testCases := []struct {
		name           string
		data           *Data
		expectedReason string
		expectedHealth string
	}{
		{
			name:           "active service",
			data:           &Data{TailscaledServiceActive: true},
			expectedReason: "tailscaled service is active/running",
			expectedHealth: components.StateHealthy,
		},
		{
			name:           "inactive service",
			data:           &Data{TailscaledServiceActive: false},
			expectedReason: "tailscaled service is not active",
			expectedHealth: components.StateHealthy,
		},
		{
			name:           "error state",
			data:           &Data{err: errors.New("test error")},
			expectedReason: "tailscaled check failed -- test error",
			expectedHealth: components.StateUnhealthy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states, err := tc.data.getStates()

			// No longer checking if err is returned from getStates
			assert.NoError(t, err)
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

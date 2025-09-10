package disk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func TestComponent_SetHealthy(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockEventBucket)
		expectedError error
		checkResult   func(*testing.T, *mockEventBucket)
	}{
		{
			name: "successful purge with event bucket",
			setupMocks: func(mb *mockEventBucket) {
				mb.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(10, nil)
			},
			expectedError: nil,
			checkResult: func(t *testing.T, mb *mockEventBucket) {
				mb.AssertCalled(t, "Purge", mock.Anything, mock.AnythingOfType("int64"))
			},
		},
		{
			name: "purge returns error",
			setupMocks: func(mb *mockEventBucket) {
				mb.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(0, errors.New("purge failed"))
			},
			expectedError: errors.New("purge failed"),
			checkResult: func(t *testing.T, mb *mockEventBucket) {
				mb.AssertCalled(t, "Purge", mock.Anything, mock.AnythingOfType("int64"))
			},
		},
		{
			name:          "nil event bucket - no error",
			setupMocks:    nil,
			expectedError: nil,
			checkResult:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create a test component
			gpudInstance := &components.GPUdInstance{
				RootCtx: ctx,
			}

			comp, err := New(gpudInstance)
			require.NoError(t, err)
			defer comp.Close()

			c := comp.(*component)

			// Set up a fixed time for testing
			fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
			c.getTimeNowFunc = func() time.Time {
				return fixedTime
			}

			// Set up mocks if needed
			if tt.setupMocks != nil {
				mockBucket := &mockEventBucket{}
				tt.setupMocks(mockBucket)
				// Add the Close expectation for the mock
				mockBucket.On("Close").Return()
				c.eventBucket = mockBucket

				// Verify the interface implementation at compile time
				var _ components.HealthSettable = c
			}

			// Call SetHealthy
			err = c.SetHealthy()

			// Check error
			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}

			// Check mock expectations
			if tt.checkResult != nil && c.eventBucket != nil {
				tt.checkResult(t, c.eventBucket.(*mockEventBucket))

				// Verify that Purge was called with the correct timestamp
				mockBucket := c.eventBucket.(*mockEventBucket)
				calls := mockBucket.Calls
				for _, call := range calls {
					if call.Method == "Purge" {
						timestamp := call.Arguments[1].(int64)
						assert.Equal(t, fixedTime.Unix(), timestamp)
					}
				}
			}
		})
	}
}

func TestComponent_SetHealthy_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a test component
	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	defer comp.Close()

	c := comp.(*component)

	// Set up mock that simulates a slow Purge operation
	mockBucket := &mockEventBucket{}
	mockBucket.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		// Wait for context to be canceled
		<-ctx.Done()
	}).Return(0, context.Canceled)
	mockBucket.On("Close").Return()

	c.eventBucket = mockBucket

	// Cancel context before calling SetHealthy to ensure it handles context properly
	cancel()

	// Call SetHealthy - should handle canceled context gracefully
	err = c.SetHealthy()
	assert.Error(t, err)
}

func TestComponent_ImplementsHealthSettable(t *testing.T) {
	// This test ensures that component implements the HealthSettable interface
	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,
	}

	comp, err := New(gpudInstance)
	require.NoError(t, err)
	defer comp.Close()

	// This will fail to compile if component doesn't implement HealthSettable
	var _ components.HealthSettable = comp.(*component)
}

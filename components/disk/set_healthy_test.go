package disk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestComponent_SetHealthy(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockEventBucket)
		expectedError error
		checkResult   func(*testing.T, *mockEventBucket)
	}{
		{
			name: "successful purge with event bucket - inserts SetHealthy event",
			setupMocks: func(mb *mockEventBucket) {
				mb.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(10, nil)
				mb.On("Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				})).Return(nil, nil) // No existing SetHealthy event
				mb.On("Insert", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				})).Return(nil)
			},
			expectedError: nil,
			checkResult: func(t *testing.T, mb *mockEventBucket) {
				mb.AssertCalled(t, "Purge", mock.Anything, mock.AnythingOfType("int64"))
				mb.AssertCalled(t, "Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				}))
				mb.AssertCalled(t, "Insert", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				}))
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
		{
			name: "SetHealthy event already exists - skips insertion",
			setupMocks: func(mb *mockEventBucket) {
				mb.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(5, nil)
				existingEvent := &eventstore.Event{
					Time: time.Now(),
					Name: "SetHealthy",
				}
				mb.On("Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				})).Return(existingEvent, nil) // Existing SetHealthy event
			},
			expectedError: nil,
			checkResult: func(t *testing.T, mb *mockEventBucket) {
				mb.AssertCalled(t, "Purge", mock.Anything, mock.AnythingOfType("int64"))
				mb.AssertCalled(t, "Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				}))
				mb.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything) // Should not insert
			},
		},
		{
			name: "Find returns error",
			setupMocks: func(mb *mockEventBucket) {
				mb.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(3, nil)
				mb.On("Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				})).Return(nil, errors.New("find failed"))
			},
			expectedError: errors.New("find failed"),
			checkResult: func(t *testing.T, mb *mockEventBucket) {
				mb.AssertCalled(t, "Purge", mock.Anything, mock.AnythingOfType("int64"))
				mb.AssertCalled(t, "Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				}))
				mb.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
			},
		},
		{
			name: "Insert returns error",
			setupMocks: func(mb *mockEventBucket) {
				mb.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(7, nil)
				mb.On("Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				})).Return(nil, nil) // No existing event
				mb.On("Insert", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				})).Return(errors.New("insert failed"))
			},
			expectedError: errors.New("insert failed"),
			checkResult: func(t *testing.T, mb *mockEventBucket) {
				mb.AssertCalled(t, "Purge", mock.Anything, mock.AnythingOfType("int64"))
				mb.AssertCalled(t, "Find", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				}))
				mb.AssertCalled(t, "Insert", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
					return ev.Name == "SetHealthy"
				}))
			},
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

func TestComponent_SetHealthy_EventFields(t *testing.T) {
	ctx := context.Background()
	fixedTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	mockBucket := &mockEventBucket{}
	mockBucket.On("Purge", mock.Anything, mock.AnythingOfType("int64")).Return(1, nil)
	mockBucket.On("Find", mock.Anything, mock.Anything).Return(nil, nil)
	mockBucket.On("Insert", mock.Anything, mock.Anything).Return(nil)

	c := &component{
		ctx:         ctx,
		eventBucket: mockBucket,
		getTimeNowFunc: func() time.Time {
			return fixedTime
		},
	}

	err := c.SetHealthy()
	assert.NoError(t, err)

	// Verify the Insert call received the correct event fields
	mockBucket.AssertCalled(t, "Insert", mock.Anything, mock.MatchedBy(func(ev eventstore.Event) bool {
		return ev.Name == "SetHealthy" && ev.Component == Name && ev.Type == string(apiv1.EventTypeInfo)
	}))
}

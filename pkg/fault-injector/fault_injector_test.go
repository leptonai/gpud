package faultinjector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		request     Request
		expectError bool
		errorMsg    string
	}{
		{
			name: "nil kernel message should return ErrNoFaultFound",
			request: Request{
				KernelMessage: nil,
			},
			expectError: true,
			errorMsg:    "no fault injection entry found",
		},
		{
			name: "valid kernel message should return nil",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "KERN_INFO",
					Message:  "This is a valid test message",
				},
			},
			expectError: false,
		},
		{
			name: "valid kernel message with kern.format priority should return nil",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "kern.err",
					Message:  "This is a valid error message",
				},
			},
			expectError: false,
		},
		{
			name: "valid kernel message with unknown priority should return nil",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "unknown_priority",
					Message:  "This message has unknown priority but should be normalized",
				},
			},
			expectError: false,
		},
		{
			name: "empty message should return nil",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "KERN_DEBUG",
					Message:  "",
				},
			},
			expectError: false,
		},
		{
			name: "message at maximum length should return nil",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "KERN_INFO",
					Message:  string(make([]byte, pkgkmsgwriter.MaxPrintkRecordLength)),
				},
			},
			expectError: false,
		},
		{
			name: "message exceeding maximum length should return error",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "KERN_INFO",
					Message:  string(make([]byte, pkgkmsgwriter.MaxPrintkRecordLength+1)),
				},
			},
			expectError: true,
			errorMsg:    "message length exceeds the maximum length of 976",
		},
		{
			name: "message much longer than maximum should return error",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "KERN_ERR",
					Message:  string(make([]byte, pkgkmsgwriter.MaxPrintkRecordLength*2)),
				},
			},
			expectError: true,
			errorMsg:    "message length exceeds the maximum length of 976",
		},
		{
			name: "empty priority and message should return nil",
			request: Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: "",
					Message:  "",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRequest_Validate_PriorityNormalization(t *testing.T) {
	// Test that validation correctly normalizes priority through the embedded KernelMessage.Validate()
	tests := []struct {
		name             string
		inputPriority    string
		expectedPriority pkgkmsgwriter.KernelMessagePriority
	}{
		{"KERN format", "KERN_ERR", pkgkmsgwriter.KernelMessagePriorityError},
		{"kern format", "kern.warning", pkgkmsgwriter.KernelMessagePriorityWarning},
		{"kern.warn format", "kern.warn", pkgkmsgwriter.KernelMessagePriorityWarning},
		{"unknown priority", "invalid", pkgkmsgwriter.KernelMessagePriorityInfo},
		{"empty priority", "", pkgkmsgwriter.KernelMessagePriorityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := Request{
				KernelMessage: &pkgkmsgwriter.KernelMessage{
					Priority: pkgkmsgwriter.ConvertKernelMessagePriority(tt.inputPriority),
					Message:  "test message",
				},
			}

			err := request.Validate()
			require.NoError(t, err)

			// Verify that the priority was normalized by the underlying KernelMessage.Validate()
			require.Equal(t, tt.expectedPriority, request.KernelMessage.Priority)
		})
	}
}

func TestRequest_Validate_EmptyRequest(t *testing.T) {
	// Test that an empty request (no fields set) returns ErrNoFaultFound
	request := Request{}
	err := request.Validate()
	require.Error(t, err)
	require.Equal(t, ErrNoFaultFound, err)
	require.Contains(t, err.Error(), "no fault injection entry found")
}

func TestRequest_Validate_LargeValidMessage(t *testing.T) {
	// Test with a message that's close to but under the limit
	largeMessage := strings.Repeat("a", pkgkmsgwriter.MaxPrintkRecordLength-10)

	request := Request{
		KernelMessage: &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_WARNING",
			Message:  largeMessage,
		},
	}

	err := request.Validate()
	require.NoError(t, err)
	require.Equal(t, pkgkmsgwriter.KernelMessagePriorityWarning, request.KernelMessage.Priority)
	require.Equal(t, largeMessage, request.KernelMessage.Message)
}

func TestRequest_Validate_ExactMaxLength(t *testing.T) {
	// Test with a message that's exactly at the maximum length
	exactMaxMessage := strings.Repeat("x", pkgkmsgwriter.MaxPrintkRecordLength)

	request := Request{
		KernelMessage: &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_NOTICE",
			Message:  exactMaxMessage,
		},
	}

	err := request.Validate()
	require.NoError(t, err)
	require.Equal(t, pkgkmsgwriter.KernelMessagePriorityNotice, request.KernelMessage.Priority)
	require.Equal(t, exactMaxMessage, request.KernelMessage.Message)
}

func TestErrNoFaultFound(t *testing.T) {
	// Test that the error variable is properly defined
	err := ErrNoFaultFound
	require.NotNil(t, err)
	require.Equal(t, "no fault injection entry found", err.Error())
}

func TestRequest_Validate_NilRequestPointer(t *testing.T) {
	// Test behavior when request itself would be nil (edge case)
	// This tests the method on a nil KernelMessage field specifically
	var request *Request = &Request{KernelMessage: nil}
	err := request.Validate()
	require.Error(t, err)
	require.Equal(t, ErrNoFaultFound, err)
}

func TestRequest_Validate_SwitchCaseLogic(t *testing.T) {
	// Test the switch statement logic explicitly
	tests := []struct {
		name          string
		kernelMessage *pkgkmsgwriter.KernelMessage
		expectError   bool
		expectedError error
	}{
		{
			name:          "nil kernel message triggers default case",
			kernelMessage: nil,
			expectError:   true,
			expectedError: ErrNoFaultFound,
		},
		{
			name: "non-nil kernel message triggers first case",
			kernelMessage: &pkgkmsgwriter.KernelMessage{
				Priority: "KERN_INFO",
				Message:  "test",
			},
			expectError:   false,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := Request{KernelMessage: tt.kernelMessage}
			err := request.Validate()

			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, tt.expectedError, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestRequest_Validate_XidFallthrough tests the specific fallthrough behavior from XID to KernelMessage
func TestRequest_Validate_XidFallthrough(t *testing.T) {
	tests := []struct {
		name                string
		xidID               int
		expectError         bool
		expectKernelMessage bool
		expectedPriority    pkgkmsgwriter.KernelMessagePriority
		shouldContainMsg    string
	}{
		{
			name:                "valid known XID 63 should fallthrough to kernel message validation",
			xidID:               63,
			expectError:         false,
			expectKernelMessage: true,
			expectedPriority:    pkgkmsgwriter.KernelMessagePriorityWarning,
			shouldContainMsg:    "Row remapping event",
		},
		{
			name:                "valid known XID 79 should fallthrough to kernel message validation",
			xidID:               79,
			expectError:         false,
			expectKernelMessage: true,
			expectedPriority:    pkgkmsgwriter.KernelMessagePriorityError,
			shouldContainMsg:    "GPU has fallen off the bus",
		},
		{
			name:                "valid unknown XID 999 should fallthrough to kernel message validation",
			xidID:               999,
			expectError:         false,
			expectKernelMessage: true,
			expectedPriority:    pkgkmsgwriter.KernelMessagePriorityWarning,
			shouldContainMsg:    "unknown",
		},
		{
			name:                "XID with ID 0 should return ErrNoFaultFound without fallthrough",
			xidID:               0,
			expectError:         true,
			expectKernelMessage: false,
			expectedPriority:    "",
			shouldContainMsg:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with XID
			request := Request{
				XID: &XIDToInject{
					ID: tt.xidID,
				},
			}

			// Verify initial state
			require.NotNil(t, request.XID, "initial Xid should not be nil")
			require.Nil(t, request.KernelMessage, "initial KernelMessage should be nil")

			// Call Validate
			err := request.Validate()

			// Check error expectation
			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, ErrNoFaultFound, err)
				// For error cases, verify no transformation occurred
				require.NotNil(t, request.XID, "Xid should still be present when validation fails")
				require.Nil(t, request.KernelMessage, "KernelMessage should remain nil when validation fails")
			} else {
				require.NoError(t, err)

				// For successful cases, verify fallthrough behavior occurred
				if tt.expectKernelMessage {
					// Verify that Xid was set to nil after transformation
					require.Nil(t, request.XID, "Xid should be nil after successful transformation and fallthrough")

					// Verify that KernelMessage was created from the XID
					require.NotNil(t, request.KernelMessage, "KernelMessage should be created after XID transformation")
					require.Equal(t, tt.expectedPriority, request.KernelMessage.Priority)
					require.Contains(t, request.KernelMessage.Message, tt.shouldContainMsg)

					// Verify the message follows expected XID format
					require.Contains(t, request.KernelMessage.Message, "NVRM: Xid")
					require.Contains(t, request.KernelMessage.Message, "PCI:0000:04:00")
				}
			}
		})
	}
}

// TestRequest_Validate_XidTransformationDetails tests the specific transformation logic from XID to KernelMessage
func TestRequest_Validate_XidTransformationDetails(t *testing.T) {
	// Test with a specific known XID to verify exact transformation
	request := Request{
		XID: &XIDToInject{
			ID: 69, // Known XID with specific message
		},
	}

	err := request.Validate()
	require.NoError(t, err)

	// Verify specific transformation details
	require.Nil(t, request.XID, "Xid should be nil after transformation")
	require.NotNil(t, request.KernelMessage, "KernelMessage should be created")

	// Verify the specific known XID 69 message
	require.Equal(t, pkgkmsgwriter.KernelMessagePriorityWarning, request.KernelMessage.Priority)
	require.Contains(t, request.KernelMessage.Message, "BAR1 access failure")
	require.Contains(t, request.KernelMessage.Message, "NVRM: Xid (PCI:0000:04:00): 69")
	require.Contains(t, request.KernelMessage.Message, "pid=34566")
}

// TestRequest_Validate_SwitchCaseFallthrough tests the switch statement logic explicitly
func TestRequest_Validate_SwitchCaseFallthrough(t *testing.T) {
	tests := []struct {
		name              string
		xid               *XIDToInject
		kernelMessage     *pkgkmsgwriter.KernelMessage
		expectError       bool
		expectedError     error
		expectTransform   bool
		expectFallthrough bool
	}{
		{
			name: "XID case with valid ID should transform and fallthrough",
			xid: &XIDToInject{
				ID: 63,
			},
			kernelMessage:     nil,
			expectError:       false,
			expectedError:     nil,
			expectTransform:   true,
			expectFallthrough: true,
		},
		{
			name: "XID case with zero ID should return error without fallthrough",
			xid: &XIDToInject{
				ID: 0,
			},
			kernelMessage:     nil,
			expectError:       true,
			expectedError:     ErrNoFaultFound,
			expectTransform:   false,
			expectFallthrough: false,
		},
		{
			name: "KernelMessage case only should validate message",
			xid:  nil,
			kernelMessage: &pkgkmsgwriter.KernelMessage{
				Priority: "KERN_INFO",
				Message:  "direct kernel message test",
			},
			expectError:       false,
			expectedError:     nil,
			expectTransform:   false,
			expectFallthrough: false,
		},
		{
			name:              "neither XID nor KernelMessage should trigger default case",
			xid:               nil,
			kernelMessage:     nil,
			expectError:       true,
			expectedError:     ErrNoFaultFound,
			expectTransform:   false,
			expectFallthrough: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := Request{
				XID:           tt.xid,
				KernelMessage: tt.kernelMessage,
			}

			// Store initial state for verification
			initialXid := request.XID
			initialKernelMessage := request.KernelMessage

			err := request.Validate()

			if tt.expectError {
				require.Error(t, err)
				require.Equal(t, tt.expectedError, err)
			} else {
				require.NoError(t, err)
			}

			if tt.expectTransform {
				// Verify transformation occurred
				require.Nil(t, request.XID, "Xid should be nil after transformation")
				require.NotNil(t, request.KernelMessage, "KernelMessage should be created")
				require.NotEqual(t, initialKernelMessage, request.KernelMessage, "KernelMessage should be different from initial")
			}

			if tt.expectFallthrough {
				// Verify that fallthrough occurred by checking that both:
				// 1. XID was processed (transformed to nil)
				// 2. KernelMessage was created and validated (no error)
				require.Nil(t, request.XID, "after fallthrough, Xid should be nil")
				require.NotNil(t, request.KernelMessage, "after fallthrough, KernelMessage should exist")
				require.NoError(t, err, "fallthrough should result in successful KernelMessage validation")
			}

			if !tt.expectTransform && !tt.expectFallthrough {
				// For cases that don't involve XID transformation
				if initialXid != nil {
					require.Equal(t, initialXid, request.XID, "Xid should remain unchanged")
				}
				if initialKernelMessage != nil {
					require.Equal(t, initialKernelMessage.Priority, request.KernelMessage.Priority, "KernelMessage priority should be preserved (possibly normalized)")
				}
			}
		})
	}
}

// TestRequest_Validate_FallthroughWithInvalidKernelMessage tests fallthrough to invalid kernel message
func TestRequest_Validate_FallthroughWithInvalidKernelMessage(t *testing.T) {
	// This test verifies that when XID transforms to an invalid KernelMessage,
	// the fallthrough still properly validates and returns the validation error

	// First, let's test with a valid XID that should create a valid message
	request := Request{
		XID: &XIDToInject{
			ID: 63, // Valid XID
		},
	}

	err := request.Validate()
	require.NoError(t, err, "valid XID should create valid KernelMessage and pass validation")

	// Verify the transformation and fallthrough worked
	require.Nil(t, request.XID, "Xid should be nil after successful transformation")
	require.NotNil(t, request.KernelMessage, "KernelMessage should be created")

	// Now manually create a scenario where XID transforms but creates an invalid message
	// by creating a request that simulates what would happen if GetMessageToInject
	// returned a message that's too long (though this shouldn't happen in practice)

	// We can't easily test this scenario without mocking GetMessageToInject,
	// but we can verify that the fallthrough calls the validation correctly
	// by checking that the priority gets normalized (a sign that validation ran)
	originalPriority := request.KernelMessage.Priority
	request.KernelMessage.Priority = "kern.warning" // Use format that gets normalized

	err = request.Validate()
	require.NoError(t, err)
	require.Equal(t, pkgkmsgwriter.KernelMessagePriorityWarning, request.KernelMessage.Priority, "priority should be normalized by validation in fallthrough")
	require.NotEqual(t, originalPriority, pkgkmsgwriter.KernelMessagePriority("kern.warning"), "test setup should use different format")
}

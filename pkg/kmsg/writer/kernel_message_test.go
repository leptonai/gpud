package writer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKernelMessage_JSON(t *testing.T) {
	tests := []struct {
		name     string
		msg      KernelMessage
		expected string
	}{
		{
			name: "basic kernel message",
			msg: KernelMessage{
				Priority: KernelMessagePriorityInfo,
				Message:  "test message",
			},
			expected: `{"priority":"KERN_INFO","message":"test message"}`,
		},
		{
			name: "empty message",
			msg: KernelMessage{
				Priority: KernelMessagePriorityError,
				Message:  "",
			},
			expected: `{"priority":"KERN_ERR","message":""}`,
		},
		{
			name: "empty priority",
			msg: KernelMessage{
				Priority: "",
				Message:  "test message",
			},
			expected: `{"priority":"","message":"test message"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Failed to marshal KernelMessage: %v", err)
			}

			if string(data) != tt.expected {
				t.Errorf("Marshal result = %s, want %s", string(data), tt.expected)
			}

			// Test unmarshaling
			var unmarshaled KernelMessage
			err = json.Unmarshal(data, &unmarshaled)
			if err != nil {
				t.Fatalf("Failed to unmarshal KernelMessage: %v", err)
			}

			if unmarshaled.Priority != tt.msg.Priority {
				t.Errorf("Unmarshaled Priority = %s, want %s", unmarshaled.Priority, tt.msg.Priority)
			}

			if unmarshaled.Message != tt.msg.Message {
				t.Errorf("Unmarshaled Message = %s, want %s", unmarshaled.Message, tt.msg.Message)
			}
		})
	}
}

func TestConvertKernelMessagePriority(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected KernelMessagePriority
	}{
		// KERN_ format cases
		{
			name:     "KERN_EMERG",
			input:    "KERN_EMERG",
			expected: KernelMessagePriorityEmerg,
		},
		{
			name:     "KERN_ALERT",
			input:    "KERN_ALERT",
			expected: KernelMessagePriorityAlert,
		},
		{
			name:     "KERN_CRIT",
			input:    "KERN_CRIT",
			expected: KernelMessagePriorityCrit,
		},
		{
			name:     "KERN_ERR",
			input:    "KERN_ERR",
			expected: KernelMessagePriorityError,
		},
		{
			name:     "KERN_WARNING",
			input:    "KERN_WARNING",
			expected: KernelMessagePriorityWarning,
		},
		{
			name:     "KERN_NOTICE",
			input:    "KERN_NOTICE",
			expected: KernelMessagePriorityNotice,
		},
		{
			name:     "KERN_INFO",
			input:    "KERN_INFO",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "KERN_DEBUG",
			input:    "KERN_DEBUG",
			expected: KernelMessagePriorityDebug,
		},
		{
			name:     "KERN_DEFAULT",
			input:    "KERN_DEFAULT",
			expected: KernelMessagePriorityDefault,
		},
		// kern. format cases
		{
			name:     "kern.emerg",
			input:    "kern.emerg",
			expected: KernelMessagePriorityEmerg,
		},
		{
			name:     "kern.alert",
			input:    "kern.alert",
			expected: KernelMessagePriorityAlert,
		},
		{
			name:     "kern.crit",
			input:    "kern.crit",
			expected: KernelMessagePriorityCrit,
		},
		{
			name:     "kern.err",
			input:    "kern.err",
			expected: KernelMessagePriorityError,
		},
		{
			name:     "kern.warning",
			input:    "kern.warning",
			expected: KernelMessagePriorityWarning,
		},
		{
			name:     "kern.warn",
			input:    "kern.warn",
			expected: KernelMessagePriorityWarning,
		},
		{
			name:     "kern.notice",
			input:    "kern.notice",
			expected: KernelMessagePriorityNotice,
		},
		{
			name:     "kern.info",
			input:    "kern.info",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "kern.debug",
			input:    "kern.debug",
			expected: KernelMessagePriorityDebug,
		},
		{
			name:     "kern.default",
			input:    "kern.default",
			expected: KernelMessagePriorityDefault,
		},
		// Edge cases
		{
			name:     "unknown priority",
			input:    "unknown",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "empty string",
			input:    "",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "case sensitive - lowercase KERN_ERR",
			input:    "kern_err",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "case sensitive - uppercase kern.err",
			input:    "KERN.ERR",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "numeric input",
			input:    "123",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "special characters",
			input:    "KERN@ERR",
			expected: KernelMessagePriorityInfo,
		},
		{
			name:     "whitespace",
			input:    " KERN_ERR ",
			expected: KernelMessagePriorityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertKernelMessagePriority(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertKernelMessagePriority(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConvertKernelMessagePriority_AllValidCases(t *testing.T) {
	// Test all valid cases to ensure complete coverage
	validCases := map[string]KernelMessagePriority{
		"KERN_EMERG":   KernelMessagePriorityEmerg,
		"kern.emerg":   KernelMessagePriorityEmerg,
		"KERN_ALERT":   KernelMessagePriorityAlert,
		"kern.alert":   KernelMessagePriorityAlert,
		"KERN_CRIT":    KernelMessagePriorityCrit,
		"kern.crit":    KernelMessagePriorityCrit,
		"KERN_ERR":     KernelMessagePriorityError,
		"kern.err":     KernelMessagePriorityError,
		"KERN_WARNING": KernelMessagePriorityWarning,
		"kern.warning": KernelMessagePriorityWarning,
		"kern.warn":    KernelMessagePriorityWarning,
		"KERN_NOTICE":  KernelMessagePriorityNotice,
		"kern.notice":  KernelMessagePriorityNotice,
		"KERN_INFO":    KernelMessagePriorityInfo,
		"kern.info":    KernelMessagePriorityInfo,
		"KERN_DEBUG":   KernelMessagePriorityDebug,
		"kern.debug":   KernelMessagePriorityDebug,
		"KERN_DEFAULT": KernelMessagePriorityDefault,
		"kern.default": KernelMessagePriorityDefault,
	}

	for input, expected := range validCases {
		result := ConvertKernelMessagePriority(input)
		if result != expected {
			t.Errorf("ConvertKernelMessagePriority(%q) = %q, want %q", input, result, expected)
		}
	}
}

func TestConvertKernelMessagePriority_DefaultBehavior(t *testing.T) {
	// Test that unknown inputs default to KERN_INFO
	unknownInputs := []string{
		"random",
		"invalid",
		"KERN_UNKNOWN",
		"kern.unknown",
		"123",
		"!@#$%",
		"KERN_ERR_TYPO",
		"kern.info.extra",
	}

	for _, input := range unknownInputs {
		result := ConvertKernelMessagePriority(input)
		if result != KernelMessagePriorityInfo {
			t.Errorf("ConvertKernelMessagePriority(%q) = %q, want %q (default)", input, result, KernelMessagePriorityInfo)
		}
	}
}

func TestKernelMessage_Validate(t *testing.T) {
	tests := []struct {
		name        string
		msg         KernelMessage
		expectedErr bool
		expectedMsg string
	}{
		{
			name: "valid message with correct priority",
			msg: KernelMessage{
				Priority: KernelMessagePriorityInfo,
				Message:  "This is a valid message",
			},
			expectedErr: false,
		},
		{
			name: "valid message with kern.format priority",
			msg: KernelMessage{
				Priority: "kern.err",
				Message:  "This is a valid message",
			},
			expectedErr: false,
		},
		{
			name: "valid message with unknown priority (should be normalized)",
			msg: KernelMessage{
				Priority: "unknown_priority",
				Message:  "This is a valid message",
			},
			expectedErr: false,
		},
		{
			name: "valid message at maximum length",
			msg: KernelMessage{
				Priority: KernelMessagePriorityInfo,
				Message:  string(make([]byte, MaxPrintkRecordLength)),
			},
			expectedErr: false,
		},
		{
			name: "invalid message exceeding maximum length",
			msg: KernelMessage{
				Priority: KernelMessagePriorityInfo,
				Message:  string(make([]byte, MaxPrintkRecordLength+1)),
			},
			expectedErr: true,
			expectedMsg: "message length exceeds the maximum length of 976",
		},
		{
			name: "invalid message much longer than maximum",
			msg: KernelMessage{
				Priority: KernelMessagePriorityError,
				Message:  string(make([]byte, MaxPrintkRecordLength*2)),
			},
			expectedErr: true,
			expectedMsg: "message length exceeds the maximum length of 976",
		},
		{
			name: "empty message",
			msg: KernelMessage{
				Priority: KernelMessagePriorityDebug,
				Message:  "",
			},
			expectedErr: false,
		},
		{
			name: "empty priority and message",
			msg: KernelMessage{
				Priority: "",
				Message:  "",
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalPriority := tt.msg.Priority
			err := tt.msg.Validate()

			if tt.expectedErr {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if err.Error() != tt.expectedMsg {
					t.Errorf("Expected error message %q, got %q", tt.expectedMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
					return
				}

				// Check that priority was normalized correctly
				expectedPriority := ConvertKernelMessagePriority(string(originalPriority))
				if tt.msg.Priority != expectedPriority {
					t.Errorf("Priority not normalized correctly: got %q, want %q", tt.msg.Priority, expectedPriority)
				}
			}
		})
	}
}

func TestMaxPrintkRecordLength(t *testing.T) {
	// Test that the constant has the expected value
	expected := 1024 - 48
	if MaxPrintkRecordLength != expected {
		t.Errorf("MaxPrintkRecordLength = %d, want %d", MaxPrintkRecordLength, expected)
	}

	// Test that the constant value is 976
	if MaxPrintkRecordLength != 976 {
		t.Errorf("MaxPrintkRecordLength = %d, want 976", MaxPrintkRecordLength)
	}
}

func TestKernelMessage_Validate_PriorityNormalization(t *testing.T) {
	// Test that Validate correctly normalizes various priority formats
	tests := []struct {
		name             string
		inputPriority    KernelMessagePriority
		expectedPriority KernelMessagePriority
	}{
		{"KERN format", KernelMessagePriorityError, KernelMessagePriorityError},
		{"kern format", "kern.warning", KernelMessagePriorityWarning},
		{"kern.warn format", "kern.warn", KernelMessagePriorityWarning},
		{"unknown priority", "invalid", KernelMessagePriorityInfo},
		{"empty priority", "", KernelMessagePriorityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := KernelMessage{
				Priority: tt.inputPriority,
				Message:  "test message",
			}

			err := msg.Validate()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if msg.Priority != tt.expectedPriority {
				t.Errorf("Priority normalization failed: got %q, want %q", msg.Priority, tt.expectedPriority)
			}
		})
	}
}

func Test_kmsgWriterWithDummyDevice_UpdateKernelMessage(t *testing.T) {
	writer := &kmsgWriterWithDummyDevice{}

	msg := KernelMessage{
		Priority: "kern.info",
		Message:  "test message",
	}

	_ = writer.Write(&msg)

	require.Equal(t, msg.Priority, KernelMessagePriorityInfo)
}

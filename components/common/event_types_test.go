package common

import "testing"

func TestEventTypeFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected EventType
	}{
		{
			name:     "Info event type",
			input:    "Info",
			expected: EventTypeInfo,
		},
		{
			name:     "Warning event type",
			input:    "Warning",
			expected: EventTypeWarning,
		},
		{
			name:     "Critical event type",
			input:    "Critical",
			expected: EventTypeCritical,
		},
		{
			name:     "Fatal event type",
			input:    "Fatal",
			expected: EventTypeFatal,
		},
		{
			name:     "Unknown event type",
			input:    "NonExistent",
			expected: EventTypeUnknown,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: EventTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EventTypeFromString(tt.input)
			if got != tt.expected {
				t.Errorf("EventTypeFromString(%q) = %v, want %v",
					tt.input, got, tt.expected)
			}
		})
	}
}

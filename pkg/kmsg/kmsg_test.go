package kmsg

import (
	"bufio"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseMessage(t *testing.T) {
	bootTime := time.Unix(0xb100, 0x5ea1).Round(time.Microsecond)

	msg, err := parseMessage(bootTime, "6,2565,102258085667,-;docker0: port 2(vethc1bb733) entered blocking state")
	require.NoError(t, err)

	assert.Equal(t, msg.Message, "docker0: port 2(vethc1bb733) entered blocking state")
	assert.Equal(t, msg.Priority, 6)
	assert.Equal(t, msg.SequenceNumber, 2565)
	assert.Equal(t, msg.Timestamp, metav1.NewTime(bootTime.Add(102258085667*time.Microsecond)))
}

func TestReadAll(t *testing.T) {
	bootTime := time.Unix(0xb100, 0x5ea1).Round(time.Microsecond)

	f, err := os.Open("testdata/kmsg.1.log")
	require.NoError(t, err)
	defer f.Close()

	buf := bufio.NewScanner(f)
	for buf.Scan() {
		line := buf.Text()
		if len(line) == 0 {
			continue
		}
		msg, err := parseMessage(bootTime, line)
		require.NotNil(t, msg)
		require.NoError(t, err)
	}
}

func TestParseMessageComprehensive(t *testing.T) {
	bootTime := time.Unix(1000, 0).Round(time.Microsecond)

	tests := []struct {
		name        string
		input       string
		expected    *Message
		expectError bool
		errorMsg    string
	}{
		{
			name:  "valid message with standard format",
			input: "6,2565,102258085667,-;docker0: port 2(vethc1bb733) entered blocking state",
			expected: &Message{
				Priority:       6,
				SequenceNumber: 2565,
				Timestamp:      metav1.NewTime(bootTime.Add(102258085667 * time.Microsecond)),
				Message:        "docker0: port 2(vethc1bb733) entered blocking state",
			},
			expectError: false,
		},
		{
			name:  "valid message with high priority",
			input: "0,1234,5000000,-;Critical kernel message",
			expected: &Message{
				Priority:       0,
				SequenceNumber: 1234,
				Timestamp:      metav1.NewTime(bootTime.Add(5000000 * time.Microsecond)),
				Message:        "Critical kernel message",
			},
			expectError: false,
		},
		{
			name:  "valid message with low priority",
			input: "7,9999,10000,-;Debug kernel message",
			expected: &Message{
				Priority:       7,
				SequenceNumber: 9999,
				Timestamp:      metav1.NewTime(bootTime.Add(10000 * time.Microsecond)),
				Message:        "Debug kernel message",
			},
			expectError: false,
		},
		{
			name:  "valid message with large sequence number",
			input: "3,2147483647,50000,-;Message with max int32 sequence",
			expected: &Message{
				Priority:       3,
				SequenceNumber: 2147483647,
				Timestamp:      metav1.NewTime(bootTime.Add(50000 * time.Microsecond)),
				Message:        "Message with max int32 sequence",
			},
			expectError: false,
		},
		{
			name:  "valid message with zero timestamp",
			input: "4,100,0,-;Message at boot time",
			expected: &Message{
				Priority:       4,
				SequenceNumber: 100,
				Timestamp:      metav1.NewTime(bootTime),
				Message:        "Message at boot time",
			},
			expectError: false,
		},
		{
			name:  "valid message with semicolons in message part",
			input: "3,123,5000,-;Message with; semicolons; in it",
			expected: &Message{
				Priority:       3,
				SequenceNumber: 123,
				Timestamp:      metav1.NewTime(bootTime.Add(5000 * time.Microsecond)),
				Message:        "Message with; semicolons; in it",
			},
			expectError: false,
		},
		{
			name:  "valid message with extra metadata fields",
			input: "2,456,7890,extra,fields,-;Message with extra metadata",
			expected: &Message{
				Priority:       2,
				SequenceNumber: 456,
				Timestamp:      metav1.NewTime(bootTime.Add(7890 * time.Microsecond)),
				Message:        "Message with extra metadata",
			},
			expectError: false,
		},
		{
			name:        "error - missing semicolon",
			input:       "6,2565,102258085667",
			expectError: true,
			errorMsg:    "invalid kmsg; must contain a ';'",
		},
		{
			name:        "error - insufficient metadata parts",
			input:       "6,2565;message",
			expectError: true,
			errorMsg:    "invalid kmsg: must contain at least 3 ',' separated pieces at the start",
		},
		{
			name:        "error - invalid priority",
			input:       "invalid,2565,102258085667,-;message",
			expectError: true,
			errorMsg:    "could not parse",
		},
		{
			name:        "error - invalid sequence number",
			input:       "6,invalid,102258085667,-;message",
			expectError: true,
			errorMsg:    "could not parse",
		},
		{
			name:        "error - invalid timestamp",
			input:       "6,2565,invalid,-;message",
			expectError: true,
			errorMsg:    "could not parse",
		},
		{
			name:        "error - empty input",
			input:       "",
			expectError: true,
			errorMsg:    "invalid kmsg; must contain a ';'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := parseMessage(bootTime, tc.input)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
				assert.Nil(t, msg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, msg)
				assert.Equal(t, tc.expected.Priority, msg.Priority)
				assert.Equal(t, tc.expected.SequenceNumber, msg.SequenceNumber)
				assert.Equal(t, tc.expected.Timestamp, msg.Timestamp)
				assert.Equal(t, tc.expected.Message, msg.Message)
			}
		})
	}
}

func TestParseMessageWithDifferentBootTimes(t *testing.T) {
	// Test that the timestamp calculation correctly uses the bootTime
	input := "3,123,5000000,-;Test message"

	bootTimes := []time.Time{
		time.Unix(0, 0),
		time.Unix(1000000000, 0),
		time.Unix(1700000000, 0),
		time.Now().Add(-24 * time.Hour),
	}

	for _, bootTime := range bootTimes {
		t.Run(bootTime.String(), func(t *testing.T) {
			msg, err := parseMessage(bootTime, input)
			require.NoError(t, err)

			// The message timestamp should be bootTime + 5 seconds
			expectedTime := metav1.NewTime(bootTime.Add(5000000 * time.Microsecond))
			assert.Equal(t, expectedTime, msg.Timestamp)
		})
	}
}

func TestParseMessageEdgeCases(t *testing.T) {
	bootTime := time.Unix(1000, 0)

	// Test with very large timestamp value
	t.Run("very large timestamp", func(t *testing.T) {
		// Large but safe timestamp value that won't overflow
		input := "1,100,9223372036854,-;Large timestamp message"
		msg, err := parseMessage(bootTime, input)
		require.NoError(t, err)

		// This should be bootTime + a large duration
		expectedTime := metav1.NewTime(bootTime.Add(9223372036854 * time.Microsecond))
		assert.Equal(t, expectedTime, msg.Timestamp)
	})

	// Test with negative priority (should still parse, even if unusual)
	t.Run("negative priority", func(t *testing.T) {
		input := "-1,100,5000,-;Negative priority message"
		msg, err := parseMessage(bootTime, input)
		require.NoError(t, err)
		assert.Equal(t, -1, msg.Priority)
	})

	// Test with negative sequence number (should still parse, even if unusual)
	t.Run("negative sequence", func(t *testing.T) {
		input := "1,-100,5000,-;Negative sequence message"
		msg, err := parseMessage(bootTime, input)
		require.NoError(t, err)
		assert.Equal(t, -100, msg.SequenceNumber)
	})

	// Test with empty message
	t.Run("empty message", func(t *testing.T) {
		input := "1,100,5000,-;"
		msg, err := parseMessage(bootTime, input)
		require.NoError(t, err)
		assert.Equal(t, "", msg.Message)
	})
}

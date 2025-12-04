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

func Test_parseLineWithTestData(t *testing.T) {
	bootTime := time.Unix(0xb100, 0x5ea1).Round(time.Microsecond)

	f, err := os.Open("testdata/kmsg.1.log")
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	buf := bufio.NewScanner(f)
	for buf.Scan() {
		line := buf.Text()
		if len(line) == 0 {
			continue
		}
		msg, err := parseLine(bootTime, line)
		require.NotNil(t, msg)
		require.NoError(t, err)
	}
}

func Test_parseLine(t *testing.T) {
	bootTime := time.Unix(0xb100, 0x5ea1).Round(time.Microsecond)

	msg, err := parseLine(bootTime, "6,2565,102258085667,-;docker0: port 2(vethc1bb733) entered blocking state")
	require.NoError(t, err)

	assert.Equal(t, msg.Message, "docker0: port 2(vethc1bb733) entered blocking state")
	assert.Equal(t, msg.Priority, 6)
	assert.Equal(t, msg.SequenceNumber, 2565)
	assert.Equal(t, msg.Timestamp, metav1.NewTime(bootTime.Add(102258085667*time.Microsecond)))
}

func Test_parseLineComprehensive(t *testing.T) {
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
			msg, err := parseLine(bootTime, tc.input)

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

func Test_parseLineWithDifferentBootTimes(t *testing.T) {
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
			msg, err := parseLine(bootTime, input)
			require.NoError(t, err)

			// The message timestamp should be bootTime + 5 seconds
			expectedTime := metav1.NewTime(bootTime.Add(5000000 * time.Microsecond))
			assert.Equal(t, expectedTime, msg.Timestamp)
		})
	}
}

func Test_parseLineEdgeCases(t *testing.T) {
	bootTime := time.Unix(1000, 0)

	// Test with very large timestamp value
	t.Run("very large timestamp", func(t *testing.T) {
		// Large but safe timestamp value that won't overflow
		input := "1,100,9223372036854,-;Large timestamp message"
		msg, err := parseLine(bootTime, input)
		require.NoError(t, err)

		// This should be bootTime + a large duration
		expectedTime := metav1.NewTime(bootTime.Add(9223372036854 * time.Microsecond))
		assert.Equal(t, expectedTime, msg.Timestamp)
	})

	// Test with negative priority (should still parse, even if unusual)
	t.Run("negative priority", func(t *testing.T) {
		input := "-1,100,5000,-;Negative priority message"
		msg, err := parseLine(bootTime, input)
		require.NoError(t, err)
		assert.Equal(t, -1, msg.Priority)
	})

	// Test with negative sequence number (should still parse, even if unusual)
	t.Run("negative sequence", func(t *testing.T) {
		input := "1,-100,5000,-;Negative sequence message"
		msg, err := parseLine(bootTime, input)
		require.NoError(t, err)
		assert.Equal(t, -100, msg.SequenceNumber)
	})

	// Test with empty message
	t.Run("empty message", func(t *testing.T) {
		input := "1,100,5000,-;"
		msg, err := parseLine(bootTime, input)
		require.NoError(t, err)
		assert.Equal(t, "", msg.Message)
	})
}

// Test_readFollow tests the readFollow function by reading the test data file
func Test_readFollow(t *testing.T) {
	// Open the test data file
	testFile, err := os.Open("testdata/kmsg.1.log")
	require.NoError(t, err)
	defer func() {
		_ = testFile.Close()
	}()

	// Use a fixed boot time for deterministic testing
	bootTime := time.Unix(1000, 0)

	// Create a channel to receive messages
	msgChan := make(chan Message, 100)

	// Start a goroutine to read messages
	errChan := make(chan error, 1)
	go func() {
		errChan <- readFollow(testFile, bootTime, msgChan, nil)
	}()

	// Collect messages for a short time
	receivedMessages := []Message{}
	timeout := time.After(500 * time.Millisecond)

	messageCollection := func() {
		for {
			select {
			case msg, ok := <-msgChan:
				if !ok {
					return
				}
				receivedMessages = append(receivedMessages, msg)
			case <-timeout:
				return
			}
		}
	}

	// Close the file after a short delay to trigger termination
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = testFile.Close()
	}()

	messageCollection()

	// Wait for readFollow to return
	err = <-errChan
	require.NoError(t, err)

	// Verify we received messages
	require.NotEmpty(t, receivedMessages, "Should have received messages from the test file")

	// Verify some message fields
	for _, msg := range receivedMessages {
		assert.NotZero(t, msg.Priority)
		assert.NotZero(t, msg.SequenceNumber)
		assert.NotEmpty(t, msg.Message)
		assert.False(t, msg.Timestamp.IsZero())
	}
}

// Test_readFollowMalformedData tests reading malformed data
func Test_readFollowMalformedData(t *testing.T) {
	// Create a temporary file with malformed data
	tmpFile, err := os.CreateTemp("", "kmsg-test-malformed")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	// Write malformed data to the file
	_, err = tmpFile.WriteString("malformed data without proper format\n")
	require.NoError(t, err)

	// Rewind the file
	_, err = tmpFile.Seek(0, 0)
	require.NoError(t, err)

	// Try to read from the file
	bootTime := time.Unix(1000, 0)
	msgChan := make(chan Message, 10)

	err = readFollow(tmpFile, bootTime, msgChan, nil)

	// Expect an error about malformed message
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed kmsg message")
}

func Test_errIfStarted(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*watcher)
		wantErr bool
	}{
		{
			name:    "not started",
			setup:   func(w *watcher) {},
			wantErr: false,
		},
		{
			name: "already started",
			setup: func(w *watcher) {
				w.watchStarted.Store(true)
			},
			wantErr: true,
		},
		{
			name: "call twice",
			setup: func(w *watcher) {
				_ = w.errIfStarted() // First call will set to true
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &watcher{}

			tt.setup(w)

			err := w.errIfStarted()

			if tt.wantErr {
				assert.Equal(t, ErrWatcherAlreadyStarted, err)
			} else {
				assert.NoError(t, err)
				// Verify that watchStarted is now true
				assert.True(t, w.watchStarted.Load())
			}
		})
	}
}

func Test_DescribeTimestamp(t *testing.T) {
	// Reference time for all tests
	referenceTime := time.Date(2023, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		messageTime    time.Time
		referenceTime  time.Time
		expectedOutput string
	}{
		{
			name:           "exactly the same time",
			messageTime:    referenceTime,
			referenceTime:  referenceTime,
			expectedOutput: "now",
		},
		{
			name:           "1 minute ago",
			messageTime:    referenceTime.Add(-1 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "1 minute ago",
		},
		{
			name:           "5 minutes ago",
			messageTime:    referenceTime.Add(-5 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "5 minutes ago",
		},
		{
			name:           "1 hour ago",
			messageTime:    referenceTime.Add(-1 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 hour ago",
		},
		{
			name:           "1 day ago",
			messageTime:    referenceTime.Add(-24 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 day ago",
		},
		{
			name:           "1 minute in future",
			messageTime:    referenceTime.Add(1 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "1 minute from now",
		},
		{
			name:           "5 minutes in future",
			messageTime:    referenceTime.Add(5 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "5 minutes from now",
		},
		{
			name:           "1 hour in future",
			messageTime:    referenceTime.Add(1 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 hour from now",
		},
		{
			name:           "1 day in future",
			messageTime:    referenceTime.Add(24 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 day from now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Timestamp: metav1.NewTime(tt.messageTime),
			}
			result := msg.DescribeTimestamp(tt.referenceTime)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}

package streamer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test parse function for fabric manager logs
func parseFabricManagerLog(line string) LogLine {
	const fabricmanagerLogTimeFormat = "Jan 02 2006 15:04:05"
	logLine := LogLine{Time: time.Now().UTC(), Content: line}

	// Look for timestamp pattern [Feb 25 2025 13:59:45]
	if len(line) < 3 || line[0] != '[' {
		logLine.Error = errors.New("no timestamp found")
		return logLine
	}

	// Find closing bracket
	endIdx := strings.Index(line, "]")
	if endIdx == -1 {
		logLine.Error = errors.New("invalid timestamp format")
		return logLine
	}

	timestampStr := line[1:endIdx]
	parsedTime, err := time.Parse(fabricmanagerLogTimeFormat, timestampStr)
	if err != nil {
		logLine.Error = err
		return logLine
	}

	logLine.Time = parsedTime.UTC()
	if endIdx+2 < len(line) {
		logLine.Content = line[endIdx+2:]
	} else {
		logLine.Content = ""
	}

	return logLine
}

func TestNewCmdStreamer(t *testing.T) {
	t.Run("new streamer with empty commands", func(t *testing.T) {
		s, err := newCmdStreamer([][]string{}, parseFabricManagerLog)
		assert.Error(t, err)
		assert.Nil(t, s)
		assert.Contains(t, err.Error(), "no commands provided")
	})

	t.Run("new streamer with valid commands", func(t *testing.T) {
		s, err := newCmdStreamer([][]string{{"echo", "test"}}, parseFabricManagerLog)
		assert.NoError(t, err)
		assert.NotNil(t, s)
		defer s.close()
	})

	t.Run("watch and close", func(t *testing.T) {
		s, err := newCmdStreamer([][]string{{"echo", "test message"}}, parseFabricManagerLog)
		assert.NoError(t, err)
		assert.NotNil(t, s)

		ch := s.watch()
		assert.NotNil(t, ch)

		// Close should not panic
		s.close()
		s.close() // Second close should be safe
	})
}

func TestStreamCommandOutputs(t *testing.T) {
	t.Run("stream simple echo command", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := streamCommandOutputs(
			ctx,
			[][]string{{"echo", "[Feb 25 2025 13:59:45] [INFO] Test message"}},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		// Collect output
		var lines []LogLine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		// Wait for completion
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for command output")
		}

		require.Len(t, lines, 1)
		assert.Equal(t, "[INFO] Test message", lines[0].Content)
		assert.Equal(t, time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC), lines[0].Time)
		assert.Nil(t, lines[0].Error)
	})

	t.Run("stream from file", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create a temporary file with test data
		tmpFile := t.TempDir() + "/test.log"
		testData := `[Feb 25 2025 13:59:45] [INFO] Line 1
[Feb 25 2025 13:59:46] [INFO] Line 2
[Feb 25 2025 13:59:47] [INFO] Line 3`

		// Write test data to file
		err := os.WriteFile(tmpFile, []byte(testData), 0644)
		require.NoError(t, err)

		// Use cat to read the file - this is more reliable than echo
		ch, err := streamCommandOutputs(
			ctx,
			[][]string{
				{"cat", tmpFile},
			},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		// Collect all output
		var lines []LogLine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		// Wait for completion with timeout
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for command output")
		}

		// Filter out any error lines
		var validLines []LogLine
		for _, line := range lines {
			if line.Error == nil && line.Content != "" {
				validLines = append(validLines, line)
			}
		}

		// We should get exactly 3 valid lines
		require.Len(t, validLines, 3)

		// Verify each line
		for i, line := range validLines {
			assert.Contains(t, line.Content, fmt.Sprintf("Line %d", i+1))
			assert.Equal(t, fmt.Sprintf("[INFO] Line %d", i+1), line.Content)
			expectedTime := time.Date(2025, time.February, 25, 13, 59, 45+i, 0, time.UTC)
			assert.Equal(t, expectedTime, line.Time)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		ch, err := streamCommandOutputs(
			ctx,
			[][]string{{"sleep", "5"}}, // Long running command
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		// Cancel context quickly
		time.Sleep(100 * time.Millisecond)
		cancel()

		// Channel should close
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range ch {
				// Drain channel
			}
		}()

		select {
		case <-done:
			// Success - channel closed
		case <-time.After(2 * time.Second):
			t.Fatal("channel did not close after context cancellation")
		}
	})
}

func TestParseFabricManagerLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		expectedTime time.Time
		expectedCont string
		expectErr    bool
	}{
		{
			name:         "valid log line",
			input:        "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Received an inband message",
			expectedTime: time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC),
			expectedCont: "[INFO] [tid 1803] Received an inband message",
			expectErr:    false,
		},
		{
			name:         "valid log line with error level",
			input:        "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error",
			expectedTime: time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC),
			expectedCont: "[ERROR] [tid 12727] detected NVSwitch non-fatal error",
			expectErr:    false,
		},
		{
			name:         "no timestamp",
			input:        "This is a line without a timestamp",
			expectedTime: time.Time{},
			expectedCont: "This is a line without a timestamp",
			expectErr:    true,
		},
		{
			name:         "invalid timestamp format",
			input:        "[2025-02-25 13:59:45] Some content",
			expectedTime: time.Time{},
			expectedCont: "[2025-02-25 13:59:45] Some content",
			expectErr:    true,
		},
		{
			name:         "empty line",
			input:        "",
			expectedTime: time.Time{},
			expectedCont: "",
			expectErr:    true,
		},
		{
			name:         "timestamp only",
			input:        "[Feb 25 2025 13:59:45]",
			expectedTime: time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC),
			expectedCont: "",
			expectErr:    false,
		},
		{
			name:         "malformed bracket",
			input:        "[Feb 25 2025 13:59:45 [INFO] message",
			expectedTime: time.Time{},
			expectedCont: "[Feb 25 2025 13:59:45 [INFO] message",
			expectErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFabricManagerLog(tt.input)

			if tt.expectErr {
				assert.NotNil(t, result.Error)
			} else {
				assert.Nil(t, result.Error)
				assert.Equal(t, tt.expectedTime, result.Time, "timestamp should match")
				assert.Equal(t, tt.expectedCont, result.Content, "content should match")
			}
		})
	}
}

func TestStreamFabricManagerLogs(t *testing.T) {
	t.Run("stream fabric manager logs with test data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Read the test data file
		testData, err := os.ReadFile("testdata/fabricmanager.log")
		require.NoError(t, err)

		// Create a temporary file
		tmpFile, err := os.CreateTemp("", "fabricmanager_stream_test_*.log")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		// Write test data to temp file
		_, err = tmpFile.Write(testData)
		require.NoError(t, err)
		tmpFile.Close()

		// Use tail -n +1 -f to read from the beginning and follow the file
		ch, err := streamCommandOutputs(
			ctx,
			[][]string{
				{"tail", "-n", "+1", "-f", tmpFile.Name()},
			},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		var lines []LogLine
		foundErrorLine := false
		deadline := time.Now().Add(5 * time.Second)

		// Read lines until we find the error or timeout
		for time.Now().Before(deadline) && !foundErrorLine {
			select {
			case line, ok := <-ch:
				if !ok {
					// Channel closed
					break
				}
				if line.Error == nil {
					lines = append(lines, line)

					// Check if this is the error line we're looking for
					if strings.Contains(line.Content, "[ERROR] [tid 12727] detected NVSwitch non-fatal error") {
						foundErrorLine = true
						// Verify timestamp
						expectedTime := time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC)
						assert.Equal(t, expectedTime, line.Time, "error log should have correct timestamp")
					}
				}
			case <-time.After(100 * time.Millisecond):
				// Continue polling
			}
		}

		// Check if we got log lines
		assert.NotEmpty(t, lines, "should have parsed log lines")
		assert.True(t, foundErrorLine, "should find NVSwitch error log line")

		// Cancel context to stop tail
		cancel()

		// Drain the channel
		go func() {
			for range ch {
				// Drain remaining lines
			}
		}()
	})

	t.Run("stream with deduplication", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create commands that output duplicate lines
		ch, err := streamCommandOutputs(
			ctx,
			[][]string{
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Duplicate message 1"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Duplicate message 1"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Duplicate message 1"},
				{"echo", "[Feb 25 2025 13:59:46] [INFO] [tid 1803] Different second"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Different message"},
			},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		// Collect output
		var lines []LogLine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		// Wait for completion
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for lines")
		}

		// We should have 3 unique lines due to deduplication:
		// 1. Duplicate message 1 (only once)
		// 2. Different second
		// 3. Different message
		assert.Equal(t, 3, len(lines), "expected 3 unique log lines after deduplication")

		// Verify messages
		messages := make(map[string]bool)
		for _, line := range lines {
			messages[line.Content] = true
		}

		assert.True(t, messages["[INFO] [tid 1803] Duplicate message 1"], "should have the duplicate line once")
		assert.True(t, messages["[INFO] [tid 1803] Different second"], "should have the different second line")
		assert.True(t, messages["[INFO] [tid 1803] Different message"], "should have the different message line")
	})
}

func TestReadProcessOutputs(t *testing.T) {
	t.Run("handle empty lines", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := streamCommandOutputs(
			ctx,
			[][]string{
				{"echo", ""},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] Valid line"},
				{"echo", ""},
			},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		var lines []LogLine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		// Wait for completion
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for lines")
		}

		// Should only have the valid line (empty lines are skipped)
		assert.Len(t, lines, 1)
		assert.Contains(t, lines[0].Content, "Valid line")
	})

	t.Run("handle parse errors", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := streamCommandOutputs(
			ctx,
			[][]string{
				{"echo", "Invalid line without timestamp"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] Valid line"},
			},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		var lines []LogLine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		// Wait for completion
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for lines")
		}

		// Should have both lines
		assert.Len(t, lines, 2)

		// First line should have parse error
		assert.NotNil(t, lines[0].Error)
		assert.Equal(t, "Invalid line without timestamp", lines[0].Content)

		// Second line should be valid
		assert.Nil(t, lines[1].Error)
		assert.Contains(t, lines[1].Content, "Valid line")
	})
}

func TestCmdStreamerMultipleCommands(t *testing.T) {
	t.Run("multiple commands in sequence", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch, err := streamCommandOutputs(
			ctx,
			[][]string{
				{"echo", "[Feb 25 2025 13:59:45] [INFO] First command"},
				{"echo", "[Feb 25 2025 13:59:46] [INFO] Second command"},
				{"echo", "[Feb 25 2025 13:59:47] [INFO] Third command"},
			},
			parseFabricManagerLog,
			defaultCacheExpiration,
			defaultCachePurgeInterval,
		)
		require.NoError(t, err)
		require.NotNil(t, ch)

		var lines []LogLine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		// Wait for completion
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for lines")
		}

		assert.Len(t, lines, 3)
		assert.Contains(t, lines[0].Content, "First command")
		assert.Contains(t, lines[1].Content, "Second command")
		assert.Contains(t, lines[2].Content, "Third command")
	})
}

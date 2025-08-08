package fabricmanager

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
			input:        "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0",
			expectedTime: time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC),
			expectedCont: "[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0",
			expectErr:    false,
		},
		{
			name:         "valid multiline message header",
			input:        "[Feb 25 2025 14:01:18] [INFO] [tid 1803] Received an inband message:  Message header details: magic Id:adbc request Id:73a12417a65c5380 status:0 type:2 length:56",
			expectedTime: time.Date(2025, time.February, 25, 14, 1, 18, 0, time.UTC),
			expectedCont: "[INFO] [tid 1803] Received an inband message:  Message header details: magic Id:adbc request Id:73a12417a65c5380 status:0 type:2 length:56",
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
			expectedCont: "[Feb 25 2025 13:59:45]",
			expectErr:    false,
		},
		{
			name:         "malformed bracket",
			input:        "[Feb 25 2025 13:59:45 [INFO] message",
			expectedTime: time.Time{},
			expectedCont: "[Feb 25 2025 13:59:45 [INFO] message",
			expectErr:    true,
		},
		{
			name:         "timestamp with extra spaces",
			input:        "[Feb 25 2025 13:59:45]   [INFO] extra spaces",
			expectedTime: time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC),
			expectedCont: "[INFO] extra spaces",
			expectErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFabricManagerLog(tt.input)

			if tt.expectErr {
				assert.NotNil(t, result.Error, "expected error but got none")
			} else {
				assert.Nil(t, result.Error, "expected no error but got: %v", result.Error)
				assert.Equal(t, tt.expectedTime, result.Time, "timestamp should match")
				assert.Equal(t, tt.expectedCont, result.Content, "content should match")
			}
		})
	}
}

func TestParseFabricManagerLogErrorCases(t *testing.T) {
	t.Parallel()

	// Test with invalid date in timestamp
	t.Run("invalid date in timestamp", func(t *testing.T) {
		input := "[Feb 31 2025 13:59:45] [INFO] Invalid date"
		result := ParseFabricManagerLog(input)
		assert.NotNil(t, result.Error, "should have parse error for invalid date")
		assert.Contains(t, result.Error.Error(), "parsing time", "error should mention time parsing")
	})

	// Test with incomplete timestamp
	t.Run("incomplete timestamp", func(t *testing.T) {
		input := "[Feb 25 2025] [INFO] Missing time"
		result := ParseFabricManagerLog(input)
		assert.NotNil(t, result.Error, "should have parse error for incomplete timestamp")
	})
}

func TestParseFabricManagerLogWithActualLogSamples(t *testing.T) {
	t.Parallel()

	// Test with actual log samples from the fabric manager
	samples := []struct {
		name         string
		input        string
		expectedTime time.Time
		hasError     bool
	}{
		{
			name:         "multicast group freed",
			input:        "[Feb 25 2025 13:59:45] [INFO] [tid 1808] multicast group 0 is freed.",
			expectedTime: time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC),
			hasError:     false,
		},
		{
			name:         "team setup request",
			input:        "[Feb 25 2025 14:01:18] [INFO] [tid 1803] Received an inband message:  Message header details: magic Id:adbc request Id:73a12417a65c5380 status:0 type:2 length:56",
			expectedTime: time.Date(2025, time.February, 25, 14, 1, 18, 0, time.UTC),
			hasError:     false,
		},
		{
			name:         "nvswitch error",
			input:        "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expectedTime: time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC),
			hasError:     false,
		},
		{
			name:         "empty log line between entries",
			input:        "",
			expectedTime: time.Time{},
			hasError:     true,
		},
	}

	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			result := ParseFabricManagerLog(sample.input)

			if sample.hasError {
				assert.NotNil(t, result.Error, "expected error for sample")
			} else {
				assert.Nil(t, result.Error, "expected no error for sample")
				assert.Equal(t, sample.expectedTime, result.Time, "timestamp should match for sample")
			}
		})
	}
}

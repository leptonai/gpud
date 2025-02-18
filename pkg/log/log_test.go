package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCreateLoggerWithLumberjackErrors(t *testing.T) {
	// Test with invalid directory
	logger := CreateLoggerWithLumberjack("/nonexistent/directory/test.log", 1, zap.InfoLevel)
	require.NotNil(t, logger)

	// Writing to invalid path should not panic
	assert.NotPanics(t, func() {
		logger.Info("test message")
	})
}

func TestCreateLoggerWithLumberjackBasic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gpud-log-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logFile := filepath.Join(tmpDir, "test.log")
	maxSize := 5 // 5MB

	logger := CreateLoggerWithLumberjack(logFile, maxSize, zap.InfoLevel)
	require.NotNil(t, logger)

	// Test basic logging functionality
	testMsg := "test message"
	logger.Info(testMsg)

	// Verify log file exists and contains the message
	content, err := os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), testMsg)

	// Test error logging
	errorMsg := "error message"
	logger.Error(errorMsg)
	content, err = os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), errorMsg)

	// Test warning logging
	warnMsg := "warning message"
	logger.Warn(warnMsg)
	content, err = os.ReadFile(logFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), warnMsg)
}

func TestLogRotation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gpud-log-rotation-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		name          string
		maxSize       int // in MB
		numWrites     int // number of writes to perform
		bytesPerWrite int // bytes per write
		expectedFiles int // expected number of files (including current)
	}{
		{
			name:          "single_file_no_rotation",
			maxSize:       1,
			numWrites:     1,
			bytesPerWrite: 512 * 1024, // 512KB
			expectedFiles: 1,
		},
		{
			name:          "multiple_rotations",
			maxSize:       1,
			numWrites:     15,
			bytesPerWrite: 100 * 1024, // 100KB per write makes 1.5 MB
			expectedFiles: 2,
		},
		{
			name:          "multiple_rotations_more",
			maxSize:       1,
			numWrites:     30,
			bytesPerWrite: 100 * 1024, // 100KB per write makes 3 MB
			expectedFiles: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logFile := filepath.Join(tmpDir, tc.name)
			logger := CreateLoggerWithLumberjack(logFile, tc.maxSize, zap.InfoLevel)
			require.NotNil(t, logger)

			padding := strings.Repeat("a", tc.bytesPerWrite)
			for i := 0; i < tc.numWrites; i++ {
				logger.Infof("test message %d: %s", i, padding)
			}

			// enough time for rotation to occur
			time.Sleep(time.Second)

			pattern := logFile + "*"
			matches, err := filepath.Glob(pattern)
			require.NoError(t, err)

			assert.GreaterOrEqual(t, len(matches), tc.expectedFiles,
				"expected >=%d files, got %d: %q", tc.expectedFiles, len(matches), matches)
		})
	}
}

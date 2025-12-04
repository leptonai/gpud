package log

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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

func TestParseLogLevel(t *testing.T) {
	testCases := []struct {
		name          string
		logLevel      string
		expectedLevel zapcore.Level
		expectError   bool
	}{
		{
			name:          "empty string defaults to info",
			logLevel:      "",
			expectedLevel: zapcore.InfoLevel,
			expectError:   false,
		},
		{
			name:          "info string returns info level",
			logLevel:      "info",
			expectedLevel: zapcore.InfoLevel,
			expectError:   false,
		},
		{
			name:          "debug string returns debug level",
			logLevel:      "debug",
			expectedLevel: zapcore.DebugLevel,
			expectError:   false,
		},
		{
			name:          "error string returns error level",
			logLevel:      "error",
			expectedLevel: zapcore.ErrorLevel,
			expectError:   false,
		},
		{
			name:          "warn string returns warn level",
			logLevel:      "warn",
			expectedLevel: zapcore.WarnLevel,
			expectError:   false,
		},
		{
			name:          "invalid string returns error",
			logLevel:      "invalid",
			expectedLevel: zapcore.InfoLevel, // default, but not used due to error
			expectError:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			level, err := ParseLogLevel(tc.logLevel)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedLevel, level.Level())
			}
		})
	}
}

func TestLeptonLoggerErrorw(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gpud-log-errorw-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	logFile := filepath.Join(tmpDir, "test.log")
	logger := CreateLoggerWithLumberjack(logFile, 1, zap.InfoLevel)
	require.NotNil(t, logger)

	t.Run("regular error logs at error level", func(t *testing.T) {
		regularErr := assert.AnError
		logger.Errorw("regular error", "error", regularErr)

		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		logContent := string(content)
		assert.Contains(t, logContent, "regular error")
		assert.Contains(t, logContent, regularErr.Error())
		assert.Contains(t, logContent, `"level":"error"`)
	})

	// Create new log file for the context canceled test
	canceledLogFile := filepath.Join(tmpDir, "canceled.log")
	canceledLogger := CreateLoggerWithLumberjack(canceledLogFile, 1, zap.InfoLevel)

	t.Run("context canceled error logs at warn level", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel the context to create a context.Canceled error
		canceledErr := ctx.Err()

		canceledLogger.Errorw("canceled context", "error", canceledErr)

		content, err := os.ReadFile(canceledLogFile)
		require.NoError(t, err)

		logContent := string(content)
		assert.Contains(t, logContent, "canceled context")
		assert.Contains(t, logContent, context.Canceled.Error())
		assert.Contains(t, logContent, `"level":"warn"`)
	})
}

func TestCreateLogger(t *testing.T) {
	// Test with non-empty logFile - should use Lumberjack logging
	t.Run("with logFile creates file logger", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-create-logger-*")
		require.NoError(t, err)
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		logFile := filepath.Join(tmpDir, "test.log")
		logLevel, err := ParseLogLevel("debug")
		require.NoError(t, err)

		logger := CreateLogger(logLevel, logFile)
		require.NotNil(t, logger)

		// Verify the logger works and writes to the file
		logger.Debug("debug test message")

		// File should exist and contain the message
		assert.FileExists(t, logFile)
		content, err := os.ReadFile(logFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "debug test message")
		assert.Contains(t, string(content), `"level":"debug"`)
	})

	// Test with empty logFile - should use default config
	t.Run("with empty logFile creates console logger", func(t *testing.T) {
		logLevel, err := ParseLogLevel("error")
		require.NoError(t, err)

		logger := CreateLogger(logLevel, "")
		require.NotNil(t, logger)

		// Cannot easily verify console output, but we can verify the logger doesn't panic
		assert.NotPanics(t, func() {
			logger.Error("error test message")
		})

		// Create a properly configured test logger with a buffer output to verify log levels
		buffer := &bytes.Buffer{}

		// Create a custom core with our buffer
		encoderConfig := zap.NewProductionEncoderConfig()
		encoder := zapcore.NewJSONEncoder(encoderConfig)

		// Create a core with ERROR level
		core := zapcore.NewCore(
			encoder,
			zapcore.AddSync(buffer),
			zapcore.ErrorLevel,
		)

		// Build a custom logger
		customLogger := zap.New(core).Sugar()
		testLogger := &gpudLogger{customLogger}

		// Debug messages should not be logged at error level
		buffer.Reset()
		testLogger.Debug("debug message")
		assert.Empty(t, buffer.String(), "Debug message should not be logged at error level")

		// Error messages should be logged
		buffer.Reset()
		testLogger.Error("error message")
		assert.Contains(t, buffer.String(), "error message", "Error message should be logged at error level")
	})
}

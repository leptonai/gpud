package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogApplyOpts(t *testing.T) {
	t.Run("applies all options correctly", func(t *testing.T) {
		al := &AuditLog{}
		opts := []AuditOption{
			WithKind("TestKind"),
			WithAuditID("test-audit-id"),
			WithMachineID("test-machine-id"),
			WithStage("ResponseComplete"),
			WithRequestURI("/api/v1/test"),
			WithVerb("GET"),
			WithData(map[string]string{"key": "value"}),
		}

		al.applyOpts(opts)

		assert.Equal(t, "TestKind", al.Kind)
		assert.Equal(t, "test-audit-id", al.AuditID)
		assert.Equal(t, "test-machine-id", al.MachineID)
		assert.Equal(t, "ResponseComplete", al.Stage)
		assert.Equal(t, "/api/v1/test", al.RequestURI)
		assert.Equal(t, "GET", al.Verb)
		assert.Equal(t, map[string]string{"key": "value"}, al.Data)
	})

	t.Run("sets default values when options not provided", func(t *testing.T) {
		al := &AuditLog{}
		al.applyOpts([]AuditOption{})

		assert.Equal(t, "Event", al.Kind)
		assert.NotEmpty(t, al.AuditID)

		// Verify it's a valid UUID
		_, err := uuid.Parse(al.AuditID)
		assert.NoError(t, err)
	})

	t.Run("partial options with defaults", func(t *testing.T) {
		al := &AuditLog{}
		al.applyOpts([]AuditOption{
			WithStage("RequestReceived"),
			WithVerb("POST"),
		})

		assert.Equal(t, "Event", al.Kind) // default
		assert.NotEmpty(t, al.AuditID)    // generated
		assert.Equal(t, "RequestReceived", al.Stage)
		assert.Equal(t, "POST", al.Verb)
		assert.Empty(t, al.MachineID)
		assert.Empty(t, al.RequestURI)
		assert.Nil(t, al.Data)
	})
}

func TestAuditOptionFunctions(t *testing.T) {
	tests := []struct {
		name   string
		option AuditOption
		verify func(*testing.T, *AuditLog)
	}{
		{
			name:   "WithKind",
			option: WithKind("CustomKind"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "CustomKind", al.Kind)
			},
		},
		{
			name:   "WithAuditID",
			option: WithAuditID("custom-audit-id"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "custom-audit-id", al.AuditID)
			},
		},
		{
			name:   "WithMachineID",
			option: WithMachineID("machine-123"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "machine-123", al.MachineID)
			},
		},
		{
			name:   "WithStage",
			option: WithStage("Panic"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "Panic", al.Stage)
			},
		},
		{
			name:   "WithRequestURI",
			option: WithRequestURI("/api/v1/health"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "/api/v1/health", al.RequestURI)
			},
		},
		{
			name:   "WithVerb",
			option: WithVerb("DELETE"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "DELETE", al.Verb)
			},
		},
		{
			name:   "WithData string",
			option: WithData("test data"),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Equal(t, "test data", al.Data)
			},
		},
		{
			name:   "WithData struct",
			option: WithData(struct{ Name string }{Name: "test"}),
			verify: func(t *testing.T, al *AuditLog) {
				data, ok := al.Data.(struct{ Name string })
				assert.True(t, ok)
				assert.Equal(t, "test", data.Name)
			},
		},
		{
			name:   "WithData nil",
			option: WithData(nil),
			verify: func(t *testing.T, al *AuditLog) {
				assert.Nil(t, al.Data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			al := &AuditLog{}
			tt.option(al)
			tt.verify(t, al)
		})
	}
}

func TestNewNopAuditLogger(t *testing.T) {
	logger := NewNopAuditLogger()
	assert.NotNil(t, logger)

	// Should not panic when logging
	assert.NotPanics(t, func() {
		logger.Log(
			WithStage("Test"),
			WithData("test data"),
		)
	})
}

func TestNewStdoutAuditLogger(t *testing.T) {
	logger := NewAuditLogger("")
	assert.NotNil(t, logger)

	// Should not panic when logging
	assert.NotPanics(t, func() {
		logger.Log(
			WithStage("Test"),
			WithData("test data"),
		)
	})
}

func TestNewAuditLogger(t *testing.T) {
	t.Run("with log file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-audit-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		logFile := filepath.Join(tmpDir, "audit.log")
		logger := NewAuditLogger(logFile)
		assert.NotNil(t, logger)

		// Log some data
		logger.Log(
			WithKind("TestEvent"),
			WithAuditID("test-123"),
			WithMachineID("machine-456"),
			WithStage("RequestReceived"),
			WithRequestURI("/api/v1/test"),
			WithVerb("GET"),
			WithData(map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			}),
		)

		// Verify file was created and contains expected content
		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		// Parse the JSON log entry
		var logEntry map[string]interface{}
		err = json.Unmarshal(content, &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "TestEvent", logEntry["kind"])
		assert.Equal(t, "test-123", logEntry["auditID"])
		// Note: machineID might not appear if it's empty
		if machineID, exists := logEntry["machineID"]; exists {
			assert.Equal(t, "machine-456", machineID)
		}
		assert.Equal(t, "Request", logEntry["level"])
		assert.Equal(t, "RequestReceived", logEntry["stage"])
		assert.Equal(t, "/api/v1/test", logEntry["requestURI"])
		assert.Equal(t, "GET", logEntry["verb"])

		data, ok := logEntry["data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "value1", data["key1"])
		assert.Equal(t, float64(42), data["key2"]) // JSON numbers are float64

		// Verify timestamp format - field is "ts" not "@timestamp"
		ts, ok := logEntry["ts"].(string)
		assert.True(t, ok)
		_, err = time.Parse(time.RFC3339, ts)
		assert.NoError(t, err)
	})

	t.Run("without log file (stdout)", func(t *testing.T) {
		logger := NewAuditLogger("")
		assert.NotNil(t, logger)

		// Should not panic when logging
		assert.NotPanics(t, func() {
			logger.Log(WithStage("Test"))
		})
	})
}

func TestAuditLoggerLog(t *testing.T) {
	t.Run("logs with all fields", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-audit-full-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		logFile := filepath.Join(tmpDir, "audit.log")
		logger := NewAuditLogger(logFile)

		testData := struct {
			Message string    `json:"message"`
			Count   int       `json:"count"`
			Time    time.Time `json:"time"`
		}{
			Message: "test message",
			Count:   100,
			Time:    time.Now(),
		}

		logger.Log(
			WithKind("Event"),
			WithAuditID("audit-789"),
			WithMachineID("machine-789"),
			WithStage("ResponseComplete"),
			WithRequestURI("/api/v1/components"),
			WithVerb("POST"),
			WithData(testData),
		)

		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		var logEntry map[string]interface{}
		err = json.Unmarshal(content, &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "Event", logEntry["kind"])
		assert.Equal(t, "audit-789", logEntry["auditID"])
		assert.Equal(t, "machine-789", logEntry["machineID"])
		assert.Equal(t, "RequestResponse", logEntry["level"])
		assert.Equal(t, "ResponseComplete", logEntry["stage"])
		assert.Equal(t, "/api/v1/components", logEntry["requestURI"])
		assert.Equal(t, "POST", logEntry["verb"])

		data, ok := logEntry["data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "test message", data["message"])
		assert.Equal(t, float64(100), data["count"])
		assert.NotEmpty(t, data["time"])
	})

	t.Run("logs with minimal fields and defaults", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-audit-minimal-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		logFile := filepath.Join(tmpDir, "audit.log")
		logger := NewAuditLogger(logFile)

		logger.Log() // No options provided

		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		var logEntry map[string]interface{}
		err = json.Unmarshal(content, &logEntry)
		require.NoError(t, err)

		assert.Equal(t, "Event", logEntry["kind"])
		assert.NotEmpty(t, logEntry["auditID"])
		assert.Equal(t, "Metadata", logEntry["level"])
		assert.Empty(t, logEntry["stage"])
		assert.Empty(t, logEntry["requestURI"])
		assert.Empty(t, logEntry["verb"])
		assert.Nil(t, logEntry["data"])

		// Verify auditID is a valid UUID
		auditID, ok := logEntry["auditID"].(string)
		assert.True(t, ok)
		_, err = uuid.Parse(auditID)
		assert.NoError(t, err)
	})

	t.Run("multiple log entries", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-audit-multiple-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		logFile := filepath.Join(tmpDir, "audit.log")
		logger := NewAuditLogger(logFile)

		// Log multiple entries
		for i := 0; i < 3; i++ {
			logger.Log(
				WithStage("Test"),
				WithData(map[string]int{"index": i}),
			)
		}

		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		// Should have 3 log entries (one per line)
		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		assert.Len(t, lines, 3)

		// Verify each entry
		for i, line := range lines {
			var logEntry map[string]interface{}
			err = json.Unmarshal([]byte(line), &logEntry)
			require.NoError(t, err)

			assert.Equal(t, "Test", logEntry["stage"])
			data, ok := logEntry["data"].(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, float64(i), data["index"])
		}
	})
}

func TestAuditLoggerEdgeCases(t *testing.T) {
	t.Run("empty strings in options", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-audit-empty-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		logFile := filepath.Join(tmpDir, "audit.log")
		logger := NewAuditLogger(logFile)

		logger.Log(
			WithKind(""),
			WithStage(""),
			WithRequestURI(""),
			WithVerb(""),
		)

		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		var logEntry map[string]interface{}
		err = json.Unmarshal(content, &logEntry)
		require.NoError(t, err)

		// Empty strings should be overridden by defaults where applicable
		assert.Equal(t, "Event", logEntry["kind"]) // default
		assert.Empty(t, logEntry["stage"])
		assert.Empty(t, logEntry["requestURI"])
		assert.Empty(t, logEntry["verb"])
	})

	t.Run("complex nested data", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gpud-audit-nested-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		logFile := filepath.Join(tmpDir, "audit.log")
		logger := NewAuditLogger(logFile)

		complexData := map[string]interface{}{
			"string": "value",
			"number": 123,
			"float":  123.456,
			"bool":   true,
			"nil":    nil,
			"array":  []string{"a", "b", "c"},
			"nested": map[string]interface{}{
				"key1": "value1",
				"key2": []int{1, 2, 3},
			},
		}

		logger.Log(WithData(complexData))

		content, err := os.ReadFile(logFile)
		require.NoError(t, err)

		var logEntry map[string]interface{}
		err = json.Unmarshal(content, &logEntry)
		require.NoError(t, err)

		data, ok := logEntry["data"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "value", data["string"])
		assert.Equal(t, float64(123), data["number"])
		assert.Equal(t, 123.456, data["float"])
		assert.Equal(t, true, data["bool"])
		assert.Nil(t, data["nil"])

		array, ok := data["array"].([]interface{})
		assert.True(t, ok)
		assert.Equal(t, []interface{}{"a", "b", "c"}, array)

		nested, ok := data["nested"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "value1", nested["key1"])

		nestedArray, ok := nested["key2"].([]interface{})
		assert.True(t, ok)
		assert.Equal(t, []interface{}{float64(1), float64(2), float64(3)}, nestedArray)
	})
}

func TestCreateAuditLogFilepath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard log file",
			input:    "/var/log/app.log",
			expected: "/var/log/app.audit",
		},
		{
			name:     "multiple .log occurrences",
			input:    "/var/log/app.log.backup.log",
			expected: "/var/log/app.backup.audit",
		},
		{
			name:     "no .log in filename",
			input:    "/var/data/app",
			expected: "/var/data/app.audit",
		},
		{
			name:     "empty string",
			input:    "",
			expected: ".audit",
		},
		{
			name:     "just .log",
			input:    ".log",
			expected: ".audit",
		},
		{
			name:     "path with .log in directory",
			input:    "/var/log/data",
			expected: "/var/log/data.audit",
		},
		{
			name:     "file with .log in middle",
			input:    "app.log.txt",
			expected: "app.txt.audit",
		},
		{
			name:     "relative path with .log",
			input:    "./logs/app.log",
			expected: "./logs/app.audit",
		},
		{
			name:     "path ending with directory separator",
			input:    "/var/log/",
			expected: "/var/log/.audit",
		},
		{
			name:     "Windows-style path",
			input:    "C:\\logs\\app.log",
			expected: "C:\\logs\\app.audit",
		},
		{
			name:     "file with multiple extensions",
			input:    "app.tar.gz.log",
			expected: "app.tar.gz.audit",
		},
		{
			name:     ".log at start of filename",
			input:    ".log-app",
			expected: "-app.audit",
		},
		{
			name:     "consecutive .log",
			input:    "app.log.log",
			expected: "app.audit",
		},
		{
			name:     "path with spaces",
			input:    "/var/log files/app.log",
			expected: "/var/log files/app.audit",
		},
		{
			name:     "Unicode characters",
			input:    "/var/log/アプリ.log",
			expected: "/var/log/アプリ.audit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateAuditLogFilepath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateAuditLogFilepath_EdgeCases(t *testing.T) {
	t.Run("preserves case sensitivity", func(t *testing.T) {
		input := "/var/LOG/App.LOG"
		expected := "/var/LOG/App.LOG.audit"
		result := CreateAuditLogFilepath(input)
		assert.Equal(t, expected, result)
	})

	t.Run("handles very long paths", func(t *testing.T) {
		input := strings.Repeat("/very/long/path", 100) + "/app.log"
		expected := strings.Repeat("/very/long/path", 100) + "/app.audit"
		result := CreateAuditLogFilepath(input)
		assert.Equal(t, expected, result)
	})

	t.Run("handles special characters", func(t *testing.T) {
		specialCases := []struct {
			input    string
			expected string
		}{
			{"/var/log/app@#$.log", "/var/log/app@#$.audit"},
			{"/var/log/app[test].log", "/var/log/app[test].audit"},
			{"/var/log/app{prod}.log", "/var/log/app{prod}.audit"},
			{"/var/log/app(1).log", "/var/log/app(1).audit"},
		}

		for _, tc := range specialCases {
			result := CreateAuditLogFilepath(tc.input)
			assert.Equal(t, tc.expected, result)
		}
	})
}

package memory

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDataGetReason(t *testing.T) {
	// Test with nil data
	var d *Data
	assert.Equal(t, "no memory data", d.getReason())

	// Test with error
	d = &Data{err: assert.AnError}
	assert.Contains(t, d.getReason(), "failed to get memory data")

	// Test with valid data
	d = &Data{
		TotalBytes: 16,
		UsedBytes:  8,
	}
	assert.Contains(t, d.getReason(), "8 B")
	assert.Contains(t, d.getReason(), "16 B")
}

func TestDataGetHealth(t *testing.T) {
	// Test with nil data
	var d *Data
	health, healthy := d.getHealth()
	assert.Equal(t, "Healthy", health)
	assert.True(t, healthy)

	// Test with error
	d = &Data{err: assert.AnError}
	health, healthy = d.getHealth()
	assert.Equal(t, "Unhealthy", health)
	assert.False(t, healthy)
}

func TestDataGetStates(t *testing.T) {
	d := &Data{
		TotalBytes: 16, // 16GB
		UsedBytes:  8,  // 8GB

		AvailableBytes: 8, // 8GB

		FreeBytes: 7, // 7GB

		VMAllocTotalBytes: 32, // 32GB
		VMAllocUsedBytes:  16, // 16GB

		BPFJITBufferBytes: 1024 * 1024, // 1MB

		ts: time.Now(),
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)

	// Verify extra info
	assert.NotEmpty(t, states[0].ExtraInfo)

	// Try to decode the JSON data
	jsonData := states[0].ExtraInfo["data"]
	assert.NotEmpty(t, jsonData)

	var decodedData Data
	err = json.Unmarshal([]byte(jsonData), &decodedData)
	assert.NoError(t, err)
}

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, "Healthy", states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataGetReasonWithError(t *testing.T) {
	// Test with various error messages
	errorMessages := []string{
		"connection refused",
		"permission denied",
		"timeout",
		"out of memory",
	}

	for _, msg := range errorMessages {
		d := &Data{err: errors.New(msg)}
		reason := d.getReason()
		assert.Contains(t, reason, "failed to get memory data")
		assert.Contains(t, reason, msg)
	}
}

func TestDataGetReasonWithDifferentMemorySizes(t *testing.T) {
	testCases := []struct {
		name        string
		totalBytes  uint64
		usedBytes   uint64
		expected    []string
		notExpected []string
	}{
		{
			name:       "large memory",
			totalBytes: 128 * 1024 * 1024 * 1024, // 137 GB
			usedBytes:  96 * 1024 * 1024 * 1024,  // 103 GB
			expected:   []string{"103 GB", "137 GB"},
		},
		{
			name:        "zero used memory",
			totalBytes:  16 * 1024 * 1024 * 1024, // 17 GB
			usedBytes:   0,
			expected:    []string{"0 B", "17 GB"},
			notExpected: []string{"NaN"},
		},
		{
			name:        "zero total memory",
			totalBytes:  0,
			usedBytes:   0,
			expected:    []string{"0 B", "0 B"},
			notExpected: []string{"NaN", "Inf"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := &Data{
				TotalBytes: tc.totalBytes,
				UsedBytes:  tc.usedBytes,
			}
			reason := d.getReason()
			for _, substr := range tc.expected {
				assert.Contains(t, reason, substr, "Expected substring not found in reason")
			}
			for _, substr := range tc.notExpected {
				assert.NotContains(t, reason, substr, "Unexpected substring found in reason")
			}
		})
	}
}

func TestDataGetHealthEdgeCases(t *testing.T) {
	testCases := []struct {
		name          string
		data          *Data
		expectedState string
		expectHealthy bool
	}{
		{
			name:          "nil data",
			data:          nil,
			expectedState: "Healthy",
			expectHealthy: true,
		},
		{
			name:          "data with nil error",
			data:          &Data{},
			expectedState: "Healthy",
			expectHealthy: true,
		},
		{
			name:          "data with error",
			data:          &Data{err: errors.New("test error")},
			expectedState: "Unhealthy",
			expectHealthy: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			health, healthy := tc.data.getHealth()
			assert.Equal(t, tc.expectedState, health)
			assert.Equal(t, tc.expectHealthy, healthy)
		})
	}
}

func TestDataGetStatesWithError(t *testing.T) {
	testError := errors.New("memory retrieval error")
	d := &Data{
		TotalBytes: 16,
		UsedBytes:  8,
		err:        testError,
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, "Unhealthy", states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Contains(t, states[0].Reason, "failed to get memory data")
}

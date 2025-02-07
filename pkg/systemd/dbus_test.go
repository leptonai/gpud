package systemd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatUnitName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bare service name",
			input:    "nginx",
			expected: "nginx.service",
		},
		{
			name:     "already has .service suffix",
			input:    "nginx.service",
			expected: "nginx.service",
		},
		{
			name:     "has .target suffix",
			input:    "network.target",
			expected: "network.target",
		},
		{
			name:     "empty string",
			input:    "",
			expected: ".service",
		},
		{
			name:     "with dots in name",
			input:    "my.custom.service.name",
			expected: "my.custom.service.name.service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatUnitName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckActiveState(t *testing.T) {
	tests := []struct {
		name        string
		props       map[string]interface{}
		unitName    string
		expected    bool
		expectError bool
		errorMsg    string
	}{
		{
			name: "active service",
			props: map[string]interface{}{
				"ActiveState": "active",
			},
			unitName:    "test.service",
			expected:    true,
			expectError: false,
		},
		{
			name: "inactive service",
			props: map[string]interface{}{
				"ActiveState": "inactive",
			},
			unitName:    "test.service",
			expected:    false,
			expectError: false,
		},
		{
			name: "failed service",
			props: map[string]interface{}{
				"ActiveState": "failed",
			},
			unitName:    "test.service",
			expected:    false,
			expectError: false,
		},
		{
			name:        "missing ActiveState",
			props:       map[string]interface{}{},
			unitName:    "test.service",
			expected:    false,
			expectError: true,
			errorMsg:    "ActiveState property not found for unit test.service",
		},
		{
			name: "wrong type for ActiveState",
			props: map[string]interface{}{
				"ActiveState": 123,
			},
			unitName:    "test.service",
			expected:    false,
			expectError: true,
			errorMsg:    "ActiveState property is not a string for unit test.service",
		},
		{
			name: "wrong type for ActiveState (bool)",
			props: map[string]interface{}{
				"ActiveState": true,
			},
			unitName:    "test.service",
			expected:    false,
			expectError: true,
			errorMsg:    "ActiveState property is not a string for unit test.service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := checkActiveState(tt.props, tt.unitName)
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMsg, err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestIsActive(t *testing.T) {
	tests := []struct {
		name        string
		unitName    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil connection",
			unitName:    "test.service",
			expectError: true,
			errorMsg:    "connection not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &DbusConn{conn: nil}
			result, err := conn.IsActive(context.Background(), tt.unitName)
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMsg, err.Error())
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

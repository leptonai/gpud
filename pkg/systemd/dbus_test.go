package systemd

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockDbusConn implements dbusConn interface for testing
type mockDbusConn struct {
	connected bool
	props     map[string]interface{}
	err       error
}

func (m *mockDbusConn) Close() {}

func (m *mockDbusConn) Connected() bool {
	return m.connected
}

func (m *mockDbusConn) GetUnitPropertiesContext(_ context.Context, _ string) (map[string]interface{}, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.props, nil
}

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
			result := normalizeServiceUnitName(tt.input)
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
		conn        *mockDbusConn
		unitName    string
		expected    bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil connection",
			conn:        nil,
			unitName:    "test.service",
			expectError: true,
			errorMsg:    "connection not initialized",
		},
		{
			name: "disconnected connection",
			conn: &mockDbusConn{
				connected: false,
			},
			unitName:    "test.service",
			expectError: true,
			errorMsg:    "connection disconnected",
		},
		{
			name: "error getting properties",
			conn: &mockDbusConn{
				connected: true,
				err:       fmt.Errorf("dbus error"),
			},
			unitName:    "test.service",
			expectError: true,
			errorMsg:    "unable to get unit properties for test.service: dbus error",
		},
		{
			name: "active service",
			conn: &mockDbusConn{
				connected: true,
				props: map[string]interface{}{
					"ActiveState": "active",
				},
			},
			unitName:    "test.service",
			expected:    true,
			expectError: false,
		},
		{
			name: "inactive service",
			conn: &mockDbusConn{
				connected: true,
				props: map[string]interface{}{
					"ActiveState": "inactive",
				},
			},
			unitName:    "test.service",
			expected:    false,
			expectError: false,
		},
		{
			name: "service with target suffix",
			conn: &mockDbusConn{
				connected: true,
				props: map[string]interface{}{
					"ActiveState": "active",
				},
			},
			unitName:    "test.target",
			expected:    true,
			expectError: false,
		},
		{
			name: "service without suffix",
			conn: &mockDbusConn{
				connected: true,
				props: map[string]interface{}{
					"ActiveState": "active",
				},
			},
			unitName:    "test",
			expected:    true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conn *DbusConn
			if tt.conn != nil {
				conn = &DbusConn{conn: tt.conn}
			} else {
				conn = &DbusConn{conn: nil}
			}

			result, err := conn.IsActive(context.Background(), tt.unitName)
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMsg, err.Error())
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

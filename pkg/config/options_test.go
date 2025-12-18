package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
)

func TestOpApplyOpts(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		op := &Op{}
		err := op.ApplyOpts([]OpOption{})

		assert.NoError(t, err)
	})
}

func TestWithFailureInjector(t *testing.T) {
	tests := []struct {
		name     string
		injector *components.FailureInjector
	}{
		{
			name:     "nil injector",
			injector: nil,
		},
		{
			name: "empty injector",
			injector: &components.FailureInjector{
				GPUUUIDsWithRowRemappingPending: []string{},
				GPUUUIDsWithRowRemappingFailed:  []string{},
			},
		},
		{
			name: "injector with UUIDs",
			injector: &components.FailureInjector{
				GPUUUIDsWithRowRemappingPending: []string{"GPU-12345678-1234-1234-1234-123456789012"},
				GPUUUIDsWithRowRemappingFailed:  []string{"GPU-87654321-4321-4321-4321-210987654321"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithFailureInjector(tt.injector)
			option(op)

			assert.Equal(t, tt.injector, op.FailureInjector)
		})
	}
}

func TestWithExcludedInfinibandDevices(t *testing.T) {
	t.Parallel()

	op := &Op{}
	WithExcludedInfinibandDevices([]string{"mlx5_0", "mlx5_1"})(op)

	assert.Equal(t, []string{"mlx5_0", "mlx5_1"}, op.ExcludedInfinibandDevices)
}

// TestWithSessionToken tests the WithSessionToken option.
// Note: SessionToken is the SESSION token returned by the control plane after login,
// NOT the registration token passed via --token flag.
// The registration token is used only for login.Login() authentication.
// The session token (loginResp.Token) is what gets stored in the DB and used for session keepalive.
func TestWithSessionToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty token",
			token: "",
		},
		{
			name:  "valid session token from control plane",
			token: "session-token-from-login-response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithSessionToken(tt.token)
			option(op)

			assert.Equal(t, tt.token, op.SessionToken)
		})
	}
}

// TestWithSessionMachineID tests the WithSessionMachineID option.
// Note: SessionMachineID is the machine ID ASSIGNED by the control plane after login,
// NOT the machine ID passed via --machine-id flag (which is optional for override).
// The assigned machine ID (loginResp.MachineID) is what gets stored in the DB.
func TestWithSessionMachineID(t *testing.T) {
	tests := []struct {
		name      string
		machineID string
	}{
		{
			name:      "empty machine ID",
			machineID: "",
		},
		{
			name:      "valid assigned machine ID from control plane",
			machineID: "assigned-machine-id-from-login-response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithSessionMachineID(tt.machineID)
			option(op)

			assert.Equal(t, tt.machineID, op.SessionMachineID)
		})
	}
}

// TestDBInMemoryWithSessionCredentials tests the combination of
// --db-in-memory with session credentials.
//
// Token flow distinction:
// - Registration token (--token flag): Used only for login.Login() to authenticate with control plane
// - Session token (loginResp.Token): Returned by control plane, stored in DB, used for session keepalive
// - Assigned machine ID (loginResp.MachineID): Returned by control plane, stored in DB
// - Endpoint: Stored in DB, needed for session keepalive (server reads from DB, not config)
//
// This tests the scenario where:
// 1. login.Login() authenticates with registration token, receives session token + assigned machine ID
// 2. login.Login() writes SESSION token, ASSIGNED machine ID, and ENDPOINT to persistent file
// 3. When --db-in-memory, gpud run reads session credentials from persistent file
// 4. gpud run passes them via WithSessionToken, WithSessionMachineID, WithSessionEndpoint to config
// 5. server.New() seeds them into the in-memory database for session keepalive
func TestDBInMemoryWithSessionCredentials(t *testing.T) {
	op := &Op{}

	// Apply all options as gpud run would
	// Note: These are SESSION credentials (from login response), not registration credentials
	opts := []OpOption{
		WithDBInMemory(true),
		WithSessionToken("session-token-from-login-response"),
		WithSessionMachineID("assigned-machine-id-from-login-response"),
		WithSessionEndpoint("https://api.example.com"),
	}

	err := op.ApplyOpts(opts)
	assert.NoError(t, err)

	assert.True(t, op.DBInMemory)
	assert.Equal(t, "session-token-from-login-response", op.SessionToken)
	assert.Equal(t, "assigned-machine-id-from-login-response", op.SessionMachineID)
	assert.Equal(t, "https://api.example.com", op.SessionEndpoint)
}

func TestWithDBInMemory(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected bool
	}{
		{
			name:     "enabled",
			value:    true,
			expected: true,
		},
		{
			name:     "disabled",
			value:    false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithDBInMemory(tt.value)
			option(op)
			assert.Equal(t, tt.expected, op.DBInMemory)
		})
	}
}

func TestWithDataDir(t *testing.T) {
	tests := []struct {
		name     string
		dataDir  string
		expected string
	}{
		{
			name:     "empty data dir",
			dataDir:  "",
			expected: "",
		},
		{
			name:     "custom data dir",
			dataDir:  "/custom/path",
			expected: "/custom/path",
		},
		{
			name:     "home directory",
			dataDir:  "/home/user/.gpud",
			expected: "/home/user/.gpud",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithDataDir(tt.dataDir)
			option(op)
			assert.Equal(t, tt.expected, op.DataDir)
		})
	}
}

func TestWithInfinibandClassRootDir(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		expected string
	}{
		{
			name:     "empty dir",
			dir:      "",
			expected: "",
		},
		{
			name:     "custom dir",
			dir:      "/sys/class/infiniband",
			expected: "/sys/class/infiniband",
		},
		{
			name:     "alternate path",
			dir:      "/custom/infiniband/class",
			expected: "/custom/infiniband/class",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithInfinibandClassRootDir(tt.dir)
			option(op)
			assert.Equal(t, tt.expected, op.InfinibandClassRootDir)
		})
	}
}

// TestWithSessionEndpoint tests the WithSessionEndpoint option.
// Note: SessionEndpoint is needed because the server reads the endpoint from
// the metadata DB (not from config) for session keepalive.
func TestWithSessionEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
	}{
		{
			name:     "empty endpoint",
			endpoint: "",
		},
		{
			name:     "valid endpoint",
			endpoint: "https://api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			option := WithSessionEndpoint(tt.endpoint)
			option(op)

			assert.Equal(t, tt.endpoint, op.SessionEndpoint)
		})
	}
}

func TestApplyOptsWithMultipleOptions(t *testing.T) {
	op := &Op{}

	opts := []OpOption{
		WithDataDir("/custom/data"),
		WithDBInMemory(true),
		WithSessionToken("token123"),
		WithSessionMachineID("machine456"),
		WithSessionEndpoint("https://api.example.com"),
		WithInfinibandClassRootDir("/sys/class/infiniband"),
		WithExcludedInfinibandDevices([]string{"mlx5_0"}),
	}

	err := op.ApplyOpts(opts)
	assert.NoError(t, err)

	assert.Equal(t, "/custom/data", op.DataDir)
	assert.True(t, op.DBInMemory)
	assert.Equal(t, "token123", op.SessionToken)
	assert.Equal(t, "machine456", op.SessionMachineID)
	assert.Equal(t, "https://api.example.com", op.SessionEndpoint)
	assert.Equal(t, "/sys/class/infiniband", op.InfinibandClassRootDir)
	assert.Equal(t, []string{"mlx5_0"}, op.ExcludedInfinibandDevices)
}

package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("default values", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx)
		require.NoError(t, err)

		// Check basic properties that don't depend on the environment
		assert.Equal(t, DefaultAPIVersion, cfg.APIVersion)
		assert.Equal(t, fmt.Sprintf(":%d", DefaultGPUdPort), cfg.Address)
		assert.Equal(t, DefaultRetentionPeriod, cfg.RetentionPeriod)
		assert.Equal(t, DefaultEventsRetentionPeriod, cfg.EventsRetentionPeriod)
		assert.Equal(t, DefaultCompactPeriod, cfg.CompactPeriod)
		assert.False(t, cfg.Pprof)
		assert.True(t, cfg.EnableAutoUpdate)
		assert.NotEmpty(t, cfg.DataDir)

		// We can't reliably test State path since it depends on environment
		assert.NotEmpty(t, cfg.State, "State path should be set")
	})

	t.Run("with options", func(t *testing.T) {
		classDir := "/custom/class"

		cfg, err := DefaultConfig(ctx, WithInfinibandClassRootDir(classDir))
		require.NoError(t, err)

		assert.Equal(t, classDir, cfg.NvidiaToolOverwrites.InfinibandClassRootDir)
	})

	t.Run("with custom data dir", func(t *testing.T) {
		tempDir := t.TempDir()
		customDir := filepath.Join(tempDir, "data-dir")

		cfg, err := DefaultConfig(ctx, WithDataDir(customDir))
		require.NoError(t, err)

		assert.Equal(t, customDir, cfg.DataDir)
		assert.Equal(t, filepath.Join(customDir, "gpud.state"), cfg.State)

		_, err = os.Stat(customDir)
		assert.NoError(t, err)
	})
}

func TestDefaultStateAndFifoFiles(t *testing.T) {
	// We can only verify these functions don't error in a cross-platform way
	// since the actual paths depend on environment (root/non-root, OS, etc.)

	t.Run("DefaultStateFile", func(t *testing.T) {
		path, err := DefaultStateFile()
		require.NoError(t, err)
		assert.NotEmpty(t, path, "Path should not be empty")
	})

	t.Run("DefaultFifoFile", func(t *testing.T) {
		path, err := DefaultFifoFile()
		require.NoError(t, err)
		assert.NotEmpty(t, path, "Path should not be empty")
	})
}

func TestPackagesDir(t *testing.T) {
	tests := []struct {
		name     string
		dataDir  string
		expected string
	}{
		{
			name:     "default data dir",
			dataDir:  "/var/lib/gpud",
			expected: "/var/lib/gpud/packages",
		},
		{
			name:     "custom data dir",
			dataDir:  "/custom/path",
			expected: "/custom/path/packages",
		},
		{
			name:     "home directory",
			dataDir:  "/home/user/.gpud",
			expected: "/home/user/.gpud/packages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PackagesDir(tt.dataDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVersionFilePath(t *testing.T) {
	tests := []struct {
		name     string
		dataDir  string
		expected string
	}{
		{
			name:     "default data dir",
			dataDir:  "/var/lib/gpud",
			expected: "/var/lib/gpud/target_version",
		},
		{
			name:     "custom data dir",
			dataDir:  "/custom/path",
			expected: "/custom/path/target_version",
		},
		{
			name:     "home directory",
			dataDir:  "/home/user/.gpud",
			expected: "/home/user/.gpud/target_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VersionFilePath(tt.dataDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStateFilePath(t *testing.T) {
	tests := []struct {
		name     string
		dataDir  string
		expected string
	}{
		{
			name:     "default data dir",
			dataDir:  "/var/lib/gpud",
			expected: "/var/lib/gpud/gpud.state",
		},
		{
			name:     "custom data dir",
			dataDir:  "/custom/path",
			expected: "/custom/path/gpud.state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StateFilePath(tt.dataDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFifoFilePath(t *testing.T) {
	tests := []struct {
		name     string
		dataDir  string
		expected string
	}{
		{
			name:     "default data dir",
			dataDir:  "/var/lib/gpud",
			expected: "/var/lib/gpud/gpud.fifo",
		},
		{
			name:     "custom data dir",
			dataDir:  "/custom/path",
			expected: "/custom/path/gpud.fifo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FifoFilePath(tt.dataDir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDefaultConfigWithSessionCredentials tests DefaultConfig with session credentials.
//
// Token flow distinction:
// - Registration token (--token flag): Used only for login.Login() to authenticate
// - Session token (loginResp.Token): Returned by control plane, stored in DB, used for keepalive
// - Assigned machine ID (loginResp.MachineID): Returned by control plane, stored in DB
// - Endpoint: Stored in DB, server reads from DB (not config) for session keepalive
//
// The config options SessionToken, SessionMachineID, SessionEndpoint are for SESSION credentials,
// NOT the registration token.
func TestDefaultConfigWithSessionCredentials(t *testing.T) {
	ctx := context.Background()

	t.Run("with session token, assigned machine ID, and endpoint", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx,
			WithSessionToken("session-token-from-login-response"),
			WithSessionMachineID("assigned-machine-id-from-login-response"),
			WithSessionEndpoint("https://api.example.com"),
		)
		require.NoError(t, err)

		assert.Equal(t, "session-token-from-login-response", cfg.SessionToken)
		assert.Equal(t, "assigned-machine-id-from-login-response", cfg.SessionMachineID)
		assert.Equal(t, "https://api.example.com", cfg.SessionEndpoint)
	})

	t.Run("with db in memory and all session credentials", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx,
			WithDBInMemory(true),
			WithSessionToken("session-token-for-in-memory-mode"),
			WithSessionMachineID("assigned-machine-id-for-in-memory-mode"),
			WithSessionEndpoint("https://api.example.com"),
		)
		require.NoError(t, err)

		assert.True(t, cfg.DBInMemory)
		assert.Equal(t, "session-token-for-in-memory-mode", cfg.SessionToken)
		assert.Equal(t, "assigned-machine-id-for-in-memory-mode", cfg.SessionMachineID)
		assert.Equal(t, "https://api.example.com", cfg.SessionEndpoint)
	})

	t.Run("empty session credentials", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx)
		require.NoError(t, err)

		assert.Empty(t, cfg.SessionToken)
		assert.Empty(t, cfg.SessionMachineID)
		assert.Empty(t, cfg.SessionEndpoint)
	})
}

func TestDefaultConfigWithDBInMemory(t *testing.T) {
	ctx := context.Background()

	t.Run("db in memory true", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx, WithDBInMemory(true))
		require.NoError(t, err)
		assert.True(t, cfg.DBInMemory)
	})

	t.Run("db in memory false", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx, WithDBInMemory(false))
		require.NoError(t, err)
		assert.False(t, cfg.DBInMemory)
	})

	t.Run("db in memory default", func(t *testing.T) {
		cfg, err := DefaultConfig(ctx)
		require.NoError(t, err)
		assert.False(t, cfg.DBInMemory)
	})
}

func TestResolveDataDir(t *testing.T) {
	t.Run("empty data dir uses default", func(t *testing.T) {
		dir, err := ResolveDataDir("")
		require.NoError(t, err)
		assert.NotEmpty(t, dir)
	})

	t.Run("custom data dir is created", func(t *testing.T) {
		tempDir := t.TempDir()
		customDir := filepath.Join(tempDir, "custom-gpud-data")

		dir, err := ResolveDataDir(customDir)
		require.NoError(t, err)
		assert.Equal(t, customDir, dir)

		// Verify directory was created
		info, err := os.Stat(customDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

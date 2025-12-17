package run

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/config"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

func TestReadSessionCredentialsFromPersistentFile(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "gpud-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	stateFile := config.StateFilePath(tmpDir)
	dbRW, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	defer func() {
		_ = dbRW.Close()
	}()

	require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
	require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "session-token"))
	require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, "assigned-machine-id"))
	require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyEndpoint, "gpud-manager.example.com"))

	gotToken, gotMachineID, gotEndpoint, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "session-token", gotToken)
	assert.Equal(t, "assigned-machine-id", gotMachineID)
	assert.Equal(t, "gpud-manager.example.com", gotEndpoint)
}

func TestReadSessionCredentialsFromPersistentFile_StateFileNotFound(t *testing.T) {
	t.Parallel()

	// Create a temp directory but don't create any state file in it
	tmpDir, err := os.MkdirTemp("", "gpud-test-no-state-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	// Call the function - it should return errStateFileNotFound
	_, _, _, err = readSessionCredentialsFromPersistentFile(ctx, tmpDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, errStateFileNotFound)
}

// TestReadSessionCredentialsFromPersistentFile_EmptyMetadata verifies that when the
// metadata table exists but has no entries, the function returns empty strings (not errors).
// This covers the code path where ReadToken, ReadMachineID, and ReadMetadata are called
// but return empty strings because the keys don't exist.
func TestReadSessionCredentialsFromPersistentFile_EmptyMetadata(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "gpud-test-empty-metadata-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	// Create state file with metadata table but no entries
	stateFile := config.StateFilePath(tmpDir)
	dbRW, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	defer func() {
		_ = dbRW.Close()
	}()

	require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
	// Don't set any metadata - table exists but is empty

	// The function should succeed but return empty strings for all values
	// (ReadMetadata returns empty string + nil error when key doesn't exist)
	gotToken, gotMachineID, gotEndpoint, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
	require.NoError(t, err)
	assert.Empty(t, gotToken, "token should be empty when not set")
	assert.Empty(t, gotMachineID, "machine ID should be empty when not set")
	assert.Empty(t, gotEndpoint, "endpoint should be empty when not set")
}

// TestReadSessionCredentialsFromPersistentFile_PartialMetadata verifies that when only
// some metadata keys are set, the function returns the set values and empty strings
// for the missing ones.
func TestReadSessionCredentialsFromPersistentFile_PartialMetadata(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "gpud-test-partial-metadata-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	stateFile := config.StateFilePath(tmpDir)
	dbRW, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	defer func() {
		_ = dbRW.Close()
	}()

	require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
	// Only set token - machine ID and endpoint will be empty
	require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "session-token"))

	gotToken, gotMachineID, gotEndpoint, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "session-token", gotToken)
	assert.Empty(t, gotMachineID, "machine ID should be empty when not set")
	assert.Empty(t, gotEndpoint, "endpoint should be empty when not set")
}

// TestReadSessionCredentialsFromPersistentFile_TokenAndMachineID verifies that when
// token and machine ID are set but endpoint is missing, all are returned correctly.
func TestReadSessionCredentialsFromPersistentFile_TokenAndMachineID(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "gpud-test-token-machineid-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	stateFile := config.StateFilePath(tmpDir)
	dbRW, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	defer func() {
		_ = dbRW.Close()
	}()

	require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
	// Set token and machine ID but not endpoint
	require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "session-token"))
	require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, "assigned-machine-id"))

	gotToken, gotMachineID, gotEndpoint, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, "session-token", gotToken)
	assert.Equal(t, "assigned-machine-id", gotMachineID)
	assert.Empty(t, gotEndpoint, "endpoint should be empty when not set")
}

// TestReadSessionCredentialsFromPersistentFile_InvalidStateFile verifies that when the state
// file exists but is not a valid SQLite database, the function returns an error.
func TestReadSessionCredentialsFromPersistentFile_InvalidStateFile(t *testing.T) {
	t.Parallel()

	tmpDir, err := os.MkdirTemp("", "gpud-test-invalid-state-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	ctx := context.Background()

	// Create an invalid state file (not a SQLite database)
	stateFile := config.StateFilePath(tmpDir)
	err = os.WriteFile(stateFile, []byte("not a sqlite database"), 0644)
	require.NoError(t, err)

	// The function should return an error when trying to open the invalid file
	_, _, _, err = readSessionCredentialsFromPersistentFile(ctx, tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read session token")
}

func TestParseInfinibandExcludeDevices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "commas and spaces only",
			input: " , , ",
			want:  nil,
		},
		{
			name:  "single device",
			input: "mlx5_0",
			want:  []string{"mlx5_0"},
		},
		{
			name:  "multiple devices with spaces and empties",
			input: " mlx5_0, ,mlx5_1 ,",
			want:  []string{"mlx5_0", "mlx5_1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseInfinibandExcludeDevices(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

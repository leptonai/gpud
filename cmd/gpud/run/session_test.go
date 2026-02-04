package run

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/bytedance/mockey"
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

func TestGetSessionCredentialsOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		dbInMemory           bool
		setupState           func(t *testing.T, dataDir string)
		controlPlaneEndpoint string
		wantOps              bool
	}{
		{
			name:       "dbInMemory false",
			dbInMemory: false,
			setupState: func(t *testing.T, dataDir string) {},
			wantOps:    false,
		},
		{
			name:       "dbInMemory true, state file missing",
			dbInMemory: true,
			setupState: func(t *testing.T, dataDir string) {},
			wantOps:    false,
		},
		{
			name:       "dbInMemory true, state file corrupted",
			dbInMemory: true,
			setupState: func(t *testing.T, dataDir string) {
				stateFile := config.StateFilePath(dataDir)
				err := os.WriteFile(stateFile, []byte("corrupted"), 0644)
				require.NoError(t, err)
			},
			wantOps: false,
		},
		{
			name:       "dbInMemory true, valid credentials",
			dbInMemory: true,
			setupState: func(t *testing.T, dataDir string) {
				ctx := context.Background()
				stateFile := config.StateFilePath(dataDir)
				dbRW, err := pkgsqlite.Open(stateFile)
				require.NoError(t, err)
				defer func() { _ = dbRW.Close() }()

				require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "token"))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, "machine-id"))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyEndpoint, "endpoint"))
			},
			wantOps: true,
		},
		{
			name:       "dbInMemory true, incomplete credentials with fallback endpoint",
			dbInMemory: true,
			setupState: func(t *testing.T, dataDir string) {
				ctx := context.Background()
				stateFile := config.StateFilePath(dataDir)
				dbRW, err := pkgsqlite.Open(stateFile)
				require.NoError(t, err)
				defer func() { _ = dbRW.Close() }()

				require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "token"))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, "machine-id"))
				// Endpoint missing in DB
			},
			controlPlaneEndpoint: "fallback-endpoint",
			wantOps:              true,
		},
		{
			name:       "dbInMemory true, incomplete credentials without fallback endpoint",
			dbInMemory: true,
			setupState: func(t *testing.T, dataDir string) {
				ctx := context.Background()
				stateFile := config.StateFilePath(dataDir)
				dbRW, err := pkgsqlite.Open(stateFile)
				require.NoError(t, err)
				defer func() { _ = dbRW.Close() }()

				require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "token"))
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, "machine-id"))
				// Endpoint missing in DB
			},
			controlPlaneEndpoint: "",
			wantOps:              false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, err := os.MkdirTemp("", "gpud-test-creds-*")
			require.NoError(t, err)
			defer func() { _ = os.RemoveAll(tmpDir) }()

			tt.setupState(t, tmpDir)

			ops := getSessionCredentialsOptions(tt.dbInMemory, tmpDir, tt.controlPlaneEndpoint)
			if tt.wantOps {
				assert.NotEmpty(t, ops)
			} else {
				assert.Empty(t, ops)
			}
		})
	}
}

// TestRecordLoginSuccessState_Success tests successful recording of login state.
func TestRecordLoginSuccessState_Success(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("recordLoginSuccessState success", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.NoError(t, err)
	})
}

// TestRecordLoginSuccessState_ResolveDataDirError tests error handling when ResolveDataDir fails.
func TestRecordLoginSuccessState_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("recordLoginSuccessState resolve error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve data dir")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, "/tmp/test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestRecordLoginSuccessState_SqliteOpenError tests error handling when sqlite open fails.
func TestRecordLoginSuccessState_SqliteOpenError(t *testing.T) {
	tmpDir := t.TempDir()

	mockey.PatchConvey("recordLoginSuccessState sqlite open error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(pkgsqlite.Open).To(func(dbPath string, opts ...pkgsqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		ctx := context.Background()
		err := recordLoginSuccessState(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestReadSessionCredentialsFromPersistentFile_ResolveDataDirError tests error when ResolveDataDir fails.
func TestReadSessionCredentialsFromPersistentFile_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("readSessionCredentials resolve error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve")
		}).Build()

		ctx := context.Background()
		_, _, _, err := readSessionCredentialsFromPersistentFile(ctx, "/tmp/test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

// TestReadSessionCredentialsFromPersistentFile_ReadTokenError tests error when ReadToken fails.
func TestReadSessionCredentialsFromPersistentFile_ReadTokenError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := config.StateFilePath(tmpDir)
	realDB, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	// Ping to ensure the database file is created (sql.Open is lazy)
	require.NoError(t, realDB.Ping())
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("readSessionCredentials read token error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("failed to read token")
		}).Build()

		ctx := context.Background()
		_, _, _, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read session token")
	})
}

// TestReadSessionCredentialsFromPersistentFile_ReadMachineIDError tests error when ReadMachineID fails.
func TestReadSessionCredentialsFromPersistentFile_ReadMachineIDError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := config.StateFilePath(tmpDir)
	realDB, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	// Ping to ensure the database file is created (sql.Open is lazy)
	require.NoError(t, realDB.Ping())
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("readSessionCredentials read machine ID error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "token", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("failed to read machine ID")
		}).Build()

		ctx := context.Background()
		_, _, _, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read machine ID")
	})
}

// TestReadSessionCredentialsFromPersistentFile_ReadEndpointError tests error when ReadMetadata fails.
func TestReadSessionCredentialsFromPersistentFile_ReadEndpointError(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := config.StateFilePath(tmpDir)
	realDB, err := pkgsqlite.Open(stateFile)
	require.NoError(t, err)
	// Ping to ensure the database file is created (sql.Open is lazy)
	require.NoError(t, realDB.Ping())
	t.Cleanup(func() { _ = realDB.Close() })

	mockey.PatchConvey("readSessionCredentials read endpoint error", t, func() {
		mockey.Mock(config.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "token", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", errors.New("failed to read endpoint")
		}).Build()

		ctx := context.Background()
		_, _, _, err := readSessionCredentialsFromPersistentFile(ctx, tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read endpoint")
	})
}

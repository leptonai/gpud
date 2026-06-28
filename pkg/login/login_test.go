package login

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestShouldRefreshLoginForNodeLabels(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))

	t.Run("ignores metadata when labels are not configured", func(t *testing.T) {
		refresh, err := shouldRefreshLoginForNodeLabels(ctx, dbRO, `{"user.node.lepton.ai/team":"ml"}`, false)
		require.NoError(t, err)
		assert.False(t, refresh)
	})

	t.Run("refreshes when no previous labels were recorded", func(t *testing.T) {
		refresh, err := shouldRefreshLoginForNodeLabels(ctx, dbRO, `{"user.node.lepton.ai/team":"ml"}`, true)
		require.NoError(t, err)
		assert.True(t, refresh)
	})

	t.Run("skips refresh when canonical labels match", func(t *testing.T) {
		require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyLastSentNodeLabels, `{"user.node.lepton.ai/team":"ml"}`))

		refresh, err := shouldRefreshLoginForNodeLabels(ctx, dbRO, `{"user.node.lepton.ai/team":"ml"}`, true)
		require.NoError(t, err)
		assert.False(t, refresh)
	})

	t.Run("refreshes when canonical labels differ", func(t *testing.T) {
		refresh, err := shouldRefreshLoginForNodeLabels(ctx, dbRO, `{}`, true)
		require.NoError(t, err)
		assert.True(t, refresh)
	})

	t.Run("returns metadata read errors", func(t *testing.T) {
		canceledCtx, cancelCanceled := context.WithCancel(context.Background())
		cancelCanceled()

		refresh, err := shouldRefreshLoginForNodeLabels(canceledCtx, dbRO, `{}`, true)
		require.Error(t, err)
		assert.False(t, refresh)
	})
}

func TestReconcileMachineID(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// seed installs a persisted login identity (machine ID + session token) into a
	// fresh test DB so each subtest starts from a "previously logged-in" state.
	seed := func(t *testing.T, machineID string) (*sql.DB, *sql.DB) {
		t.Helper()
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		t.Cleanup(cleanup)
		require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
		if machineID != "" {
			require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, machineID))
			require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "session-token-for-"+machineID))
		}
		return dbRW, dbRO
	}

	t.Run("no persisted ID is a no-op", func(t *testing.T) {
		dbRW, _ := seed(t, "")
		// overwrite=false must NOT error when there is nothing persisted to overwrite.
		got, err := reconcileMachineID(ctx, dbRW, "", "new-machine", false)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("empty requested ID keeps persisted identity (non-container path)", func(t *testing.T) {
		dbRW, dbRO := seed(t, "old-machine")
		// No --machine-id supplied: must keep the persisted ID untouched, even with
		// overwrite=false. This proves non-container/CLI usage is unaffected.
		got, err := reconcileMachineID(ctx, dbRW, "old-machine", "", false)
		require.NoError(t, err)
		assert.Equal(t, "old-machine", got)

		persisted, err := pkgmetadata.ReadMachineID(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, "old-machine", persisted)
	})

	t.Run("matching IDs is a no-op", func(t *testing.T) {
		dbRW, dbRO := seed(t, "same-machine")
		got, err := reconcileMachineID(ctx, dbRW, "same-machine", "same-machine", false)
		require.NoError(t, err)
		assert.Equal(t, "same-machine", got)

		persisted, err := pkgmetadata.ReadMachineID(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, "same-machine", persisted)
	})

	t.Run("mismatch without overwrite fails and leaves identity intact", func(t *testing.T) {
		dbRW, dbRO := seed(t, "old-machine")
		got, err := reconcileMachineID(ctx, dbRW, "old-machine", "new-machine", false)
		require.Error(t, err)
		// the returned prevMachineID is unchanged so callers never act on a half-state
		assert.Equal(t, "old-machine", got)
		// actionable, mentions the escape hatch
		assert.Contains(t, err.Error(), "differs from requested")
		assert.Contains(t, err.Error(), "--machine-id-overwrite")

		// persisted identity must be untouched on the failure path
		persisted, err := pkgmetadata.ReadMachineID(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, "old-machine", persisted)
		token, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyToken)
		require.NoError(t, err)
		assert.Equal(t, "session-token-for-old-machine", token)
	})

	t.Run("mismatch with overwrite clears identity for fresh registration", func(t *testing.T) {
		dbRW, dbRO := seed(t, "old-machine")
		got, err := reconcileMachineID(ctx, dbRW, "old-machine", "new-machine", true)
		require.NoError(t, err)
		// returns "" so the caller performs a fresh login as the new machine
		assert.Empty(t, got)

		// the whole persisted login identity (machine ID + session token) is gone
		persisted, err := pkgmetadata.ReadMachineID(ctx, dbRO)
		require.NoError(t, err)
		assert.Empty(t, persisted)
		token, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyToken)
		require.NoError(t, err)
		assert.Empty(t, token)
	})
}

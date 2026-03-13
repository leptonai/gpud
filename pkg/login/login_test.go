package login

import (
	"context"
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

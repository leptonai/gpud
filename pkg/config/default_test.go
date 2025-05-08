package config

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/version"
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
		assert.Equal(t, DefaultCompactPeriod, cfg.CompactPeriod)
		assert.False(t, cfg.Pprof)
		assert.True(t, cfg.EnableAutoUpdate)
		assert.Equal(t, version.Version, cfg.Annotations["version"])

		// We can't reliably test State path since it depends on environment
		assert.NotEmpty(t, cfg.State, "State path should be set")
	})

	t.Run("with options", func(t *testing.T) {
		ibstatCmd := "/custom/ibstat"
		ibstatusCmd := "/custom/ibstatus"

		cfg, err := DefaultConfig(ctx, WithIbstatCommand(ibstatCmd), WithIbstatusCommand(ibstatusCmd))
		require.NoError(t, err)

		assert.Equal(t, ibstatCmd, cfg.NvidiaToolOverwrites.IbstatCommand)
		assert.Equal(t, ibstatusCmd, cfg.NvidiaToolOverwrites.IbstatusCommand)
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

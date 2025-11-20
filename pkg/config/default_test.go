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

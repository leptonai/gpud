package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDataDir(t *testing.T) {
	t.Run("nil context uses default", func(t *testing.T) {
		dir, err := ResolveDataDir(nil)
		require.NoError(t, err)
		assert.NotEmpty(t, dir)
	})
}

func TestStateFileFromContext(t *testing.T) {
	t.Run("nil context returns valid state file path", func(t *testing.T) {
		path, err := StateFileFromContext(nil)
		require.NoError(t, err)
		assert.NotEmpty(t, path)
		assert.Contains(t, path, "gpud.state")
	})
}

func TestVersionFileFromContext(t *testing.T) {
	t.Run("nil context with flag set returns flag value", func(t *testing.T) {
		customPath := "/custom/version/file"
		path, err := VersionFileFromContext(nil, customPath, true)
		require.NoError(t, err)
		assert.Equal(t, customPath, path)
	})

	t.Run("nil context without flag set returns default", func(t *testing.T) {
		path, err := VersionFileFromContext(nil, "", false)
		require.NoError(t, err)
		assert.NotEmpty(t, path)
		assert.Contains(t, path, "target_version")
	})
}

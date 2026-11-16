package disk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
)

func TestLocalUsageSucceedsWhenRemoteScanFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "df")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
set -eu
# Model an unavailable remote filesystem: only the local scan succeeds.
[ "$*" = '-T -B1 -P --local' ] || exit 7
printf '%s\n' '/dev/root ext4 1000 400 600 40% /'
`), 0700))
	c, err := New(&components.GPUdInstance{RootCtx: t.Context(), BlockdevUsageCommands: "sh '" + path + "'"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	collector := c.(*component)
	parts, err := collector.getExt4PartitionsFunc(t.Context())
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "/", parts[0].MountPoint)
	assert.Equal(t, uint64(1000), parts[0].Usage.TotalBytes)
	_, err = collector.getNFSPartitionsFunc(t.Context())
	require.Error(t, err, "remote collection failures must still reach the health check")
}

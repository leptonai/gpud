package disk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
)

func TestLocalUsageSucceedsWhenRemoteScanFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "df")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
set -eu
# Model an unavailable remote filesystem: only the local scan succeeds.
[ "$*" = '-T -B1 -P -t ext4' ] || exit 7
printf '%s\n' '/dev/root ext4 1000 400 600 40% /'
`), 0700))
	c, err := New(&components.GPUdInstance{RootCtx: t.Context(), FindmntCommands: nfsInventoryCommand(t), BlockdevUsageCommands: "sh '" + path + "'"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	collector := c.(*component)
	parts, err := collector.getExt4PartitionsFunc(t.Context())
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "/", parts[0].MountPoint)
	assert.Equal(t, uint64(1000), parts[0].Usage.TotalBytes)
	parts, err = collector.getNFSPartitionsFunc(t.Context())
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.NotEmpty(t, parts[0].StatError, "remote collection failures must reach the health check")
}

func TestBackgroundNFSScanPublishesUsage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "df")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
[ "$FAIL_NFS_SCAN" = 0 ] || exit 7
printf '%s\n' 'server:/export nfs4 2000 1000 1000 50% /remote'
`), 0700))
	command := "sh '" + path + "'"
	c, err := New(&components.GPUdInstance{RootCtx: t.Context(), FindmntCommands: nfsInventoryCommand(t), BlockdevUsageCommands: command})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(); disk.PublishNFSUsage(command, nil) })
	collector := c.(*component)
	part := disk.Partition{Device: "server:/export", Fstype: "nfs4", MountPoint: "/remote"}
	assert.Nil(t, disk.CachedNFSUsage(command, part))
	t.Setenv("FAIL_NFS_SCAN", "0")
	_, err = collector.getNFSPartitionsFunc(t.Context())
	require.NoError(t, err)
	want := &disk.Usage{TotalBytes: 2000, UsedBytes: 1000, FreeBytes: 1000}
	assert.Equal(t, want, disk.CachedNFSUsage(command, part))
	t.Setenv("FAIL_NFS_SCAN", "1")
	parts, err := collector.getNFSPartitionsFunc(t.Context())
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.NotEmpty(t, parts[0].StatError)
	assert.Equal(t, want, disk.CachedNFSUsage(command, part), "failed scans retain the last successful sample until expiration")
}

func nfsInventoryCommand(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "findmnt")
	require.NoError(t, os.WriteFile(path, []byte(`#!/bin/sh
printf '%s\n' '{"filesystems":[{"source":"server:/export","fstype":"nfs4","target":"/remote"}]}'
`), 0700))
	return "sh '" + path + "'"
}

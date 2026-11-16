package disk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func diskCommandFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0700))
	return "sh '" + path + "'"
}

func TestGetMountsDoesNotProbeFilesystems(t *testing.T) {
	command := diskCommandFixture(t, `
[ "$*" = '--kernel --list --json --nocanonicalize --nofsroot --output SOURCE,FSTYPE,TARGET' ] || exit 9
cat <<'JSON'
{"filesystems":[
  {"source":"/dev/root","fstype":"ext4","target":"/"},
  {"source":"server:/export","fstype":"nfs4","target":"/unreachable mount"},
  {"source":"systemd-1","fstype":"autofs","target":"/unreachable mount"},
  {"source":"server:/export","fstype":"nfs4","target":"/bind"},
  {"source":"server:/export","fstype":"nfs4","target":""}
]}
JSON
`)
	parts, err := GetMounts(context.Background(), WithFindmntCommand(command), WithFstype(DefaultNFSFsTypeFunc), WithMountPoint(DefaultMountPointFunc))
	require.NoError(t, err)
	require.Len(t, parts, 2)
	assert.Equal(t, "/unreachable mount", parts[0].MountPoint)
	assert.Equal(t, "/bind", parts[1].MountPoint)
	for _, part := range parts {
		assert.Equal(t, "server:/export", part.Device)
		assert.True(t, part.Mounted)
		assert.Nil(t, part.Usage)
	}
}

func TestGetMountsErrors(t *testing.T) {
	for _, body := range []string{"exit 7", "printf 'invalid json'"} {
		t.Run(body, func(t *testing.T) {
			parts, err := GetMounts(context.Background(), WithFindmntCommand(diskCommandFixture(t, body)))
			require.Error(t, err)
			assert.Nil(t, parts)
		})
	}
}

func TestDiskCommandTimeouts(t *testing.T) {
	command := diskCommandFixture(t, "exec sleep 30")
	for _, collect := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"mounts", func(ctx context.Context) error {
			_, err := GetMounts(ctx, WithFindmntCommand(command))
			return err
		}},
		{"usage", func(ctx context.Context) error {
			_, err := GetUsageWithCommand(ctx, "/", command)
			return err
		}},
	} {
		t.Run(collect.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			start := time.Now()
			require.ErrorIs(t, collect.run(ctx), context.DeadlineExceeded)
			assert.Less(t, time.Since(start), 5*time.Second)
		})
	}
}

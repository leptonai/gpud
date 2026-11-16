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
	for _, body := range []string{"exit 7", "printf 'invalid json'", "printf '{}'", "printf '{\"filesystems\":null}'"} {
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
		{"mount source", func(ctx context.Context) error {
			_, err := FindMountSource(ctx, "/var/lib/kubelet", command)
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

func TestFindMountSource(t *testing.T) {
	command := diskCommandFixture(t, `
[ "$*" = '--kernel --list --json --nofsroot --output SOURCE,FSTYPE,TARGET --target /var/lib/kubelet' ] || exit 9
printf '%s\n' '{"filesystems":[{"source":"/dev/root","target":"/","fstype":"ext4"}]}'
`)
	source, err := FindMountSource(context.Background(), "/var/lib/kubelet", command)
	require.NoError(t, err)
	assert.Equal(t, "/dev/root", source)
	for _, output := range []string{`{"filesystems":[]}`, `{"filesystems":[{"source":""}]}`} {
		_, err := FindMountSource(context.Background(), "/", diskCommandFixture(t, "printf '%s' '"+output+"'"))
		require.Error(t, err)
	}
}

func TestDiskCommandsDrainStderr(t *testing.T) {
	// More than a pipe buffer: collecting stderr after stdout would deadlock.
	noisy := "dd if=/dev/zero bs=1048576 count=1 >&2 2>/dev/null\n"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	parts, err := GetMounts(ctx, WithFindmntCommand(diskCommandFixture(t, noisy+`printf '%s' '{"filesystems":[]}'`)))
	require.NoError(t, err)
	assert.Empty(t, parts)
	usage, err := GetUsageWithCommand(ctx, "/", diskCommandFixture(t, noisy+`printf '%s\n' '/dev/root ext4 1000 400 600 40% /'`))
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), usage.TotalBytes)
}

func TestRootUsageRejectsWrongFilesystem(t *testing.T) {
	_, err := GetUsageWithCommand(context.Background(), "/", diskCommandFixture(t, `printf '%s\n' '/dev/data ext4 1000 400 600 40% /data'`))
	require.ErrorContains(t, err, "expected root filesystem usage")
}

func TestFindMountSourceStackedMounts(t *testing.T) {
	for _, tc := range []struct{ name, output, want string }{
		{"missing source", `{"filesystems":[{"source":"/dev/root"},{"source":""}]}`, "/dev/root"},
		{"self bind", `{"filesystems":[{"source":"/dev/root"},{"source":"/dev/root"}]}`, "/dev/root"},
		{"overmount", `{"filesystems":[{"source":"/dev/root"},{"source":"/dev/data"}]}`, "/dev/data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, err := FindMountSource(t.Context(), "/var/lib/kubelet", diskCommandFixture(t, "printf '%s' '"+tc.output+"'"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, source)
		})
	}
}

func TestDiskCommandFailureWithInheritedStderrHonorsDeadline(t *testing.T) {
	command := diskCommandFixture(t, `sleep 30 >&2 &
printf '%s\n' 'df: no file systems processed' >&2
exit 1
`)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := GetPartitions(ctx, WithBlockdevUsageCommand(command), WithFstypeName("ext4"))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
}

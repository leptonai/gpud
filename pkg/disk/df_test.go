package disk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunDfCommandExecutes is a regression test for the blockdev-usage override:
// runDfCommand must Start() the process before reading it. A missing Start()
// previously surfaced at runtime as "failed to read df output: process not
// started", failing the disk component's ext4 partitions check. Using "echo"
// keeps the test portable (no dependency on df flag support).
func TestRunDfCommandExecutes(t *testing.T) {
	out, err := runDfCommand(context.Background(), "echo disk-ok")
	require.NoError(t, err)
	assert.Contains(t, out, "disk-ok")
}

func TestParseDfOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    Partitions
		wantErr bool
	}{
		{
			name:   "empty output",
			output: "",
			want:   Partitions{},
		},
		{
			name: "typical df -T -B1 -P output",
			output: `Filesystem     Type 1B-blocks         Used     Available Capacity Mounted on
/dev/root      ext4 133003395072 36369936384   96616681472      28% /
/dev/sdb1      ext4 3063790665728      32768 2908082184192       1% /mnt`,
			want: Partitions{
				{
					Device:     "/dev/root",
					Fstype:     "ext4",
					MountPoint: "/",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 133003395072,
						FreeBytes:  96616681472,
						UsedBytes:  36369936384,
					},
				},
				{
					Device:     "/dev/sdb1",
					Fstype:     "ext4",
					MountPoint: "/mnt",
					Mounted:    true,
					Usage: &Usage{
						TotalBytes: 3063790665728,
						FreeBytes:  2908082184192,
						UsedBytes:  32768,
					},
				},
			},
		},
		{
			name: "mount point with spaces is preserved",
			output: `Filesystem Type 1B-blocks Used Available Capacity Mounted on
/dev/sdc1 ext4 1000 100 900 10% /mnt/my disk`,
			want: Partitions{
				{
					Device:     "/dev/sdc1",
					Fstype:     "ext4",
					MountPoint: "/mnt/my disk",
					Mounted:    true,
					Usage:      &Usage{TotalBytes: 1000, FreeBytes: 900, UsedBytes: 100},
				},
			},
		},
		{
			name: "non-numeric and short lines are skipped defensively",
			output: `Filesystem Type 1B-blocks Used Available Capacity Mounted on
df: /broken: Permission denied
overlay overlay - - - - /
/dev/root ext4 1000 100 900 10% /
short line`,
			want: Partitions{
				{
					Device:     "/dev/root",
					Fstype:     "ext4",
					MountPoint: "/",
					Mounted:    true,
					Usage:      &Usage{TotalBytes: 1000, FreeBytes: 900, UsedBytes: 100},
				},
			},
		},
		{
			name: "blank lines are ignored",
			output: `
Filesystem Type 1B-blocks Used Available Capacity Mounted on

/dev/root ext4 1000 100 900 10% /
`,
			want: Partitions{
				{
					Device:     "/dev/root",
					Fstype:     "ext4",
					MountPoint: "/",
					Mounted:    true,
					Usage:      &Usage{TotalBytes: 1000, FreeBytes: 900, UsedBytes: 100},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDfOutput(tt.output)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterDfPartitions(t *testing.T) {
	parsed := Partitions{
		{Device: "/dev/root", Fstype: "ext4", MountPoint: "/", Usage: &Usage{TotalBytes: 1000}},
		{Device: "overlay", Fstype: "overlay", MountPoint: "/", Usage: &Usage{TotalBytes: 9999}},
		{Device: "/dev/sdb1", Fstype: "ext4", MountPoint: "/mnt", Usage: &Usage{TotalBytes: 5000}},
		{Device: "nfsserver:/exp", Fstype: "nfs4", MountPoint: "/nfs", Usage: &Usage{TotalBytes: 7000}},
	}

	t.Run("ext4 filter keeps only ext4 and sorts by total desc", func(t *testing.T) {
		op := &Op{}
		require.NoError(t, op.applyOpts([]OpOption{WithFstype(DefaultExt4FsTypeFunc), WithMountPoint(DefaultMountPointFunc)}))

		got := filterDfPartitions(parsed, op)
		require.Len(t, got, 2)
		// sorted descending by total bytes: /mnt (5000) before / (1000)
		assert.Equal(t, "/mnt", got[0].MountPoint)
		assert.Equal(t, "/", got[1].MountPoint)
		for _, p := range got {
			assert.True(t, p.Mounted, "df partition should be marked mounted")
			require.NotNil(t, p.Usage)
		}
	})

	t.Run("skipUsage leaves usage nil", func(t *testing.T) {
		op := &Op{}
		require.NoError(t, op.applyOpts([]OpOption{WithFstype(DefaultExt4FsTypeFunc), WithMountPoint(DefaultMountPointFunc), WithSkipUsage()}))

		got := filterDfPartitions(parsed, op)
		for _, p := range got {
			assert.Nil(t, p.Usage, "expected usage to be nil with WithSkipUsage")
		}
	})

	t.Run("nfs filter keeps only nfs", func(t *testing.T) {
		op := &Op{}
		require.NoError(t, op.applyOpts([]OpOption{WithFstype(DefaultNFSFsTypeFunc), WithMountPoint(DefaultMountPointFunc)}))

		got := filterDfPartitions(parsed, op)
		require.Len(t, got, 1)
		assert.Equal(t, "nfs4", got[0].Fstype)
	})
}

// TestDiskCommandOptionsDefaultEmpty asserts the backward-compatibility
// invariant: with no override options, the command fields are empty, which makes
// FindMnt/lsblk/GetPartitions take the legacy in-namespace code paths.
func TestDiskCommandOptionsDefaultEmpty(t *testing.T) {
	op := &Op{}
	require.NoError(t, op.applyOpts(nil))
	assert.Empty(t, op.findmntCommand)
	assert.Empty(t, op.lsblkCommand)
	assert.Empty(t, op.blockdevUsageCommand)
}

func TestDiskCommandOptionsSet(t *testing.T) {
	op := &Op{}
	err := op.applyOpts([]OpOption{
		WithFindmntCommand("nsenter --target 1 --mount -- findmnt"),
		WithLsblkCommand("nsenter --target 1 --mount -- lsblk"),
		WithBlockdevUsageCommand("nsenter --target 1 --mount -- df"),
	})
	require.NoError(t, err)
	assert.Equal(t, "nsenter --target 1 --mount -- findmnt", op.findmntCommand)
	assert.Equal(t, "nsenter --target 1 --mount -- lsblk", op.lsblkCommand)
	assert.Equal(t, "nsenter --target 1 --mount -- df", op.blockdevUsageCommand)
}

// TestFindMntCommandWithOverrideBase verifies the command builder uses the
// override invocation prefix verbatim and still appends the flags gpud controls.
func TestFindMntCommandWithOverrideBase(t *testing.T) {
	got := findMntCommand("nsenter --target 1 --mount -- findmnt", "/var/lib/kubelet")
	assert.Equal(t, "nsenter --target 1 --mount -- findmnt --target /var/lib/kubelet --json --df", got)
}

package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/require"
)

func TestNativeNFSUsagePinnedMount(t *testing.T) {
	// Exercise O_PATH, fdinfo and statfs together on a real mount without an NFS server.
	mounts, err := procfs.GetMounts()
	require.NoError(t, err)
	for _, m := range mounts {
		if m.MountPoint == "/proc" && m.FSType == "proc" {
			part := Partition{MountPoint: m.MountPoint, Fstype: m.FSType, Device: m.Source}
			want, err := GetUsage(t.Context(), part.MountPoint)
			require.NoError(t, err)
			got, err := probeNFSUsage(t.Context(), part, "")
			require.NoError(t, err)
			require.Equal(t, want, got)
			return
		}
	}
	t.Fatal("missing procfs mount")
}

func TestNativeNFSUsageDiscardsSameExportRemount(t *testing.T) {
	part := Partition{MountPoint: t.TempDir(), Fstype: "nfs4", Device: "server:/export"}
	identities := mockey.Sequence("42", nil).Then("43", nil)
	identity := mockey.Mock(readNFSUsageIdentity).Return(identities).Build()
	defer identity.UnPatch()
	got, err := probeNFSUsage(t.Context(), part, "")
	require.ErrorContains(t, err, "identity changed")
	require.Nil(t, got)
}

func TestNativeNFSUsageDiscardsFailedSamples(t *testing.T) {
	t.Run("mount disappeared before probe", func(t *testing.T) {
		got, err := probeNFSUsage(t.Context(), Partition{MountPoint: filepath.Join(t.TempDir(), "gone")}, "")
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Nil(t, got)
	})

	t.Run("identity mismatch releases descriptor", func(t *testing.T) {
		part := Partition{MountPoint: t.TempDir(), Device: "server:/export", Fstype: "nfs"}
		got, err := probeNFSUsage(t.Context(), part, "")
		require.Error(t, err)
		require.Nil(t, got)
		fds, err := os.ReadDir("/proc/self/fd")
		require.NoError(t, err)
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
			if err == nil {
				require.NotEqual(t, part.MountPoint, target)
			}
		}
	})

	t.Run("statfs failed", func(t *testing.T) {
		part := Partition{MountPoint: t.TempDir()}
		defer mockey.Mock(readNFSUsageIdentity).Return("42", nil).Build().UnPatch()
		defer mockey.Mock(GetUsage).Return(nil, os.ErrPermission).Build().UnPatch()
		got, err := probeNFSUsage(t.Context(), part, "")
		require.ErrorIs(t, err, os.ErrPermission)
		require.Nil(t, got)
	})

	t.Run("path disappeared during statfs", func(t *testing.T) {
		part := Partition{MountPoint: t.TempDir()}
		defer mockey.Mock(readNFSUsageIdentity).Return("42", nil).Build().UnPatch()
		defer mockey.Mock(GetUsage).To(func(_ context.Context, fdPath string) (*Usage, error) {
			require.True(t, strings.HasPrefix(fdPath, "/proc/self/fd/"))
			require.NoError(t, os.Remove(part.MountPoint))
			_, err := os.Stat(fdPath)
			require.NoError(t, err, "the usage probe must still refer to the pinned directory")
			return &Usage{TotalBytes: 100}, nil
		}).Build().UnPatch()
		got, err := probeNFSUsage(t.Context(), part, "")
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Nil(t, got)
	})
}

func TestNativeNFSUsageStopsAfterCancellation(t *testing.T) {
	for _, cancelDuring := range []string{"initial identity check", "statfs"} {
		t.Run(cancelDuring, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			identities := mockey.Mock(readNFSUsageIdentity).To(func(Partition, uintptr) (string, error) {
				if cancelDuring == "initial identity check" {
					cancel()
				}
				return "42", nil
			}).Build()
			defer identities.UnPatch()
			usage := mockey.Mock(GetUsage).To(func(context.Context, string) (*Usage, error) {
				cancel()
				return &Usage{TotalBytes: 100}, nil
			}).Build()
			defer usage.UnPatch()

			got, err := probeNFSUsage(ctx, Partition{MountPoint: t.TempDir()}, "")
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, got)
			require.Equal(t, 1, identities.Times())
			if cancelDuring == "initial identity check" {
				require.Zero(t, usage.Times())
			} else {
				require.Equal(t, 1, usage.Times())
			}
		})
	}
}

func TestReadNFSUsageIdentityProcErrors(t *testing.T) {
	t.Run("process not accessible", func(t *testing.T) {
		defer mockey.Mock(procfs.Self).Return(procfs.Proc{}, os.ErrPermission).Build().UnPatch()
		_, err := readNFSUsageIdentity(Partition{}, 0)
		require.ErrorIs(t, err, os.ErrPermission)
	})
	t.Run("invalid descriptor", func(t *testing.T) {
		_, err := readNFSUsageIdentity(Partition{}, ^uintptr(0))
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("mountinfo unavailable", func(t *testing.T) {
		file, err := os.Open(t.TempDir())
		require.NoError(t, err)
		defer func() { _ = file.Close() }()
		readErr := errors.New("mountinfo unavailable")
		defer mockey.Mock(os.Open).When(func(path string) bool { return path == "/proc/self/mountinfo" }).
			Return(nil, readErr).Build().UnPatch()
		_, err = readNFSUsageIdentity(Partition{}, file.Fd())
		require.ErrorIs(t, err, readErr)
	})
}

package disk

import (
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
	const record = " 1 0:50 / /tools rw - nfs4 server:/export rw"
	identities := mockey.Sequence("42"+record, nil).Then("43"+record, nil)
	identity := mockey.Mock(readNFSUsageIdentity).Return(identities).Build()
	defer identity.UnPatch()
	got, err := probeNFSUsage(t.Context(), part, "")
	require.ErrorContains(t, err, "identity changed")
	require.Nil(t, got)
}

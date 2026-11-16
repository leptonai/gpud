package machineinfo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/disk"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/providers"
)

func TestLoginAndGossipDoNotProbeRemoteMounts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux disk inventory")
	}
	for _, tc := range []struct {
		name           string
		inventoryFails bool
		kubeletFails   bool
		warmCache      bool
	}{
		{name: "remote mount present"},
		{name: "remote usage available from background sample", warmCache: true},
		{name: "optional inventory unavailable", inventoryFails: true},
		{name: "optional kubelet lookup unavailable", kubeletFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inventoryFails := tc.inventoryFails
			command := func(name, body string) string {
				path := filepath.Join(t.TempDir(), name)
				require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0700))
				return "sh '" + path + "'"
			}
			if inventoryFails {
				t.Setenv("FAIL_MOUNT_INVENTORY", "1")
			} else {
				t.Setenv("FAIL_MOUNT_INVENTORY", "0")
			}
			if tc.kubeletFails {
				t.Setenv("FAIL_KUBELET_LOOKUP", "1")
			} else {
				t.Setenv("FAIL_KUBELET_LOOKUP", "0")
			}
			findmnt := command("findmnt", `
case "$*" in
  '--kernel --list --json --nocanonicalize --nofsroot --output SOURCE,FSTYPE,TARGET')
    [ "$FAIL_MOUNT_INVENTORY" = 0 ] || exit 7
    printf '%s\n' '{"filesystems":[{"source":"server:/export","fstype":"nfs4","target":"/unresponsive"}]}'
    ;;
  '--kernel --list --json --nofsroot --output SOURCE,FSTYPE,TARGET --target /var/lib/kubelet')
    [ "$FAIL_KUBELET_LOOKUP" = 0 ] || exit 7
    printf '%s\n' '{"filesystems":[{"source":"/dev/root","fstype":"ext4","target":"/","size":"1000B","used":"400B","avail":"600B","use%":"40%"}]}'
    ;;
  *) exit 9 ;;
esac
`)
			lsblk := command("lsblk", `
if [ "$*" = '--version' ]; then
  printf '%s\n' 'lsblk from util-linux 2.39'
else
  printf '%s\n' '{"blockdevices":[{"name":"/dev/root","type":"part","size":1000,"mountpoint":"/","fstype":"ext4","fsused":400}]}'
fi
`)
			df := command("df", `
# A global scan would touch the unresponsive remote filesystem.
[ "$*" = '-T -B1 -P -- /' ] || exit 9
printf '%s\n' '/dev/root ext4 1000 400 600 40% /'
`)
			previous := getDiskCommands()
			SetDiskCommands(findmnt, lsblk, df)
			if tc.warmCache {
				disk.PublishNFSUsage(df, disk.Partitions{{Device: "server:/export", MountPoint: "/unresponsive", Fstype: "nfs4", Mounted: true, Usage: &disk.Usage{TotalBytes: 2000, UsedBytes: 1000}}})
				t.Cleanup(func() { disk.PublishNFSUsage(df, nil) })
			}
			t.Cleanup(func() { SetDiskCommands(previous.findmntCommand, previous.lsblkCommand, previous.blockdevUsageCommand) })

			getInfo := func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
				disks, err := GetMachineDiskInfo(context.Background())
				if err != nil {
					return nil, err
				}
				return &apiv1.MachineInfo{
					CPUInfo:    &apiv1.MachineCPUInfo{LogicalCores: 4},
					MemoryInfo: &apiv1.MachineMemoryInfo{TotalBytes: 1024},
					DiskInfo:   disks,
				}, nil
			}
			req, err := createLoginRequest(
				"token", "machine", "group", "0", &mockNvmlInstance{},
				func() (string, error) { return "", nil },
				func() *apiv1.MachineLocation { return nil },
				getInfo,
				func(string) *providers.Info { return &providers.Info{Provider: "unknown", Region: "test"} },
				GetSystemResourceRootVolumeTotal,
				func(nvidianvml.Instance) (string, error) { return "0", nil },
				noDefaultRouteHostIPv4,
			)
			require.NoError(t, err)
			assert.Equal(t, "1k", req.Resources[string(corev1.ResourceEphemeralStorage)])
			gossip, err := createGossipRequest("machine", &mockNvmlInstance{}, getInfo)
			require.NoError(t, err)
			assert.Equal(t, req.MachineInfo.DiskInfo, gossip.MachineInfo.DiskInfo)
			disks := req.MachineInfo.DiskInfo
			if tc.kubeletFails {
				assert.Empty(t, disks.ContainerRootDisk)
			} else {
				assert.Equal(t, "/dev/root", disks.ContainerRootDisk)
			}
			want := 2
			if inventoryFails {
				want = 1
			}
			require.Len(t, disks.BlockDevices, want)
			assert.Equal(t, int64(1000), disks.BlockDevices[0].Size)
			if !inventoryFails {
				assert.Equal(t, "/unresponsive", disks.BlockDevices[1].MountPoint)
				if tc.warmCache {
					assert.Equal(t, int64(2000), disks.BlockDevices[1].Size)
					assert.Equal(t, int64(1000), disks.BlockDevices[1].Used)
				} else {
					assert.Zero(t, disks.BlockDevices[1].Size)
					assert.Zero(t, disks.BlockDevices[1].Used)
				}
			}
		})
	}
}

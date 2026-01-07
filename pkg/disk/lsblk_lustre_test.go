package disk

import (
	"bufio"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLsblkWithAzureLustreEnvironment tests lsblk parsing in an Azure environment
// with Lustre filesystem. This test demonstrates that:
// 1. lsblk correctly parses block devices (NVMe, LVM, SCSI disks)
// 2. Lustre mounts DON'T appear in lsblk (they're network filesystems)
// 3. Lustre detection must happen via /proc/self/mountinfo, not lsblk
//
// Real-world scenario from customer running Azure Managed Lustre File System (AMLFS)
// mounted at /lustre/fs1 with 2PB capacity.
func TestLsblkWithAzureLustreEnvironment(t *testing.T) {
	// Customer's actual lsblk JSON output - note NO Lustre mount present
	// because Lustre is a network filesystem, not a block device
	lsblkJSON := `{
   "blockdevices": [
      {
         "name": "/dev/loop1",
         "type": "loop",
         "size": 66871296,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": null,
         "model": null,
         "rev": null,
         "mountpoint": "/snap/core20/2682",
         "fstype": null,
         "fsused": "66977792",
         "partuuid": null
      },{
         "name": "/dev/loop2",
         "type": "loop",
         "size": 66871296,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": null,
         "model": null,
         "rev": null,
         "mountpoint": "/snap/core20/2686",
         "fstype": null,
         "fsused": "66977792",
         "partuuid": null
      },{
         "name": "/dev/loop3",
         "type": "loop",
         "size": 95842304,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": null,
         "model": null,
         "rev": null,
         "mountpoint": "/snap/lxd/36558",
         "fstype": null,
         "fsused": "95944704",
         "partuuid": null
      },{
         "name": "/dev/loop4",
         "type": "loop",
         "size": 53235712,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": null,
         "model": null,
         "rev": null,
         "mountpoint": "/snap/snapd/25202",
         "fstype": null,
         "fsused": "53346304",
         "partuuid": null
      },{
         "name": "/dev/loop5",
         "type": "loop",
         "size": 53399552,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": null,
         "model": null,
         "rev": null,
         "mountpoint": "/snap/snapd/25577",
         "fstype": null,
         "fsused": "53477376",
         "partuuid": null
      },{
         "name": "/dev/loop6",
         "type": "loop",
         "size": 95842304,
         "rota": true,
         "serial": null,
         "wwn": null,
         "vendor": null,
         "model": null,
         "rev": null,
         "mountpoint": "/snap/lxd/36918",
         "fstype": null,
         "fsused": "95944704",
         "partuuid": null
      },{
         "name": "/dev/sda",
         "type": "disk",
         "size": 274877906944,
         "rota": true,
         "serial": "6002248033fb5393b8cd58ce7390f610",
         "wwn": "0x6002248033fb5393b8cd58ce7390f610",
         "vendor": "Msft    ",
         "model": "Virtual Disk",
         "rev": "1.0 ",
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/sda1",
               "type": "part",
               "size": 274761498112,
               "rota": true,
               "serial": null,
               "wwn": "0x6002248033fb5393b8cd58ce7390f610",
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/",
               "fstype": "ext4",
               "fsused": "18742321152",
               "partuuid": "4d63527a-331e-4b70-a0ac-7e5563fe9724"
            },{
               "name": "/dev/sda14",
               "type": "part",
               "size": 4194304,
               "rota": true,
               "serial": null,
               "wwn": "0x6002248033fb5393b8cd58ce7390f610",
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": null,
               "fstype": null,
               "fsused": null,
               "partuuid": "2c9064ba-ff26-45de-b16f-26dcdf143c71"
            },{
               "name": "/dev/sda15",
               "type": "part",
               "size": 111149056,
               "rota": true,
               "serial": null,
               "wwn": "0x6002248033fb5393b8cd58ce7390f610",
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/boot/efi",
               "fstype": "vfat",
               "fsused": "6342144",
               "partuuid": "2f2feac8-cc2e-4a61-9a88-7dff537ab860"
            }
         ]
      },{
         "name": "/dev/sdb",
         "type": "disk",
         "size": 3113851289600,
         "rota": true,
         "serial": "60022480ce64a9aea4d28c47f7ba8f3a",
         "wwn": "0x60022480ce64a9aea4d28c47f7ba8f3a",
         "vendor": "Msft    ",
         "model": "Virtual Disk",
         "rev": "1.0 ",
         "mountpoint": null,
         "fstype": null,
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/sdb1",
               "type": "part",
               "size": 3113833463808,
               "rota": true,
               "serial": null,
               "wwn": "0x60022480ce64a9aea4d28c47f7ba8f3a",
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/mnt",
               "fstype": "ext4",
               "fsused": "32768",
               "partuuid": "5acddf67-f3b4-4982-a47d-e7917c4ce051"
            }
         ]
      },{
         "name": "/dev/nvme0n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000001",
         "wwn": "eui.36334c31541856680025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme1n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000002",
         "wwn": "eui.36334c31541856180025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme4n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000005",
         "wwn": "eui.36334c31541856380025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme2n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000003",
         "wwn": "eui.36334c31541856370025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme6n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000007",
         "wwn": "eui.36334c31541856200025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme7n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000008",
         "wwn": "eui.36334c31541856460025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme5n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000006",
         "wwn": "eui.36334c31541856390025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      },{
         "name": "/dev/nvme3n1",
         "type": "disk",
         "size": 960197124096,
         "rota": false,
         "serial": "e9924df9a5de00000004",
         "wwn": "eui.36334c31541856570025384100000001",
         "vendor": null,
         "model": "Microsoft NVMe Direct Disk",
         "rev": null,
         "mountpoint": null,
         "fstype": "LVM2_member",
         "fsused": null,
         "partuuid": null,
         "children": [
            {
               "name": "/dev/mapper/lepton_vg-lepton_lv",
               "type": "lvm",
               "size": 7681549008896,
               "rota": false,
               "serial": null,
               "wwn": null,
               "vendor": null,
               "model": null,
               "rev": null,
               "mountpoint": "/lepton-data-disk",
               "fstype": "ext4",
               "fsused": "171840147456",
               "partuuid": null
            }
         ]
      }
   ]
}`

	// Parse the lsblk JSON output
	ctx := context.Background()
	parsed, err := parseLsblkJSON(ctx, []byte(lsblkJSON))
	require.NoError(t, err)
	require.NotNil(t, parsed)

	// Verify block devices are correctly parsed
	// 6 loop devices + 8 NVMe + 2 SCSI = 16 top-level devices
	assert.Len(t, parsed, 16, "should have 16 top-level block devices (6 loops + 2 SCSI + 8 NVMe)")

	// Find specific devices to verify parsing
	var rootDisk, dataDisk *BlockDevice
	var nvmeDisks []*BlockDevice
	var loopDevices []*BlockDevice

	for i := range parsed {
		dev := &parsed[i]
		switch {
		case dev.Name == "/dev/sda":
			rootDisk = dev
		case dev.Name == "/dev/sdb":
			dataDisk = dev
		case strings.HasPrefix(dev.Name, "/dev/nvme"):
			nvmeDisks = append(nvmeDisks, dev)
		case strings.HasPrefix(dev.Name, "/dev/loop"):
			loopDevices = append(loopDevices, dev)
		}
	}

	// Verify root disk (256GB Azure OS disk)
	require.NotNil(t, rootDisk, "should find /dev/sda")
	assert.Equal(t, "disk", rootDisk.Type)
	assert.Equal(t, uint64(274877906944), rootDisk.Size.Uint64) // ~256GB
	assert.Equal(t, "Msft    ", rootDisk.Vendor)
	assert.Equal(t, "Virtual Disk", rootDisk.Model)
	require.Len(t, rootDisk.Children, 3, "root disk should have 3 partitions")

	// Verify root partition
	rootPart := rootDisk.Children[0]
	assert.Equal(t, "/dev/sda1", rootPart.Name)
	assert.Equal(t, "ext4", rootPart.FSType)
	assert.Equal(t, "/", rootPart.MountPoint)

	// Verify EFI partition
	efiPart := rootDisk.Children[2]
	assert.Equal(t, "/dev/sda15", efiPart.Name)
	assert.Equal(t, "vfat", efiPart.FSType)
	assert.Equal(t, "/boot/efi", efiPart.MountPoint)

	// Verify data disk (2.9TB Azure data disk)
	require.NotNil(t, dataDisk, "should find /dev/sdb")
	assert.Equal(t, uint64(3113851289600), dataDisk.Size.Uint64) // ~2.9TB
	require.Len(t, dataDisk.Children, 1)
	assert.Equal(t, "/dev/sdb1", dataDisk.Children[0].Name)
	assert.Equal(t, "ext4", dataDisk.Children[0].FSType)
	assert.Equal(t, "/mnt", dataDisk.Children[0].MountPoint)

	// Verify NVMe drives (8x ~960GB Microsoft NVMe)
	assert.Len(t, nvmeDisks, 8, "should have 8 NVMe drives")
	for _, nvme := range nvmeDisks {
		assert.Equal(t, "disk", nvme.Type)
		assert.Equal(t, "LVM2_member", nvme.FSType)
		assert.Equal(t, uint64(960197124096), nvme.Size.Uint64) // ~960GB
		assert.False(t, nvme.Rota.Bool)                         // NVMe is not rotational
		assert.Equal(t, "Microsoft NVMe Direct Disk", nvme.Model)

		// Each NVMe should have the LVM child
		require.Len(t, nvme.Children, 1)
		lvm := nvme.Children[0]
		assert.Equal(t, "/dev/mapper/lepton_vg-lepton_lv", lvm.Name)
		assert.Equal(t, "lvm", lvm.Type)
		assert.Equal(t, "ext4", lvm.FSType)
		assert.Equal(t, "/lepton-data-disk", lvm.MountPoint)
		assert.Equal(t, uint64(7681549008896), lvm.Size.Uint64) // ~7TB combined LVM
	}

	// Verify loop devices (snap packages)
	assert.Len(t, loopDevices, 6, "should have 6 loop devices for snap")
	for _, loop := range loopDevices {
		assert.Equal(t, "loop", loop.Type)
		assert.True(t, strings.HasPrefix(loop.MountPoint, "/snap/"))
	}

	// CRITICAL: Verify no Lustre mount in lsblk output
	// This demonstrates that Lustre detection must happen via mountinfo
	for _, dev := range parsed {
		assert.NotEqual(t, "lustre", dev.FSType, "Lustre should NOT appear in lsblk output")
		assert.NotContains(t, dev.MountPoint, "/lustre", "Lustre mount point should NOT appear in lsblk")
		for _, child := range dev.Children {
			assert.NotEqual(t, "lustre", child.FSType)
			assert.NotContains(t, child.MountPoint, "/lustre")
		}
	}
}

// TestLustreMountDetectionViaMountinfo demonstrates that Lustre mounts
// must be detected via /proc/self/mountinfo, not lsblk.
func TestLustreMountDetectionViaMountinfo(t *testing.T) {
	// This is what /proc/self/mountinfo looks like for a Lustre mount
	// The customer's df showed: 172.16.0.100@tcp:/lustrefs  2.0P  424T  1.5P  23%  /lustre/fs1
	mountinfoData := `2357 2350 259:1 / / rw,relatime shared:518 master:1 - ext4 /dev/root rw,discard,errors=remount-ro
2434 2357 259:15 / /boot/efi rw,relatime shared:518 master:1 - vfat /dev/sda15 rw,fmask=0077,dmask=0077,codepage=437
2500 2357 0:100 / /lustre/fs1 rw,relatime shared:600 master:1 - lustre 172.16.0.100@tcp:/lustrefs rw,flock,lazystatfs`

	scanner := bufio.NewScanner(strings.NewReader(mountinfoData))

	// Verify Lustre mount is detected via mountinfo
	dev, fsType, err := findMntTargetDevice(scanner, "/lustre/fs1")
	require.NoError(t, err)
	assert.Equal(t, "172.16.0.100@tcp:/lustrefs", dev)
	assert.Equal(t, "lustre", fsType)

	// Verify DefaultNFSFsTypeFunc recognizes Lustre
	assert.True(t, DefaultNFSFsTypeFunc(fsType), "DefaultNFSFsTypeFunc should recognize 'lustre' as a valid shared filesystem type")
}

// TestNFSComponentAcceptsLustre verifies that the NFS component's fstype check
// will now accept Lustre mounts after our fix.
func TestNFSComponentAcceptsLustre(t *testing.T) {
	// These are all the filesystem types that the NFS component should accept
	acceptedFsTypes := []string{
		"nfs",
		"nfs4",
		"wekafs",
		"virtiofs",
		"lustre", // Now supported after our fix
	}

	for _, fsType := range acceptedFsTypes {
		t.Run(fsType, func(t *testing.T) {
			assert.True(t, DefaultNFSFsTypeFunc(fsType),
				"DefaultNFSFsTypeFunc should return true for %q", fsType)
		})
	}

	// These filesystem types should NOT be accepted by NFS component
	rejectedFsTypes := []string{
		"ext4",
		"xfs",
		"btrfs",
		"tmpfs",
		"overlay",
		"",
	}

	for _, fsType := range rejectedFsTypes {
		t.Run("reject_"+fsType, func(t *testing.T) {
			assert.False(t, DefaultNFSFsTypeFunc(fsType),
				"DefaultNFSFsTypeFunc should return false for %q", fsType)
		})
	}
}

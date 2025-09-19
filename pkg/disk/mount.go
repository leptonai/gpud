package disk

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// FindMntTargetDevice returns the device name and file system type of the mount target.
// Implements "findmnt --target [DIRECTORY]".
// It returns an empty string and no error if the target is not found.
func FindMntTargetDevice(dir string) (string, string, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	return findMntTargetDevice(bufio.NewScanner(file), dir)
}

// findMntTargetDevice is a helper function to find the mount target device and its file system type
// for a given target directory.
func findMntTargetDevice(scanner *bufio.Scanner, dir string) (string, string, error) {
	var autofsDev, autofsFsType string
	var foundAutofs bool

	for scanner.Scan() {
		line := scanner.Text()

		fields := strings.Fields(line)

		// "optional fields" (shared, master, propagate_from) can be missing in some systems, so requiring 11+ fields was too strict.
		if len(fields) < 10 {
			continue
		}

		// e.g.,
		// 2914 838 253:0 /var/lib/lxc/ny2g2r14hh2-lxc/rootfs/etc /var/lib/kubelet/pods/545812e1-e899-4d9d-9c5e-ce1a72cd9fa6/volume-subpaths/host-root/gpu-feature-discovery-imex-init/2 rw,relatime shared:518 master:1 - ext4 /dev/mapper/vgroot-lvroot rw
		mountPoint := fields[4] // "/var/lib/lxc/ny2g2r14hh2-lxc/rootfs/etc"
		if !strings.HasPrefix(mountPoint, dir) {
			continue
		}

		splits := strings.Split(line, " - ")
		if len(splits) < 2 {
			continue
		}
		second := splits[1]
		fields = strings.Fields(second)
		if len(fields) < 2 {
			continue
		}

		fsType := fields[0] // "ext4"
		dev := fields[1]    // "/dev/mapper/vgroot-lvroot"

		if strings.Contains(fsType, "overlay") {
			continue
		}
		if strings.Contains(fsType, "tmpfs") {
			continue
		}
		if strings.Contains(fsType, "shm") {
			continue
		}

		// Handle autofs mount stack: autofs creates placeholder mounts that trigger
		// actual filesystem mounts (like NFS) upon access. We need to prioritize the
		// real filesystem over the autofs placeholder.
		//
		// Example mount stack:
		// 2424 2357 0:37 / /mnt/nfs-share rw,relatime - autofs systemd-1 ...
		// 2425 2424 0:163 / /mnt/nfs-share rw,relatime - nfs 172.31.64.223:/nfs-share ...
		//
		// The autofs mount (2424) is the placeholder, but the NFS mount (2425) is the
		// actual active filesystem. We store the autofs mount as a fallback but continue
		// searching for the real filesystem mount.
		if strings.Contains(fsType, "autofs") {
			autofsDev = dev
			autofsFsType = fsType
			foundAutofs = true
			continue
		}

		// Found a non-autofs mount, return it immediately
		return dev, fsType, nil
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	// If we found an autofs mount but no non-autofs mount, return the autofs mount
	if foundAutofs {
		return autofsDev, autofsFsType, nil
	}

	return "", "", nil
}

// FindFsTypeAndDeviceByMinorNumber retrieves the filesystem type and device name for a given minor number.
// If not found, it returns empty strings.
func FindFsTypeAndDeviceByMinorNumber(minor int) (string, string, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	return findFsTypeAndDeviceByMinorNumber(bufio.NewScanner(file), minor)
}

func findFsTypeAndDeviceByMinorNumber(scanner *bufio.Scanner, minor int) (string, string, error) {
	for scanner.Scan() {
		line := scanner.Text()

		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		// e.g.,
		// 1573 899 0:53 / /mnt/remote-volume/dev rw,nosuid,nodev,relatime shared:697 - fuse.testfs TestFS:ws-test-lepton-ai-us-east-dev rw,user_id=0,group_id=0,default_permissions,allow_other
		deviceNumber := fields[2] // "0:53"
		splits := strings.Split(deviceNumber, ":")
		if len(splits) < 2 {
			continue
		}
		minorRaw := splits[1] // "53"
		if minorRaw != fmt.Sprintf("%d", minor) {
			continue
		}

		splits = strings.Split(line, " - ")
		if len(splits) < 2 {
			continue
		}
		second := splits[1]
		fields = strings.Fields(second)
		if len(fields) < 2 {
			continue
		}

		fsType := fields[0] // "fuse.testfs"
		dev := fields[1]    // "TestFS:ws-test-lepton-ai-us-east-dev"

		return fsType, dev, nil
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	return "", "", nil
}

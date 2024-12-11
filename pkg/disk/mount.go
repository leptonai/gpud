package disk

import (
	"bufio"
	"os"
	"strings"
)

// FindMntTargetDevice retrieves mount information for a given target directory.
// Implements "findmnt --target [DIRECTORY]".
// It returns an empty string and no error if the target is not found.
func FindMntTargetDevice(target string) (string, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	defer file.Close()

	return findMntTargetDevice(bufio.NewScanner(file), target)
}

func findMntTargetDevice(scanner *bufio.Scanner, target string) (string, error) {
	for scanner.Scan() {
		line := scanner.Text()

		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		// e.g.,
		// 2914 838 253:0 /var/lib/lxc/ny2g2r14hh2-lxc/rootfs/etc /var/lib/kubelet/pods/545812e1-e899-4d9d-9c5e-ce1a72cd9fa6/volume-subpaths/host-root/gpu-feature-discovery-imex-init/2 rw,relatime shared:518 master:1 - ext4 /dev/mapper/vgroot-lvroot rw
		mountPoint := fields[4]
		fsType := fields[9]
		dev := fields[10]

		if !strings.HasPrefix(mountPoint, target) {
			continue
		}
		if strings.Contains(fsType, "overlay") {
			continue
		}
		if strings.Contains(fsType, "tmpfs") {
			continue
		}
		if strings.Contains(fsType, "shm") {
			continue
		}

		return dev, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

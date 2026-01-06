package disk

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func Test_findMntTargetDevice(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	buf := bufio.NewScanner(f)

	mountPoint, fsType, err := findMntTargetDevice(buf, "/var/lib/kubelet")
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if mountPoint != "/dev/mapper/vgroot-lvroot" {
		t.Fatalf("expected mount point: %s, got: %s", "/dev/mapper/vgroot-lvroot", mountPoint)
	}
	if fsType != "ext4" {
		t.Fatalf("expected fsType ext4, got: %s", fsType)
	}
}

func Test_findFsTypeAndDeviceByMinorNumber1(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	buf := bufio.NewScanner(f)

	fsType, dev, err := findFsTypeAndDeviceByMinorNumber(buf, 81)
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if fsType != "fuse.testfs" {
		t.Fatalf("expected fsType: %s, got: %s", "fuse.testfs", fsType)
	}
	if dev != "TestFS:test-lepton-ai-us-east-dev" {
		t.Fatalf("expected dev: %s, got: %s", "TestFS:test-lepton-ai-us-east-dev", dev)
	}
}

func Test_findFsTypeAndDeviceByMinorNumber2(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	buf := bufio.NewScanner(f)

	fsType, dev, err := findFsTypeAndDeviceByMinorNumber(buf, 550)
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if fsType != "fuse.testfs" {
		t.Fatalf("expected fsType: %s, got: %s", "fuse.testfs", fsType)
	}
	if dev != "TestFS:ws-test-us-east-training" {
		t.Fatalf("expected dev: %s, got: %s", "TestFS:ws-test-us-east-training", dev)
	}
}

// Test for FindMntTargetDevice public function
func TestFindMntTargetDevice(t *testing.T) {
	// This test will use the actual /proc/self/mountinfo if it exists
	// We'll skip the test if not running on Linux
	if _, err := os.Stat("/proc/self/mountinfo"); os.IsNotExist(err) {
		t.Skip("Skipping test: /proc/self/mountinfo not available")
	}

	// Test with a common mount point that should exist on most Linux systems
	dev, fsType, err := FindMntTargetDevice("/")
	if err != nil {
		t.Fatalf("FindMntTargetDevice failed: %v", err)
	}

	// Root should have some device and filesystem type
	if dev == "" && fsType == "" {
		t.Log("No device found for root mount point (may be expected in some environments)")
	}
}

// Test for FindFsTypeAndDeviceByMinorNumber public function
func TestFindFsTypeAndDeviceByMinorNumber(t *testing.T) {
	// This test will use the actual /proc/self/mountinfo if it exists
	// We'll skip the test if not running on Linux
	if _, err := os.Stat("/proc/self/mountinfo"); os.IsNotExist(err) {
		t.Skip("Skipping test: /proc/self/mountinfo not available")
	}

	// Test with minor number that likely doesn't exist
	fsType, dev, err := FindFsTypeAndDeviceByMinorNumber(999999)
	if err != nil {
		t.Fatalf("FindFsTypeAndDeviceByMinorNumber failed: %v", err)
	}

	// Should return empty strings for non-existent minor number
	if fsType != "" || dev != "" {
		t.Errorf("Expected empty strings for non-existent minor number, got fsType=%s, dev=%s", fsType, dev)
	}
}

// Test edge cases for findMntTargetDevice
func Test_findMntTargetDevice_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		mountinfoData  string
		targetDir      string
		expectedDev    string
		expectedFsType string
		expectError    bool
	}{
		{
			name:           "empty mountinfo",
			mountinfoData:  "",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "malformed line - too few fields",
			mountinfoData:  "1 2 3 4 5",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "no matching mount point",
			mountinfoData:  "1 2 3 4 /other 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "overlay filesystem should be skipped",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - overlay overlay rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "tmpfs should be skipped",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - tmpfs tmpfs rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "shm should be skipped",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - shm /dev/shm rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "valid mount point match",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			targetDir:      "/test",
			expectedDev:    "/dev/sda1",
			expectedFsType: "ext4",
			expectError:    false,
		},
		{
			name:           "prefix match for subdirectory",
			mountinfoData:  "1 2 3 4 /test/subdir 6 7 8 9 10 11 - xfs /dev/sdb1 rw",
			targetDir:      "/test",
			expectedDev:    "/dev/sdb1",
			expectedFsType: "xfs",
			expectError:    false,
		},
		{
			name: "autofs with nfs mount - should return nfs",
			mountinfoData: `2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469
2425 2424 0:163 / /mnt/nfs-share rw,relatime shared:519 master:1 - nfs 172.31.64.223:/nfs-share rw,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.31.64.223,mountvers=3,mountport=32767,mountproto=tcp,local_lock=none,addr=172.31.64.223`,
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "172.31.64.223:/nfs-share",
			expectedFsType: "nfs",
			expectError:    false,
		},
		{
			name:           "autofs only - should return autofs",
			mountinfoData:  "2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469",
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "systemd-1",
			expectedFsType: "autofs",
			expectError:    false,
		},
		{
			name:           "nfs only - should return nfs",
			mountinfoData:  "2425 2424 0:163 / /mnt/nfs-share rw,relatime shared:519 master:1 - nfs 172.31.64.223:/nfs-share rw,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.31.64.223,mountvers=3,mountport=32767,mountproto=tcp,local_lock=none,addr=172.31.64.223",
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "172.31.64.223:/nfs-share",
			expectedFsType: "nfs",
			expectError:    false,
		},
		{
			name: "autofs with multiple non-autofs mounts - should return first non-autofs",
			mountinfoData: `2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469
2425 2424 0:163 / /mnt/nfs-share rw,relatime shared:519 master:1 - nfs 172.31.64.223:/nfs-share rw,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.31.64.223,mountvers=3,mountport=32767,mountproto=tcp,local_lock=none,addr=172.31.64.223
2426 2424 0:164 / /mnt/nfs-share rw,relatime shared:520 master:1 - ext4 /dev/sda1 rw`,
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "172.31.64.223:/nfs-share",
			expectedFsType: "nfs",
			expectError:    false,
		},
		{
			name: "nfs mount before autofs - should return nfs (order independence)",
			mountinfoData: `2425 2424 0:163 / /mnt/nfs-share rw,relatime shared:519 master:1 - nfs 172.31.64.223:/nfs-share rw,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.31.64.223,mountvers=3,mountport=32767,mountproto=tcp,local_lock=none,addr=172.31.64.223
2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469`,
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "172.31.64.223:/nfs-share",
			expectedFsType: "nfs",
			expectError:    false,
		},
		{
			name: "ext4 mount before autofs - should return ext4 (order independence)",
			mountinfoData: `2426 2424 0:164 / /mnt/nfs-share rw,relatime shared:520 master:1 - ext4 /dev/sda1 rw
2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469`,
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "/dev/sda1",
			expectedFsType: "ext4",
			expectError:    false,
		},
		{
			name: "multiple non-autofs mounts before autofs - should return first non-autofs",
			mountinfoData: `2425 2424 0:163 / /mnt/nfs-share rw,relatime shared:519 master:1 - nfs 172.31.64.223:/nfs-share rw,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.31.64.223,mountvers=3,mountport=32767,mountproto=tcp,local_lock=none,addr=172.31.64.223
2426 2424 0:164 / /mnt/nfs-share rw,relatime shared:520 master:1 - ext4 /dev/sda1 rw
2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469`,
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "172.31.64.223:/nfs-share",
			expectedFsType: "nfs",
			expectError:    false,
		},
		{
			name: "autofs between non-autofs mounts - should return first non-autofs",
			mountinfoData: `2425 2424 0:163 / /mnt/nfs-share rw,relatime shared:519 master:1 - nfs 172.31.64.223:/nfs-share rw,vers=3,rsize=1048576,wsize=1048576,namlen=255,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.31.64.223,mountvers=3,mountport=32767,mountproto=tcp,local_lock=none,addr=172.31.64.223
2424 2357 0:37 / /mnt/nfs-share rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469
2426 2424 0:164 / /mnt/nfs-share rw,relatime shared:520 master:1 - ext4 /dev/sda1 rw`,
			targetDir:      "/mnt/nfs-share",
			expectedDev:    "172.31.64.223:/nfs-share",
			expectedFsType: "nfs",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - root filesystem",
			mountinfoData:  "2357 2350 259:1 / / rw,relatime shared:518 master:1 - ext4 /dev/root rw,discard,errors=remount-ro",
			targetDir:      "/",
			expectedDev:    "/dev/root",
			expectedFsType: "ext4",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - dev filesystem should be skipped (contains tmpfs)",
			mountinfoData:  "2358 2357 0:5 / /dev rw,nosuid,noexec,relatime shared:518 master:1 - devtmpfs devtmpfs rw,size=129253052k,nr_inodes=32313263,mode=755,inode64",
			targetDir:      "/dev",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - proc filesystem",
			mountinfoData:  "2363 2357 0:23 / /proc rw,nosuid,nodev,noexec,relatime shared:518 master:1 - proc proc rw",
			targetDir:      "/proc",
			expectedDev:    "proc",
			expectedFsType: "proc",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - sys filesystem",
			mountinfoData:  "2366 2357 0:24 / /sys rw,nosuid,nodev,noexec,relatime shared:518 master:1 - sysfs sysfs rw",
			targetDir:      "/sys",
			expectedDev:    "sysfs",
			expectedFsType: "sysfs",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - boot efi vfat",
			mountinfoData:  "2434 2357 259:3 / /boot/efi rw,relatime shared:518 master:1 - vfat /dev/nvme0n1p15 rw,fmask=0077,dmask=0077,codepage=437,iocharset=iso8859-1,shortname=mixed,errors=remount-ro",
			targetDir:      "/boot/efi",
			expectedDev:    "/dev/nvme0n1p15",
			expectedFsType: "vfat",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - snap squashfs",
			mountinfoData:  "2426 2357 7:0 / /snap/amazon-ssm-agent/11797 ro,nodev,relatime shared:518 master:1 - squashfs /dev/loop0 ro,errors=continue,threads=single",
			targetDir:      "/snap/amazon-ssm-agent/11797",
			expectedDev:    "/dev/loop0",
			expectedFsType: "squashfs",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - overlay filesystem should be skipped",
			mountinfoData:  "2387 2376 0:67 / /run/containerd/io.containerd.runtime.v2.task/k8s.io/6c277d3d569cb86e866293d335dc67793da8779bb6d46b3a92d830830481c187/rootfs rw,relatime shared:518 master:1 - overlay overlay rw,lowerdir=/var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots/1/fs,upperdir=/var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots/786/fs,workdir=/var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots/786/work,uuid=on,nouserxattr",
			targetDir:      "/run/containerd/io.containerd.runtime.v2.task/k8s.io/6c277d3d569cb86e866293d335dc67793da8779bb6d46b3a92d830830481c187/rootfs",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - tmpfs should be skipped",
			mountinfoData:  "2359 2358 0:25 / /dev/shm rw,nosuid,nodev shared:518 master:1 - tmpfs tmpfs rw,inode64",
			targetDir:      "/dev/shm",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - cgroup2 filesystem",
			mountinfoData:  "2368 2366 0:29 / /sys/fs/cgroup rw,nosuid,nodev,noexec,relatime shared:518 master:1 - cgroup2 cgroup2 rw",
			targetDir:      "/sys/fs/cgroup",
			expectedDev:    "cgroup2",
			expectedFsType: "cgroup2",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - bpf filesystem",
			mountinfoData:  "2371 2366 0:32 / /sys/fs/bpf rw,nosuid,nodev,noexec,relatime shared:518 master:1 - bpf bpf rw,mode=700",
			targetDir:      "/sys/fs/bpf",
			expectedDev:    "bpf",
			expectedFsType: "bpf",
			expectError:    false,
		},
		{
			name: "real world mountinfo - autofs with binfmt_misc - should return binfmt_misc",
			mountinfoData: `2364 2363 0:33 / /proc/sys/fs/binfmt_misc rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=29,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=8201
2365 2364 0:38 / /proc/sys/fs/binfmt_misc rw,nosuid,nodev,noexec,relatime shared:519 master:1 - binfmt_misc binfmt_misc rw`,
			targetDir:      "/proc/sys/fs/binfmt_misc",
			expectedDev:    "binfmt_misc",
			expectedFsType: "binfmt_misc",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - autofs only for binfmt_misc",
			mountinfoData:  "2364 2363 0:33 / /proc/sys/fs/binfmt_misc rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=29,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=8201",
			targetDir:      "/proc/sys/fs/binfmt_misc",
			expectedDev:    "systemd-1",
			expectedFsType: "autofs",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - kubelet pod volume",
			mountinfoData:  "2435 2357 0:50 / /var/lib/kubelet/pods/e1f98068-83ea-4926-934b-f76250f4f7fe/volumes/kubernetes.io~secret/node-certs rw,relatime shared:518 master:1 - tmpfs tmpfs rw,size=255977716k,inode64,noswap",
			targetDir:      "/var/lib/kubelet/pods/e1f98068-83ea-4926-934b-f76250f4f7fe/volumes/kubernetes.io~secret/node-certs",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "real world mountinfo - containerd rootfs with proc",
			mountinfoData:  "2418 2417 0:245 / /run/containerd/io.containerd.runtime.v2.task/k8s.io/2d5c5196d0028473127d1a0f29fcf7fa8ad528a09584f4cc1f9edcadd98c3663/rootfs/proc rw,nosuid,nodev,noexec,relatime shared:518 master:1 - proc proc rw",
			targetDir:      "/run/containerd/io.containerd.runtime.v2.task/k8s.io/2d5c5196d0028473127d1a0f29fcf7fa8ad528a09584f4cc1f9edcadd98c3663/rootfs/proc",
			expectedDev:    "proc",
			expectedFsType: "proc",
			expectError:    false,
		},
		{
			name:           "line without separator",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 ext4 /dev/sda1 rw",
			targetDir:      "/test",
			expectedDev:    "",
			expectedFsType: "",
			expectError:    false,
		},
		{
			name:           "trailing slash in target dir - should match mount point without slash",
			mountinfoData:  "1 2 3 4 /test 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			targetDir:      "/test/",
			expectedDev:    "/dev/sda1",
			expectedFsType: "ext4",
			expectError:    false,
		},
		// Lustre filesystem tests (Azure Managed Lustre, AWS FSx for Lustre, etc.)
		{
			name:           "lustre mount - Azure Managed Lustre (AMLFS)",
			mountinfoData:  "2500 2357 0:100 / /lustre/fs1 rw,relatime shared:518 master:1 - lustre 172.16.0.100@tcp:/lustrefs rw,flock,lazystatfs",
			targetDir:      "/lustre/fs1",
			expectedDev:    "172.16.0.100@tcp:/lustrefs",
			expectedFsType: "lustre",
			expectError:    false,
		},
		{
			name:           "lustre mount - AWS FSx for Lustre",
			mountinfoData:  "2501 2357 0:101 / /fsx/data rw,relatime shared:519 master:1 - lustre fs-0123456789abcdef0.fsx.us-east-1.amazonaws.com@tcp:/fsx rw,flock,lazystatfs",
			targetDir:      "/fsx/data",
			expectedDev:    "fs-0123456789abcdef0.fsx.us-east-1.amazonaws.com@tcp:/fsx",
			expectedFsType: "lustre",
			expectError:    false,
		},
		{
			name:           "lustre mount - nested mount point under search dir",
			mountinfoData:  "2502 2357 0:102 / /lustre/fs1/shared/project rw,relatime shared:520 master:1 - lustre 172.16.0.100@tcp:/lustrefs rw,flock,lazystatfs",
			targetDir:      "/lustre/fs1",
			expectedDev:    "172.16.0.100@tcp:/lustrefs",
			expectedFsType: "lustre",
			expectError:    false,
		},
		{
			name: "autofs with lustre mount - should return lustre",
			mountinfoData: `2503 2357 0:37 / /lustre/fs1 rw,relatime shared:518 master:1 - autofs systemd-1 rw,fd=62,pgrp=1,timeout=0,minproto=5,maxproto=5,direct,pipe_ino=18469
2504 2503 0:103 / /lustre/fs1 rw,relatime shared:519 master:1 - lustre 172.16.0.100@tcp:/lustrefs rw,flock,lazystatfs`,
			targetDir:      "/lustre/fs1",
			expectedDev:    "172.16.0.100@tcp:/lustrefs",
			expectedFsType: "lustre",
			expectError:    false,
		},
		{
			name:           "lustre mount with multiple tcp connections",
			mountinfoData:  "2505 2357 0:104 / /lustre/shared rw,relatime shared:521 master:1 - lustre 10.0.0.10@tcp1,10.0.0.11@tcp1:/shared rw,flock,lazystatfs",
			targetDir:      "/lustre/shared",
			expectedDev:    "10.0.0.10@tcp1,10.0.0.11@tcp1:/shared",
			expectedFsType: "lustre",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.mountinfoData))
			dev, fsType, err := findMntTargetDevice(scanner, tt.targetDir)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if dev != tt.expectedDev {
				t.Errorf("Expected dev=%s, got=%s", tt.expectedDev, dev)
			}
			if fsType != tt.expectedFsType {
				t.Errorf("Expected fsType=%s, got=%s", tt.expectedFsType, fsType)
			}
		})
	}
}

// Test edge cases for findFsTypeAndDeviceByMinorNumber
func Test_findFsTypeAndDeviceByMinorNumber_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		mountinfoData  string
		minor          int
		expectedFsType string
		expectedDev    string
		expectError    bool
	}{
		{
			name:           "empty mountinfo",
			mountinfoData:  "",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "malformed line - too few fields",
			mountinfoData:  "1 2",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "no device number field",
			mountinfoData:  "1 2 : 4 5 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "matching minor number",
			mountinfoData:  "1 2 8:10 4 5 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "ext4",
			expectedDev:    "/dev/sda1",
			expectError:    false,
		},
		{
			name:           "non-matching minor number",
			mountinfoData:  "1 2 8:20 4 5 6 7 8 9 10 11 - ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "line without separator",
			mountinfoData:  "1 2 8:10 4 5 6 7 8 9 10 11 ext4 /dev/sda1 rw",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
		{
			name:           "separator but too few fields after",
			mountinfoData:  "1 2 8:10 4 5 6 7 8 9 10 11 - ext4",
			minor:          10,
			expectedFsType: "",
			expectedDev:    "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.mountinfoData))
			fsType, dev, err := findFsTypeAndDeviceByMinorNumber(scanner, tt.minor)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if fsType != tt.expectedFsType {
				t.Errorf("Expected fsType=%s, got=%s", tt.expectedFsType, fsType)
			}
			if dev != tt.expectedDev {
				t.Errorf("Expected dev=%s, got=%s", tt.expectedDev, dev)
			}
		})
	}
}

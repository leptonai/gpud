package disk

import (
	"bufio"
	"os"
	"testing"
)

func Test_findMntTargetDevice(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer f.Close()

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
	defer f.Close()

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
	defer f.Close()

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

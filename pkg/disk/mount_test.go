package disk

import (
	"bufio"
	"os"
	"testing"
)

func TestParseMountInfoLine(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatalf("failed to open testdata/mountinfo: %v", err)
	}
	defer f.Close()

	buf := bufio.NewScanner(f)

	mountPoint, err := findMntTargetDevice(buf, "/var/lib/kubelet")
	if err != nil {
		t.Fatalf("failed to find mount point: %v", err)
	}
	if mountPoint != "/dev/mapper/vgroot-lvroot" {
		t.Fatalf("expected mount point: %s, got: %s", "/dev/mapper/vgroot-lvroot", mountPoint)
	}
}

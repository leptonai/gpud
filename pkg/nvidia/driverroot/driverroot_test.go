package driverroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingFrom(t *testing.T) {
	tmp := t.TempDir()
	existing := filepath.Join(tmp, "host")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "missing")
	fileNotDir := filepath.Join(tmp, "file")
	if err := os.WriteFile(fileNotDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ExistingFrom(existing, missing, fileNotDir)
	if len(got) != 1 || got[0] != existing {
		t.Fatalf("Expected only the existing directory root, got %v", got)
	}

	if got := ExistingFrom(missing, fileNotDir); len(got) != 0 {
		t.Fatalf("Expected no roots, got %v", got)
	}
}

func TestExistingOrder(t *testing.T) {
	// host root must win over the operator root, mirroring the GPU Operator's
	// own driver-validation order (pkg/nvidia/nvml/lib uses the same order)
	got := Existing()
	for i := 1; i < len(got); i++ {
		if got[i] == HostRoot {
			t.Fatalf("HostRoot must sort before OperatorRoot, got %v", got)
		}
	}
}

func TestLibraryDirs(t *testing.T) {
	dirs := LibraryDirs("/host")
	expected := []string{
		"/host/usr/lib/x86_64-linux-gnu",
		"/host/usr/lib/aarch64-linux-gnu",
		"/host/usr/lib64",
		"/host/usr/lib",
	}
	if len(dirs) != len(expected) {
		t.Fatalf("Expected %d dirs, got %v", len(expected), dirs)
	}
	for i, dir := range expected {
		if dirs[i] != dir {
			t.Fatalf("Expected dir %q at index %d, got %q", dir, i, dirs[i])
		}
	}
}

func TestBinDirs(t *testing.T) {
	dirs := BinDirs("/run/nvidia/driver")
	expected := []string{
		"/run/nvidia/driver/usr/bin",
		"/run/nvidia/driver/usr/sbin",
		"/run/nvidia/driver/bin",
		"/run/nvidia/driver/sbin",
	}
	if len(dirs) != len(expected) {
		t.Fatalf("Expected %d dirs, got %v", len(expected), dirs)
	}
	for i, dir := range expected {
		if dirs[i] != dir {
			t.Fatalf("Expected dir %q at index %d, got %q", dir, i, dirs[i])
		}
	}
}

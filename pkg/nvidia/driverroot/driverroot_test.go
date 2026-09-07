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

func TestContractRoots(t *testing.T) {
	// Missing contract file.
	if got := ContractRoots(t.TempDir()); len(got) != 0 {
		t.Fatalf("Expected no roots without a contract, got %v", got)
	}

	hostRoot := t.TempDir()
	writeContract := func(contents string) {
		t.Helper()
		contractPath := filepath.Join(hostRoot, DriverReadyContractPath)
		if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contractPath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The host-driver selection ("/") is covered by the standard host-root
	// probe, so it contributes no contract root.
	writeContract("NVIDIA_DRIVER_ROOT=/\n")
	if got := ContractRoots(hostRoot); len(got) != 0 {
		t.Fatalf("Expected no roots for the host-driver selection, got %v", got)
	}

	// Relative paths are rejected.
	writeContract("NVIDIA_DRIVER_ROOT=run/nvidia/driver\n")
	if got := ContractRoots(hostRoot); len(got) != 0 {
		t.Fatalf("Expected no roots for a relative path, got %v", got)
	}

	// A quoted custom install directory resolves under the host root.
	writeContract("NVIDIA_DRIVER_ROOT=\"/opt/nvidia/driver\"\n")
	got := ContractRoots(hostRoot)
	if len(got) != 1 || got[0] != filepath.Join(hostRoot, "opt", "nvidia", "driver") {
		t.Fatalf("Expected the custom install directory under the host root, got %v", got)
	}

	// Multiple selections are returned in file order.
	writeContract("NVIDIA_DRIVER_ROOT=/opt/nvidia/driver\nDRIVER_ROOT_CTR_PATH=/driver-root\nNVIDIA_DRIVER_ROOT=/alt/driver\n")
	got = ContractRoots(hostRoot)
	if len(got) != 2 ||
		got[0] != filepath.Join(hostRoot, "opt", "nvidia", "driver") ||
		got[1] != filepath.Join(hostRoot, "alt", "driver") {
		t.Fatalf("Expected both contract roots in file order, got %v", got)
	}
}

func TestCandidatesOrder(t *testing.T) {
	hostRoot := t.TempDir()
	operatorRoot := t.TempDir()

	// Without a contract: host root, then operator root.
	got := Candidates(hostRoot, operatorRoot)
	if len(got) != 2 || got[0] != hostRoot || got[1] != operatorRoot {
		t.Fatalf("Expected [host operator], got %v", got)
	}

	// A contract-selected install directory sorts between the two.
	contractPath := filepath.Join(hostRoot, DriverReadyContractPath)
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, []byte("NVIDIA_DRIVER_ROOT=/opt/nvidia/driver\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = Candidates(hostRoot, operatorRoot)
	if len(got) != 3 ||
		got[0] != hostRoot ||
		got[1] != filepath.Join(hostRoot, "opt", "nvidia", "driver") ||
		got[2] != operatorRoot {
		t.Fatalf("Expected [host contract operator], got %v", got)
	}
}

func TestExistingWithContract(t *testing.T) {
	hostRoot := t.TempDir()
	operatorRoot := t.TempDir()

	// The contract-selected root only surfaces in Existing once it exists.
	contractPath := filepath.Join(hostRoot, DriverReadyContractPath)
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contractPath, []byte("NVIDIA_DRIVER_ROOT=/opt/nvidia/driver\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ExistingFrom(Candidates(hostRoot, operatorRoot)...); len(got) != 2 {
		t.Fatalf("Expected contract root to be existence-filtered, got %v", got)
	}

	contractRoot := filepath.Join(hostRoot, "opt", "nvidia", "driver")
	if err := os.MkdirAll(contractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	got := ExistingFrom(Candidates(hostRoot, operatorRoot)...)
	if len(got) != 3 || got[1] != contractRoot {
		t.Fatalf("Expected the existing contract root in probe order, got %v", got)
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

// Package driverroot locates the well-known NVIDIA driver installation roots
// that gpud probes when it runs inside a container: the host root filesystem
// mount (exposing a pre-installed host driver) and the GPU Operator driver
// tree. All probes are existence-based, so on bare-metal/systemd
// installations (where neither path exists) the helpers return empty results
// and callers keep their standard system-path behavior.
//
// Why this package exists (LEP-6440): the NVML loader (pkg/nvidia/nvml/lib)
// already probed these roots, but other components hardcoded divergent path
// lists and produced false unhealthy reports on GPU Operator-managed nodes
// (library: "libcuda.so does not exist" while the library was present under
// /run/nvidia/driver/usr/lib; fabric-manager: "executable not found" while
// /run/nvidia/driver/usr/bin/nv-fabricmanager existed -- both verified on
// aws-iad-nkxdev-1, GPU Operator 26.3.2). Host root sorts before the Operator
// root to mirror the Operator's own driver-validation order: a pre-installed
// host driver wins because its userspace libraries are guaranteed to match
// the loaded kernel module.
//
// The candidate list also honors the GPU Operator's driver-ready contract
// (DriverReadyContractPath): when the Operator validates a non-default driver
// install directory (spec.hostPaths.driverInstallDir, e.g. /opt/nvidia/driver),
// it records the selected NVIDIA_DRIVER_ROOT there, and this package resolves
// it under the host root -- the same discovery the NVML loader performs, so
// every consumer sees the same driver tree NVML loaded from.
package driverroot

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// HostRoot is the well-known container path where the gpud DaemonSet
	// mounts the host root filesystem read-only (helm value
	// gpud.mountHostRoot), exposing a pre-installed host driver's files
	// under "<root>/usr/...".
	HostRoot = "/host"

	// OperatorRoot is the well-known container path where the gpud DaemonSet
	// mounts the GPU Operator's driver installation tree (helm value
	// gpud.mountNVIDIADriverRoot).
	OperatorRoot = "/run/nvidia/driver"

	// DriverReadyContractPath is the GPU Operator's driver validation
	// contract, relative to the host root. The Operator's driver-validation
	// step records the selected NVIDIA_DRIVER_ROOT here after the driver
	// validates, so it reflects a non-default driver install directory
	// (spec.hostPaths.driverInstallDir).
	DriverReadyContractPath = "run/nvidia/validations/driver-ready"
)

// Existing returns the well-known driver roots that currently exist,
// in probe order: the host root first (a pre-installed host driver wins,
// mirroring the GPU Operator's own driver-validation order), then any
// contract-selected driver root, then the GPU Operator driver root.
func Existing() []string {
	return ExistingFrom(Candidates(HostRoot, OperatorRoot)...)
}

// Candidates returns the ordered driver-root candidates to probe: the host
// root first (a pre-installed host driver wins, mirroring the GPU Operator's
// own driver-validation order), then any NVIDIA_DRIVER_ROOT selected by the
// Operator's driver-ready contract (resolved under the host root), then the
// GPU Operator driver root. The list is unfiltered; use ExistingFrom to
// reduce it to the roots that exist.
func Candidates(hostRoot, operatorRoot string) []string {
	roots := []string{hostRoot}
	roots = append(roots, ContractRoots(hostRoot)...)
	return append(roots, operatorRoot)
}

// ContractRoots returns the driver roots selected by the GPU Operator's
// driver-ready contract under the given host root, resolved under the same
// host root. It returns nil when the contract is absent, unparseable, or
// names a non-absolute path, and skips the host-driver selection ("/",
// already covered by the standard host-root probes). The contract's
// DRIVER_ROOT_CTR_PATH is the DRA container's own mount path and does not
// apply to gpud's mount layout.
func ContractRoots(hostRoot string) []string {
	b, err := os.ReadFile(filepath.Join(hostRoot, DriverReadyContractPath))
	if err != nil {
		return nil
	}
	roots := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "NVIDIA_DRIVER_ROOT=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if value == "" || value == "/" || !filepath.IsAbs(value) {
			continue
		}
		roots = append(roots, filepath.Join(hostRoot, value))
	}
	return roots
}

// ExistingFrom returns the subset of the given roots that exist as
// directories, preserving the given order. It takes the roots as parameters
// (rather than reading the package constants) so tests can point the probes
// at temporary directories.
func ExistingFrom(roots ...string) []string {
	existing := []string{}
	for _, root := range roots {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			existing = append(existing, root)
		}
	}
	return existing
}

// LibraryDirs returns the NVIDIA userspace library directories under the
// given roots, covering the Debian/Ubuntu multiarch, RHEL lib64, and plain
// usr/lib layouts.
func LibraryDirs(roots ...string) []string {
	dirs := []string{}
	for _, root := range roots {
		dirs = append(dirs,
			filepath.Join(root, "usr", "lib", "x86_64-linux-gnu"),
			filepath.Join(root, "usr", "lib", "aarch64-linux-gnu"),
			filepath.Join(root, "usr", "lib64"),
			filepath.Join(root, "usr", "lib"),
		)
	}
	return dirs
}

// BinDirs returns the executable directories under the given roots.
func BinDirs(roots ...string) []string {
	dirs := []string{}
	for _, root := range roots {
		dirs = append(dirs,
			filepath.Join(root, "usr", "bin"),
			filepath.Join(root, "usr", "sbin"),
			filepath.Join(root, "bin"),
			filepath.Join(root, "sbin"),
		)
	}
	return dirs
}

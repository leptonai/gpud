// Package driverroot locates the well-known NVIDIA driver installation roots
// that gpud probes when it runs inside a container: the host root filesystem
// mount (exposing a pre-installed host driver) and the GPU Operator driver
// tree. All probes are existence-based, so on bare-metal/systemd
// installations (where neither path exists) the helpers return empty results
// and callers keep their standard system-path behavior.
package driverroot

import (
	"os"
	"path/filepath"
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
)

// Existing returns the well-known driver roots that currently exist,
// in probe order: the host root first (a pre-installed host driver wins,
// mirroring the GPU Operator's own driver-validation order), then the GPU
// Operator driver root.
func Existing() []string {
	return ExistingFrom(HostRoot, OperatorRoot)
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

package lib

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const (
	// EnvNVMLLibraryPath explicitly selects the NVML shared library.
	EnvNVMLLibraryPath = "GPUD_NVML_LIBRARY_PATH"
	// EnvNVIDIADriverRoot points at a containerized NVIDIA driver installation,
	// such as the GPU Operator's /run/nvidia/driver tree.
	EnvNVIDIADriverRoot = "GPUD_NVIDIA_DRIVER_ROOT"
	// EnvNVIDIAHostRoot points at the container path where the host root
	// filesystem is mounted read-only (e.g., "/host"), exposing a
	// pre-installed host driver's libraries under "<root>/usr/lib...".
	// Mirroring the GPU Operator's own driver-validation order, a
	// pre-installed host driver takes precedence over the
	// EnvNVIDIADriverRoot tree when both are present. The host root also
	// exposes the Operator's driver-ready validation contract, which gpud
	// uses to discover a non-default driver install directory.
	EnvNVIDIAHostRoot = "GPUD_NVIDIA_HOST_ROOT"
)

type Op struct {
	nvmlLib nvml.Interface

	initReturn        *nvml.Return
	propertyExtractor nvinfo.PropertyExtractor
	devicesToReturn   []nvlibdevice.Device

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
	devGetRemappedRowsForAllDevs func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	devGetCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)

	// devGetDevicesError is the error to return from Device().GetDevices().
	// Used for testing NVML device enumeration failure scenarios.
	devGetDevicesError error
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
	if op.nvmlLib == nil {
		libraryPath := resolveNVMLLibraryPath()
		if libraryPath == "" {
			op.nvmlLib = nvml.New()
		} else {
			op.nvmlLib = nvml.New(nvml.WithLibraryPath(libraryPath))
		}
	}
}

// resolveNVMLLibraryPath returns an explicitly configured library first, then
// searches a configured host root and driver root. With no environment
// variable set it returns empty, preserving go-nvml's standard system-library
// lookup.
func resolveNVMLLibraryPath() string {
	if libraryPath := os.Getenv(EnvNVMLLibraryPath); libraryPath != "" {
		return libraryPath
	}

	// A pre-installed host driver takes precedence over a GPU
	// Operator-managed driver root when both are present, mirroring the
	// Operator's own driver-validation order (pre-installed wins when both
	// validate). Loading the host's own libnvidia-ml guarantees the
	// userspace library matches the loaded kernel module.
	if hostRoot := os.Getenv(EnvNVIDIAHostRoot); hostRoot != "" {
		if libraryPath := probeNVMLLibrary(hostRoot); libraryPath != "" {
			return libraryPath
		}

		// The GPU Operator records its validated driver root in the
		// driver-ready contract; honor it to cover a non-default driver
		// install directory (spec.hostPaths.driverInstallDir).
		if libraryPath := probeDriverReadyContract(hostRoot); libraryPath != "" {
			return libraryPath
		}
	}

	if driverRoot := os.Getenv(EnvNVIDIADriverRoot); driverRoot != "" {
		return probeNVMLLibrary(driverRoot)
	}

	return ""
}

// probeNVMLLibrary returns the first existing NVML shared library under the
// given root, covering the Debian/Ubuntu multiarch, RHEL lib64, and plain
// usr/lib layouts. It returns empty when the root has no NVML library (e.g.,
// the GPU Operator has not finished installing the driver yet).
func probeNVMLLibrary(root string) string {
	archLibraryDir := nvmlArchLibraryDir(runtime.GOARCH)
	candidates := []string{
		filepath.Join(root, "usr", "lib", archLibraryDir, "libnvidia-ml.so.1"),
		filepath.Join(root, "usr", "lib64", "libnvidia-ml.so.1"),
		filepath.Join(root, "usr", "lib", "libnvidia-ml.so.1"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// driverReadyContractPath is the GPU Operator's driver validation contract,
// relative to the host root. The Operator's driver-validation step records
// the selected NVIDIA_DRIVER_ROOT here after the driver validates, so it
// reflects a non-default driver install directory
// (spec.hostPaths.driverInstallDir).
const driverReadyContractPath = "run/nvidia/validations/driver-ready"

// probeDriverReadyContract reads the GPU Operator's driver-ready contract
// under the given host root and probes the NVIDIA_DRIVER_ROOT it selects,
// resolved under the same host root. It returns empty when the contract is
// absent, unparseable, names a non-absolute path, or selects the host driver
// ("/", already covered by the standard host-root probes). The contract's
// DRIVER_ROOT_CTR_PATH is the DRA container's own mount path and does not
// apply to gpud's mount layout.
func probeDriverReadyContract(hostRoot string) string {
	b, err := os.ReadFile(filepath.Join(hostRoot, driverReadyContractPath))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "NVIDIA_DRIVER_ROOT=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if value == "" || value == "/" || !filepath.IsAbs(value) {
			continue
		}
		if libraryPath := probeNVMLLibrary(filepath.Join(hostRoot, value)); libraryPath != "" {
			return libraryPath
		}
	}
	return ""
}

func nvmlArchLibraryDir(goarch string) string {
	switch goarch {
	case "arm64":
		return "aarch64-linux-gnu"
	case "ppc64le":
		return "powerpc64le-linux-gnu"
	default:
		return "x86_64-linux-gnu"
	}
}

// Specifies the NVML library instance.
// Otherwise, defaults to the NVML library instance returned by nvml.New().
func WithNVML(nvmlLib nvml.Interface) OpOption {
	return func(op *Op) {
		op.nvmlLib = nvmlLib
	}
}

// Specifies the return value of the NVML library's Init() function.
// Otherwise, defaults to the return value of the NVML library's Init() function.
func WithInitReturn(initReturn nvml.Return) OpOption {
	return func(op *Op) {
		op.initReturn = &initReturn
	}
}

// Specifies the property extractor for the NVML library.
func WithPropertyExtractor(propertyExtractor nvinfo.PropertyExtractor) OpOption {
	return func(op *Op) {
		op.propertyExtractor = propertyExtractor
	}
}

func WithDevice(dev nvlibdevice.Device) OpOption {
	return func(op *Op) {
		op.devicesToReturn = append(op.devicesToReturn, dev)
	}
}

// Specifies the function for all devices to get the remapped rows of the device.
// Otherwise, defaults to the function returned by device.GetRemappedRows().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
func WithDeviceGetRemappedRowsForAllDevs(f func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetRemappedRowsForAllDevs = f
	}
}

// Specifies the function for all devices  to get the current clocks event reasons of the device.
// Otherwise, defaults to the function returned by device.GetCurrentClocksEventReasons().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
func WithDeviceGetCurrentClocksEventReasonsForAllDevs(f func() (uint64, nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetCurrentClocksEventReasonsForAllDevs = f
	}
}

// WithDeviceGetDevicesError specifies the error to return from Device().GetDevices().
// This is used for testing NVML device enumeration failure scenarios, such as when
// nvidia-smi shows "Unable to determine the device handle for GPU: Unknown Error".
func WithDeviceGetDevicesError(err error) OpOption {
	return func(op *Op) {
		op.devGetDevicesError = err
	}
}

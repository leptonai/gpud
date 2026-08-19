package lib

import (
	"os"
	"path/filepath"
	"runtime"

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
// searches a configured driver root. With neither environment variable set it
// returns empty, preserving go-nvml's standard system-library lookup.
func resolveNVMLLibraryPath() string {
	if libraryPath := os.Getenv(EnvNVMLLibraryPath); libraryPath != "" {
		return libraryPath
	}

	driverRoot := os.Getenv(EnvNVIDIADriverRoot)
	if driverRoot == "" {
		return ""
	}

	archLibraryDir := "x86_64-linux-gnu"
	switch runtime.GOARCH {
	case "arm64":
		archLibraryDir = "aarch64-linux-gnu"
	case "ppc64le":
		archLibraryDir = "powerpc64le-linux-gnu"
	}
	candidates := []string{
		filepath.Join(driverRoot, "usr", "lib", archLibraryDir, "libnvidia-ml.so.1"),
		filepath.Join(driverRoot, "usr", "lib64", "libnvidia-ml.so.1"),
		filepath.Join(driverRoot, "usr", "lib", "libnvidia-ml.so.1"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
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

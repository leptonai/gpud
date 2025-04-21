// Package nvml implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package nvml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

// architectureNames maps architecture codes to human-readable names
// as declared in nvml/nvml.h
var architectureNames = map[uint]string{
	2:          "Kepler",
	3:          "Maxwell",
	4:          "Pascal",
	5:          "Volta",
	6:          "Turing",
	7:          "Ampere",
	8:          "Ada",
	9:          "Hopper",
	0xffffffff: "Unknown",
}

func GetArchitecture(dev device.Device) (string, error) {
	arch, ret := dev.GetArchitecture()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device architecture: %v", nvml.ErrorString(ret))
	}

	if name, ok := architectureNames[uint(arch)]; ok {
		return name, nil
	}
	return fmt.Sprintf("UnknownArchitecture(%d)", arch), nil
}

// brandNames maps brand codes to human-readable names
// as declared in nvml/nvml.h
var brandNames = map[nvml.BrandType]string{
	nvml.BRAND_UNKNOWN:             "Unknown",
	nvml.BRAND_QUADRO:              "Quadro",
	nvml.BRAND_TESLA:               "Tesla",
	nvml.BRAND_NVS:                 "NVS",
	nvml.BRAND_GRID:                "GRID",
	nvml.BRAND_GEFORCE:             "GeForce",
	nvml.BRAND_TITAN:               "TITAN",
	nvml.BRAND_NVIDIA_VAPPS:        "NVIDIA vApps",
	nvml.BRAND_NVIDIA_VPC:          "NVIDIA Virtual PC",
	nvml.BRAND_NVIDIA_VCS:          "NVIDIA Virtual Compute Server",
	nvml.BRAND_NVIDIA_VWS:          "NVIDIA Virtual Workstation",
	nvml.BRAND_NVIDIA_CLOUD_GAMING: "NVIDIA Cloud Gaming",
	nvml.BRAND_QUADRO_RTX:          "Quadro RTX",
	nvml.BRAND_NVIDIA_RTX:          "NVIDIA RTX",
	nvml.BRAND_NVIDIA:              "NVIDIA",
	nvml.BRAND_GEFORCE_RTX:         "GeForce RTX",
	nvml.BRAND_TITAN_RTX:           "TITAN RTX",
}

func GetBrand(dev device.Device) (string, error) {
	brand, ret := dev.GetBrand()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device brand: %v", nvml.ErrorString(ret))
	}

	if name, ok := brandNames[brand]; ok {
		return name, nil
	}
	return fmt.Sprintf("UnknownBrand(%d)", brand), nil
}

func GetDriverVersion() (string, error) {
	nvmlLib, err := nvmllib.New()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = nvmlLib.Shutdown()
	}()

	return GetSystemDriverVersion(nvmlLib.NVML())
}

func GetSystemDriverVersion(nvmlLib nvml.Interface) (string, error) {
	ver, ret := nvmlLib.SystemGetDriverVersion()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get driver version: %v", nvml.ErrorString(ret))
	}

	// e.g.,
	// 525.85.12  == does not support clock events
	// 535.161.08 == supports clock events
	return ver, nil
}

func ParseDriverVersion(version string) (major, minor, patch int, err error) {
	splits := strings.Split(version, ".")
	if len(splits) < 2 {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (expected at least 2 parts): %v", version)
	}
	if len(splits) > 3 {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (expected at most 3 parts): %v", version)
	}

	major, err = strconv.Atoi(splits[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (major): %v", err)
	}
	minor, err = strconv.Atoi(splits[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (minor): %v", err)
	}
	patch = 0
	if len(splits) > 2 {
		patch, err = strconv.Atoi(splits[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to parse driver version (patch): %v", err)
		}
	}

	return major, minor, patch, nil
}

func GetCUDAVersion() (string, error) {
	nvmlLib, err := nvmllib.New()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = nvmlLib.Shutdown()
	}()

	return getCUDAVersion(nvmlLib.NVML())
}

func getCUDAVersion(nvmlLib nvml.Interface) (string, error) {
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlSystemQueries.html#group__nvmlSystemQueries_1g1d12b603a42805ee7e4160557ffc2128
	ver, ret := nvmlLib.SystemGetCudaDriverVersion_v2()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get driver version: %v", nvml.ErrorString(ret))
	}

	// #define NVML_CUDA_DRIVER_VERSION_MAJOR ( v ) ((v)/1000)
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlSystemQueries.html#group__nvmlSystemQueries_1g40a4eb255d9766f6bc4c9402ce9102c2
	major := ver / 1000

	// #define NVML_CUDA_DRIVER_VERSION_MINOR ( v ) (((v) % 1000) / 10)
	minor := (ver % 1000) / 10

	return fmt.Sprintf("%d.%d", major, minor), nil
}

// clock events are supported in versions 535 and above
// otherwise, CGO call just exits with
// undefined symbol: nvmlDeviceGetCurrentClocksEventReasons
func ClockEventsSupportedVersion(major int) bool {
	return major >= 535
}

// Loads the product name of the NVIDIA GPU device.
func LoadGPUDeviceName() (string, error) {
	nvmlLib, err := nvmllib.New()
	if err != nil {
		return "", err
	}
	defer func() {
		_ = nvmlLib.Shutdown()
	}()

	nvmlExists, nvmlExistsMsg := nvmlLib.Info().HasNvml()
	if !nvmlExists {
		return "", fmt.Errorf("NVML not found: %s", nvmlExistsMsg)
	}

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	devices, err := nvmlLib.Device().GetDevices()
	if err != nil {
		return "", err
	}

	for _, d := range devices {
		name, ret := d.GetName()
		if ret != nvml.SUCCESS {
			return "", fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		if name != "" {
			return name, nil
		}
	}

	return "", nil
}

func GetProductName(dev device.Device) (string, error) {
	name, ret := dev.GetName()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
	}
	return name, nil
}

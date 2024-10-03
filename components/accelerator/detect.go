package accelerator

import (
	"context"
	"fmt"

	"github.com/leptonai/gpud/pkg/file"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Type string

const (
	TypeUnknown Type = "unknown"
	TypeNVIDIA  Type = "nvidia"
)

// Returns the GPU type (e.g., "NVIDIA") and product name (e.g., "A100")
func DetectTypeAndProductName(ctx context.Context) (Type, string, error) {
	if p, err := file.LocateExecutable("nvidia-smi"); p != "" && err == nil {
		productName, err := LoadNVIDIAProductName(ctx)
		if err != nil {
			return TypeNVIDIA, "unknown", err
		}
		return TypeNVIDIA, productName, nil
	}

	return TypeUnknown, "unknown", nil
}

func LoadNVIDIAProductName(ctx context.Context) (string, error) {
	nvmlLib := nvml.New()
	if ret := nvmlLib.Init(); ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}

	deviceLib := device.New(nvmlLib)
	infoLib := nvinfo.New(
		nvinfo.WithNvmlLib(nvmlLib),
		nvinfo.WithDeviceLib(deviceLib),
	)

	nvmlExists, nvmlExistsMsg := infoLib.HasNvml()
	if !nvmlExists {
		return "", fmt.Errorf("NVML not found: %s", nvmlExistsMsg)
	}

	devices, err := deviceLib.GetDevices()
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

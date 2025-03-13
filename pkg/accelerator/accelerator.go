package accelerator

import (
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

type Type string

const (
	TypeUnknown Type = "unknown"
	TypeNVIDIA  Type = "nvidia"
)

// Returns the GPU type (e.g., "NVIDIA") and product name (e.g., "A100")
func DetectTypeAndProductName() (Type, string, error) {
	if _, err := file.LocateExecutable("nvidia-smi"); err == nil {
		productName, err := nvml.LoadGPUDeviceName()
		if err != nil {
			return TypeNVIDIA, "unknown", err
		}
		return TypeNVIDIA, productName, nil
	}

	return TypeUnknown, "unknown", nil
}

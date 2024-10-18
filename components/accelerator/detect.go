package accelerator

import (
	"context"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	"github.com/leptonai/gpud/pkg/file"
)

type Type string

const (
	TypeUnknown Type = "unknown"
	TypeNVIDIA  Type = "nvidia"
)

// Returns the GPU type (e.g., "NVIDIA") and product name (e.g., "A100")
func DetectTypeAndProductName(ctx context.Context) (Type, string, error) {
	if p, err := file.LocateExecutable("nvidia-smi"); p != "" && err == nil {
		productName, err := nvidia_query.LoadProductName(ctx)
		if err != nil {
			return TypeNVIDIA, "unknown", err
		}
		return TypeNVIDIA, productName, nil
	}

	return TypeUnknown, "unknown", nil
}

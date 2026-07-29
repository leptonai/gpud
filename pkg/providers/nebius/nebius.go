package nebius

import (
	"context"
	"fmt"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nebius/imds"
)

const Name = "nebius"

func New() providers.Detector {
	return providers.NewWithRegion(Name, detectProvider, nil, nil, imds.FetchRegion, nil, GetInstanceID)
}

func detectProvider(ctx context.Context) (string, error) {
	region, err := imds.FetchRegion(ctx)
	if err != nil {
		return "", err
	}
	if region != "" {
		return Name, nil
	}
	return "", nil
}

// GetInstanceID fetches the Nebius VM identity from HTTP IMDS.
func GetInstanceID(ctx context.Context) (string, error) {
	data, err := imds.FetchInstanceData(ctx)
	if err != nil {
		return "", err
	}
	if data.ParentID == "" || data.ID == "" {
		return "", fmt.Errorf("nebius instance metadata is missing parent_id or id")
	}
	return formatInstanceID(data.ParentID, data.GPUClusterID, data.ID), nil
}

func formatInstanceID(parentID, gpuClusterID, instanceID string) string {
	if gpuClusterID != "" {
		return fmt.Sprintf("%s/%s/%s", parentID, gpuClusterID, instanceID)
	}
	return fmt.Sprintf("%s/%s", parentID, instanceID)
}

package nebius

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nebius/imds"
)

const Name = "nebius"

var (
	metadataPath = "/mnt/cloud-metadata"
)

func New() providers.Detector {
	return providers.NewWithRegion(Name, detectProvider, nil, nil, imds.FetchRegion, nil, fetchInstanceID)
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

func fetchInstanceID(ctx context.Context) (string, error) {
	data, err := imds.FetchInstanceData(ctx)
	if err != nil {
		return "", err
	}
	if data.ParentID == "" || data.ID == "" {
		return "", fmt.Errorf("Nebius instance metadata is missing parent_id or id")
	}
	return formatInstanceID(data.ParentID, data.GPUClusterID, data.ID), nil
}

func GetInstanceID() (string, error) {
	projectID, err := os.ReadFile(filepath.Join(metadataPath, "parent-id"))
	if err != nil {
		return "", err
	}
	gpuClusterID, err := os.ReadFile(filepath.Join(metadataPath, "gpu-cluster-id"))
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	instanceID, err := os.ReadFile(filepath.Join(metadataPath, "instance-id"))
	if err != nil {
		return "", err
	}
	return formatInstanceID(string(projectID), string(gpuClusterID), string(instanceID)), nil
}

func formatInstanceID(parentID, gpuClusterID, instanceID string) string {
	if gpuClusterID != "" {
		return fmt.Sprintf("%s/%s/%s", parentID, gpuClusterID, instanceID)
	}
	return fmt.Sprintf("%s/%s", parentID, instanceID)
}

package nebius

import (
	"fmt"
	"os"
	"path/filepath"
)

var (
	metadataPath = "/mnt/cloud-metadata"
)

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
	if len(gpuClusterID) > 0 {
		return fmt.Sprintf("%s/%s/%s", string(projectID), string(gpuClusterID), string(instanceID)), nil
	}
	return fmt.Sprintf("%s/%s", string(projectID), string(instanceID)), nil
}

package nebius

import (
	"fmt"
	"os"
)

func GetInstanceID() (string, error) {
	projectID, err := os.ReadFile("/mnt/cloud-metadata/parent-id")
	if err != nil {
		return "", err
	}
	gpuClusterID, err := os.ReadFile("/mnt/cloud-metadata/gpu-cluster-id")
	if err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	instanceID, err := os.ReadFile("/mnt/cloud-metadata/instance-id")
	if err != nil {
		return "", err
	}
	if len(gpuClusterID) > 0 {
		return fmt.Sprintf("projects/%s/gpu-clusters/%s/instances/%s", string(projectID), string(gpuClusterID), string(instanceID)), nil
	}
	return fmt.Sprintf("projects/%s/instances/%s", string(projectID), string(instanceID)), nil
}

package machineinfo

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func CreateLoginRequest(token string, nvmlInstance nvidianvml.Instance, machineID string, gpuCount string, privateIP string, publicIP string) (*apiv1.LoginRequest, error) {
	req := &apiv1.LoginRequest{
		Token:     token,
		MachineID: machineID,
		Location:  GetMachineLocation(),
		Resources: map[string]string{},
	}

	req.Provider = GetProvider(publicIP)

	var err error
	req.MachineInfo, err = GetMachineInfo(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine info: %w", err)
	}

	if req.MachineInfo.NetworkInfo == nil {
		req.MachineInfo.NetworkInfo = &apiv1.MachineNetworkInfo{}
	}
	req.MachineInfo.NetworkInfo.PrivateIP = privateIP
	req.MachineInfo.NetworkInfo.PublicIP = publicIP

	cpu, _, err := GetSystemResourceLogicalCores()
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource logical cores: %w", err)
	}
	req.Resources[string(corev1.ResourceCPU)] = cpu

	memory, err := GetSystemResourceMemoryTotal()
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource memory total: %w", err)
	}
	req.Resources[string(corev1.ResourceMemory)] = memory

	volumeSize, err := GetSystemResourceRootVolumeTotal()
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource root volume total: %w", err)
	}
	req.Resources[string(corev1.ResourceEphemeralStorage)] = volumeSize

	gpuCnt := gpuCount
	if gpuCnt == "" {
		gpuCnt, err = GetSystemResourceGPUCount(nvmlInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to get system resource gpu count: %w", err)
		}
	}
	if gpuCnt != "0" {
		req.Resources["nvidia.com/gpu"] = gpuCnt
	}

	return req, nil
}

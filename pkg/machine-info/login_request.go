package machineinfo

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func CreateLoginRequest(token string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
	req := &apiv1.LoginRequest{
		Token:     token,
		Network:   GetMachineNetwork(),
		Location:  GetMachineLocation(),
		CPUInfo:   GetMachineCPUInfo(),
		Resources: map[string]string{},
	}

	var err error
	req.GPUInfo, err = GetMachineGPUInfo(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine gpu info: %w", err)
	}

	req.Provider = GetProvider(req.Network.PublicIP)

	req.MachineInfo, err = GetMachineInfo(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine info: %w", err)
	}

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

	gpuCnt, err := GetSystemResourceGPUCount(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource gpu count: %w", err)
	}
	if gpuCnt != "0" {
		req.Resources["nvidia.com/gpu"] = gpuCnt
	}

	return req, nil
}

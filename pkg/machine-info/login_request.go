package machineinfo

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func CreateLoginRequest(token string, nvmlInstance nvidianvml.Instance, machineID string, gpuCount string) (*apiv1.LoginRequest, error) {
	return createLoginRequest(
		token,
		nvmlInstance,
		machineID,
		gpuCount,
		GetMachineLocation,
		GetMachineInfo,
		GetProvider,
		GetSystemResourceLogicalCores,
		GetSystemResourceMemoryTotal,
		GetSystemResourceRootVolumeTotal,
		GetSystemResourceGPUCount,
	)
}

func createLoginRequest(
	token string,
	nvmlInstance nvidianvml.Instance,
	machineID string,
	gpuCount string,
	getMachineLocationFunc func() *apiv1.MachineLocation,
	getMachineInfoFunc func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error),
	getProviderFunc func(ip string) string,
	getSystemResourceLogicalCoresFunc func() (string, int64, error),
	getSystemResourceMemoryTotalFunc func() (string, error),
	getSystemResourceRootVolumeTotalFunc func() (string, error),
	getSystemResourceGPUCountFunc func(nvmlInstance nvidianvml.Instance) (string, error),
) (*apiv1.LoginRequest, error) {
	req := &apiv1.LoginRequest{
		Token:     token,
		MachineID: machineID,
		Network:   &apiv1.MachineNetwork{},
		Location:  getMachineLocationFunc(),
		Resources: map[string]string{},
	}

	var err error
	req.MachineInfo, err = getMachineInfoFunc(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine info: %w", err)
	}

	// get the default values from the machine info
	if req.MachineInfo != nil && req.MachineInfo.NetworkInfo != nil {
		for _, iface := range req.MachineInfo.NetworkInfo.Interfaces {
			if req.Network.PublicIP == "" && iface.IP != "" {
				req.Network.PublicIP = iface.IP
				req.Provider = getProviderFunc(iface.IP)
				continue
			}
			if req.Network.PrivateIP == "" && iface.PrivateIP != "" && iface.Addr.Is4() {
				req.Network.PrivateIP = iface.PrivateIP
				continue
			}
		}
	}

	cpu, _, err := getSystemResourceLogicalCoresFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource logical cores: %w", err)
	}
	req.Resources[string(corev1.ResourceCPU)] = cpu

	memory, err := getSystemResourceMemoryTotalFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource memory total: %w", err)
	}
	req.Resources[string(corev1.ResourceMemory)] = memory

	volumeSize, err := getSystemResourceRootVolumeTotalFunc()
	if err != nil {
		return nil, fmt.Errorf("failed to get system resource root volume total: %w", err)
	}
	req.Resources[string(corev1.ResourceEphemeralStorage)] = volumeSize

	gpuCnt := gpuCount
	if gpuCnt == "" {
		gpuCnt, err = getSystemResourceGPUCountFunc(nvmlInstance)
		if err != nil {
			return nil, fmt.Errorf("failed to get system resource gpu count: %w", err)
		}
	}
	if gpuCnt != "0" {
		req.Resources["nvidia.com/gpu"] = gpuCnt
	}

	return req, nil
}

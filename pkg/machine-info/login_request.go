package machineinfo

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/providers"
)

func CreateLoginRequest(token string, machineID string, nodeGroup string, gpuCount string, nvmlInstance nvidianvml.Instance) (*apiv1.LoginRequest, error) {
	return createLoginRequest(
		token,
		machineID,
		nodeGroup,
		gpuCount,
		nvmlInstance,
		netutil.PublicIP,
		GetMachineLocation,
		GetMachineInfo,
		GetProvider,
		GetSystemResourceRootVolumeTotal,
		GetSystemResourceGPUCount,
	)
}

func createLoginRequest(
	token string,
	machineID string,
	nodeGroup string,
	gpuCount string,
	nvmlInstance nvidianvml.Instance,
	getPublicIPFunc func() (string, error),
	getMachineLocationFunc func() *apiv1.MachineLocation,
	getMachineInfoFunc func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error),
	getProviderFunc func(ip string) *providers.Info,
	getSystemResourceRootVolumeTotalFunc func() (string, error),
	getSystemResourceGPUCountFunc func(nvmlInstance nvidianvml.Instance) (string, error),
) (*apiv1.LoginRequest, error) {
	donec := make(chan struct{})
	defer close(donec)

	// deciding machine location can take awhile
	// depending on the network latency
	// run async
	machineLocationCh := make(chan *apiv1.MachineLocation, 1)
	go func() {
		select {
		case <-donec:
			return
		case machineLocationCh <- getMachineLocationFunc():
		}
	}()

	req := &apiv1.LoginRequest{
		Token:     token,
		MachineID: machineID,
		NodeGroup: nodeGroup,
		Network:   &apiv1.MachineNetwork{},
		Resources: map[string]string{},
	}

	var err error
	req.Network.PublicIP, err = getPublicIPFunc()
	if err != nil {
		log.Logger.Errorw("failed to get public ip", "error", err)
	}
	detectedProvider := getProviderFunc(req.Network.PublicIP)
	req.Provider = detectedProvider.Provider
	req.ProviderInstanceID = detectedProvider.InstanceID
	req.Network.PublicIP = detectedProvider.PublicIP

	req.MachineInfo, err = getMachineInfoFunc(nvmlInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get machine info: %w", err)
	}

	// get the default values from the machine info
	if req.MachineInfo != nil && req.MachineInfo.NICInfo != nil {
		for _, iface := range req.MachineInfo.NICInfo.PrivateIPInterfaces {
			if iface.IP == "" {
				continue
			}
			if req.Network.PrivateIP == "" && iface.Addr.IsPrivate() && iface.Addr.Is4() {
				req.Network.PrivateIP = iface.IP
				break
			}
		}
	}

	if req.Network.PrivateIP == "" {
		log.Logger.Warnw("no private ip found, falling back to provider private ip", "provider", req.Provider, "privateIP", detectedProvider.PrivateIP)
		req.Network.PrivateIP = detectedProvider.PrivateIP
	}

	// represents the CPU, in cores (500m = .5 cores).
	// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
	req.Resources[string(corev1.ResourceCPU)] = resource.NewQuantity(req.MachineInfo.CPUInfo.LogicalCores, resource.DecimalSI).String()

	// represents the Memory, in bytes (500Gi = 500GiB = 500 * 1024 * 1024 * 1024).
	// Must be parsed using the "resource.ParseQuantity" function in https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource.
	req.Resources[string(corev1.ResourceMemory)] = resource.NewQuantity(int64(req.MachineInfo.MemoryInfo.TotalBytes), resource.DecimalSI).String()

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

	req.Location = <-machineLocationCh

	return req, nil
}

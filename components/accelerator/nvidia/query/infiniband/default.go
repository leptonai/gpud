package infiniband

import "strings"

// Configures the expected state of the ports.
type ExpectedPortStates struct {
	// The minimum number of ports.
	// If not set, it defaults to the number of GPUs.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 200.
	AtLeastRate int `json:"at_least_rate"`
}

var gpuPortConfigs = map[string]ExpectedPortStates{
	// "NVIDIA ConnectX-6 or ConnectX-7 Single Port InfiniBand (default): Up to 200Gbps"
	// ref. https://docs.nvidia.com/dgx/dgxa100-user-guide/introduction-to-dgxa100.html
	"a100": {AtLeastPorts: 1, AtLeastRate: 200},

	// "NDR (Next Data Rate) 400 Gb/s InfiniBand networking acceleration"
	// "InfiniBand (default): Up to 400Gbps"
	// ref. https://docs.nvidia.com/dgx/dgxh100-user-guide/introduction-to-dgxh100.html
	// ref. https://docs.nvidia.com/launchpad/ai/h100-mig/latest/h100-mig-gpu.html
	"h100": {AtLeastPorts: 8, AtLeastRate: 400},
	"h200": {AtLeastPorts: 8, AtLeastRate: 400},

	// "InfiniBand (default): Up to 400Gbps"
	// ref. https://docs.nvidia.com/dgx/dgxb200-user-guide/introduction-to-dgxb200.html
	"b200": {AtLeastPorts: 8, AtLeastRate: 400},

	// "provides 1.8 terabytes per second (TB/s) of GPU-to-GPU interconnect, InfiniBand networking"
	// ref. https://www.nvidia.com/en-us/data-center/gb200-nvl72/
	"gb200": {AtLeastPorts: 8, AtLeastRate: 1800},
}

func SupportsInfinibandPortRate(gpuProductName string) ExpectedPortStates {
	p := strings.ToLower(gpuProductName)

	for gpuType, config := range gpuPortConfigs {
		if strings.Contains(p, gpuType) {
			return config
		}
	}

	return ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
}

package infiniband

import (
	"errors"
	"strings"
)

// Configures the expected state of the ports.
type ExpectedPortStates struct {
	// The minimum number of ports.
	// If not set, it defaults to 0.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 0.
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

	// "Multiple racks connect with NVIDIA Quantum InfiniBand to scale up to hundreds of thousands of GB200 Superchips."
	// "NVIDIA Quantum-2 InfiniBand switches deliver 400Gb/s throughput,"
	// ref. https://www.nvidia.com/en-us/data-center/gb200-nvl2/
	// ref. https://www.nvidia.com/en-us/data-center/dgx-superpod-gb200/
	// ref. https://www.nvidia.com/en-us/networking/infiniband-switching/
	"gb200": {AtLeastPorts: 8, AtLeastRate: 400},
}

var ErrNoExpectedPortStates = errors.New("no expected port states found (not supported)")

func SupportsInfinibandPortRate(gpuProductName string) (ExpectedPortStates, error) {
	p := strings.ToLower(gpuProductName)

	longestMatch := ""
	for gpuType := range gpuPortConfigs {
		if strings.Contains(p, gpuType) {
			if len(gpuType) > len(longestMatch) {
				longestMatch = gpuType
			}
		}
	}
	if longestMatch == "" {
		return ExpectedPortStates{}, ErrNoExpectedPortStates
	}

	return gpuPortConfigs[longestMatch], nil
}

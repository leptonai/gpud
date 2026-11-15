package infiniband

import (
	"errors"
	"strings"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

var gpuPortConfigs = map[string]types.ExpectedPortStates{
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

// ErrNoExpectedPortStates reports that the GPU product does not have a known InfiniBand profile.
var ErrNoExpectedPortStates = errors.New("no expected port states found (not supported)")

// SupportsInfinibandPortRate returns the expected InfiniBand port thresholds for a GPU product.
func SupportsInfinibandPortRate(gpuProductName string) (types.ExpectedPortStates, error) {
	// Only an explicit NVL72 profile establishes the reference compute tray's
	// eight 800 Gb/s ports: the DGX datasheet lists 144 single-port ConnectX-9
	// at 800 Gb/s across 18 trays.
	// https://resources.nvidia.com/en-us-dgx-software/dgx-vera-rubin-nvl72-datasheet
	//
	// Neither a Rubin GPU name nor C2C/fabric evidence identifies the NICs:
	// NVL72 racks are also built by OEMs with their own scale-out networking,
	// so inferring ports from GPU topology could fail healthy racks. Nodes that
	// report a generic name (for example "NVIDIA Graphics Device" on early
	// drivers) get no InfiniBand expectation by default; deployments that match
	// the DGX reference must configure it explicitly:
	//
	//	gpud run --infiniband-expected-port-states='{"at_least_ports":8,"at_least_rate":800}'
	if product.IsRubin(gpuProductName, "") && product.HasExternalFabricByGPUProduct(gpuProductName) {
		return types.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 800}, nil
	}
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
		return types.ExpectedPortStates{}, ErrNoExpectedPortStates
	}

	return gpuPortConfigs[longestMatch], nil
}

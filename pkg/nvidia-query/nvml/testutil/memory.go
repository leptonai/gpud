package testutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
)

// CreateMemoryDevice creates a new mock device specifically for memory tests
func CreateMemoryDevice(uuid string, memoryV2 nvml.Memory_v2, memoryV2Ret nvml.Return, memory nvml.Memory, memoryRet nvml.Return) device.Device {
	mockDevice := &mock.Device{
		GetMemoryInfo_v2Func: func() (nvml.Memory_v2, nvml.Return) {
			return memoryV2, memoryV2Ret
		},
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			return memory, memoryRet
		},
		GetUUIDFunc: func() (string, nvml.Return) {
			return uuid, nvml.SUCCESS
		},
	}

	return NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")
}

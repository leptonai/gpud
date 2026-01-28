package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestCreateMemoryDevice(t *testing.T) {
	tests := []struct {
		name        string
		uuid        string
		memoryV2    nvml.Memory_v2
		memoryV2Ret nvml.Return
		memory      nvml.Memory
		memoryRet   nvml.Return
	}{
		{
			name: "Basic memory info",
			uuid: "test-uuid-1",
			memoryV2: nvml.Memory_v2{
				Free:  1024,
				Total: 8192,
				Used:  7168,
			},
			memoryV2Ret: nvml.SUCCESS,
			memory: nvml.Memory{
				Free:  1024,
				Total: 8192,
				Used:  7168,
			},
			memoryRet: nvml.SUCCESS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := CreateMemoryDevice(tt.uuid, tt.memoryV2, tt.memoryV2Ret, tt.memory, tt.memoryRet)
			require.NotNil(t, device)

			uuid, ret := device.GetUUID()
			require.Equal(t, nvml.SUCCESS, ret)
			require.Equal(t, tt.uuid, uuid)
		})
	}
}

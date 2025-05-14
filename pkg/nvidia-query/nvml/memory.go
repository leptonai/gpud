package nvml

import (
	"fmt"
	"strconv"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/dustin/go-humanize"
)

type Memory struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`

	ReservedBytes     uint64 `json:"reserved_bytes"`
	ReservedHumanized string `json:"reserved_humanized"`

	UsedBytes     uint64 `json:"used_bytes"`
	UsedHumanized string `json:"used_humanized"`

	FreeBytes     uint64 `json:"free_bytes"`
	FreeHumanized string `json:"free_humanized"`

	UsedPercent string `json:"used_percent"`

	// Supported is true if the memory is supported by the device.
	Supported bool `json:"supported"`
}

func (mem Memory) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(mem.UsedPercent, 64)
}

func GetMemory(uuid string, dev device.Device) (Memory, error) {
	mem := Memory{
		UUID:      uuid,
		Supported: true,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlMemory__v2__t.html#structnvmlMemory__v2__t
	infoV2, retV2 := dev.GetMemoryInfo_v2()
	if retV2 == nvml.SUCCESS {
		mem.TotalBytes = infoV2.Total
		mem.ReservedBytes = infoV2.Reserved
		mem.FreeBytes = infoV2.Free
		mem.UsedBytes = infoV2.Used
	} else { // fallback to old API
		// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlMemory__t.html
		infoV1, retV1 := dev.GetMemoryInfo()
		if retV1 == nvml.SUCCESS {
			mem.TotalBytes = infoV1.Total
			mem.FreeBytes = infoV1.Free
			mem.UsedBytes = infoV1.Used
		} else {
			if IsNotSupportError(retV1) {
				mem.Supported = false
				return mem, nil
			}

			if IsGPULostError(retV1) {
				return mem, ErrGPULost
			}

			// v2 API failed AND v1 API fallback failed
			return mem, fmt.Errorf("failed to get device memory info: %v (v2 API error %v)", nvml.ErrorString(retV1), nvml.ErrorString(retV2))
		}
	}

	mem.TotalHumanized = humanize.Bytes(mem.TotalBytes)
	mem.ReservedHumanized = humanize.Bytes(mem.ReservedBytes)
	mem.FreeHumanized = humanize.Bytes(mem.FreeBytes)
	mem.UsedHumanized = humanize.Bytes(mem.UsedBytes)

	if mem.TotalBytes > 0 {
		mem.UsedPercent = fmt.Sprintf("%.2f", float64(mem.UsedBytes)/float64(mem.TotalBytes)*100)
	} else {
		mem.UsedPercent = "0.0"
	}

	return mem, nil
}

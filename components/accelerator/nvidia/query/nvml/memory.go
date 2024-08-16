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
}

func (mem Memory) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(mem.UsedPercent, 64)
}

func GetMemory(uuid string, dev device.Device) (Memory, error) {
	mem := Memory{
		UUID: uuid,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlMemory__v2__t.html#structnvmlMemory__v2__t
	infoV2, ret := dev.GetMemoryInfo_v2()
	if ret != nvml.SUCCESS {
		// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlMemory__t.html
		info, ret := dev.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			return Memory{}, fmt.Errorf("failed to get device memory info: %v", nvml.ErrorString(ret))
		}
		mem.TotalBytes = info.Total
		mem.FreeBytes = info.Free
		mem.UsedBytes = info.Used
	} else {
		mem.TotalBytes = infoV2.Total
		mem.ReservedBytes = infoV2.Reserved
		mem.FreeBytes = infoV2.Free
		mem.UsedBytes = infoV2.Used
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

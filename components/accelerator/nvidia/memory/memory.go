package memory

import (
	"fmt"
	"strconv"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/dustin/go-humanize"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
)

type Memory struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

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
	if mem.UsedPercent == "" {
		return 0.0, nil
	}
	return strconv.ParseFloat(mem.UsedPercent, 64)
}

func GetMemory(uuid string, dev device.Device) (Memory, error) {
	mem := Memory{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
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
		log.Logger.Warnw("failed to get device memory info v2, falling back to v1", "error", nvml.ErrorString(retV2))

		// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlMemory__t.html
		infoV1, retV1 := dev.GetMemoryInfo()
		if retV1 == nvml.SUCCESS {
			mem.TotalBytes = infoV1.Total
			mem.FreeBytes = infoV1.Free
			mem.UsedBytes = infoV1.Used
		} else {
			log.Logger.Warnw("failed to get device memory info v1", "error", nvml.ErrorString(retV1))

			if nvmlerrors.IsNotSupportError(retV1) {
				// NOTE: "NVIDIA-GB10" NVIDIA RTX blackwell with "blackwell" architecture does not support v2/v1 API
				// with the error "Not Supported" for both v2 and v1 API
				log.Logger.Warnw("device memory info v1 is not supported", "error", nvml.ErrorString(retV1))

				mem.Supported = false
				return mem, nil
			}

			if nvmlerrors.IsGPULostError(retV1) {
				log.Logger.Warnw("device memory info v1 is GPU lost", "error", nvml.ErrorString(retV1))

				return mem, nvmlerrors.ErrGPULost
			}

			if nvmlerrors.IsGPURequiresReset(retV1) {
				log.Logger.Warnw("device memory info v1 requires reset", "error", nvml.ErrorString(retV1))

				return mem, nvmlerrors.ErrGPURequiresReset
			}

			// v2 API failed AND v1 API fallback failed
			return mem, fmt.Errorf("failed to get device memory info: %v (v2 API error %v)", nvml.ErrorString(retV1), nvml.ErrorString(retV2))
		}
	}

	mem.TotalHumanized = humanize.IBytes(mem.TotalBytes)
	mem.ReservedHumanized = humanize.IBytes(mem.ReservedBytes)
	mem.FreeHumanized = humanize.IBytes(mem.FreeBytes)
	mem.UsedHumanized = humanize.IBytes(mem.UsedBytes)

	if mem.TotalBytes > 0 {
		mem.UsedPercent = fmt.Sprintf("%.2f", float64(mem.UsedBytes)/float64(mem.TotalBytes)*100)
	} else {
		mem.UsedPercent = "0.0"
	}

	return mem, nil
}

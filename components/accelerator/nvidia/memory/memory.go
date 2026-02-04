package memory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/dustin/go-humanize"
	gopsutilmem "github.com/shirou/gopsutil/v4/mem"

	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
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

	// IsUnifiedMemory is true when the GPU uses unified memory architecture
	// (shared CPU/GPU memory) and traditional NVML memory APIs are not supported.
	// In this case, system memory values are reported instead.
	// ref. https://docs.nvidia.com/dgx/dgx-spark/known-issues.html
	IsUnifiedMemory bool `json:"is_unified_memory"`
}

func (mem Memory) GetUsedPercent() (float64, error) {
	if mem.UsedPercent == "" {
		return 0.0, nil
	}
	return strconv.ParseFloat(mem.UsedPercent, 64)
}

// GetVirtualMemoryFunc is the function type for getting virtual memory stats.
// This allows for dependency injection in tests.
type GetVirtualMemoryFunc func(context.Context) (*gopsutilmem.VirtualMemoryStat, error)

func GetMemory(uuid string, dev device.Device, productName string, getVirtualMemoryFunc GetVirtualMemoryFunc) (Memory, error) {
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
				// DGX Spark (GB10) uses unified memory architecture where 128GB RAM is shared
				// between CPU and GPU. Traditional NVML memory APIs return "Not Supported"
				// because there's no dedicated GPU framebuffer.
				// ref. https://docs.nvidia.com/dgx/dgx-spark/known-issues.html
				// ref. https://forums.developer.nvidia.com/t/nvtop-with-dgx-spark-unified-memory-support/351284
				// Following NVTOP's approach: fall back to system memory via gopsutil when
				// NVML GetMemoryInfo fails on unified memory devices.
				if strings.HasSuffix(productName, "GB10") && getVirtualMemoryFunc != nil {
					log.Logger.Warnw("NVML memory APIs not supported for unified memory device, falling back to system memory",
						"product_name", productName,
						"error", nvml.ErrorString(retV1),
					)

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					vm, err := getVirtualMemoryFunc(ctx)
					cancel()
					if err != nil {
						log.Logger.Warnw("failed to get system memory for unified memory device", "error", err)
						mem.Supported = false
						return mem, nil
					}

					mem.TotalBytes = vm.Total
					mem.FreeBytes = vm.Free
					mem.UsedBytes = vm.Used
					mem.TotalHumanized = humanize.IBytes(vm.Total)
					mem.FreeHumanized = humanize.IBytes(vm.Free)
					mem.UsedHumanized = humanize.IBytes(vm.Used)
					if vm.Total > 0 {
						mem.UsedPercent = fmt.Sprintf("%.2f", vm.UsedPercent)
					} else {
						mem.UsedPercent = "0.0"
					}
					mem.IsUnifiedMemory = true
					mem.Supported = true
					return mem, nil
				}

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

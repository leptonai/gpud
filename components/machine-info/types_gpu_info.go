package machineinfo

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
)

type GPUInfo struct {
	Driver GPUDriver `json:"driver"`
	CUDA   CUDA      `json:"cuda"`

	GPUCount GPUCount `json:"gpu_count"`
	GPUIDs   []GPUID  `json:"gpu_ids"`

	Memory  GPUMemory  `json:"memory"`
	Product GPUProduct `json:"products"`
}

func (info *GPUInfo) RenderTable(wr io.Writer) {
	if info == nil {
		return
	}

	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Product", info.Product.Name})
	table.Append([]string{"Brand", info.Product.Brand})
	table.Append([]string{"Architecture", info.Product.Architecture})
	table.Append([]string{"GPU Driver Version", info.Driver.Version})
	table.Append([]string{"CUDA Version", info.CUDA.Version})
	table.Append([]string{"GPU Count", fmt.Sprintf("%d", info.GPUCount.DeviceCount)})
	table.Append([]string{"GPU Attached", fmt.Sprintf("%d", info.GPUCount.Attached)})
	table.Append([]string{"GPU Memory", info.Memory.TotalHumanized})
	table.Render()
}

// GPUDriver is the driver version of the NVIDIA GPU.
type GPUDriver struct {
	Version string `json:"version"`
}

// CUDA is the CUDA version of the NVIDIA GPU.
type CUDA struct {
	Version string `json:"version"`
}

// GPUCount is the GPUCount information of the NVIDIA GPUCount.
type GPUCount struct {
	// DeviceCount is the number of GPU devices based on the /dev directory.
	DeviceCount int `json:"device_count"`

	// Attached is the number of GPU devices that are attached to the system,
	// based on the nvidia-smi or NVML.
	Attached int `json:"attached"`
}

// GPUID represents the unique identifier of a GPU.
type GPUID struct {
	UUID    string `json:"uuid,omitempty"`
	SN      string `json:"sn,omitempty"`
	MinorID string `json:"minor_id,omitempty"`
}

// GPUMemory is the memory information of the NVIDIA GPU.
type GPUMemory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`
}

// GPUProduct is the product information of the NVIDIA GPU.
type GPUProduct struct {
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	Architecture string `json:"architecture"`
}

package info

import (
	"fmt"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
)

// ToOutput converts nvidia_query.Output to Output.
// It returns an empty non-nil object, if the input or the required field is nil (e.g., i.SMI).
func ToOutput(i *nvidia_query.Output) *Output {
	if i == nil {
		return &Output{}
	}

	var totalMem uint64
	var totalMemHumanized string

	if i.SMI != nil && len(i.SMI.GPUs) > 0 {
		if i.NVML != nil && len(i.NVML.DeviceInfos) > 0 {
			totalMem = i.NVML.DeviceInfos[0].Memory.TotalBytes
			totalMemHumanized = humanize.Bytes(i.NVML.DeviceInfos[0].Memory.TotalBytes)
		}
	}

	o := &Output{
		GPU: GPU{
			DeviceCount: i.GPUDeviceCount,
			Attached:    i.GPUCount(),
		},
		Memory: Memory{
			TotalBytes:     totalMem,
			TotalHumanized: totalMemHumanized,
		},
	}

	if i.SMI != nil {
		o.Driver.Version = i.SMI.DriverVersion
		o.CUDA.Version = i.SMI.CUDAVersion

		for _, g := range i.SMI.GPUs {
			o.Product = Product{
				Name:         g.ProductName,
				Brand:        g.ProductBrand,
				Architecture: g.ProductArchitecture,
			}
			break
		}
	}

	return o
}

type Output struct {
	Driver  Driver  `json:"driver"`
	CUDA    CUDA    `json:"cuda"`
	GPU     GPU     `json:"gpu"`
	Memory  Memory  `json:"memory"`
	Product Product `json:"products"`
}

type Driver struct {
	Version string `json:"version"`
}

type CUDA struct {
	Version string `json:"version"`
}

type GPU struct {
	// DeviceCount is the number of GPU devices based on the /dev directory.
	DeviceCount int `json:"device_count"`

	// Attached is the number of GPU devices that are attached to the system,
	// based on the nvidia-smi or NVML.
	Attached int `json:"attached"`
}

type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`
}

type Product struct {
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	Architecture string `json:"architecture"`
}

const (
	StateKeyDriver        = "driver"
	StateKeyDriverVersion = "version"

	StateKeyCUDA        = "cuda"
	StateKeyCUDAVersion = "version"

	StateKeyGPU            = "gpu"
	StateKeyGPUDeviceCount = "device_count"
	StateKeyGPUAttached    = "attached"

	StateKeyMemory               = "memory"
	StateKeyMemoryTotalBytes     = "total_bytes"
	StateKeyMemoryTotalHumanized = "total_humanized"

	StateKeyProduct             = "product"
	StateKeyProductName         = "name"
	StateKeyProductBrand        = "brand"
	StateKeyProductArchitecture = "architecture"
)

func (o *Output) States() ([]components.State, error) {
	cs := []components.State{
		{
			Name:    StateKeyDriver,
			Healthy: true,
			Reason:  fmt.Sprintf("driver version is %s", o.Driver.Version),
			ExtraInfo: map[string]string{
				StateKeyDriverVersion: o.Driver.Version,
			},
		},
		{
			Name:    StateKeyCUDA,
			Healthy: true,
			Reason:  fmt.Sprintf("cuda version is %s", o.CUDA.Version),
			ExtraInfo: map[string]string{
				StateKeyCUDAVersion: o.CUDA.Version,
			},
		},
		{
			Name:    StateKeyGPU,
			Healthy: o.GPU.DeviceCount == o.GPU.Attached,
			Reason:  fmt.Sprintf("%d gpu(s) in /dev and %d gpu(s) found/attached", o.GPU.DeviceCount, o.GPU.Attached),
			ExtraInfo: map[string]string{
				StateKeyGPUDeviceCount: strconv.Itoa(o.GPU.DeviceCount),
				StateKeyGPUAttached:    strconv.Itoa(o.GPU.Attached),
			},
		},
		{
			Name:    StateKeyMemory,
			Healthy: true,
			Reason:  fmt.Sprintf("total memory is %s", o.Memory.TotalHumanized),
			ExtraInfo: map[string]string{
				StateKeyMemoryTotalBytes:     strconv.FormatUint(o.Memory.TotalBytes, 10),
				StateKeyMemoryTotalHumanized: o.Memory.TotalHumanized,
			},
		},
		{
			Name:    StateKeyProduct,
			Healthy: true,
			Reason:  fmt.Sprintf("product name is %s, brand is %s, architecture is %s", o.Product.Name, o.Product.Brand, o.Product.Architecture),
			ExtraInfo: map[string]string{
				StateKeyProductName:         o.Product.Name,
				StateKeyProductBrand:        o.Product.Brand,
				StateKeyProductArchitecture: o.Product.Architecture,
			},
		},
	}
	return cs, nil
}

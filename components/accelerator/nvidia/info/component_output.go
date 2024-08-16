package info

import (
	"fmt"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
)

func ToOutput(i *nvidia_query.Output) *Output {
	var totalMem uint64
	var totalMemHumanized string
	parsed, err := i.SMI.GPUs[0].FBMemoryUsage.Parse()
	if err == nil {
		totalMem = parsed.TotalBytes
		totalMemHumanized = parsed.TotalHumanized
	} else if i.NVML != nil && len(i.NVML.DeviceInfos) > 0 {
		totalMem = i.NVML.DeviceInfos[0].Memory.TotalBytes
		totalMemHumanized = humanize.Bytes(i.NVML.DeviceInfos[0].Memory.TotalBytes)
	}

	o := &Output{
		Driver: Driver{Version: i.SMI.DriverVersion},
		CUDA:   CUDA{Version: i.SMI.CUDAVersion},
		GPU:    GPU{Attached: i.SMI.AttachedGPUs},
		Memory: Memory{
			TotalBytes:     totalMem,
			TotalHumanized: totalMemHumanized,
		},
	}
	for _, g := range i.SMI.GPUs {
		o.Product = Product{
			Name:         g.ProductName,
			Brand:        g.ProductBrand,
			Architecture: g.ProductArchitecture,
		}
		break
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

	StateKeyGPU         = "gpu"
	StateKeyGPUAttached = "attached"

	StateKeyMemory               = "memory"
	StateKeyMemoryTotalBytes     = "total_bytes"
	StateKeyMemoryTotalHumanized = "total_humanized"

	StateKeyProduct             = "product"
	StateKeyProductName         = "name"
	StateKeyProductBrand        = "brand"
	StateKeyProductArchitecture = "architecture"
)

func ParseStateKeyDriver(m map[string]string) (Driver, error) {
	d := Driver{}
	d.Version = m[StateKeyDriver]
	return d, nil
}

func ParseStateKeyCUDA(m map[string]string) (CUDA, error) {
	c := CUDA{}
	c.Version = m[StateKeyCUDA]
	return c, nil
}

func ParseStateKeyGPU(m map[string]string) (GPU, error) {
	g := GPU{}

	var err error
	g.Attached, err = strconv.Atoi(m[StateKeyGPU])
	if err != nil {
		return g, err
	}

	return g, nil
}

func ParseStateKeyMemory(m map[string]string) (Memory, error) {
	mem := Memory{}

	var err error
	mem.TotalBytes, err = strconv.ParseUint(m[StateKeyMemoryTotalBytes], 10, 64)
	if err != nil {
		return mem, err
	}

	mem.TotalHumanized = m[StateKeyMemoryTotalHumanized]

	return mem, nil
}

func ParseStateKeyProduct(m map[string]string) (Product, error) {
	p := Product{}
	p.Name = m[StateKeyProduct]
	p.Brand = m[StateKeyProductBrand]
	p.Architecture = m[StateKeyProductArchitecture]
	return p, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case StateKeyDriver:
			driver, err := ParseStateKeyDriver(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Driver = driver

		case StateKeyCUDA:
			cuda, err := ParseStateKeyCUDA(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.CUDA = cuda

		case StateKeyGPU:
			gpu, err := ParseStateKeyGPU(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.GPU = gpu

		case StateKeyMemory:
			mem, err := ParseStateKeyMemory(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Memory = mem

		case StateKeyProduct:
			product, err := ParseStateKeyProduct(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Product = product

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return o, nil
}

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
			Healthy: true,
			Reason:  fmt.Sprintf("%d gpu(s) found/attached", o.GPU.Attached),
			ExtraInfo: map[string]string{
				StateKeyGPUAttached: strconv.Itoa(o.GPU.Attached),
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

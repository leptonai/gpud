// Package info provides relatively static information about the NVIDIA accelerator (e.g., GPU product names).
package info

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/olekukonko/tablewriter"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the name of the component.
const Name = "accelerator-nvidia-info"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance         nvidianvml.InstanceV2
	getDriverVersionFunc func() (string, error)
	getCUDAVersionFunc   func() (string, error)
	getDeviceCountFunc   func() (int, error)
	getMemoryFunc        func(uuid string, dev device.Device) (nvidianvml.Memory, error)
	getProductNameFunc   func(dev device.Device) (string, error)
	getArchitectureFunc  func(dev device.Device) (string, error)
	getBrandFunc         func(dev device.Device) (string, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                  cctx,
		cancel:               ccancel,
		nvmlInstance:         gpudInstance.NVMLInstance,
		getDriverVersionFunc: nvidianvml.GetDriverVersion,
		getCUDAVersionFunc:   nvidianvml.GetCUDAVersion,
		getDeviceCountFunc:   nvidiaquery.CountAllDevicesFromDevDir,
		getMemoryFunc:        nvidianvml.GetMemory,
		getProductNameFunc:   nvidianvml.GetProductName,
		getArchitectureFunc:  nvidianvml.GetArchitecture,
		getBrandFunc:         nvidianvml.GetBrand,
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu info")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	driverVersion, err := c.getDriverVersionFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting driver version: %s", err)
		return d
	}
	if driverVersion == "" {
		d.err = fmt.Errorf("driver version is empty")
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = "driver version is empty"
		return d
	}
	d.Driver.Version = driverVersion

	cudaVersion, err := c.getCUDAVersionFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting CUDA version: %s", err)
		return d
	}
	if cudaVersion == "" {
		d.err = fmt.Errorf("CUDA version is empty")
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = "CUDA version is empty"
		return d
	}
	d.CUDA.Version = cudaVersion

	deviceCount, err := c.getDeviceCountFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = fmt.Sprintf("error getting device count: %s", err)
		return d
	}
	d.GPU.DeviceCount = deviceCount

	devs := c.nvmlInstance.Devices()
	d.GPU.Attached = len(devs)

	for uuid, dev := range devs {
		mem, err := c.getMemoryFunc(uuid, dev)
		if err != nil {
			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting memory: %s", err)
			return d
		}
		d.Memory.TotalBytes = mem.TotalBytes
		d.Memory.TotalHumanized = mem.TotalHumanized

		productName, err := c.getProductNameFunc(dev)
		if err != nil {
			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting product name: %s", err)
			return d
		}
		d.Product.Name = productName

		architecture, err := c.getArchitectureFunc(dev)
		if err != nil {
			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting architecture: %s", err)
			return d
		}
		d.Product.Architecture = architecture

		brand, err := c.getBrandFunc(dev)
		if err != nil {
			d.err = err
			d.health = apiv1.HealthStateTypeUnhealthy
			d.reason = fmt.Sprintf("error getting brand: %s", err)
			return d
		}
		d.Product.Brand = brand
		break
	}

	d.health = apiv1.HealthStateTypeHealthy
	d.reason = fmt.Sprintf("all %d GPU(s) were checked", len(devs))

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	Driver  Driver  `json:"driver"`
	CUDA    CUDA    `json:"cuda"`
	GPU     GPU     `json:"gpu"`
	Memory  Memory  `json:"memory"`
	Product Product `json:"products"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

// Driver is the driver version of the NVIDIA GPU.
type Driver struct {
	Version string `json:"version"`
}

// CUDA is the CUDA version of the NVIDIA GPU.
type CUDA struct {
	Version string `json:"version"`
}

// GPU is the GPU information of the NVIDIA GPU.
type GPU struct {
	// DeviceCount is the number of GPU devices based on the /dev directory.
	DeviceCount int `json:"device_count"`

	// Attached is the number of GPU devices that are attached to the system,
	// based on the nvidia-smi or NVML.
	Attached int `json:"attached"`
}

// Memory is the memory information of the NVIDIA GPU.
type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`
}

// Product is the product information of the NVIDIA GPU.
type Product struct {
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	Architecture string `json:"architecture"`
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Product", d.Product.Name})
	table.Append([]string{"Brand", d.Product.Brand})
	table.Append([]string{"Architecture", d.Product.Architecture})
	table.Append([]string{"Driver Version", d.Driver.Version})
	table.Append([]string{"CUDA Version", d.CUDA.Version})
	table.Append([]string{"GPU Count", fmt.Sprintf("%d", d.GPU.DeviceCount)})
	table.Append([]string{"GPU Attached", fmt.Sprintf("%d", d.GPU.Attached)})
	table.Append([]string{"GPU Memory", d.Memory.TotalHumanized})
	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.HealthStateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}

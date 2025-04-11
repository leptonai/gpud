// Package info provides relatively static information about the NVIDIA accelerator (e.g., GPU product names).
package info

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-info"

var _ apiv1.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstanceV2       nvml.InstanceV2
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

func New(ctx context.Context, nvmlInstanceV2 nvml.InstanceV2) apiv1.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstanceV2:       nvmlInstanceV2,
		getDriverVersionFunc: nvidianvml.GetDriverVersion,
		getCUDAVersionFunc:   nvidianvml.GetCUDAVersion,
		getDeviceCountFunc:   nvidiaquery.CountAllDevicesFromDevDir,
		getMemoryFunc:        nvidianvml.GetMemory,
		getProductNameFunc:   nvidianvml.GetProductName,
		getArchitectureFunc:  nvidianvml.GetArchitecture,
		getBrandFunc:         nvidianvml.GetBrand,
	}
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking ecc")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	driverVersion, err := c.getDriverVersionFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error getting driver version: %s", err)
		return
	}
	if driverVersion == "" {
		d.err = fmt.Errorf("driver version is empty")
		d.healthy = false
		d.reason = "driver version is empty"
		return
	}
	d.Driver.Version = driverVersion

	cudaVersion, err := c.getCUDAVersionFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error getting CUDA version: %s", err)
		return
	}
	if cudaVersion == "" {
		d.err = fmt.Errorf("CUDA version is empty")
		d.healthy = false
		d.reason = "CUDA version is empty"
		return
	}
	d.CUDA.Version = cudaVersion

	deviceCount, err := c.getDeviceCountFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error getting device count: %s", err)
		return
	}
	d.GPU.DeviceCount = deviceCount

	devs := c.nvmlInstanceV2.Devices()
	d.GPU.Attached = len(devs)

	for uuid, dev := range devs {
		mem, err := c.getMemoryFunc(uuid, dev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting memory: %s", err)
			return
		}
		d.Memory.TotalBytes = mem.TotalBytes
		d.Memory.TotalHumanized = mem.TotalHumanized

		productName, err := c.getProductNameFunc(dev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting product name: %s", err)
			return
		}
		d.Product.Name = productName

		architecture, err := c.getArchitectureFunc(dev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting architecture: %s", err)
			return
		}
		d.Product.Architecture = architecture

		brand, err := c.getBrandFunc(dev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error getting brand: %s", err)
			return
		}
		d.Product.Brand = brand
		break
	}

	d.healthy = true
	d.reason = fmt.Sprintf("all %d GPU(s) were checked", len(devs))
}

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
	healthy bool
	// tracks the reason of the last check
	reason string
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

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]apiv1.State, error) {
	if d == nil {
		return []apiv1.State{
			{
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
